package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq"
)

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Type           string
	Host           string
	Port           int
	Name           string
	User           string
	Password       string
	SSLMode        string
	MaxConnections int
}

// Database represents the database connection and operations
type Database struct {
	config *DatabaseConfig
	db     *sql.DB
}

// NewDatabase creates a new database instance
func NewDatabase(cfg *DatabaseConfig) *Database {
	return &Database{
		config: cfg,
	}
}

// Connect establishes a connection to the database
func (d *Database) Connect(ctx context.Context) error {
	connectionString := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.config.Host,
		d.config.Port,
		d.config.User,
		d.config.Password,
		d.config.Name,
		d.config.SSLMode,
	)

	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		return fmt.Errorf("failed to open database connection: %w", err)
	}

	// Test the connection
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(d.config.MaxConnections)
	db.SetMaxIdleConns(d.config.MaxConnections / 2)
	db.SetConnMaxLifetime(time.Hour)
	db.SetConnMaxIdleTime(30 * time.Minute)

	d.db = db
	log.Printf("Connected to database: %s@%s:%d/%s", d.config.User, d.config.Host, d.config.Port, d.config.Name)

	return nil
}

// Close closes the database connection
func (d *Database) Close() error {
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

// InitializeSchema creates the database schema if it doesn't exist
func (d *Database) InitializeSchema(ctx context.Context) error {
	schema := `
	-- Enable UUID extension
	CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

	-- Models table
	CREATE TABLE IF NOT EXISTS models (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		name VARCHAR(255) NOT NULL,
		version VARCHAR(100) NOT NULL,
		format VARCHAR(50) NOT NULL,
		source TEXT NOT NULL,
		checksum VARCHAR(128),
		status VARCHAR(50) DEFAULT 'inactive',
		context_length INTEGER DEFAULT 2048,
		temperature REAL DEFAULT 1.0,
		max_tokens INTEGER DEFAULT 512,
		ram_min INTEGER DEFAULT 4096,
		gpu_memory INTEGER DEFAULT 0,
		cpu_cores INTEGER DEFAULT 2,
		instances INTEGER DEFAULT 0,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(name, version)
	);

	-- Model instances table
	CREATE TABLE IF NOT EXISTS model_instances (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		model_id UUID NOT NULL REFERENCES models(id) ON DELETE CASCADE,
		container_id VARCHAR(255),
		status VARCHAR(50) DEFAULT 'stopped',
		port INTEGER,
		gpu_device INTEGER,
		memory_used BIGINT DEFAULT 0,
		cpu_usage REAL DEFAULT 0.0,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		started_at TIMESTAMP,
		stopped_at TIMESTAMP
	);

	-- Configurations table
	CREATE TABLE IF NOT EXISTS configurations (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		key VARCHAR(255) UNIQUE NOT NULL,
		value TEXT NOT NULL,
		category VARCHAR(100),
		description TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	-- Inference logs table
	CREATE TABLE IF NOT EXISTS inference_logs (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		model_id UUID REFERENCES models(id) ON DELETE SET NULL,
		instance_id UUID REFERENCES model_instances(id) ON DELETE SET NULL,
		request_id VARCHAR(255),
		input_tokens INTEGER DEFAULT 0,
		output_tokens INTEGER DEFAULT 0,
		latency_ms INTEGER,
		status VARCHAR(50),
		error_message TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	-- System metrics table
	CREATE TABLE IF NOT EXISTS system_metrics (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		metric_type VARCHAR(100) NOT NULL,
		metric_name VARCHAR(255) NOT NULL,
		value REAL NOT NULL,
		unit VARCHAR(50),
		tags JSONB,
		timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	-- API keys table (for authentication)
	CREATE TABLE IF NOT EXISTS api_keys (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		key_hash VARCHAR(255) UNIQUE NOT NULL,
		name VARCHAR(255) NOT NULL,
		user_id VARCHAR(255),
		permissions JSONB,
		is_active BOOLEAN DEFAULT true,
		expires_at TIMESTAMP,
		last_used_at TIMESTAMP,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	-- Create indexes for better query performance
	CREATE INDEX IF NOT EXISTS idx_models_status ON models(status);
	CREATE INDEX IF NOT EXISTS idx_models_name_version ON models(name, version);
	CREATE INDEX IF NOT EXISTS idx_model_instances_model_id ON model_instances(model_id);
	CREATE INDEX IF NOT EXISTS idx_model_instances_status ON model_instances(status);
	CREATE INDEX IF NOT EXISTS idx_inference_logs_model_id ON inference_logs(model_id);
	CREATE INDEX IF NOT EXISTS idx_inference_logs_created_at ON inference_logs(created_at);
	CREATE INDEX IF NOT EXISTS idx_system_metrics_type_name ON system_metrics(metric_type, metric_name);
	CREATE INDEX IF NOT EXISTS idx_system_metrics_timestamp ON system_metrics(timestamp);
	CREATE INDEX IF NOT EXISTS idx_configurations_key ON configurations(key);
	CREATE INDEX IF NOT EXISTS idx_api_keys_key_hash ON api_keys(key_hash);

	-- Insert default configurations
	INSERT INTO configurations (key, value, category, description)
	VALUES
		('system.max_concurrent_models', '10', 'models', 'Maximum number of concurrent models'),
		('system.auto_scale', 'true', 'models', 'Enable auto-scaling of model instances'),
		('system.scale_threshold', '0.8', 'models', 'CPU/GPU utilization threshold for scaling'),
		('system.idle_timeout', '300', 'models', 'Idle timeout in seconds before stopping instances'),
		('api.rate_limit', '100', 'api', 'API rate limit per minute'),
		('monitoring.metrics_interval', '15', 'monitoring', 'Metrics collection interval in seconds')
	ON CONFLICT (key) DO NOTHING;
	`

	_, err := d.db.ExecContext(ctx, schema)
	if err != nil {
		return fmt.Errorf("failed to initialize schema: %w", err)
	}

	log.Println("Database schema initialized successfully")
	return nil
}

// GetDB returns the database connection
func (d *Database) GetDB() *sql.DB {
	return d.db
}

// HealthCheck checks if the database connection is healthy
func (d *Database) HealthCheck(ctx context.Context) error {
	if d.db == nil {
		return fmt.Errorf("database connection not initialized")
	}

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if err := d.db.PingContext(ctx); err != nil {
		return fmt.Errorf("database health check failed: %w", err)
	}

	return nil
}

// Model represents a model in the database
type Model struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Version       string    `json:"version"`
	Format        string    `json:"format"`
	Source        string    `json:"source"`
	Checksum      string    `json:"checksum"`
	Status        string    `json:"status"`
	ContextLength int       `json:"context_length"`
	Temperature   float64   `json:"temperature"`
	MaxTokens     int       `json:"max_tokens"`
	RAMMin        int       `json:"ram_min"`
	GPUMemory     int       `json:"gpu_memory"`
	CPUCores      int       `json:"cpu_cores"`
	Instances     int       `json:"instances"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// CreateModel creates a new model in the database
func (d *Database) CreateModel(ctx context.Context, model *Model) error {
	query := `
		INSERT INTO models (name, version, format, source, checksum, status, context_length,
							temperature, max_tokens, ram_min, gpu_memory, cpu_cores, instances)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, created_at, updated_at
	`

	err := d.db.QueryRowContext(
		ctx,
		query,
		model.Name,
		model.Version,
		model.Format,
		model.Source,
		model.Checksum,
		model.Status,
		model.ContextLength,
		model.Temperature,
		model.MaxTokens,
		model.RAMMin,
		model.GPUMemory,
		model.CPUCores,
		model.Instances,
	).Scan(&model.ID, &model.CreatedAt, &model.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create model: %w", err)
	}

	return nil
}

// GetModel retrieves a model by ID
func (d *Database) GetModel(ctx context.Context, id string) (*Model, error) {
	query := `
		SELECT id, name, version, format, source, checksum, status, context_length,
			   temperature, max_tokens, ram_min, gpu_memory, cpu_cores, instances,
			   created_at, updated_at
		FROM models
		WHERE id = $1
	`

	model := &Model{}
	err := d.db.QueryRowContext(ctx, query, id).Scan(
		&model.ID,
		&model.Name,
		&model.Version,
		&model.Format,
		&model.Source,
		&model.Checksum,
		&model.Status,
		&model.ContextLength,
		&model.Temperature,
		&model.MaxTokens,
		&model.RAMMin,
		&model.GPUMemory,
		&model.CPUCores,
		&model.Instances,
		&model.CreatedAt,
		&model.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("model not found")
		}
		return nil, fmt.Errorf("failed to get model: %w", err)
	}

	return model, nil
}

// ListModels retrieves all models
func (d *Database) ListModels(ctx context.Context, status string) ([]*Model, error) {
	var query string
	var args []interface{}

	if status != "" {
		query = `
			SELECT id, name, version, format, source, checksum, status, context_length,
				   temperature, max_tokens, ram_min, gpu_memory, cpu_cores, instances,
				   created_at, updated_at
			FROM models
			WHERE status = $1
			ORDER BY created_at DESC
		`
		args = []interface{}{status}
	} else {
		query = `
			SELECT id, name, version, format, source, checksum, status, context_length,
				   temperature, max_tokens, ram_min, gpu_memory, cpu_cores, instances,
				   created_at, updated_at
			FROM models
			ORDER BY created_at DESC
		`
		args = []interface{}{}
	}

	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}
	defer rows.Close()

	var models []*Model
	for rows.Next() {
		model := &Model{}
		err := rows.Scan(
			&model.ID,
			&model.Name,
			&model.Version,
			&model.Format,
			&model.Source,
			&model.Checksum,
			&model.Status,
			&model.ContextLength,
			&model.Temperature,
			&model.MaxTokens,
			&model.RAMMin,
			&model.GPUMemory,
			&model.CPUCores,
			&model.Instances,
			&model.CreatedAt,
			&model.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan model: %w", err)
		}
		models = append(models, model)
	}

	return models, nil
}

// UpdateModel updates a model
func (d *Database) UpdateModel(ctx context.Context, model *Model) error {
	query := `
		UPDATE models
		SET name = $2, version = $3, format = $4, source = $5, checksum = $6,
			status = $7, context_length = $8, temperature = $9, max_tokens = $10,
			ram_min = $11, gpu_memory = $12, cpu_cores = $13, instances = $14,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`

	result, err := d.db.ExecContext(
		ctx,
		query,
		model.ID,
		model.Name,
		model.Version,
		model.Format,
		model.Source,
		model.Checksum,
		model.Status,
		model.ContextLength,
		model.Temperature,
		model.MaxTokens,
		model.RAMMin,
		model.GPUMemory,
		model.CPUCores,
		model.Instances,
	)

	if err != nil {
		return fmt.Errorf("failed to update model: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("model not found")
	}

	return nil
}

// DeleteModel deletes a model by ID
func (d *Database) DeleteModel(ctx context.Context, id string) error {
	query := `DELETE FROM models WHERE id = $1`

	result, err := d.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete model: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("model not found")
	}

	return nil
}

// Configuration represents a system configuration
type Configuration struct {
	Key         string    `json:"key"`
	Value       string    `json:"value"`
	Category    string    `json:"category"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// GetConfiguration retrieves a configuration by key
func (d *Database) GetConfiguration(ctx context.Context, key string) (*Configuration, error) {
	query := `
		SELECT key, value, category, description, created_at, updated_at
		FROM configurations
		WHERE key = $1
	`

	config := &Configuration{}
	err := d.db.QueryRowContext(ctx, query, key).Scan(
		&config.Key,
		&config.Value,
		&config.Category,
		&config.Description,
		&config.CreatedAt,
		&config.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("configuration not found")
		}
		return nil, fmt.Errorf("failed to get configuration: %w", err)
	}

	return config, nil
}

