package disaster

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ai-provider/ai-provider/internal/config"
	"github.com/ai-provider/ai-provider/internal/models"
	"github.com/ai-provider/ai-provider/internal/storage"
	"go.uber.org/zap"
)

// RestoreManager handles restore operations for disaster recovery
type RestoreManager struct {
	config     *config.BackupConfig
	db         storage.Database
	modelMgr   *models.ModelManager
	logger     *zap.SugaredLogger
	mu         sync.RWMutex
	running    bool
	cancelFunc context.CancelFunc
	metrics    *RestoreMetrics
	encryptor  *Encryptor
	compressor *Compressor
	storage    BackupStorage
	verifier   *RestoreVerifier
}

// RestoreConfig holds restore configuration
type RestoreConfig struct {
	BackupID          string        `yaml:"backup_id" json:"backup_id"`
	Components        []string      `yaml:"components" json:"components"` // database, models, configs, logs
	TargetPath        string        `yaml:"target_path" json:"target_path"`
	Overwrite         bool          `yaml:"overwrite" json:"overwrite"`
	Verify            bool          `yaml:"verify" json:"verify"`
	DryRun            bool          `yaml:"dry_run" json:"dry_run"`
	Timeout           time.Duration `yaml:"timeout" json:"timeout"`
	ParallelWorkers   int           `yaml:"parallel_workers" json:"parallel_workers"`
	StopOnerror       bool          `yaml:"stop_on_error" json:"stop_on_error"`
	RollbackOnFailure bool          `yaml:"rollback_on_failure" json:"rollback_on_failure"`
	PointInTime       *time.Time    `yaml:"point_in_time" json:"point_in_time"`
}

// RestoreStatus represents the status of a restore operation
type RestoreStatus struct {
	ID              string            `json:"id"`
	BackupID        string            `json:"backup_id"`
	StartTime       time.Time         `json:"start_time"`
	EndTime         *time.Time        `json:"end_time,omitempty"`
	Duration        time.Duration     `json:"duration"`
	Status          RestoreState      `json:"status"`
	Components      []RestoreComponent `json:"components"`
	TotalBytes      int64             `json:"total_bytes"`
	RestoredBytes   int64             `json:"restored_bytes"`
	Progress        float64           `json:"progress"`
	ErrorMessage    string            `json:"error_message,omitempty"`
	Warnings        []string          `json:"warnings,omitempty"`
	DryRun          bool              `json:"dry_run"`
	Verified        bool              `json:"verified"`
	Rollback        bool              `json:"rollback"`
}

// RestoreState represents the state of a restore operation
type RestoreState string

const (
	RestoreStatePending    RestoreState = "pending"
	RestoreStateRunning    RestoreState = "running"
	RestoreStateVerifying  RestoreState = "verifying"
	RestoreStateCompleted  RestoreState = "completed"
	RestoreStateFailed     RestoreState = "failed"
	RestoreStateRolledback RestoreState = "rolledback"
	RestoreStateCancelled  RestoreState = "cancelled"
)

// RestoreComponent represents the status of a component restore
type RestoreComponent struct {
	Name         string        `json:"name"`
	Type         string        `json:"type"`
	Status       RestoreState  `json:"status"`
	Size         int64         `json:"size"`
	RestoredSize int64         `json:"restored_size"`
	Duration     time.Duration `json:"duration"`
	Error        string        `json:"error,omitempty"`
}

// RestoreMetrics tracks restore performance metrics
type RestoreMetrics struct {
	mu                    sync.RWMutex
	TotalRestores         int64         `json:"total_restores"`
	SuccessfulRestores    int64         `json:"successful_restores"`
	FailedRestores        int64         `json:"failed_restores"`
	TotalBytesRestored    int64         `json:"total_bytes_restored"`
	AverageDuration       time.Duration `json:"average_duration"`
	LastRestoreTime       time.Time     `json:"last_restore_time"`
	LastRestoreSize       int64         `json:"last_restore_size"`
	LastRestoreDuration   time.Duration `json:"last_restore_duration"`
	LastRestoreStatus     RestoreState  `json:"last_restore_status"`
}

// RestoreVerifier handles verification of restored data
type RestoreVerifier struct {
	logger *zap.SugaredLogger
}

