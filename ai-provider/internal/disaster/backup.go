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

// BackupManager handles automated backup operations for disaster recovery
type BackupManager struct {
	config      *config.BackupConfig
	db          storage.Database
	modelMgr    *models.ModelManager
	logger      *zap.SugaredLogger
	mu          sync.RWMutex
	running     bool
	cancelFunc  context.CancelFunc
	metrics     *BackupMetrics
	encryptor   *Encryptor
	compressor  *Compressor
	storage     BackupStorage
	scheduler   *BackupScheduler
	verifier    *BackupVerifier
	retention   *RetentionManager
}

// BackupConfig holds backup configuration
type BackupConfig struct {
	Enabled           bool          `yaml:"enabled" json:"enabled"`
	Schedule          string        `yaml:"schedule" json:"schedule"`
	Path              string        `yaml:"path" json:"path"`
	RetentionDays     int           `yaml:"retention_days" json:"retention_days"`
	Compression       string        `yaml:"compression" json:"compression"` // gzip, lz4, zstd
	Encryption        bool          `yaml:"encryption" json:"encryption"`
	EncryptionKey     string        `yaml:"encryption_key" json:"encryption_key"`
	MaxBackups        int           `yaml:"max_backups" json:"max_backups"`
	BackupDatabase    bool          `yaml:"backup_database" json:"backup_database"`
	BackupModels      bool          `yaml:"backup_models" json:"backup_models"`
	BackupConfigs     bool          `yaml:"backup_configs" json:"backup_configs"`
	BackupLogs        bool          `yaml:"backup_logs" json:"backup_logs"`
	Incremental       bool          `yaml:"incremental" json:"incremental"`
	ParallelWorkers   int           `yaml:"parallel_workers" json:"parallel_workers"`
	Timeout           time.Duration `yaml:"timeout" json:"timeout"`
	VerifyBackups     bool          `yaml:"verify_backups" json:"verify_backups"`
	StorageType       string        `yaml:"storage_type" json:"storage_type"` // local, s3, gcs, azure
	StorageConfig     StorageConfig `yaml:"storage_config" json:"storage_config"`
	NotificationConfig NotificationConfig `yaml:"notification_config" json:"notification_config"`
}

// StorageConfig holds storage backend configuration
type StorageConfig struct {
	// Local storage
	LocalPath string `yaml:"local_path" json:"local_path"`

	// S3 configuration
	S3Bucket          string `yaml:"s3_bucket" json:"s3_bucket"`
	S3Region          string `yaml:"s3_region" json:"s3_region"`
	S3Endpoint        string `yaml:"s3_endpoint" json:"s3_endpoint"`
	S3AccessKeyID     string `yaml:"s3_access_key_id" json:"s3_access_key_id"`
	S3SecretAccessKey string `yaml:"s3_secret_access_key" json:"s3_secret_access_key"`
	S3Prefix          string `yaml:"s3_prefix" json:"s3_prefix"`

	// GCS configuration
	GCSBucket          string `yaml:"gcs_bucket" json:"gcs_bucket"`
	GCSCredentialsFile string `yaml:"gcs_credentials_file" json:"gcs_credentials_file"`
	GCSPrefix          string `yaml:"gcs_prefix" json:"gcs_prefix"`

	// Azure configuration
	AzureContainer      string `yaml:"azure_container" json:"azure_container"`
	AzureStorageAccount string `yaml:"azure_storage_account" json:"azure_storage_account"`
	AzureStorageKey     string `yaml:"azure_storage_key" json:"azure_storage_key"`
	AzurePrefix         string `yaml:"azure_prefix" json:"azure_prefix"`
}

// NotificationConfig holds notification settings
type NotificationConfig struct {
	Enabled    bool     `yaml:"enabled" json:"enabled"`
	OnSuccess  bool     `yaml:"on_success" json:"on_success"`
	OnFailure  bool     `yaml:"on_failure" json:"on_failure"`
	Emails     []string `yaml:"emails" json:"emails"`
	SlackWebhook string  `yaml:"slack_webhook" json:"slack_webhook"`
}

// BackupMetadata contains information about a backup
type BackupMetadata struct {
	ID              string            `json:"id"`
	Timestamp       time.Time         `json:"timestamp"`
	Type            BackupType        `json:"type"`
	Size            int64             `json:"size"`
	CompressedSize  int64             `json:"compressed_size"`
	Checksum        string            `json:"checksum"`
	Duration        time.Duration     `json:"duration"`
	Status          BackupStatus      `json:"status"`
	Components      []BackupComponent `json:"components"`
	Encryption      bool              `json:"encryption"`
	Compression     string            `json:"compression"`
	StorageLocation string            `json:"storage_location"`
	Verified        bool              `json:"verified"`
	VerifiedAt      *time.Time        `json:"verified_at,omitempty"`
	ErrorMessage    string            `json:"error_message,omitempty"`
	Version         string            `json:"version"`
	CreatedBy       string            `json:"created_by"`
	Tags            map[string]string `json:"tags,omitempty"`
}

