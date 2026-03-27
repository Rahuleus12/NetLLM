package operations

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// MigrationManager handles database migration operations
type MigrationManager struct {
	config      *MigrationConfig
	logger      *zap.SugaredLogger
	db          Database
	registry    *MigrationRegistry
	executor    *MigrationExecutor
	validator   *MigrationValidator
	history     *MigrationHistory
	metrics     *MigrationMetrics
	mu          sync.RWMutex
	running     bool
	cancelFunc  context.CancelFunc
}

// MigrationConfig holds migration configuration
type MigrationConfig struct {
	Enabled              bool          `yaml:"enabled" json:"enabled"`
	Path                 string        `yaml:"path" json:"path"`
	TableName            string        `yaml:"table_name" json:"table_name"`
	LockTable            string        `yaml:"lock_table" json:"lock_table"`
	Timeout              time.Duration `yaml:"timeout" json:"timeout"`
	EnableRollback       bool          `yaml:"enable_rollback" json:"enable_rollback"`
	MaxBatchSize         int           `yaml:"max_batch_size" json:"max_batch_size"`
	DryRun               bool          `yaml:"dry_run" json:"dry_run"`
	ValidateBeforeApply  bool          `yaml:"validate_before_apply" json:"validate_before_apply"`
	BackupBeforeMigrate  bool          `yaml:"backup_before_migrate" json:"backup_before_migrate"`
	StopOnError          bool          `yaml:"stop_on_error" json:"stop_on_error"`
	ContinueOnError      bool          `yaml:"continue_on_error" json:"continue_on_error"`
	PreMigrateHooks      []MigrationHook `yaml:"pre_migrate_hooks" json:"pre_migrate_hooks"`
	PostMigrateHooks     []MigrationHook `yaml:"post_migrate_hooks" json:"post_migrate_hooks"`
	PreRollbackHooks     []MigrationHook `yaml:"pre_rollback_hooks" json:"pre_rollback_hooks"`
	PostRollbackHooks    []MigrationHook `yaml:"post_rollback_hooks" json:"post_rollback_hooks"`
	NotificationConfig   NotificationConfig `yaml:"notification_config" json:"notification_config"`
}

// MigrationHook represents a migration hook
type MigrationHook struct {
	Name        string            `yaml:"name" json:"name"`
	Type        HookType          `yaml:"type" json:"type"`
	Command     string            `yaml:"command" json:"command"`
	Args        []string          `yaml:"args" json:"args"`
	Env         map[string]string `yaml:"env" json:"env"`
	Timeout     time.Duration     `yaml:"timeout" json:"timeout"`
	IgnoreError bool              `yaml:"ignore_error" json:"ignore_error"`
}

// Migration represents a database migration
type Migration struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	UpSQL       string            `json:"up_sql"`
	DownSQL     string            `json:"down_sql,omitempty"`
	Checksum    string            `json:"checksum"`
	Status      MigrationStatus   `json:"status"`
	AppliedAt   *time.Time        `json:"applied_at,omitempty"`
	RolledBackAt *time.Time       `json:"rolled_back_at,omitempty"`
	Duration    time.Duration     `json:"duration"`
	Error       string            `json:"error,omitempty"`
	Dependencies []string         `json:"dependencies,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Author      string            `json:"author,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	ModifiedAt  time.Time         `json:"modified_at"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// MigrationStatus defines the status of a migration
type MigrationStatus string

const (
	MigrationStatusPending   MigrationStatus = "pending"
	MigrationStatusRunning   MigrationStatus = "running"
	MigrationStatusApplied   MigrationStatus = "applied"
	MigrationStatusFailed    MigrationStatus = "failed"
	MigrationStatusRolledBack MigrationStatus = "rolled_back"
	MigrationStatusSkipped   MigrationStatus = "skipped"
)

// MigrationType defines the type of migration
type MigrationType string

const (
	MigrationTypeSchema   MigrationType = "schema"
	MigrationTypeData     MigrationType = "data"
	MigrationTypeIndex    MigrationType = "index"
	MigrationTypeFunction MigrationType = "function"
	MigrationTypeTrigger  MigrationType = "trigger"
)

// Database interface for database operations
type Database interface {
	Exec(ctx context.Context, query string, args ...interface{}) error
	Query(ctx context.Context, query string, args ...interface{}) ([]map[string]interface{}, error)
	QueryRow(ctx context.Context, query string, args ...interface{}) (map[string]interface{}, error)
	BeginTx(ctx context.Context) (Transaction, error)
}

// Transaction interface for database transactions
type Transaction interface {
	Exec(ctx context.Context, query string, args ...interface{}) error
	Query(ctx context.Context, query string, args ...interface{}) ([]map[string]interface{}, error)
	Commit() error
	Rollback() error
}

// MigrationRegistry manages migration files
type MigrationRegistry struct {
	migrations map[string]*Migration
	sorted     []string
	mu         sync.RWMutex
	logger     *zap.SugaredLogger
}