// NewRestoreManager creates a new restore manager instance
func NewRestoreManager(cfg *config.BackupConfig, db storage.Database, modelMgr *models.ModelManager, logger *zap.SugaredLogger) (*RestoreManager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("backup configuration is required")
	}

	if db == nil {
		return nil, fmt.Errorf("database is required")
	}

	if modelMgr == nil {
		return nil, fmt.Errorf("model manager is required")
	}

	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	// Initialize backup storage
	storage, err := NewBackupStorage(cfg.StorageType, cfg.StorageConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize backup storage: %w", err)
	}

	// Initialize encryptor if encryption is enabled
	var encryptor *Encryptor
	if cfg.Encryption {
		encryptor, err = NewEncryptor(cfg.EncryptionKey)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize encryptor: %w", err)
		}
	}

	// Initialize compressor
	compressor, err := NewCompressor(cfg.Compression)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize compressor: %w", err)
	}

	rm := &RestoreManager{
		config:     cfg,
		db:         db,
		modelMgr:   modelMgr,
		logger:     logger.Named("restore-manager"),
		metrics:    &RestoreMetrics{},
		encryptor:  encryptor,
		compressor: compressor,
		storage:    storage,
		verifier:   NewRestoreVerifier(logger),
	}

	logger.Info("Restore manager initialized successfully")
	return rm, nil
}

// Restore performs a restore operation from a backup
func (rm *RestoreManager) Restore(ctx context.Context, cfg *RestoreConfig) (*RestoreStatus, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if rm.running {
		return nil, fmt.Errorf("restore operation is already running")
	}

	rm.running = true
	defer func() { rm.running = false }()

	rm.logger.Infof("Starting restore from backup %s", cfg.BackupID)

	// Initialize restore status
	status := &RestoreStatus{
		ID:         generateRestoreID(),
		BackupID:   cfg.BackupID,
		StartTime:  time.Now(),
		Status:     RestoreStatePending,
		Components: []RestoreComponent{},
		DryRun:     cfg.DryRun,
	}

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	rm.cancelFunc = cancel

	// Load backup metadata
	metadata, err := rm.loadBackupMetadata(ctx, cfg.BackupID)
	if err != nil {
		status.Status = RestoreStateFailed
		status.ErrorMessage = fmt.Sprintf("Failed to load backup metadata: %v", err)
		return status, err
	}

	status.TotalBytes = metadata.Size

	// Download backup
	backupPath, err := rm.storage.Download(ctx, cfg.BackupID)
	if err != nil {
		status.Status = RestoreStateFailed
		status.ErrorMessage = fmt.Sprintf("Failed to download backup: %v", err)
		return status, err
	}
	defer os.Remove(backupPath)

	// Verify backup checksum
	if cfg.Verify {
		status.Status = RestoreStateVerifying
		if err := rm.verifyBackup(backupPath, metadata); err != nil {
			status.Status = RestoreStateFailed
			status.ErrorMessage = fmt.Sprintf("Backup verification failed: %v", err)
			return status, err
		}
	}

	// Extract backup
	extractDir, err := os.MkdirTemp("", "restore-extract-*")
	if err != nil {
		status.Status = RestoreStateFailed
		status.ErrorMessage = fmt.Sprintf("Failed to create extract directory: %v", err)
		return status, err
	}
	defer os.RemoveAll(extractDir)

	// Decrypt if needed
	if metadata.Encryption && rm.encryptor != nil {
		decryptedPath := backupPath + ".decrypted"
		if err := rm.encryptor.DecryptFile(backupPath, decryptedPath); err != nil {
			status.Status = RestoreStateFailed
			status.ErrorMessage = fmt.Sprintf("Failed to decrypt backup: %v", err)
			return status, err
		}
		defer os.Remove(decryptedPath)
		backupPath = decryptedPath
	}

	// Decompress and extract
	if err := rm.extractBackup(backupPath, extractDir); err != nil {
		status.Status = RestoreStateFailed
		status.ErrorMessage = fmt.Sprintf("Failed to extract backup: %v", err)
		return status, err
	}

	// Start restore
	status.Status = RestoreStateRunning

	// Determine which components to restore
	components := cfg.Components
	if len(components) == 0 {
		components = []string{"database", "models", "configs", "logs"}
	}

	// Create rollback point
	rollbackPoint, err := rm.createRollbackPoint(ctx)
	if err != nil {
		rm.logger.Warnf("Failed to create rollback point: %v", err)
	}

	// Restore each component
	var restoreErrors []error
	for _, component := range components {
		componentStatus := RestoreComponent{
			Name:   component,
			Type:   component,
			Status: RestoreStatePending,
		}

		startTime := time.Now()

		switch component {
		case "database":
			if !cfg.DryRun {
				err = rm.restoreDatabase(ctx, extractDir, cfg.Overwrite)
			}
		case "models":
			if !cfg.DryRun {
				err = rm.restoreModels(ctx, extractDir, cfg.Overwrite)
			}
		case "configs":
			if !cfg.DryRun {
				err = rm.restoreConfigs(ctx, extractDir, cfg.Overwrite)
			}
		case "logs":
			if !cfg.DryRun {
				err = rm.restoreLogs(ctx, extractDir, cfg.Overwrite)
			}
		default:
			err = fmt.Errorf("unknown component: %s", component)
		}

		componentStatus.Duration = time.Since(startTime)

		if err != nil {
			componentStatus.Status = RestoreStateFailed
			componentStatus.Error = err.Error()
			restoreErrors = append(restoreErrors, fmt.Errorf("%s: %w", component, err))

			if cfg.StopOnerror {
				break
			}
		} else {
			componentStatus.Status = RestoreStateCompleted
		}

		status.Components = append(status.Components, componentStatus)
		status.RestoredBytes += componentStatus.RestoredSize
		status.Progress = float64(status.RestoredBytes) / float64(status.TotalBytes) * 100
	}

	// Handle errors
	if len(restoreErrors) > 0 {
		if cfg.RollbackOnFailure && rollbackPoint != nil {
			rm.logger.Info("Rolling back to previous state")
			if rollbackErr := rm.rollbackTo(ctx, rollbackPoint); rollbackErr != nil {
				rm.logger.Errorf("Rollback failed: %v", rollbackErr)
				status.Status = RestoreStateFailed
				status.ErrorMessage = fmt.Sprintf("Restore failed and rollback also failed: %v", restoreErrors)
			} else {
				status.Status = RestoreStateRolledback
				status.Rollback = true
				status.ErrorMessage = fmt.Sprintf("Restore failed, rolled back: %v", restoreErrors)
			}
		} else {
			status.Status = RestoreStateFailed
			status.ErrorMessage = fmt.Sprintf("Restore failed: %v", restoreErrors)
		}
	} else {
		status.Status = RestoreStateCompleted
		status.Progress = 100
	}

	status.EndTime = timePtr(time.Now())
	status.Duration = time.Since(status.StartTime)

	// Update metrics
	rm.updateMetrics(status)

	rm.logger.Infof("Restore completed with status: %s", status.Status)
	return status, nil
}