// BackupType defines the type of backup
type BackupType string

const (
	BackupTypeFull       BackupType = "full"
	BackupTypeIncremental BackupType = "incremental"
	BackupTypeDifferential BackupType = "differential"
	BackupTypeSnapshot   BackupType = "snapshot"
)

// BackupStatus defines the status of a backup
type BackupStatus string

const (
	BackupStatusPending   BackupStatus = "pending"
	BackupStatusRunning   BackupStatus = "running"
	BackupStatusCompleted BackupStatus = "completed"
	BackupStatusFailed    BackupStatus = "failed"
	BackupStatusVerifying BackupStatus = "verifying"
	BackupStatusVerified  BackupStatus = "verified"
)

// BackupComponent represents a backed up component
type BackupComponent struct {
	Name           string    `json:"name"`
	Type           string    `json:"type"` // database, model, config, log
	Size           int64     `json:"size"`
	CompressedSize int64     `json:"compressed_size"`
	Checksum       string    `json:"checksum"`
	Path           string    `json:"path"`
	Status         string    `json:"status"`
	ErrorMessage   string    `json:"error_message,omitempty"`
}

// BackupMetrics tracks backup performance metrics
type BackupMetrics struct {
	mu                  sync.RWMutex
	TotalBackups        int64         `json:"total_backups"`
	SuccessfulBackups   int64         `json:"successful_backups"`
	FailedBackups       int64         `json:"failed_backups"`
	TotalBytesBackup    int64         `json:"total_bytes_backup"`
	TotalBytesCompressed int64        `json:"total_bytes_compressed"`
	AverageDuration     time.Duration `json:"average_duration"`
	LastBackupTime      time.Time     `json:"last_backup_time"`
	LastBackupSize      int64         `json:"last_backup_size"`
	LastBackupDuration  time.Duration `json:"last_backup_duration"`
	LastBackupStatus    BackupStatus  `json:"last_backup_status"`
}

// NewBackupManager creates a new backup manager instance
func NewBackupManager(cfg *config.BackupConfig, db storage.Database, modelMgr *models.ModelManager, logger *zap.SugaredLogger) (*BackupManager, error) {
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

	bm := &BackupManager{
		config:     cfg,
		db:         db,
		modelMgr:   modelMgr,
		logger:     logger.Named("backup-manager"),
		metrics:    &BackupMetrics{},
		encryptor:  encryptor,
		compressor: compressor,
		storage:    storage,
		scheduler:  NewBackupScheduler(cfg.Schedule),
		verifier:   NewBackupVerifier(storage, logger),
		retention:  NewRetentionManager(cfg.RetentionDays, cfg.MaxBackups, logger),
	}

	// Create backup directory if it doesn't exist
	if cfg.StorageType == "local" {
		if err := os.MkdirAll(cfg.StorageConfig.LocalPath, 0755); err != nil {
			return nil, fmt.Errorf("failed to create backup directory: %w", err)
		}
	}

	logger.Info("Backup manager initialized successfully")
	return bm, nil
}

// Start starts the backup manager and scheduler
func (bm *BackupManager) Start(ctx context.Context) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if bm.running {
		return fmt.Errorf("backup manager is already running")
	}

	ctx, cancel := context.WithCancel(ctx)
	bm.cancelFunc = cancel
	bm.running = true

	// Start the scheduler
	go bm.scheduler.Start(ctx, bm.performScheduledBackup)

	bm.logger.Info("Backup manager started successfully")
	return nil
}

// Stop stops the backup manager
func (bm *BackupManager) Stop() error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if !bm.running {
		return nil
	}

	if bm.cancelFunc != nil {
		bm.cancelFunc()
	}

	bm.running = false
	bm.logger.Info("Backup manager stopped")
	return nil
}

// performScheduledBackup is called by the scheduler
func (bm *BackupManager) performScheduledBackup() {
	ctx, cancel := context.WithTimeout(context.Background(), bm.config.Timeout)
	defer cancel()

	bm.logger.Info("Starting scheduled backup")

	backup, err := bm.CreateBackup(ctx, BackupTypeFull, "scheduler")
	if err != nil {
		bm.logger.Errorf("Scheduled backup failed: %v", err)
		bm.notifyFailure(err)
		return
	}

	bm.logger.Infof("Scheduled backup completed successfully: %s", backup.ID)
	bm.notifySuccess(backup)
}