// NewMigrationRegistry creates a new migration registry
func NewMigrationRegistry(logger *zap.SugaredLogger) *MigrationRegistry {
	return &MigrationRegistry{
		migrations: make(map[string]*Migration),
		logger:     logger,
	}
}

// Load loads migrations from a directory
func (r *MigrationRegistry) Load(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Clear existing migrations
	r.migrations = make(map[string]*Migration)
	r.sorted = []string{}

	// Read migration files
	files, err := ioutil.ReadDir(path)
	if err != nil {
		return fmt.Errorf("failed to read migration directory: %w", err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		// Parse migration file name (format: V{version}_{name}.sql)
		if !strings.HasSuffix(file.Name(), ".sql") {
			continue
		}

		filePath := filepath.Join(path, file.Name())
		migration, err := r.parseMigrationFile(filePath)
		if err != nil {
			r.logger.Warnw("Failed to parse migration file",
				"file", file.Name(),
				"error", err,
			)
			continue
		}

		r.migrations[migration.Version] = migration
	}

	// Sort migrations by version
	r.sorted = make([]string, 0, len(r.migrations))
	for version := range r.migrations {
		r.sorted = append(r.sorted, version)
	}
	sort.Strings(r.sorted)

	r.logger.Infow("Loaded migrations",
		"count", len(r.migrations),
		"path", path,
	)

	return nil
}

// parseMigrationFile parses a migration file
func (r *MigrationRegistry) parseMigrationFile(path string) (*Migration, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	filename := filepath.Base(path)

	// Parse version and name from filename
	// Format: V{version}_{name}.sql or V{version}_{name}_up.sql
	parts := strings.SplitN(strings.TrimSuffix(filename, ".sql"), "_", 3)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid migration filename format: %s", filename)
	}

	version := strings.TrimPrefix(parts[0], "V")
	name := strings.Join(parts[1:], "_")

	// Parse SQL content
	// Look for -- +migrate Up and -- +migrate Down annotations
	upSQL, downSQL := r.parseSQLContent(string(content))

	// Calculate checksum
	checksum := fmt.Sprintf("%x", sha256.Sum256(content))

	migration := &Migration{
		ID:          uuid.New().String(),
		Version:     version,
		Name:        name,
		Description: fmt.Sprintf("Migration %s: %s", version, name),
		UpSQL:       upSQL,
		DownSQL:     downSQL,
		Checksum:    checksum,
		Status:      MigrationStatusPending,
		CreatedAt:   time.Now(),
		ModifiedAt:  time.Now(),
		Metadata:    make(map[string]interface{}),
	}

	// Parse metadata from comments
	r.parseMetadata(migration, string(content))

	return migration, nil
}

// parseSQLContent parses SQL content for up and down migrations
func (r *MigrationRegistry) parseSQLContent(content string) (string, string) {
	var upSQL, downSQL strings.Builder
	var inUp, inDown bool

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check for annotations
		if strings.HasPrefix(trimmed, "-- +migrate Up") {
			inUp = true
			inDown = false
			continue
		}
		if strings.HasPrefix(trimmed, "-- +migrate Down") {
			inUp = false
			inDown = true
			continue
		}

		// Collect SQL statements
		if inUp {
			upSQL.WriteString(line)
			upSQL.WriteString("\n")
		} else if inDown {
			downSQL.WriteString(line)
			downSQL.WriteString("\n")
		}
	}

	return strings.TrimSpace(upSQL.String()), strings.TrimSpace(downSQL.String())
}

// parseMetadata parses metadata from migration comments
func (r *MigrationRegistry) parseMetadata(migration *Migration, content string) {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Parse annotations
		if strings.HasPrefix(trimmed, "-- +migrate Author:") {
			migration.Author = strings.TrimSpace(strings.TrimPrefix(trimmed, "-- +migrate Author:"))
		}
		if strings.HasPrefix(trimmed, "-- +migrate Tags:") {
			tags := strings.TrimSpace(strings.TrimPrefix(trimmed, "-- +migrate Tags:"))
			migration.Tags = strings.Split(tags, ",")
			for i := range migration.Tags {
				migration.Tags[i] = strings.TrimSpace(migration.Tags[i])
			}
		}
		if strings.HasPrefix(trimmed, "-- +migrate Depends:") {
			deps := strings.TrimSpace(strings.TrimPrefix(trimmed, "-- +migrate Depends:"))
			migration.Dependencies = strings.Split(deps, ",")
			for i := range migration.Dependencies {
				migration.Dependencies[i] = strings.TrimSpace(migration.Dependencies[i])
			}
		}
	}
}

// Get retrieves a migration by version
func (r *MigrationRegistry) Get(version string) (*Migration, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	migration, exists := r.migrations[version]
	if !exists {
		return nil, false
	}

	// Return a copy to avoid mutations
	copy := *migration
	return &copy, true
}

// List returns all migrations sorted by version
func (r *MigrationRegistry) List() []*Migration {
	r.mu.RLock()
	defer r.mu.RUnlock()

	migrations := make([]*Migration, 0, len(r.sorted))
	for _, version := range r.sorted {
		if migration, exists := r.migrations[version]; exists {
			copy := *migration
			migrations = append(migrations, &copy)
		}
	}

	return migrations
}