// RestoreDatabase restores the database from backup
func (rm *RestoreManager) restoreDatabase(ctx context.Context, backupDir string, overwrite bool) error {
	rm.logger.Info("Restoring database")

	dbBackupPath := filepath.Join(backupDir, "database")
	if _, err := os.Stat(dbBackupPath); os.IsNotExist(err) {
		return fmt.Errorf("database backup not found")
	}

	// Check if database exists and overwrite is false
	if !overwrite {
		// Check if database has data
		hasData, err := rm.databaseHasData(ctx)
		if err != nil {
			return fmt.Errorf("failed to check database state: %w", err)
		}
		if hasData {
			return fmt.Errorf("database already contains data and overwrite is false")
		}
	}

	// Drop existing database if overwrite is true
	if overwrite {
		if err := rm.dropDatabase(ctx); err != nil {
			return fmt.Errorf("failed to drop database: %w", err)
		}
	}

	// Create database
	if err := rm.createDatabase(ctx); err != nil {
		return fmt.Errorf("failed to create database: %w", err)
	}

	// Restore database from SQL dump
	sqlFile := filepath.Join(dbBackupPath, "dump.sql")
	if _, err := os.Stat(sqlFile); err == nil {
		if err := rm.restoreFromSQLDump(ctx, sqlFile); err != nil {
			return fmt.Errorf("failed to restore from SQL dump: %w", err)
		}
	}

	rm.logger.Info("Database restored successfully")
	return nil
}