// CreateBackup creates a new backup
func (bm *BackupManager) CreateBackup(ctx context.Context, backupType BackupType, createdBy string) (*BackupMetadata, error) {
	startTime := time.Now()
	backupID := generateBackupID(startTime)

	bm.logger.Infof("Creating backup %s (type: %s)", backupID, backupType)

	metadata := &BackupMetadata{
		ID:         backupID,
		Timestamp:  startTime,
		Type:       backupType,
		Status:     BackupStatusRunning,
		Components: []BackupComponent{},
		Encryption: bm.config.Encryption,
		Compression: bm.config.Compression,
		Version:    "1.0.0",
		CreatedBy:  createdBy,
		Tags:       make(map[string]string),
	}

	// Create temporary directory for backup
	tempDir, err := os.MkdirTemp("", fmt.Sprintf("backup-%s-*", backupID))
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Backup components
	var totalSize, totalCompressedSize int64
	var backupErrors []error

	// Backup database
	if bm.config.BackupDatabase {
		component, err := bm.backupDatabase(ctx, tempDir)
		if err != nil {
			backupErrors = append(backupErrors, fmt.Errorf("database backup failed: %w", err))
			component.Status = "failed"
			component.ErrorMessage = err.Error()
		} else {
			component.Status = "success"
		}
		metadata.Components = append(metadata.Components, component)
		totalSize += component.Size
		totalCompressedSize += component.CompressedSize
	}

	// Backup models
	if bm.config.BackupModels {
		component, err := bm.backupModels(ctx, tempDir)
		if err != nil {
			backupErrors = append(backupErrors, fmt.Errorf("model backup failed: %w", err))
			component.Status = "failed"
			component.ErrorMessage = err.Error()
		} else {
			component.Status = "success"
		}
		metadata.Components = append(metadata.Components, component)
		totalSize += component.Size
		totalCompressedSize += component.CompressedSize
	}

	// Backup configurations
	if bm.config.BackupConfigs {
		component, err := bm.backupConfigs(ctx, tempDir)
		if err != nil {
			backupErrors = append(backupErrors, fmt.Errorf("config backup failed: %w", err))
			component.Status = "failed"
			component.ErrorMessage = err.Error()
		} else {
			component.Status = "success"
		}
		metadata.Components = append(metadata.Components, component)
		totalSize += component.Size
		totalCompressedSize += component.CompressedSize
	}

	// Backup logs
	if bm.config.BackupLogs {
		component, err := bm.backupLogs(ctx, tempDir)
		if err != nil {
			backupErrors = append(backupErrors, fmt.Errorf("log backup failed: %w", err))
			component.Status = "failed"
			component.ErrorMessage = err.Error()
		} else {
			component.Status = "success"
		}
		metadata.Components = append(metadata.Components, component)
		totalSize += component.Size
		totalCompressedSize += component.CompressedSize
	}

	// Create backup archive
	archivePath := filepath.Join(tempDir, fmt.Sprintf("%s.tar.gz", backupID))
	if err := bm.createArchive(tempDir, archivePath, metadata); err != nil {
		return nil, fmt.Errorf("failed to create backup archive: %w", err)
	}

	// Encrypt if enabled
	if bm.config.Encryption {
		encryptedPath := archivePath + ".enc"
		if err := bm.encryptor.EncryptFile(archivePath, encryptedPath); err != nil {
			return nil, fmt.Errorf("failed to encrypt backup: %w", err)
		}
		archivePath = encryptedPath
	}

	// Calculate checksum
	checksum, err := bm.calculateChecksum(archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate checksum: %w", err)
	}

	// Get file size
	fileInfo, err := os.Stat(archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get backup file info: %w", err)
	}

	// Upload to storage
	storageLocation, err := bm.storage.Upload(ctx, archivePath, backupID)
	if err != nil {
		return nil, fmt.Errorf("failed to upload backup: %w", err)
	}

	// Update metadata
	endTime := time.Now()
	metadata.Size = totalSize
	metadata.CompressedSize = fileInfo.Size()
	metadata.Checksum = checksum
	metadata.Duration = endTime.Sub(startTime)
	metadata.StorageLocation = storageLocation

	if len(backupErrors) > 0 {
		metadata.Status = BackupStatusFailed
		metadata.ErrorMessage = fmt.Sprintf("Backup completed with %d errors", len(backupErrors))
		for _, err := range backupErrors {
			metadata.ErrorMessage += fmt.Sprintf("; %v", err)
		}
	} else {
		metadata.Status = BackupStatusCompleted
	}

	// Save metadata
	if err := bm.saveMetadata(ctx, metadata); err != nil {
		return nil, fmt.Errorf("failed to save backup metadata: %w", err)
	}

	// Verify backup if enabled
	if bm.config.VerifyBackups {
		metadata.Status = BackupStatusVerifying
		if err := bm.verifier.Verify(ctx, metadata); err != nil {
			metadata.Status = BackupStatusFailed
			metadata.ErrorMessage = fmt.Sprintf("Backup verification failed: %v", err)
			bm.logger.Errorf("Backup verification failed: %v", err)
		} else {
			metadata.Status = BackupStatusVerified
			metadata.Verified = true
			now := time.Now()
			metadata.VerifiedAt = &now
			bm.logger.Info("Backup verified successfully")
		}
	}

	// Update metrics
	bm.updateMetrics(metadata)

	// Apply retention policy
	go bm.retention.Apply(ctx, bm.storage)

	bm.logger.Infof("Backup %s created successfully in %v", backupID, metadata.Duration)
	return metadata, nil
}