// GetPending returns migrations that haven't been applied
func (r *MigrationRegistry) GetPending(appliedVersions map[string]bool) []*Migration {
	r.mu.RLock()
	defer r.mu.RUnlock()

	pending := make([]*Migration, 0)
	for _, version := range r.sorted {
		if !appliedVersions[version] {
			if migration, exists := r.migrations[version]; exists {
				copy := *migration
				pending = append(pending, &copy)
			}
		}
	}

	return pending
}

// Add adds a new migration to the registry
func (r *MigrationRegistry) Add(migration *Migration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.migrations[migration.Version] = migration

	// Re-sort
	r.sorted = append(r.sorted, migration.Version)
	sort.Strings(r.sorted)
}

// MigrationExecutor executes migrations
type MigrationExecutor struct {
	config *MigrationConfig
	db     Database
	logger *zap.SugaredLogger
}

// NewMigrationExecutor creates a new migration executor
func NewMigrationExecutor(config *MigrationConfig, db Database, logger *zap.SugaredLogger) *MigrationExecutor {
	return &MigrationExecutor{
		config: config,
		db:     db,
		logger: logger,
	}
}

// Execute executes a migration
func (e *MigrationExecutor) Execute(ctx context.Context, migration *Migration, direction string) error {
	e.logger.Infow("Executing migration",
		"version", migration.Version,
		"name", migration.Name,
		"direction", direction,
	)

	var sql string
	if direction == "up" {
		sql = migration.UpSQL
	} else {
		sql = migration.DownSQL
	}

	if sql == "" {
		return fmt.Errorf("no SQL found for %s migration", direction)
	}

	// Check for dry run
	if e.config.DryRun {
		e.logger.Infow("Dry run - skipping execution",
			"version", migration.Version,
			"direction", direction,
		)
		return nil
	}

	// Start transaction
	tx, err := e.db.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Execute migration
	if err := tx.Exec(ctx, sql); err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to execute migration: %w", err)
	}

	// Record migration in history table
	if err := e.recordMigration(ctx, tx, migration, direction); err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to record migration: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	e.logger.Infow("Migration executed successfully",
		"version", migration.Version,
		"direction", direction,
	)

	return nil
}

// recordMigration records a migration in the history table
func (e *MigrationExecutor) recordMigration(ctx context.Context, tx Transaction, migration *Migration, direction string) error {
	now := time.Now()

	if direction == "up" {
		query := fmt.Sprintf(`
			INSERT INTO %s (version, name, checksum, applied_at, execution_time)
			VALUES ($1, $2, $3, $4, $5)
		`, e.config.TableName)

		if err := tx.Exec(ctx, query,
			migration.Version,
			migration.Name,
			migration.Checksum,
			now,
			migration.Duration,
		); err != nil {
			return err
		}
	} else {
		query := fmt.Sprintf(`
			UPDATE %s
			SET rolled_back_at = $1
			WHERE version = $2
		`, e.config.TableName)

		if err := tx.Exec(ctx, query, now, migration.Version); err != nil {
			return err
		}
	}

	return nil
}

// MigrationValidator validates migrations
type MigrationValidator struct {
	logger *zap.SugaredLogger
}

// NewMigrationValidator creates a new migration validator
func NewMigrationValidator(logger *zap.SugaredLogger) *MigrationValidator {
	return &MigrationValidator{
		logger: logger,
	}
}

// Validate validates a migration
func (v *MigrationValidator) Validate(migration *Migration) error {
	var errors []string

	if migration.Version == "" {
		errors = append(errors, "version is required")
	}

	if migration.Name == "" {
		errors = append(errors, "name is required")
	}

	if migration.UpSQL == "" {
		errors = append(errors, "up SQL is required")
	}

	if migration.Checksum == "" {
		errors = append(errors, "checksum is required")
	}

	// Validate SQL syntax (basic check)
	if err := v.validateSQLSyntax(migration.UpSQL); err != nil {
		errors = append(errors, fmt.Sprintf("up SQL syntax error: %v", err))
	}

	if migration.DownSQL != "" {
		if err := v.validateSQLSyntax(migration.DownSQL); err != nil {
			errors = append(errors, fmt.Sprintf("down SQL syntax error: %v", err))
		}
	}

	// Validate dependencies
	for _, dep := range migration.Dependencies {
		if dep == "" {
			errors = append(errors, "dependency version cannot be empty")
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("validation failed: %v", errors)
	}

	return nil
}

// validateSQLSyntax performs basic SQL syntax validation
func (v *MigrationValidator) validateSQLSyntax(sql string) error {
	// Basic SQL syntax checks
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return fmt.Errorf("SQL is empty")
	}

	// Check for balanced parentheses
	parenCount := 0
	for _, ch := range sql {
		if ch == '(' {
			parenCount++
		} else if ch == ')' {
			parenCount--
			if parenCount < 0 {
				return fmt.Errorf("unbalanced parentheses")
			}
		}
	}

	if parenCount != 0 {
		return fmt.Errorf("unbalanced parentheses")
	}

	return nil
}