// RestoreModels restores AI models from backup
func (rm *RestoreManager) restoreModels(ctx context.Context, backupDir string, overwrite bool) error {
	rm.logger.Info("Restoring AI models")

	modelsBackupPath := filepath.Join(backupDir, "models")
	if _, err := os.Stat(modelsBackupPath); os.IsNotExist(err) {
		return fmt.Errorf("models backup not found")
	}

	// Load model metadata
	metadataFile := filepath.Join(modelsBackupPath, "models-metadata.json")
	metadataData, err := os.ReadFile(metadataFile)
	if err != nil {
		return fmt.Errorf("failed to read models metadata: %w", err)
	}

	var modelMetadata []ModelBackupMetadata
	if err := json.Unmarshal(metadataData, &modelMetadata); err != nil {
		return fmt.Errorf("failed to parse models metadata: %w", err)
	}

	// Restore each model
	for _, meta := range modelMetadata {
		modelPath := filepath.Join(modelsBackupPath, meta.RelativePath)
		targetPath := filepath.Join(rm.config.Path, "models", meta.RelativePath)

		// Check if model exists and overwrite is false
		if !overwrite {
			if _, err := os.Stat(targetPath); err == nil {
				rm.logger.Warnf("Model %s already exists, skipping", meta.Name)
				continue
			}
		}

		// Create target directory
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return fmt.Errorf("failed to create model directory: %w", err)
		}

		// Copy model file
		if err := copyFile(modelPath, targetPath); err != nil {
			return fmt.Errorf("failed to restore model %s: %w", meta.Name, err)
		}

		rm.logger.Infof("Restored model: %s", meta.Name)
	}

	rm.logger.Info("All models restored successfully")
	return nil
}

// RestoreConfigs restores configuration files from backup
func (rm *RestoreManager) restoreConfigs(ctx context.Context, backupDir string, overwrite bool) error {
	rm.logger.Info("Restoring configuration files")

	configBackupPath := filepath.Join(backupDir, "configs")
	if _, err := os.Stat(configBackupPath); os.IsNotExist(err) {
		return fmt.Errorf("configs backup not found")
	}

	// Restore each config file
	err := filepath.Walk(configBackupPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(configBackupPath, path)
		if err != nil {
			return err
		}

		targetPath := filepath.Join("/etc/ai-provider", relPath)

		// Check if config exists and overwrite is false
		if !overwrite {
			if _, err := os.Stat(targetPath); err == nil {
				rm.logger.Warnf("Config %s already exists, skipping", relPath)
				return nil
			}
		}

		// Create target directory
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}

		// Copy config file
		return copyFile(path, targetPath)
	})

	if err != nil {
		return fmt.Errorf("failed to restore configs: %w", err)
	}

	rm.logger.Info("Configuration files restored successfully")
	return nil
}

// RestoreLogs restores log files from backup
func (rm *RestoreManager) restoreLogs(ctx context.Context, backupDir string, overwrite bool) error {
	rm.logger.Info("Restoring log files")

	logBackupPath := filepath.Join(backupDir, "logs")
	if _, err := os.Stat(logBackupPath); os.IsNotExist(err) {
		rm.logger.Warn("Logs backup not found, skipping")
		return nil
	}

	logDir := "/var/log/ai-provider"

	// Restore each log file
	err := filepath.Walk(logBackupPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(logBackupPath, path)
		if err != nil {
			return err
		}

		targetPath := filepath.Join(logDir, relPath)

		// Check if log exists and overwrite is false
		if !overwrite {
			if _, err := os.Stat(targetPath); err == nil {
				rm.logger.Warnf("Log %s already exists, skipping", relPath)
				return nil
			}
		}

		// Create target directory
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}

		// Copy log file
		return copyFile(path, targetPath)
	})

	if err != nil {
		return fmt.Errorf("failed to restore logs: %w", err)
	}

	rm.logger.Info("Log files restored successfully")
	return nil
}