// backupDatabase backs up the database
func (bm *BackupManager) backupDatabase(ctx context.Context, tempDir string) (BackupComponent, error) {
	component := BackupComponent{
		Name: "database",
		Type: "database",
	}

	startTime := time.Now()
	bm.logger.Info("Starting database backup")

	// Create database backup directory
	dbBackupDir := filepath.Join(tempDir, "database")
	if err := os.MkdirAll(dbBackupDir, 0755); err != nil {
		return component, fmt.Errorf("failed to create database backup directory: %w", err)
	}

	// Export database
	backupFile := filepath.Join(dbBackupDir, "database.sql")
	if err := bm.db.Export(ctx, backupFile); err != nil {
		return component, fmt.Errorf("failed to export database: %w", err)
	}

	// Get file size
	fileInfo, err := os.Stat(backupFile)
	if err != nil {
		return component, fmt.Errorf("failed to get database backup file info: %w", err)
	}

	component.Size = fileInfo.Size()
	component.Path = backupFile
	component.Checksum, _ = bm.calculateChecksum(backupFile)

	bm.logger.Infof("Database backup completed in %v", time.Since(startTime))
	return component, nil
}

// backupModels backs up all models
func (bm *BackupManager) backupModels(ctx context.Context, tempDir string) (BackupComponent, error) {
	component := BackupComponent{
		Name: "models",
		Type: "model",
	}

	startTime := time.Now()
	bm.logger.Info("Starting model backup")

	// Create models backup directory
	modelsBackupDir := filepath.Join(tempDir, "models")
	if err := os.MkdirAll(modelsBackupDir, 0755); err != nil {
		return component, fmt.Errorf("failed to create models backup directory: %w", err)
	}

	// Get all models
	models, err := bm.modelMgr.ListModels(ctx)
	if err != nil {
		return component, fmt.Errorf("failed to list models: %w", err)
	}

	// Copy model files
	var totalSize int64
	for _, model := range models {
		select {
		case <-ctx.Done():
			return component, ctx.Err()
		default:
		}

		modelDir := filepath.Join(modelsBackupDir, model.ID)
		if err := os.MkdirAll(modelDir, 0755); err != nil {
			bm.logger.Warnf("Failed to create model backup directory for %s: %v", model.ID, err)
			continue
		}

		// Copy model files
		if err := bm.copyModelFiles(model.StoragePath, modelDir); err != nil {
			bm.logger.Warnf("Failed to backup model %s: %v", model.ID, err)
			continue
		}

		// Save model metadata
		metadataFile := filepath.Join(modelDir, "metadata.json")
		if err := bm.saveModelMetadata(model, metadataFile); err != nil {
			bm.logger.Warnf("Failed to save model metadata for %s: %v", model.ID, err)
		}

		// Calculate size
		if size, err := bm.getDirectorySize(modelDir); err == nil {
			totalSize += size
		}
	}

	component.Size = totalSize
	component.Path = modelsBackupDir

	bm.logger.Infof("Model backup completed in %v (%d models)", time.Since(startTime), len(models))
	return component, nil
}

// backupConfigs backs up configuration files
func (bm *BackupManager) backupConfigs(ctx context.Context, tempDir string) (BackupComponent, error) {
	component := BackupComponent{
		Name: "configs",
		Type: "config",
	}

	startTime := time.Now()
	bm.logger.Info("Starting configuration backup")

	// Create configs backup directory
	configsBackupDir := filepath.Join(tempDir, "configs")
	if err := os.MkdirAll(configsBackupDir, 0755); err != nil {
		return component, fmt.Errorf("failed to create configs backup directory: %w", err)
	}

	// Backup configuration files
	configPaths := []string{
		"/etc/ai-provider/config.yaml",
		"/etc/ai-provider/secrets.yaml",
		"/etc/ai-provider/models.yaml",
	}

	var totalSize int64
	for _, configPath := range configPaths {
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			continue
		}

		destPath := filepath.Join(configsBackupDir, filepath.Base(configPath))
		if err := bm.copyFile(configPath, destPath); err != nil {
			bm.logger.Warnf("Failed to backup config file %s: %v", configPath, err)
			continue
		}

		if fileInfo, err := os.Stat(destPath); err == nil {
			totalSize += fileInfo.Size()
		}
	}

	component.Size = totalSize
	component.Path = configsBackupDir

	bm.logger.Infof("Configuration backup completed in %v", time.Since(startTime))
	return component, nil
}