// MigrationHistory tracks migration history
type MigrationHistory struct {
	applied map[string]*MigrationRecord
	mu      sync.RWMutex
}

// MigrationRecord represents a migration record in the database
type MigrationRecord struct {
	Version       string        `json:"version"`
	Name          string        `json:"name"`
	Checksum      string        `json:"checksum"`
	AppliedAt     time.Time     `json:"applied_at"`
	RolledBackAt  *time.Time    `json:"rolled_back_at,omitempty"`
	ExecutionTime time.Duration `json:"execution_time"`
}

// NewMigrationHistory creates a new migration history
func NewMigrationHistory() *MigrationHistory {
	return &MigrationHistory{
		applied: make(map[string]*MigrationRecord),
	}
}

// Load loads migration history from database
func (h *MigrationHistory) Load(ctx context.Context, db Database, tableName string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	query := fmt.Sprintf("SELECT version, name, checksum, applied_at, rolled_back_at, execution_time FROM %s", tableName)

	rows, err := db.Query(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to query migration history: %w", err)
	}

	h.applied = make(map[string]*MigrationRecord)

	for _, row := range rows {
		record := &MigrationRecord{
			Version:  row["version"].(string),
			Name:     row["name"].(string),
			Checksum: row["checksum"].(string),
		}

		if appliedAt, ok := row["applied_at"].(time.Time); ok {
			record.AppliedAt = appliedAt
		}

		if rolledBackAt, ok := row["rolled_back_at"].(*time.Time); ok {
			record.RolledBackAt = rolledBackAt
		}

		if execTime, ok := row["execution_time"].(time.Duration); ok {
			record.ExecutionTime = execTime
		}

		h.applied[record.Version] = record
	}

	return nil
}

// Get retrieves a migration record
func (h *MigrationHistory) Get(version string) (*MigrationRecord, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	record, exists := h.applied[version]
	return record, exists
}

// List returns all applied migrations
func (h *MigrationHistory) List() []*MigrationRecord {
	h.mu.RLock()
	defer h.mu.RUnlock()

	records := make([]*MigrationRecord, 0, len(h.applied))
	for _, record := range h.applied {
		records = append(records, record)
	}

	return records
}

// GetAppliedVersions returns a set of applied migration versions
func (h *MigrationHistory) GetAppliedVersions() map[string]bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	versions := make(map[string]bool)
	for version, record := range h.applied {
		// Only include if not rolled back
		if record.RolledBackAt == nil {
			versions[version] = true
		}
	}

	return versions
}

// MigrationMetrics tracks migration metrics
type MigrationMetrics struct {
	TotalMigrations    int64
	AppliedMigrations  int64
	FailedMigrations   int64
	RolledBackMigrations int64
	TotalExecutionTime time.Duration
	AverageExecutionTime time.Duration
	mu                 sync.Mutex
}

// NewMigrationMetrics creates new migration metrics
func NewMigrationMetrics() *MigrationMetrics {
	return &MigrationMetrics{}
}

// RecordMigration records a migration metric
func (m *MigrationMetrics) RecordMigration(migration *Migration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.TotalMigrations++
	m.TotalExecutionTime += migration.Duration

	switch migration.Status {
	case MigrationStatusApplied:
		m.AppliedMigrations++
	case MigrationStatusFailed:
		m.FailedMigrations++
	case MigrationStatusRolledBack:
		m.RolledBackMigrations++
	}

	// Calculate average
	m.AverageExecutionTime = time.Duration(int64(m.TotalExecutionTime) / m.TotalMigrations)
}

// GetMetrics returns current metrics
func (m *MigrationMetrics) GetMetrics() map[string]interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()

	return map[string]interface{}{
		"total_migrations":       m.TotalMigrations,
		"applied_migrations":     m.AppliedMigrations,
		"failed_migrations":      m.FailedMigrations,
		"rolled_back_migrations": m.RolledBackMigrations,
		"total_execution_time":   m.TotalExecutionTime.String(),
		"average_execution_time": m.AverageExecutionTime.String(),
	}
}

// NewMigrationManager creates a new migration manager
func NewMigrationManager(config *MigrationConfig, db Database, logger *zap.SugaredLogger) *MigrationManager {
	return &MigrationManager{
		config:    config,
		logger:    logger,
		db:        db,
		registry:  NewMigrationRegistry(logger),
		executor:  NewMigrationExecutor(config, db, logger),
		validator: NewMigrationValidator(logger),
		history:   NewMigrationHistory(),
		metrics:   NewMigrationMetrics(),
	}
}

// Initialize initializes the migration manager
func (mm *MigrationManager) Initialize(ctx context.Context) error {
	mm.logger.Infow("Initializing migration manager")

	// Create migrations table if it doesn't exist
	if err := mm.createMigrationsTable(ctx); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Load migrations from directory
	if err := mm.registry.Load(mm.config.Path); err != nil {
		return fmt.Errorf("failed to load migrations: %w", err)
	}

	// Load migration history from database
	if err := mm.history.Load(ctx, mm.db, mm.config.TableName); err != nil {
		mm.logger.Warnw("Failed to load migration history, starting fresh", "error", err)
	}

	mm.logger.Infow("Migration manager initialized",
		"migrations_loaded", len(mm.registry.List()),
		"migrations_applied", len(mm.history.List()),
	)

	return nil
}