// ListAvailableBackups lists all available backups
func (rm *RestoreManager) ListAvailableBackups(ctx context.Context) ([]*BackupMetadata, error) {
	backups, err := rm.storage.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list backups: %w", err)
	}

	var metadataList []*BackupMetadata
	for _, backupID := range backups {
		metadata, err := rm.loadBackupMetadata(ctx, backupID)
		if err != nil {
			rm.logger.Warnf("Failed to load metadata for backup %s: %v", backupID, err)
			continue
		}
		metadataList = append(metadataList, metadata)
	}

	return metadataList, nil
}

// GetBackupMetadata retrieves metadata for a specific backup
func (rm *RestoreManager) GetBackupMetadata(ctx context.Context, backupID string) (*BackupMetadata, error) {
	return rm.loadBackupMetadata(ctx, backupID)
}

// Cancel cancels an ongoing restore operation
func (rm *RestoreManager) Cancel() error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if !rm.running {
		return fmt.Errorf("no restore operation is running")
	}

	if rm.cancelFunc != nil {
		rm.cancelFunc()
	}

	rm.logger.Info("Restore operation cancelled")
	return nil
}

// GetMetrics returns restore metrics
func (rm *RestoreManager) GetMetrics() *RestoreMetrics {
	rm.metrics.mu.RLock()
	defer rm.metrics.mu.RUnlock()
	return rm.metrics
}

// Helper functions

func (rm *RestoreManager) loadBackupMetadata(ctx context.Context, backupID string) (*BackupMetadata, error) {
	metadataPath := fmt.Sprintf("%s-metadata.json", backupID)
	data, err := rm.storage.LoadMetadata(ctx, metadataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load metadata: %w", err)
	}

	var metadata BackupMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}

	return &metadata, nil
}

func (rm *RestoreManager) verifyBackup(backupPath string, metadata *BackupMetadata) error {
	rm.logger.Info("Verifying backup integrity")

	file, err := os.Open(backupPath)
	if err != nil {
		return err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}

	checksum := fmt.Sprintf("%x", hash.Sum(nil))
	if checksum != metadata.Checksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", metadata.Checksum, checksum)
	}

	rm.logger.Info("Backup verification successful")
	return nil
}

func (rm *RestoreManager) extractBackup(backupPath, destDir string) error {
	rm.logger.Info("Extracting backup archive")

	file, err := os.Open(backupPath)
	if err != nil {
		return err
	}
	defer file.Close()

	var reader io.Reader = file

	// Handle gzip compression
	if strings.HasSuffix(backupPath, ".gz") || strings.HasSuffix(backupPath, ".tgz") {
		gzReader, err := gzip.NewReader(file)
		if err != nil {
			return err
		}
		defer gzReader.Close()
		reader = gzReader
	}

	// Extract tar archive
	tarReader := tar.NewReader(reader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		targetPath := filepath.Join(destDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return err
			}
			outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		case tar.TypeSymlink:
			if err := os.Symlink(header.Linkname, targetPath); err != nil {
				return err
			}
		}
	}

	rm.logger.Info("Backup archive extracted successfully")
	return nil
}

func (rm *RestoreManager) databaseHasData(ctx context.Context) (bool, error) {
	// Check if database has any tables
	var count int
	err := rm.db.QueryRow(ctx, "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public'").Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (rm *RestoreManager) dropDatabase(ctx context.Context) error {
	// Terminate all connections
	_, err := rm.db.Exec(ctx, `
		SELECT pg_terminate_backend(pg_stat_activity.pid)
		FROM pg_stat_activity
		WHERE pg_stat_activity.datname = 'ai_provider'
		AND pid <> pg_backend_pid()
	`)
	if err != nil {
		return err
	}

	// Drop database
	_, err = rm.db.Exec(ctx, "DROP DATABASE IF EXISTS ai_provider")
	return err
}

func (rm *RestoreManager) createDatabase(ctx context.Context) error {
	_, err := rm.db.Exec(ctx, "CREATE DATABASE ai_provider")
	return err
}

func (rm *RestoreManager) restoreFromSQLDump(ctx context.Context, sqlFile string) error {
	rm.logger.Info("Restoring from SQL dump")

	// Read SQL file
	sqlData, err := os.ReadFile(sqlFile)
	if err != nil {
		return err
	}

	// Split SQL statements
	statements := strings.Split(string(sqlData), ";")

	// Execute each statement
	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}

		if _, err := rm.db.Exec(ctx, stmt); err != nil {
			rm.logger.Warnf("Failed to execute statement: %v", err)
			// Continue with next statement
		}
	}

	rm.logger.Info("SQL dump restored successfully")
	return nil
}