// backupLogs backs up log files
func (bm *BackupManager) backupLogs(ctx context.Context, tempDir string) (BackupComponent, error) {
	component := BackupComponent{
		Name: "logs",
		Type: "log",
	}

	startTime := time.Now()
	bm.logger.Info("Starting log backup")

	// Create logs backup directory
	logsBackupDir := filepath.Join(tempDir, "logs")
	if err := os.MkdirAll(logsBackupDir, 0755); err != nil {
		return component, fmt.Errorf("failed to create logs backup directory: %w", err)
	}

	// Backup log files
	logDir := "/var/log/ai-provider"
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		component.Size = 0
		component.Path = logsBackupDir
		return component, nil
	}

	var totalSize int64
	err := filepath.Walk(logDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(logDir, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(logsBackupDir, relPath)
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}

		if err := bm.copyFile(path, destPath); err != nil {
			bm.logger.Warnf("Failed to backup log file %s: %v", path, err)
			return nil
		}

		totalSize += info.Size()
		return nil
	})

	if err != nil {
		return component, fmt.Errorf("failed to backup log files: %w", err)
	}

	component.Size = totalSize
	component.Path = logsBackupDir

	bm.logger.Infof("Log backup completed in %v", time.Since(startTime))
	return component, nil
}

// createArchive creates a tar.gz archive of the backup
func (bm *BackupManager) createArchive(sourceDir, archivePath string, metadata *BackupMetadata) error {
	bm.logger.Info("Creating backup archive")

	// Save metadata to file
	metadataFile := filepath.Join(sourceDir, "metadata.json")
	metadataData, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if err := os.WriteFile(metadataFile, metadataData, 0644); err != nil {
		return fmt.Errorf("failed to write metadata file: %w", err)
	}

	// Create archive file
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		return fmt.Errorf("failed to create archive file: %w", err)
	}
	defer archiveFile.Close()

	// Create gzip writer
	gzipWriter := gzip.NewWriter(archiveFile)
	defer gzipWriter.Close()

	// Create tar writer
	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	// Walk through source directory and add files to archive
	err = filepath.Walk(sourceDir, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Create tar header
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(sourceDir, filePath)
		if err != nil {
			return err
		}
		header.Name = relPath

		// Write header
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		// If it's a regular file, copy content
		if !info.IsDir() {
			file, err := os.Open(filePath)
			if err != nil {
				return err
			}
			defer file.Close()

			if _, err := io.Copy(tarWriter, file); err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to create archive: %w", err)
	}

	bm.logger.Info("Backup archive created successfully")
	return nil
}

// RestoreBackup restores from a backup
func (bm *BackupManager) RestoreBackup(ctx context.Context, backupID string) error {
	bm.logger.Infof("Restoring backup %s", backupID)

	// Load backup metadata
	metadata, err := bm.loadMetadata(ctx, backupID)
	if err != nil {
		return fmt.Errorf("failed to load backup metadata: %w", err)
	}

	// Download backup from storage
	backupPath, err := bm.storage.Download(ctx, backupID)
	if err != nil {
		return fmt.Errorf("failed to download backup: %w", err)
	}
	defer os.Remove(backupPath)

	// Decrypt if needed
	if metadata.Encryption {
		decryptedPath := backupPath + ".dec"
		if err := bm.encryptor.DecryptFile(backupPath, decryptedPath); err != nil {
			return fmt.Errorf("failed to decrypt backup: %w", err)
		}
		backupPath = decryptedPath
		defer os.Remove(decryptedPath)
	}

	// Verify checksum
	checksum, err := bm.calculateChecksum(backupPath)
	if err != nil {
		return fmt.Errorf("failed to calculate checksum: %w", err)
	}

	if checksum != metadata.Checksum {
		return fmt.Errorf("backup checksum mismatch: expected %s, got %s", metadata.Checksum, checksum)
	}

	// Extract archive
	tempDir, err := os.MkdirTemp("", fmt.Sprintf("restore-%s-*", backupID))
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	if err := bm.extractArchive(backupPath, tempDir); err != nil {
		return fmt.Errorf("failed to extract backup: %w", err)
	}

	// Restore components
	for _, component := range metadata.Components {
		switch component.Type {
		case "database":
			if err := bm.restoreDatabase(ctx, filepath.Join(tempDir, component.Name)); err != nil {
				bm.logger.Errorf("Failed to restore database: %v", err)
				return err
			}
		case "model":
			if err := bm.restoreModels(ctx, filepath.Join(tempDir, component.Name)); err != nil {
				bm.logger.Errorf("Failed to restore models: %v", err)
				return err
			}
		case "config":
			if err := bm.restoreConfigs(ctx, filepath.Join(tempDir, component.Name)); err != nil {
				bm.logger.Errorf("Failed to restore configs: %v", err)
				return err
			}
		case "log":
			if err := bm.restoreLogs(ctx, filepath.Join(tempDir, component.Name)); err != nil {
				bm.logger.Errorf("Failed to restore logs: %v", err)
				// Don't fail restore for log restoration errors
			}
		}
	}

	bm.logger.Infof("Backup %s restored successfully", backupID)
	return nil
}