// createMigrationsTable creates the migrations tracking table
func (mm *MigrationManager) createMigrationsTable(ctx context.Context) error {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			version VARCHAR(255) PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			checksum VARCHAR(64) NOT NULL,
			applied_at TIMESTAMP NOT NULL,
			rolled_back_at TIMESTAMP,
			execution_time INTERVAL
		)
	`, mm.config.TableName)

	return mm.db.Exec(ctx, query)
}

// Migrate runs all pending migrations
func (mm *MigrationManager) Migrate(ctx context.Context) (*MigrationResult, error) {
	mm.mu.Lock()
	if mm.running {
		mm.mu.Unlock()
		return nil, fmt.Errorf("migration already in progress")
	}

	ctx, cancel := context.WithCancel(ctx)
	mm.cancelFunc = cancel
	mm.running = true
	mm.mu.Unlock()

	defer func() {
		mm.mu.Lock()
		mm.running = false
		mm.cancelFunc = nil
		mm.mu.Unlock()
	}()

	result := &MigrationResult{
		StartTime: time.Now(),
		Status:    "running",
	}

	// Get pending migrations
	appliedVersions := mm.history.GetAppliedVersions()
	pending := mm.registry.GetPending(appliedVersions)

	if len(pending) == 0 {
		result.Status = "success"
		result.Message = "No pending migrations"
		return result, nil
	}

	mm.logger.Infow("Starting migration",
		"pending_count", len(pending),
	)

	// Execute pre-migrate hooks
	if err := mm.executeHooks(ctx, mm.config.PreMigrateHooks); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("pre-migrate hooks failed: %v", err)
		return result, err
	}

	// Apply migrations
	applied := 0
	failed := 0

	for _, migration := range pending {
		select {
		case <-ctx.Done():
			result.Status = "cancelled"
			result.Message = "Migration cancelled"
			return result, ctx.Err()
		default:
		}

		// Validate migration
		if mm.config.ValidateBeforeApply {
			if err := mm.validator.Validate(migration); err != nil {
				migration.Status = MigrationStatusFailed
				migration.Error = err.Error()

				mm.logger.Errorw("Migration validation failed",
					"version", migration.Version,
					"error", err,
				)

				failed++

				if mm.config.StopOnError {
					break
				}

				if !mm.config.ContinueOnError {
					result.Status = "failed"
					result.Error = fmt.Sprintf("migration %s validation failed: %v", migration.Version, err)
					return result, fmt.Errorf(result.Error)
				}

				continue
			}
		}

		// Apply migration
		migration.Status = MigrationStatusRunning
		startTime := time.Now()

		if err := mm.executor.Execute(ctx, migration, "up"); err != nil {
			migration.Status = MigrationStatusFailed
			migration.Error = err.Error()
			migration.Duration = time.Since(startTime)

			mm.metrics.RecordMigration(migration)

			mm.logger.Errorw("Migration failed",
				"version", migration.Version,
				"error", err,
			)

			failed++

			if mm.config.StopOnError {
				break
			}

			if !mm.config.ContinueOnError {
				result.Status = "failed"
				result.Error = fmt.Sprintf("migration %s failed: %v", migration.Version, err)
				return result, fmt.Errorf(result.Error)
			}

			continue
		}

		migration.Status = MigrationStatusApplied
		migration.Duration = time.Since(startTime)
		now := time.Now()
		migration.AppliedAt = &now

		mm.metrics.RecordMigration(migration)

		mm.logger.Infow("Migration applied successfully",
			"version", migration.Version,
			"duration", migration.Duration,
		)

		applied++
		result.Migrations = append(result.Migrations, migration)
	}

	// Execute post-migrate hooks
	if err := mm.executeHooks(ctx, mm.config.PostMigrateHooks); err != nil {
		mm.logger.Warnw("Post-migrate hooks failed", "error", err)
	}

	result.EndTime = time.Time{}
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.Applied = applied
	result.Failed = failed

	if failed > 0 && applied == 0 {
		result.Status = "failed"
	} else if failed > 0 {
		result.Status = "partial"
	} else {
		result.Status = "success"
	}

	mm.logger.Infow("Migration completed",
		"status", result.Status,
		"applied", applied,
		"failed", failed,
		"duration", result.Duration,
	)

	return result, nil
}

// MigrationResult represents the result of a migration operation
type MigrationResult struct {
	StartTime  time.Time     `json:"start_time"`
	EndTime    time.Time     `json:"end_time"`
	Duration   time.Duration `json:"duration"`
	Status     string        `json:"status"`
	Message    string        `json:"message,omitempty"`
	Applied    int           `json:"applied"`
	Failed     int           `json:"failed"`
	Migrations []*Migration  `json:"migrations"`
	Error      string        `json:"error,omitempty"`
}

// Rollback rolls back the last N migrations
func (mm *MigrationManager) Rollback(ctx context.Context, steps int) (*MigrationResult, error) {
	if !mm.config.EnableRollback {
		return nil, fmt.Errorf("rollback is disabled")
	}

	mm.mu.Lock()
	if mm.running {
		mm.mu.Unlock()
		return nil, fmt.Errorf("migration already in progress")
	}

	ctx, cancel := context.WithCancel(ctx)
	mm.cancelFunc = cancel
	mm.running = true
	mm.mu.Unlock()

	defer func() {
		mm.mu.Lock()
		mm.running = false
		mm.cancelFunc = nil
		mm.mu.Unlock()
	}()

	result := &MigrationResult{
		StartTime: time.Now(),
		Status:    "running",
	}

	// Get applied migrations in reverse order
	applied := mm.history.List()
	if len(applied) == 0 {
		result.Status = "success"
		result.Message = "No migrations to rollback"
		return result, nil
	}

	// Sort by applied_at descending
	sort.Slice(applied, func(i, j int) bool {
		return applied[i].AppliedAt.After(applied[j].AppliedAt)
	})

	// Limit to steps
	if steps > len(applied) {
		steps = len(applied)
	}

	toRollback := applied[:steps]

	mm.logger.Infow("Starting rollback",
		"count", len(toRollback),
	)

	// Execute pre-rollback hooks
	if err := mm.executeHooks(ctx, mm.config.PreRollbackHooks); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("pre-rollback hooks failed: %v", err)
		return result, err
	}

	// Rollback migrations
	rolledBack := 0
	failed := 0

	for _, record := range toRollback {
		select {
		case <-ctx.Done():
			result.Status = "cancelled"
			result.Message = "Rollback cancelled"
			return result, ctx.Err()
		default:
		}

		// Get migration
		migration, exists := mm.registry.Get(record.Version)
		if !exists {
			mm.logger.Warnw("Migration not found in registry",
				"version", record.Version,
			)
			continue
		}

		// Check if down SQL exists
		if migration.DownSQL == "" {
			mm.logger.Warnw("Migration has no down SQL, skipping",
				"version", migration.Version,
			)
			continue
		}

		// Rollback migration
		migration.Status = MigrationStatusRunning
		startTime := time.Now()

		if err := mm.executor.Execute(ctx, migration, "down"); err != nil {
			migration.Status = MigrationStatusFailed
			migration.Error = err.Error()
			migration.Duration = time.Since(startTime)

			mm.metrics.RecordMigration(migration)

			mm.logger.Errorw("Rollback failed",
				"version", migration.Version,
				"error", err,
			)

			failed++

			if mm.config.StopOnError {
				break
			}

			continue
		}

		migration.Status = MigrationStatusRolledBack
		migration.Duration = time.Since(startTime)
		now := time.Now()
		migration.RolledBackAt = &now

		mm.metrics.RecordMigration(migration)

		mm.logger.Infow("Migration rolled back successfully",
			"version", migration.Version,
			"duration", migration.Duration,
		)

		rolledBack++
		result.Migrations = append(result.Migrations, migration)
	}

	// Execute post-rollback hooks
	if err := mm.executeHooks(ctx, mm.config.PostRollbackHooks); err != nil {
		mm.logger.Warnw("Post-rollback hooks failed", "error", err)
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.Applied = rolledBack
	result.Failed = failed

	if failed > 0 && rolledBack == 0 {
		result.Status = "failed"
	} else if failed > 0 {
		result.Status = "partial"
	} else {
		result.Status = "success"
	}

	mm.logger.Infow("Rollback completed",
		"status", result.Status,
		"rolled_back", rolledBack,
		"failed", failed,
		"duration", result.Duration,
	)

	return result, nil
}

// executeHooks executes migration hooks
func (mm *MigrationManager) executeHooks(ctx context.Context, hooks []MigrationHook) error {
	for _, hook := range hooks {
		mm.logger.Infow("Executing hook", "hook_name", hook.Name, "type", hook.Type)

		// In production, this would execute the actual hook
		// For now, we simulate it
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
			// Simulate hook execution
		}

		mm.logger.Infow("Hook completed", "hook_name", hook.Name)
	}
	return nil
}

// Status returns the current migration status
func (mm *MigrationManager) Status() (*MigrationStatusResponse, error) {
	appliedVersions := mm.history.GetAppliedVersions()
	pending := mm.registry.GetPending(appliedVersions)

	applied := mm.history.List()

	// Get last applied migration
	var lastApplied *MigrationRecord
	if len(applied) > 0 {
		sort.Slice(applied, func(i, j int) bool {
			return applied[i].AppliedAt.After(applied[j].AppliedAt)
		})
		lastApplied = applied[0]
	}

	return &MigrationStatusResponse{
		TotalMigrations:   len(mm.registry.List()),
		AppliedMigrations: len(applied),
		PendingMigrations: len(pending),
		LastApplied:       lastApplied,
		Pending:           pending,
		IsRunning:         mm.running,
	}, nil
}

// MigrationStatusResponse represents migration status
type MigrationStatusResponse struct {
	TotalMigrations   int               `json:"total_migrations"`
	AppliedMigrations int               `json:"applied_migrations"`
	PendingMigrations int               `json:"pending_migrations"`
	LastApplied       *MigrationRecord  `json:"last_applied,omitempty"`
	Pending           []*Migration      `json:"pending,omitempty"`
	IsRunning         bool              `json:"is_running"`
}

// Cancel cancels the current migration
func (mm *MigrationManager) Cancel() error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if !mm.running {
		return fmt.Errorf("no migration in progress")
	}

	if mm.cancelFunc != nil {
		mm.cancelFunc()
	}

	mm.logger.Infow("Migration cancelled")

	return nil
}

// GetMigration retrieves a specific migration
func (mm *MigrationManager) GetMigration(version string) (*Migration, bool) {
	return mm.registry.Get(version)
}

// ListMigrations lists all migrations
func (mm *MigrationManager) ListMigrations() []*Migration {
	return mm.registry.List()
}

// GetMetrics returns migration metrics
func (mm *MigrationManager) GetMetrics() map[string]interface{} {
	return mm.metrics.GetMetrics()
}

// Export exports migration history to JSON
func (mm *MigrationManager) Export() ([]byte, error) {
	export := struct {
		Migrations  []*Migration        `json:"migrations"`
		History     []*MigrationRecord  `json:"history"`
		Metrics     map[string]interface{} `json:"metrics"`
		ExportTime  time.Time          `json:"export_time"`
	}{
		Migrations: mm.registry.List(),
		History:    mm.history.List(),
		Metrics:    mm.metrics.GetMetrics(),
		ExportTime: time.Now(),
	}

	return json.MarshalIndent(export, "", "  ")
}

// Create creates a new migration file
func (mm *MigrationManager) Create(name string, migrationType MigrationType) (string, error) {
	// Generate version based on timestamp
	version := time.Now().Format("20060102150405")

	// Create filename
	filename := fmt.Sprintf("V%s_%s.sql", version, strings.ReplaceAll(strings.ToLower(name), " ", "_"))
	path := filepath.Join(mm.config.Path, filename)

	// Create content
	content := fmt.Sprintf(`-- +migrate Up