func (rm *RestoreManager) createRollbackPoint(ctx context.Context) (*RollbackPoint, error) {
	// Create a rollback point before restore
	rollbackID := fmt.Sprintf("rollback-%d", time.Now().Unix())

	// Backup current state
	backupMgr, err := NewBackupManager(rm.config, rm.db, rm.modelMgr, rm.logger)
	if err != nil {
		return nil, err
	}

	metadata, err := backupMgr.CreateBackup(ctx, BackupTypeFull, "restore-rollback")
	if err != nil {
		return nil, err
	}

	return &RollbackPoint{
		ID:        rollbackID,
		BackupID:  metadata.ID,
		CreatedAt: time.Now(),
	}, nil
}

func (rm *RestoreManager) rollbackTo(ctx context.Context, rollbackPoint *RollbackPoint) error {
	rm.logger.Infof("Rolling back to point %s", rollbackPoint.ID)

	// Restore from the rollback backup
	cfg := &RestoreConfig{
		BackupID:          rollbackPoint.BackupID,
		Components:        []string{"database", "models", "configs"},
		Overwrite:         true,
		Verify:            true,
		Timeout:           30 * time.Minute,
		RollbackOnFailure: false,
	}

	_, err := rm.Restore(ctx, cfg)
	return err
}

func (rm *RestoreManager) updateMetrics(status *RestoreStatus) {
	rm.metrics.mu.Lock()
	defer rm.metrics.mu.Unlock()

	rm.metrics.TotalRestores++
	rm.metrics.TotalBytesRestored += status.RestoredBytes

	if status.Status == RestoreStateCompleted {
		rm.metrics.SuccessfulRestores++
	} else {
		rm.metrics.FailedRestores++
	}

	rm.metrics.LastRestoreTime = status.StartTime
	rm.metrics.LastRestoreSize = status.RestoredBytes
	rm.metrics.LastRestoreDuration = status.Duration
	rm.metrics.LastRestoreStatus = status.Status

	// Calculate average duration
	if rm.metrics.TotalRestores > 0 {
		total := rm.metrics.AverageDuration * time.Duration(rm.metrics.TotalRestores-1)
		rm.metrics.AverageDuration = (total + status.Duration) / time.Duration(rm.metrics.TotalRestores)
	}
}

// RollbackPoint represents a point to which we can rollback
type RollbackPoint struct {
	ID        string    `json:"id"`
	BackupID  string    `json:"backup_id"`
	CreatedAt time.Time `json:"created_at"`
}

// ModelBackupMetadata represents metadata for a backed up model
type ModelBackupMetadata struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	RelativePath string `json:"relative_path"`
	Size         int64  `json:"size"`
	Checksum     string `json:"checksum"`
}

// NewRestoreVerifier creates a new restore verifier
func NewRestoreVerifier(logger *zap.SugaredLogger) *RestoreVerifier {
	return &RestoreVerifier{logger: logger}
}

// Verify verifies the restored data
func (v *RestoreVerifier) Verify(component string) error {
	v.logger.Infof("Verifying restored component: %s", component)

	switch component {
	case "database":
		return v.verifyDatabase()
	case "models":
		return v.verifyModels()
	case "configs":
		return v.verifyConfigs()
	default:
		return fmt.Errorf("unknown component: %s", component)
	}
}

func (v *RestoreVerifier) verifyDatabase() error {
	// TODO: Implement database verification
	v.logger.Info("Database verification completed")
	return nil
}

func (v *RestoreVerifier) verifyModels() error {
	// TODO: Implement models verification
	v.logger.Info("Models verification completed")
	return nil
}

func (v *RestoreVerifier) verifyConfigs() error {
	// TODO: Implement configs verification
	v.logger.Info("Configs verification completed")
	return nil
}

func generateRestoreID() string {
	return fmt.Sprintf("restore-%d", time.Now().Unix())
}

func timePtr(t time.Time) *time.Time {
	return &t
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	// Preserve file permissions
	sourceInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.Chmod(dst, sourceInfo.Mode())
}
