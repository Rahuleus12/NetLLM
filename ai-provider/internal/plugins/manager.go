package plugins

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// Manager manages plugin lifecycle and operations
type Manager struct {
	db           *sql.DB
	loader       *Loader
	sandbox      *Sandbox
	api          *PluginAPI
	marketplace  *Marketplace
	pluginsDir   string
	plugins      map[string]*Plugin
	instances    map[string]PluginInterface
	eventChan    chan PluginEvent
	mu           sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

// NewManager creates a new plugin manager
func NewManager(db *sql.DB, pluginsDir string) (*Manager, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection is required")
	}

	ctx, cancel := context.WithCancel(context.Background())

	manager := &Manager{
		db:         db,
		pluginsDir: pluginsDir,
		plugins:    make(map[string]*Plugin),
		instances:  make(map[string]PluginInterface),
		eventChan:  make(chan PluginEvent, 1000),
		ctx:        ctx,
		cancel:     cancel,
	}

	// Initialize loader
	loader, err := NewLoader(pluginsDir)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create loader: %w", err)
	}
	manager.loader = loader

	// Initialize sandbox
	sandbox, err := NewSandbox(DefaultSandboxConfig())
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create sandbox: %w", err)
	}
	manager.sandbox = sandbox

	// Initialize API
	manager.api = NewPluginAPI(manager)

	// Initialize marketplace
	manager.marketplace = NewMarketplace()

	// Create plugins directory if it doesn't exist
	if err := os.MkdirAll(pluginsDir, 0755); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create plugins directory: %w", err)
	}

	// Initialize database schema
	if err := manager.initSchema(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	// Load existing plugins from database
	if err := manager.loadPluginsFromDB(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to load plugins: %w", err)
	}

	// Start event processor
	manager.wg.Add(1)
	go manager.processEvents()

	return manager, nil
}