-- Migration: %s
-- Created: %s
-- Type: %s
-- Author: System

-- Add your up migration SQL here


-- +migrate Down
-- Add your down migration SQL here

`, name, time.Now().Format(time.RFC3339), migrationType)

	// Write file
	if err := ioutil.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to create migration file: %w", err)
	}

	mm.logger.Infow("Created migration file",
		"path", path,
		"version", version,
		"name", name,
	)

	return path, nil
}

// Verify verifies migration integrity
func (mm *MigrationManager) Verify(ctx context.Context) ([]*VerificationResult, error) {
	results := make([]*VerificationResult, 0)

	appliedVersions := mm.history.GetAppliedVersions()
	migrations := mm.registry.List()

	for _, migration := range migrations {
		result := &VerificationResult{
			Version: migration.Version,
			Name:    migration.Name,
		}

		record, applied := appliedVersions[migration.Version]

		if applied {
			// Verify checksum
			if record.Checksum != migration.Checksum {
				result.Status = "checksum_mismatch"
				result.Message = "Migration checksum does not match applied version"
			} else {
				result.Status = "valid"
				result.Message = "Migration is valid"
			}
		} else {
			result.Status = "pending"
			result.Message = "Migration is pending"
		}

		results = append(results, result)
	}

	return results, nil
}

// VerificationResult represents a migration verification result
type VerificationResult struct {
	Version string `json:"version"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// IsRunning returns whether a migration is currently running