// SetConfiguration sets or updates a configuration
func (d *Database) SetConfiguration(ctx context.Context, config *Configuration) error {
	query := `
		INSERT INTO configurations (key, value, category, description)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (key)
		DO UPDATE SET
			value = EXCLUDED.value,
			category = EXCLUDED.category,
			description = EXCLUDED.description,
			updated_at = CURRENT_TIMESTAMP
	`

	_, err := d.db.ExecContext(
		ctx,
		query,
		config.Key,
		config.Value,
		config.Category,
		config.Description,
	)

	if err != nil {
		return fmt.Errorf("failed to set configuration: %w", err)
	}

	return nil
}

// RecordMetric records a system metric
func (d *Database) RecordMetric(ctx context.Context, metricType, metricName string, value float64, unit string, tags map[string]interface{}) error {
	query := `
		INSERT INTO system_metrics (metric_type, metric_name, value, unit, tags)
		VALUES ($1, $2, $3, $4, $5)
	`

	// Convert tags to JSON
	var tagsJSON interface{}
	if tags != nil {
		tagsJSON = tags
	}

	_, err := d.db.ExecContext(ctx, query, metricType, metricName, value, unit, tagsJSON)
	if err != nil {
		return fmt.Errorf("failed to record metric: %w", err)
	}

	return nil
}