// ListBackups lists all available backups
func (bm *BackupManager) ListBackups(ctx context.Context) ([]*BackupMetadata, error) {
	backups, err := bm.storage.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list backups: %w", err)
	}

	var metadataList []*BackupMetadata
	for _, backup := range backups {
		metadata, err := bm.loadMetadata(ctx, backup)
		if err != nil {
			bm.logger.Warnf("Failed to load metadata for backup %s: %v", backup, err)
			continue
		}
		metadataList = append(metadataList, metadata)
	}

	return metadataList, nil
}

// DeleteBackup deletes a backup
func (bm *BackupManager) DeleteBackup(ctx context.Context, backupID string) error {
	bm.logger.Infof("Deleting backup %s", backupID)

	if err := bm.storage.Delete(ctx, backupID); err != nil {
		return fmt.Errorf("failed to delete backup: %w", err)
	}

	bm.logger.Infof("Backup %s deleted successfully", backupID)
	return nil
}

// GetMetrics returns backup metrics
func (bm *BackupManager) GetMetrics() *BackupMetrics {
	bm.metrics.mu.RLock()
	defer bm.metrics.mu.RUnlock()
	return bm.metrics
}

// Helper functions

func generateBackupID(timestamp time.Time) string {
	return fmt.Sprintf("backup-%s", timestamp.Format("20060102-150405"))
}

func (bm *BackupManager) calculateChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func (bm *BackupManager) copyFile(src, dst string) error {
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
	return err
}

func (bm *BackupManager) copyModelFiles(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		return bm.copyFile(path, destPath)
	})
}

func (bm *BackupManager) getDirectorySize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

func (bm *BackupManager) saveModelMetadata(model *models.Model, filePath string) error {
	data, err := json.MarshalIndent(model, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0644)
}

func (bm *BackupManager) saveMetadata(ctx context.Context, metadata *BackupMetadata) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}

	metadataPath := fmt.Sprintf("%s-metadata.json", metadata.ID)
	return bm.storage.SaveMetadata(ctx, metadataPath, data)
}

func (bm *BackupManager) loadMetadata(ctx context.Context, backupID string) (*BackupMetadata, error) {
	metadataPath := fmt.Sprintf("%s-metadata.json", backupID)
	data, err := bm.storage.LoadMetadata(ctx, metadataPath)
	if err != nil {
		return nil, err
	}

	var metadata BackupMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, err
	}

	return &metadata, nil
}

func (bm *BackupManager) extractArchive(archivePath, destDir string) error {
	archiveFile, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer archiveFile.Close()

	gzipReader, err := gzip.NewReader(archiveFile)
	if err != nil {
		return err
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		destPath := filepath.Join(destDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(destPath, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return err
			}
			file, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(file, tarReader); err != nil {
				file.Close()
				return err
			}
			file.Close()
		}
	}

	return nil
}

func (bm *BackupManager) updateMetrics(metadata *BackupMetadata) {
	bm.metrics.mu.Lock()
	defer bm.metrics.mu.Unlock()

	bm.metrics.TotalBackups++
	bm.metrics.LastBackupTime = metadata.Timestamp
	bm.metrics.LastBackupSize = metadata.CompressedSize
	bm.metrics.LastBackupDuration = metadata.Duration
	bm.metrics.LastBackupStatus = metadata.Status

	if metadata.Status == BackupStatusCompleted || metadata.Status == BackupStatusVerified {
		bm.metrics.SuccessfulBackups++
		bm.metrics.TotalBytesBackup += metadata.Size
		bm.metrics.TotalBytesCompressed += metadata.CompressedSize
	} else {
		bm.metrics.FailedBackups++
	}

	// Calculate average duration
	if bm.metrics.SuccessfulBackups > 0 {
		totalDuration := bm.metrics.AverageDuration * time.Duration(bm.metrics.SuccessfulBackups-1)
		bm.metrics.AverageDuration = (totalDuration + metadata.Duration) / time.Duration(bm.metrics.SuccessfulBackups)
	}
}

func (bm *BackupManager) notifySuccess(backup *BackupMetadata) {
	if !bm.config.NotificationConfig.Enabled || !bm.config.NotificationConfig.OnSuccess {
		return
	}

	// Send success notification
	bm.logger.Infof("Sending success notification for backup %s", backup.ID)
	// TODO: Implement notification sending
}

func (bm *BackupManager) notifyFailure(err error) {
	if !bm.config.NotificationConfig.Enabled || !bm.config.NotificationConfig.OnFailure {
		return
	}

	// Send failure notification
	bm.logger.Errorf("Sending failure notification: %v", err)
	// TODO: Implement notification sending
}