func (mm *MigrationManager) IsRunning() bool {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	return mm.running
}

// GetPendingCount returns the number of pending migrations
func (mm *MigrationManager) GetPendingCount() int {
	appliedVersions := mm.history.GetAppliedVersions()
	pending := mm.registry.GetPending(appliedVersions)
	return len(pending)
}

// Reset resets migration history (use with caution!)
func (mm *MigrationManager) Reset(ctx context.Context) error {
	if mm.running {
		return fmt.Errorf("cannot reset while migration is running")
	}

	mm.logger.Warnw("Resetting migration history - USE WITH CAUTION!")

	// Clear history table
	query := fmt.Sprintf("DELETE FROM %s", mm.config.TableName)
	if err := mm.db.Exec(ctx, query); err != nil {
		return fmt.Errorf("failed to clear migration history: %w", err)
	}

	// Reload history
	if err := mm.history.Load(ctx, mm.db, mm.config.TableName); err != nil {
		return fmt.Errorf("failed to reload migration history: %w", err)
	}

	mm.logger.Infow("Migration history reset completed")

	return nil
}

// Import imports migrations from a directory
func (mm *MigrationManager) Import(path string) error {
	return mm.registry.Load(path)
}

// Diff compares local migrations with database state
func (mm *MigrationManager) Diff() (*DiffResult, error) {
	appliedVersions := mm.history.GetAppliedVersions()
	localMigrations := mm.registry.List()

	result := &DiffResult{
		LocalOnly:  make([]*Migration, 0),
		RemoteOnly: make([]*MigrationRecord, 0),
		Common:     make([]*Migration, 0),
	}

	// Find local-only migrations
	for _, migration := range localMigrations {
		if !appliedVersions[migration.Version] {
			result.LocalOnly = append(result.LocalOnly, migration)
		} else {
			result.Common = append(result.Common, migration)
		}
	}

	// Find remote-only migrations
	for _, record := range mm.history.List() {
		if _, exists := mm.registry.Get(record.Version); !exists {
			result.RemoteOnly = append(result.RemoteOnly, record)
		}
	}

	return result, nil
}