// initSchema initializes the database schema
func (m *Manager) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS plugins (
		id VARCHAR(255) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		version VARCHAR(50) NOT NULL,
		description TEXT,
		type VARCHAR(50) NOT NULL,
		status VARCHAR(50) NOT NULL,
		enabled BOOLEAN DEFAULT false,
		manifest JSONB,
		config JSONB,
		permissions TEXT[],
		dependencies TEXT[],
		path VARCHAR(500),
		checksum VARCHAR(64),
		installed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		last_started_at TIMESTAMP,
		last_stopped_at TIMESTAMP,
		error TEXT,
		metadata JSONB
	);

	CREATE INDEX IF NOT EXISTS idx_plugins_status ON plugins(status);
	CREATE INDEX IF NOT EXISTS idx_plugins_type ON plugins(type);
	CREATE INDEX IF NOT EXISTS idx_plugins_enabled ON plugins(enabled);
	CREATE INDEX IF NOT EXISTS idx_plugins_name ON plugins(name);

	CREATE TABLE IF NOT EXISTS plugin_events (
		id VARCHAR(255) PRIMARY KEY,
		plugin_id VARCHAR(255) NOT NULL,
		type VARCHAR(50) NOT NULL,
		timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		data JSONB,
		error TEXT,
		FOREIGN KEY (plugin_id) REFERENCES plugins(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_plugin_events_plugin_id ON plugin_events(plugin_id);
	CREATE INDEX IF NOT EXISTS idx_plugin_events_timestamp ON plugin_events(timestamp);

	CREATE TABLE IF NOT EXISTS plugin_logs (
		id VARCHAR(255) PRIMARY KEY,
		plugin_id VARCHAR(255) NOT NULL,
		level VARCHAR(20) NOT NULL,
		message TEXT NOT NULL,
		timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		metadata JSONB,
		FOREIGN KEY (plugin_id) REFERENCES plugins(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_plugin_logs_plugin_id ON plugin_logs(plugin_id);
	CREATE INDEX IF NOT EXISTS idx_plugin_logs_timestamp ON plugin_logs(timestamp);
	`

	_, err := m.db.Exec(schema)
	return err
}

// loadPluginsFromDB loads plugins from database
func (m *Manager) loadPluginsFromDB() error {
	query := `
		SELECT id, name, version, description, type, status, enabled,
			   manifest, config, permissions, dependencies, path, checksum,
			   installed_at, updated_at, last_started_at, last_stopped_at, error, metadata
		FROM plugins
	`

	rows, err := m.db.Query(query)
	if err != nil {
		return fmt.Errorf("failed to query plugins: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		plugin := &Plugin{
			Manifest: &PluginManifest{},
			Config:   make(map[string]interface{}),
			Metadata: make(map[string]interface{}),
		}

		var manifestJSON, configJSON, metadataJSON []byte
		var permissions, dependencies []string

		err := rows.Scan(
			&plugin.ID,
			&plugin.Name,
			&plugin.Version,
			&plugin.Description,
			&plugin.Type,
			&plugin.Status,
			&plugin.Enabled,
			&manifestJSON,
			&configJSON,
			pq.Array(&permissions),
			pq.Array(&dependencies),
			&plugin.Path,
			&plugin.Checksum,
			&plugin.InstalledAt,
			&plugin.UpdatedAt,
			&plugin.LastStartedAt,
			&plugin.LastStoppedAt,
			&plugin.Error,
			&metadataJSON,
		)
		if err != nil {
			return fmt.Errorf("failed to scan plugin: %w", err)
		}

		plugin.Permissions = permissions
		plugin.Dependencies = dependencies

		if len(manifestJSON) > 0 {
			if err := json.Unmarshal(manifestJSON, plugin.Manifest); err != nil {
				return fmt.Errorf("failed to unmarshal manifest: %w", err)
			}
		}

		if len(configJSON) > 0 {
			if err := json.Unmarshal(configJSON, &plugin.Config); err != nil {
				return fmt.Errorf("failed to unmarshal config: %w", err)
			}
		}

		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &plugin.Metadata); err != nil {
				return fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		m.plugins[plugin.ID] = plugin
	}

	return nil
}

// Install installs a new plugin
func (m *Manager) Install(ctx context.Context, opts PluginInstallOptions) (*PluginInstallResult, error) {
	startTime := time.Now()

	// Validate options
	if opts.Source == "" {
		return nil, fmt.Errorf("plugin source is required")
	}

	// Load plugin from source
	plugin, err := m.loader.LoadFromSource(ctx, opts.Source)
	if err != nil {
		return &PluginInstallResult{
			Success:  false,
			Error:    fmt.Sprintf("Failed to load plugin: %v", err),
			Duration: time.Since(startTime),
		}, nil
	}

	// Check if plugin already exists
	m.mu.RLock()
	existing, exists := m.plugins[plugin.ID]
	m.mu.RUnlock()

	if exists && !opts.Force {
		return &PluginInstallResult{
			PluginID: plugin.ID,
			Success:  false,
			Error:    "Plugin already installed. Use force option to reinstall",
			Duration: time.Since(startTime),
		}, nil
	}

	// Validate plugin
	if err := m.validatePlugin(plugin); err != nil {
		return &PluginInstallResult{
			PluginID: plugin.ID,
			Success:  false,
			Error:    fmt.Sprintf("Plugin validation failed: %v", err),
			Duration: time.Since(startTime),
		}, nil
	}

	// Check dependencies
	if warnings := m.checkDependencies(plugin); len(warnings) > 0 {
		m.log(plugin.ID, "WARN", fmt.Sprintf("Dependency warnings: %v", warnings))
	}

	// Execute pre-install hooks
	if err := m.executeHook(ctx, plugin, HookPreInstall); err != nil {
		return &PluginInstallResult{
			PluginID: plugin.ID,
			Success:  false,
			Error:    fmt.Sprintf("Pre-install hook failed: %v", err),
			Duration: time.Since(startTime),
		}, nil
	}

	// Set plugin configuration
	if opts.Config != nil {
		plugin.Config = opts.Config
	} else {
		plugin.Config = plugin.Manifest.Config.Default
	}

	// Set plugin status
	plugin.Status = StatusInstalled
	plugin.InstalledAt = time.Now()
	plugin.UpdatedAt = time.Now()

	// Calculate checksum
	checksum, err := m.calculateChecksum(plugin.Path)
	if err != nil {
		return &PluginInstallResult{
			PluginID: plugin.ID,
			Success:  false,
			Error:    fmt.Sprintf("Failed to calculate checksum: %v", err),
			Duration: time.Since(startTime),
		}, nil
	}
	plugin.Checksum = checksum

	// Save to database
	if err := m.savePlugin(plugin); err != nil {
		return &PluginInstallResult{
			PluginID: plugin.ID,
			Success:  false,
			Error:    fmt.Sprintf("Failed to save plugin: %v", err),
			Duration: time.Since(startTime),
		}, nil
	}

	// Update in-memory cache
	m.mu.Lock()
	m.plugins[plugin.ID] = plugin
	m.mu.Unlock()

	// Execute post-install hooks
	if err := m.executeHook(ctx, plugin, HookPostInstall); err != nil {
		m.log(plugin.ID, "ERROR", fmt.Sprintf("Post-install hook failed: %v", err))
	}

	// Emit event
	m.emitEvent(PluginEvent{
		ID:        uuid.New().String(),
		PluginID:  plugin.ID,
		Type:      EventInstalled,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"version": plugin.Version,
			"source":  opts.Source,
		},
	})

	// Enable plugin if requested
	if opts.EnableAfter {
		if _, err := m.Enable(ctx, plugin.ID); err != nil {
			m.log(plugin.ID, "WARN", fmt.Sprintf("Failed to auto-enable plugin: %v", err))
		}
	}

	return &PluginInstallResult{
		PluginID: plugin.ID,
		Success:  true,
		Duration: time.Since(startTime),
	}, nil
}

// Uninstall uninstalls a plugin
func (m *Manager) Uninstall(ctx context.Context, pluginID string) error {
	m.mu.RLock()
	plugin, exists := m.plugins[pluginID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("plugin not found: %s", pluginID)
	}

	// Stop plugin if running
	if plugin.Status == StatusRunning {
		if err := m.Stop(ctx, pluginID); err != nil {
			return fmt.Errorf("failed to stop plugin: %w", err)
		}
	}

	// Disable plugin if enabled
	if plugin.Enabled {
		if _, err := m.Disable(ctx, pluginID); err != nil {
			return fmt.Errorf("failed to disable plugin: %w", err)
		}
	}

	// Execute pre-uninstall hooks
	if err := m.executeHook(ctx, plugin, HookPreUninstall); err != nil {
		return fmt.Errorf("pre-uninstall hook failed: %w", err)
	}

	// Remove plugin files
	if err := os.RemoveAll(plugin.Path); err != nil {
		m.log(pluginID, "WARN", fmt.Sprintf("Failed to remove plugin files: %v", err))
	}

	// Delete from database
	if err := m.deletePlugin(pluginID); err != nil {
		return fmt.Errorf("failed to delete plugin from database: %w", err)
	}

	// Remove from memory
	m.mu.Lock()
	delete(m.plugins, pluginID)
	delete(m.instances, pluginID)
	m.mu.Unlock()

	// Execute post-uninstall hooks
	if err := m.executeHook(ctx, plugin, HookPostUninstall); err != nil {
		m.log(pluginID, "WARN", fmt.Sprintf("Post-uninstall hook failed: %v", err))
	}

	// Emit event
	m.emitEvent(PluginEvent{
		ID:        uuid.New().String(),
		PluginID:  pluginID,
		Type:      EventUninstalled,
		Timestamp: time.Now(),
	})

	return nil
}

// Enable enables a plugin
func (m *Manager) Enable(ctx context.Context, pluginID string) (*Plugin, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	plugin, exists := m.plugins[pluginID]
	if !exists {
		return nil, fmt.Errorf("plugin not found: %s", pluginID)
	}

	if plugin.Enabled {
		return plugin, nil
	}

	// Execute pre-enable hooks
	if err := m.executeHook(ctx, plugin, HookPreEnable); err != nil {
		return nil, fmt.Errorf("pre-enable hook failed: %w", err)
	}

	// Load plugin instance
	instance, err := m.loader.LoadPlugin(ctx, plugin)
	if err != nil {
		return nil, fmt.Errorf("failed to load plugin instance: %w", err)
	}

	// Initialize plugin
	if err := instance.Initialize(ctx, plugin.Config); err != nil {
		return nil, fmt.Errorf("failed to initialize plugin: %w", err)
	}

	// Update plugin state
	plugin.Enabled = true
	plugin.Status = StatusEnabled
	plugin.UpdatedAt = time.Now()
	plugin.Error = ""

	// Save to database
	if err := m.updatePlugin(plugin); err != nil {
		return nil, fmt.Errorf("failed to update plugin: %w", err)
	}

	// Store instance
	m.instances[pluginID] = instance

	// Execute post-enable hooks
	if err := m.executeHook(ctx, plugin, HookPostEnable); err != nil {
		m.log(pluginID, "WARN", fmt.Sprintf("Post-enable hook failed: %v", err))
	}

	// Emit event
	m.emitEvent(PluginEvent{
		ID:        uuid.New().String(),
		PluginID:  pluginID,
		Type:      EventEnabled,
		Timestamp: time.Now(),
	})

	return plugin, nil
}

// Disable disables a plugin
func (m *Manager) Disable(ctx context.Context, pluginID string) (*Plugin, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	plugin, exists := m.plugins[pluginID]
	if !exists {
		return nil, fmt.Errorf("plugin not found: %s", pluginID)
	}

	if !plugin.Enabled {
		return plugin, nil
	}

	// Stop plugin if running
	if plugin.Status == StatusRunning {
		if err := m.stopPlugin(ctx, plugin); err != nil {
			return nil, fmt.Errorf("failed to stop plugin: %w", err)
		}
	}

	// Execute pre-disable hooks
	if err := m.executeHook(ctx, plugin, HookPreDisable); err != nil {
		return nil, fmt.Errorf("pre-disable hook failed: %w", err)
	}

	// Cleanup plugin instance
	if instance, exists := m.instances[pluginID]; exists {
		if err := instance.Cleanup(ctx); err != nil {
			m.log(pluginID, "WARN", fmt.Sprintf("Plugin cleanup failed: %v", err))
		}
		delete(m.instances, pluginID)
	}

	// Update plugin state
	plugin.Enabled = false
	plugin.Status = StatusDisabled
	plugin.UpdatedAt = time.Now()

	// Save to database
	if err := m.updatePlugin(plugin); err != nil {
		return nil, fmt.Errorf("failed to update plugin: %w", err)
	}

	// Execute post-disable hooks
	if err := m.executeHook(ctx, plugin, HookPostDisable); err != nil {
		m.log(pluginID, "WARN", fmt.Sprintf("Post-disable hook failed: %v", err))
	}

	// Emit event
	m.emitEvent(PluginEvent{
		ID:        uuid.New().String(),
		PluginID:  pluginID,
		Type:      EventDisabled,
		Timestamp: time.Now(),
	})

	return plugin, nil
}

// Start starts a plugin
func (m *Manager) Start(ctx context.Context, pluginID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	plugin, exists := m.plugins[pluginID]
	if !exists {
		return fmt.Errorf("plugin not found: %s", pluginID)
	}

	if !plugin.Enabled {
		return fmt.Errorf("plugin must be enabled before starting")
	}

	if plugin.Status == StatusRunning {
		return nil
	}

	// Execute pre-start hooks
	if err := m.executeHook(ctx, plugin, HookPreStart); err != nil {
		return fmt.Errorf("pre-start hook failed: %w", err)
	}

	// Get plugin instance
	instance, exists := m.instances[pluginID]
	if !exists {
		return fmt.Errorf("plugin instance not found")
	}

	// Start plugin
	if err := instance.Start(ctx); err != nil {
		plugin.Status = StatusError
		plugin.Error = err.Error()
		m.updatePlugin(plugin)
		return fmt.Errorf("failed to start plugin: %w", err)
	}

	// Update plugin state
	now := time.Now()
	plugin.Status = StatusRunning
	plugin.LastStartedAt = &now
	plugin.UpdatedAt = time.Now()
	plugin.Error = ""

	// Save to database
	if err := m.updatePlugin(plugin); err != nil {
		return fmt.Errorf("failed to update plugin: %w", err)
	}

	// Execute post-start hooks
	if err := m.executeHook(ctx, plugin, HookPostStart); err != nil {
		m.log(pluginID, "WARN", fmt.Sprintf("Post-start hook failed: %v", err))
	}

	// Emit event
	m.emitEvent(PluginEvent{
		ID:        uuid.New().String(),
		PluginID:  pluginID,
		Type:      EventStarted,
		Timestamp: time.Now(),
	})

	return nil
}

// Stop stops a plugin
func (m *Manager) Stop(ctx context.Context, pluginID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	plugin, exists := m.plugins[pluginID]
	if !exists {
		return fmt.Errorf("plugin not found: %s", pluginID)
	}

	return m.stopPlugin(ctx, plugin)
}

// stopPlugin internal method to stop a plugin (must be called with lock held)
func (m *Manager) stopPlugin(ctx context.Context, plugin *Plugin) error {
	if plugin.Status != StatusRunning {
		return nil
	}

	// Execute pre-stop hooks
	if err := m.executeHook(ctx, plugin, HookPreStop); err != nil {
		return fmt.Errorf("pre-stop hook failed: %w", err)
	}

	// Get plugin instance
	instance, exists := m.instances[plugin.ID]
	if !exists {
		return fmt.Errorf("plugin instance not found")
	}

	// Stop plugin
	if err := instance.Stop(ctx); err != nil {
		plugin.Status = StatusError
		plugin.Error = err.Error()
		m.updatePlugin(plugin)
		return fmt.Errorf("failed to stop plugin: %w", err)
	}

	// Update plugin state
	now := time.Now()
	plugin.Status = StatusStopped
	plugin.LastStoppedAt = &now
	plugin.UpdatedAt = time.Now()

	// Save to database
	if err := m.updatePlugin(plugin); err != nil {
		return fmt.Errorf("failed to update plugin: %w", err)
	}

	// Execute post-stop hooks
	if err := m.executeHook(ctx, plugin, HookPostStop); err != nil {
		m.log(plugin.ID, "WARN", fmt.Sprintf("Post-stop hook failed: %v", err))
	}

	// Emit event
	m.emitEvent(PluginEvent{
		ID:        uuid.New().String(),
		PluginID:  plugin.ID,
		Type:      EventStopped,
		Timestamp: time.Now(),
	})

	return nil
}

// Update updates a plugin
func (m *Manager) Update(ctx context.Context, pluginID string, opts PluginUpdateOptions) (*Plugin, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	plugin, exists := m.plugins[pluginID]
	if !exists {
		return nil, fmt.Errorf("plugin not found: %s", pluginID)
	}

	// Execute pre-update hooks
	if err := m.executeHook(ctx, plugin, HookPreUpdate); err != nil {
		return nil, fmt.Errorf("pre-update hook failed: %w", err)
	}

	// Update configuration if provided
	if opts.Config != nil {
		plugin.Config = opts.Config
	}

	plugin.UpdatedAt = time.Now()

	// Save to database
	if err := m.updatePlugin(plugin); err != nil {
		return nil, fmt.Errorf("failed to update plugin: %w", err)
	}

	// Execute post-update hooks
	if err := m.executeHook(ctx, plugin, HookPostUpdate); err != nil {
		m.log(pluginID, "WARN", fmt.Sprintf("Post-update hook failed: %v", err))
	}

	// Emit event
	m.emitEvent(PluginEvent{
		ID:        uuid.New().String(),
		PluginID:  pluginID,
		Type:      EventUpdated,
		Timestamp: time.Now(),
		Data:      map[string]interface{}{"version": opts.Version},
	})

	return plugin, nil
}

// Configure configures a plugin
func (m *Manager) Configure(ctx context.Context, pluginID string, config map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	plugin, exists := m.plugins[pluginID]
	if !exists {
		return fmt.Errorf("plugin not found: %s", pluginID)
	}

	// Update configuration
	for k, v := range config {
		plugin.Config[k] = v
	}

	plugin.UpdatedAt = time.Now()

	// Save to database
	if err := m.updatePlugin(plugin); err != nil {
		return fmt.Errorf("failed to update plugin: %w", err)
	}

	// Configure running instance
	if instance, exists := m.instances[pluginID]; exists {
		if err := instance.Configure(ctx, config); err != nil {
			return fmt.Errorf("failed to configure plugin instance: %w", err)
		}
	}

	// Emit event
	m.emitEvent(PluginEvent{
		ID:        uuid.New().String(),
		PluginID:  pluginID,
		Type:      EventConfigured,
		Timestamp: time.Now(),
	})

	return nil
}

// Get retrieves a plugin by ID
func (m *Manager) Get(pluginID string) (*Plugin, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	plugin, exists := m.plugins[pluginID]
	if !exists {
		return nil, fmt.Errorf("plugin not found: %s", pluginID)
	}

	return plugin, nil
}

// List lists plugins with optional filtering
func (m *Manager) List(filter PluginFilter) (*PluginList, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var plugins []Plugin

	for _, plugin := range m.plugins {
		// Apply filters
		if len(filter.Status) > 0 {
			found := false
			for _, status := range filter.Status {
				if plugin.Status == status {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		if len(filter.Type) > 0 {
			found := false
			for _, t := range filter.Type {
				if plugin.Type == t {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		if filter.Enabled != nil && plugin.Enabled != *filter.Enabled {
			continue
		}

		if filter.Search != "" {
			// Simple search in name and description
			// In production, use proper search
		}

		plugins = append(plugins, *plugin)
	}

	// Pagination
	total := len(plugins)
	pageSize := filter.PageSize
	if pageSize == 0 {
		pageSize = 20
	}

	page := filter.Page
	if page == 0 {
		page = 1
	}

	start := (page - 1) * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}

	totalPages := total / pageSize
	if total%pageSize > 0 {
		totalPages++
	}

	return &PluginList{
		Plugins:    plugins[start:end],
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

// HealthCheck performs health check on a plugin
func (m *Manager) HealthCheck(ctx context.Context, pluginID string) error {
	m.mu.RLock()
	instance, exists := m.instances[pluginID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("plugin instance not found")
	}

	return instance.HealthCheck(ctx)
}

// GetAPI returns the plugin API
func (m *Manager) GetAPI() *PluginAPI {
	return m.api
}

// GetMarketplace returns the marketplace client
func (m *Manager) GetMarketplace() *Marketplace {
	return m.marketplace
}

// validatePlugin validates a plugin
func (m *Manager) validatePlugin(plugin *Plugin) error {
	if plugin.ID == "" {
		return fmt.Errorf("plugin ID is required")
	}

	if plugin.Name == "" {
		return fmt.Errorf("plugin name is required")
	}

	if plugin.Version == "" {
		return fmt.Errorf("plugin version is required")
	}

	if plugin.Manifest == nil {
		return fmt.Errorf("plugin manifest is required")
	}

	// Validate API version compatibility
	if plugin.Manifest.APIVersion != "1.0" {
		return fmt.Errorf("unsupported API version: %s", plugin.Manifest.APIVersion)
	}

	return nil
}

// checkDependencies checks plugin dependencies
func (m *Manager) checkDependencies(plugin *Plugin) []string {
	var warnings []string

	for _, dep := range plugin.Dependencies {
		if _, exists := m.plugins[dep]; !exists {
			warnings = append(warnings, fmt.Sprintf("missing dependency: %s", dep))
		}
	}

	return warnings
}

// executeHook executes a plugin hook
func (m *Manager) executeHook(ctx context.Context, plugin *Plugin, hookType HookType) error {
	if plugin.Manifest == nil {
		return nil
	}

	for _, hook := range plugin.Manifest.Hooks {
		if hook.Type == hookType {
			// Execute hook (implementation depends on plugin system)
			m.log(plugin.ID, "INFO", fmt.Sprintf("Executing hook: %s", hook.Name))
		}
	}

	return nil
}

// savePlugin saves a plugin to database
func (m *Manager) savePlugin(plugin *Plugin) error {
	manifestJSON, _ := json.Marshal(plugin.Manifest)
	configJSON, _ := json.Marshal(plugin.Config)
	metadataJSON, _ := json.Marshal(plugin.Metadata)

	query := `
		INSERT INTO plugins (
			id, name, version, description, type, status, enabled,
			manifest, config, permissions, dependencies, path, checksum,
			installed_at, updated_at, last_started_at, last_stopped_at, error, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
	`

	_, err := m.db.Exec(query,
		plugin.ID,
		plugin.Name,
		plugin.Version,
		plugin.Description,
		plugin.Type,
		plugin.Status,
		plugin.Enabled,
		manifestJSON,
		configJSON,
		pq.Array(plugin.Permissions),
		pq.Array(plugin.Dependencies),
		plugin.Path,
		plugin.Checksum,
		plugin.InstalledAt,
		plugin.UpdatedAt,
		plugin.LastStartedAt,
		plugin.LastStoppedAt,
		plugin.Error,
		metadataJSON,
	)

	return err
}

// updatePlugin updates a plugin in database
func (m *Manager) updatePlugin(plugin *Plugin) error {
	manifestJSON, _ := json.Marshal(plugin.Manifest)
	configJSON, _ := json.Marshal(plugin.Config)
	metadataJSON, _ := json.Marshal(plugin.Metadata)

	query := `
		UPDATE plugins SET
			name = $2, version = $3, description = $4, type = $5, status = $6,
			enabled = $7, manifest = $8, config = $9, permissions = $10,
			dependencies = $11, path = $12, checksum = $13, updated_at = $14,
			last_started_at = $15, last_stopped_at = $16, error = $17, metadata = $18
		WHERE id = $1
	`

	_, err := m.db.Exec(query,
		plugin.ID,
		plugin.Name,
		plugin.Version,
		plugin.Description,
		plugin.Type,
		plugin.Status,
		plugin.Enabled,
		manifestJSON,
		configJSON,
		pq.Array(plugin.Permissions),
		pq.Array(plugin.Dependencies),
		plugin.Path,
		plugin.Checksum,
		plugin.UpdatedAt,
		plugin.LastStartedAt,
		plugin.LastStoppedAt,
		plugin.Error,
		metadataJSON,
	)

	return err
}

// deletePlugin deletes a plugin from database
func (m *Manager) deletePlugin(pluginID string) error {
	_, err := m.db.Exec("DELETE FROM plugins WHERE id = $1", pluginID)
	return err
}

// calculateChecksum calculates SHA256 checksum of plugin files
func (m *Manager) calculateChecksum(path string) (string, error) {
	hash := sha256.New()

	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			data, err := ioutil.ReadFile(filePath)
			if err != nil {
				return err
			}
			hash.Write(data)
		}

		return nil
	})

	if err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// log logs a plugin message
func (m *Manager) log(pluginID, level, message string) {
	logEntry := PluginLog{
		ID:        uuid.New().String(),
		PluginID:  pluginID,
		Level:     level,
		Message:   message,
		Timestamp: time.Now(),
		Metadata:  make(map[string]interface{}),
	}

	// Save to database
	metadataJSON, _ := json.Marshal(logEntry.Metadata)
	_, _ = m.db.Exec(`
		INSERT INTO plugin_logs (id, plugin_id, level, message, timestamp, metadata)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, logEntry.ID, logEntry.PluginID, logEntry.Level, logEntry.Message, logEntry.Timestamp, metadataJSON)
}

// emitEvent emits a plugin event
func (m *Manager) emitEvent(event PluginEvent) {
	select {
	case m.eventChan <- event:
	default:
		// Event channel full, log warning
	}
}

// processEvents processes plugin events
func (m *Manager) processEvents() {
	defer m.wg.Done()

	for {
		select {
		case event := <-m.eventChan:
			// Save event to database
			dataJSON, _ := json.Marshal(event.Data)
			_, _ = m.db.Exec(`
				INSERT INTO plugin_events (id, plugin_id, type, timestamp, data, error)
				VALUES ($1, $2, $3, $4, $5, $6)
			`, event.ID, event.PluginID, event.Type, event.Timestamp, dataJSON, event.Error)

		case <-m.ctx.Done():
			return
		}
	}
}

// Close closes the plugin manager
func (m *Manager) Close() error {
	m.cancel()
	m.wg.Wait()

	// Stop all running plugins
	m.mu.Lock()
	for pluginID, instance := range m.instances {
		if err := instance.Stop(context.Background()); err != nil {
			m.log(pluginID, "ERROR", fmt.Sprintf("Failed to stop plugin: %v", err))
		}
		if err := instance.Cleanup(context.Background()); err != nil {
			m.log(pluginID, "ERROR", fmt.Sprintf("Failed to cleanup plugin: %v", err))
		}
	}
	m.mu.Unlock()

	close(m.eventChan)

	return nil
}