// Restore helper methods

func (bm *BackupManager) restoreDatabase(ctx context.Context, backupDir string) error {
	backupFile := filepath.Join(backupDir, "database.sql")
	if _, err := os.Stat(backupFile); os.IsNotExist(err) {
		return fmt.Errorf("database backup file not found")
	}

	return bm.db.Import(ctx, backupFile)
}

func (bm *BackupManager) restoreModels(ctx context.Context, backupDir string) error {
	return filepath.Walk(backupDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() || path == backupDir {
			return nil
		}

		metadataFile := filepath.Join(path, "metadata.json")
		if _, err := os.Stat(metadataFile); os.IsNotExist(err) {
			return nil
		}

		var model models.Model
		data, err := os.ReadFile(metadataFile)
		if err != nil {
			return err
		}

		if err := json.Unmarshal(data, &model); err != nil {
			return err
		}

		// Restore model files
		modelPath := model.StoragePath
		if err := os.MkdirAll(modelPath, 0755); err != nil {
			return err
		}

		return bm.copyModelFiles(path, modelPath)
	})
}

func (bm *BackupManager) restoreConfigs(ctx context.Context, backupDir string) error {
	return filepath.Walk(backupDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		destPath := filepath.Join("/etc/ai-provider", filepath.Base(path))
		return bm.copyFile(path, destPath)
	})
}

func (bm *BackupManager) restoreLogs(ctx context.Context, backupDir string) error {
	logDir := "/var/log/ai-provider"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}

	return filepath.Walk(backupDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(backupDir, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(logDir, relPath)
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}

		return bm.copyFile(path, destPath)
	})
}

// BackupScheduler handles backup scheduling
type BackupScheduler struct {
	schedule string
}

func NewBackupScheduler(schedule string) *BackupScheduler {
	return &BackupScheduler{schedule: schedule}
}

func (s *BackupScheduler) Start(ctx context.Context, callback func()) {
	// TODO: Implement cron-based scheduling
	// For now, just run on start
	callback()
}

// BackupStorage interface for different storage backends
type BackupStorage interface {
	Upload(ctx context.Context, filePath, backupID string) (string, error)
	Download(ctx context.Context, backupID string) (string, error)
	Delete(ctx context.Context, backupID string) error
	List(ctx context.Context) ([]string, error)
	SaveMetadata(ctx context.Context, path string, data []byte) error
	LoadMetadata(ctx context.Context, path string) ([]byte, error)
}

// NewBackupStorage creates a new backup storage backend
func NewBackupStorage(storageType string, config StorageConfig) (BackupStorage, error) {
	switch strings.ToLower(storageType) {
	case "local":
		return NewLocalStorage(config.LocalPath)
	case "s3":
		return NewS3Storage(config.S3Bucket, config.S3Region, config.S3Endpoint,
			config.S3AccessKeyID, config.S3SecretAccessKey, config.S3Prefix)
	case "gcs":
		return NewGCSStorage(config.GCSBucket, config.GCSCredentialsFile, config.GCSPrefix)
	case "azure":
		return NewAzureStorage(config.AzureContainer, config.AzureStorageAccount,
			config.AzureStorageKey, config.AzurePrefix)
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", storageType)
	}
}

// LocalStorage implements BackupStorage for local filesystem
type LocalStorage struct {
	basePath string
}

func NewLocalStorage(basePath string) (*LocalStorage, error) {
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, err
	}
	return &LocalStorage{basePath: basePath}, nil
}

func (s *LocalStorage) Upload(ctx context.Context, filePath, backupID string) (string, error) {
	destPath := filepath.Join(s.basePath, filepath.Base(filePath))
	if err := copyFile(filePath, destPath); err != nil {
		return "", err
	}
	return destPath, nil
}

func (s *LocalStorage) Download(ctx context.Context, backupID string) (string, error) {
	srcPath := filepath.Join(s.basePath, backupID+".tar.gz")
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		// Try encrypted version
		srcPath = filepath.Join(s.basePath, backupID+".tar.gz.enc")
		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			return "", fmt.Errorf("backup not found: %s", backupID)
		}
	}

	tempFile, err := os.CreateTemp("", "download-*")
	if err != nil {
		return "", err
	}
	tempFile.Close()

	if err := copyFile(srcPath, tempFile.Name()); err != nil {
		return "", err
	}

	return tempFile.Name(), nil
}