// DiffResult represents a diff result
type DiffResult struct {
	LocalOnly  []*Migration       `json:"local_only"`
	RemoteOnly []*MigrationRecord `json:"remote_only"`
	Common     []*Migration       `json:"common"`
}

// GenerateDownSQL generates down SQL for migrations that don't have it
func (mm *MigrationManager) GenerateDownSQL(migration *Migration) (string, error) {
	// This is a placeholder - in production, this would use AI or heuristics
	// to generate rollback SQL based on the up SQL

	upSQL := strings.TrimSpace(migration.UpSQL)

	// Simple pattern matching for common operations
	if strings.Contains(strings.ToUpper(upSQL), "CREATE TABLE") {
		// Extract table name
		lines := strings.Split(upSQL, "\n")
		for _, line := range lines {
			if strings.Contains(strings.ToUpper(line), "CREATE TABLE") {
				parts := strings.Fields(line)
				for i, part := range parts {
					if strings.ToUpper(part) == "TABLE" && i+1 < len(parts) {
						tableName := strings.Trim(parts[i+1], "();")
						return fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE;", tableName), nil
					}
				}
			}
		}
	}

	if strings.Contains(strings.ToUpper(upSQL), "ALTER TABLE") {
		// This would need more sophisticated parsing
		return "-- TODO: Generate down SQL for ALTER TABLE operations", nil
	}

	if strings.Contains(strings.ToUpper(upSQL), "CREATE INDEX") {
		// Extract index name
		lines := strings.Split(upSQL, "\n")
		for _, line := range lines {
			if strings.Contains(strings.ToUpper(line), "CREATE INDEX") {
				parts := strings.Fields(line)
				for i, part := range parts {
					if strings.ToUpper(part) == "INDEX" && i+1 < len(parts) {
						indexName := strings.Trim(parts[i+1], "();")
						return fmt.Sprintf("DROP INDEX IF EXISTS %s;", indexName), nil
					}
				}
			}
		}
	}

	return "-- TODO: Generate down SQL", nil
}

// EnsureMigrationsTable ensures the migrations table exists
func (mm *MigrationManager) EnsureMigrationsTable(ctx context.Context) error {
	return mm.createMigrationsTable(ctx)
}

// GetMigrationPath returns the migration directory path
func (mm *MigrationManager) GetMigrationPath() string {
	return mm.config.Path
}

// SetMigrationPath sets the migration directory path
func (mm *MigrationManager) SetMigrationPath(path string) {
	mm.config.Path = path
}

// ValidateAll validates all registered migrations
func (mm *MigrationManager) ValidateAll() []error {
	var errors []error

	migrations := mm.registry.List()
	for _, migration := range migrations {
		if err := mm.validator.Validate(migration); err != nil {
			errors = append(errors, fmt.Errorf("migration %s: %w", migration.Version, err))
		}
	}

	return errors
}

// CreateBackup creates a backup before migration
func (mm *MigrationManager) CreateBackup(ctx context.Context) (string, error) {
	if !mm.config.BackupBeforeMigrate {
		return "", nil
	}

	// This would integrate with the backup system from disaster recovery
	// For now, we return a placeholder
	backupID := fmt.Sprintf("migration-backup-%d", time.Now().Unix())

	mm.logger.Infow("Creating backup before migration", "backup_id", backupID)

	return backupID, nil
}

// RestoreBackup restores a backup after failed migration
func (mm *MigrationManager) RestoreBackup(ctx context.Context, backupID string) error {
	// This would integrate with the restore system from disaster recovery
	// For now, we just log

	mm.logger.Infow("Restoring backup after failed migration", "backup_id", backupID)

	return nil
}

// GetMigrationFile returns the file path for a migration
func (mm *MigrationManager) GetMigrationFile(version string) (string, error) {
	migration, exists := mm.registry.Get(version)
	if !exists {
		return "", fmt.Errorf("migration %s not found", version)
	}

	// Find the file
	pattern := filepath.Join(mm.config.Path, fmt.Sprintf("V%s_*.sql", version))
	files, err := filepath.Glob(pattern)
	if err != nil {
		return "", fmt.Errorf("failed to find migration file: %w", err)
	}

	if len(files) == 0 {
		return "", fmt.Errorf("migration file not found for version %s", version)
	}

	return files[0], nil
}

// ReadMigrationFile reads a migration file
func (mm *MigrationManager) ReadMigrationFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read migration file: %w", err)
	}
	return string(content), nil
}