func (s *LocalStorage) Delete(ctx context.Context, backupID string) error {
	pattern := filepath.Join(s.basePath, backupID+"*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}

	for _, match := range matches {
		if err := os.Remove(match); err != nil {
			return err
		}
	}

	return nil
}

func (s *LocalStorage) List(ctx context.Context) ([]string, error) {
	files, err := os.ReadDir(s.basePath)
	if err != nil {
		return nil, err
	}

	var backups []string
	for _, file := range files {
		if strings.HasPrefix(file.Name(), "backup-") && strings.HasSuffix(file.Name(), ".tar.gz") {
			backupID := strings.TrimSuffix(file.Name(), ".tar.gz")
			backups = append(backups, backupID)
		}
	}

	return backups, nil
}

func (s *LocalStorage) SaveMetadata(ctx context.Context, path string, data []byte) error {
	metadataPath := filepath.Join(s.basePath, path)
	return os.WriteFile(metadataPath, data, 0644)
}

func (s *LocalStorage) LoadMetadata(ctx context.Context, path string) ([]byte, error) {
	metadataPath := filepath.Join(s.basePath, path)
	return os.ReadFile(metadataPath)
}

// S3Storage, GCSStorage, and AzureStorage implementations would go here
// They are stubs for brevity but follow similar patterns

func NewS3Storage(bucket, region, endpoint, accessKeyID, secretAccessKey, prefix string) (BackupStorage, error) {
	// TODO: Implement S3 storage
	return nil, fmt.Errorf("S3 storage not implemented")
}

func NewGCSStorage(bucket, credentialsFile, prefix string) (BackupStorage, error) {
	// TODO: Implement GCS storage
	return nil, fmt.Errorf("GCS storage not implemented")
}

func NewAzureStorage(container, storageAccount, storageKey, prefix string) (BackupStorage, error) {
	// TODO: Implement Azure storage
	return nil, fmt.Errorf("Azure storage not implemented")
}

// Encryptor handles backup encryption/decryption
type Encryptor struct {
	key string
}

func NewEncryptor(key string) (*Encryptor, error) {
	if key == "" {
		return nil, fmt.Errorf("encryption key is required")
	}
	return &Encryptor{key: key}, nil
}

func (e *Encryptor) EncryptFile(src, dst string) error {
	// TODO: Implement AES-256 encryption
	return copyFile(src, dst)
}

func (e *Encryptor) DecryptFile(src, dst string) error {
	// TODO: Implement AES-256 decryption
	return copyFile(src, dst)
}

// Compressor handles backup compression
type Compressor struct {
	algorithm string
}

func NewCompressor(algorithm string) (*Compressor, error) {
	return &Compressor{algorithm: algorithm}, nil
}

// BackupVerifier handles backup verification
type BackupVerifier struct {
	storage BackupStorage
	logger  *zap.SugaredLogger
}

func NewBackupVerifier(storage BackupStorage, logger *zap.SugaredLogger) *BackupVerifier {
	return &BackupVerifier{
		storage: storage,
		logger:  logger,
	}
}

func (v *BackupVerifier) Verify(ctx context.Context, metadata *BackupMetadata) error {
	v.logger.Infof("Verifying backup %s", metadata.ID)

	// Download backup
	backupPath, err := v.storage.Download(ctx, metadata.ID)
	if err != nil {
		return fmt.Errorf("failed to download backup for verification: %w", err)
	}
	defer os.Remove(backupPath)

	// Verify checksum
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
		return fmt.Errorf("checksum mismatch")
	}

	v.logger.Infof("Backup %s verified successfully", metadata.ID)
	return nil
}

// RetentionManager handles backup retention
type RetentionManager struct {
	retentionDays int
	maxBackups    int
	logger        *zap.SugaredLogger
}

func NewRetentionManager(retentionDays, maxBackups int, logger *zap.SugaredLogger) *RetentionManager {
	return &RetentionManager{
		retentionDays: retentionDays,
		maxBackups:    maxBackups,
		logger:        logger,
	}
}

func (r *RetentionManager) Apply(ctx context.Context, storage BackupStorage) error {
	r.logger.Info("Applying retention policy")

	backups, err := storage.List(ctx)
	if err != nil {
		return err
	}

	// Delete old backups
	cutoffTime := time.Now().AddDate(0, 0, -r.retentionDays)

	for _, backupID := range backups {
		metadataPath := fmt.Sprintf("%s-metadata.json", backupID)
		data, err := storage.LoadMetadata(ctx, metadataPath)
		if err != nil {
			r.logger.Warnf("Failed to load metadata for %s: %v", backupID, err)
			continue
		}

		var metadata BackupMetadata
		if err := json.Unmarshal(data, &metadata); err != nil {
			r.logger.Warnf("Failed to parse metadata for %s: %v", backupID, err)
			continue
		}

		if metadata.Timestamp.Before(cutoffTime) {
			r.logger.Infof("Deleting old backup %s (created: %s)", backupID, metadata.Timestamp)
			if err := storage.Delete(ctx, backupID); err != nil {
				r.logger.Errorf("Failed to delete backup %s: %v", backupID, err)
			}
		}
	}

	r.logger.Info("Retention policy applied successfully")
	return nil
}

// Helper function for local storage
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
	return err
}
