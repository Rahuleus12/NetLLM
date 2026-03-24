package plugins

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// Loader handles loading plugins from various sources
type Loader struct {
	pluginsDir string
}

// NewLoader creates a new plugin loader
func NewLoader(pluginsDir string) (*Loader, error) {
	if pluginsDir == "" {
		return nil, fmt.Errorf("plugins directory is required")
	}

	return &Loader{
		pluginsDir: pluginsDir,
	}, nil
}

// LoadFromSource loads a plugin from a source (URL, local path, or archive)
func (l *Loader) LoadFromSource(ctx context.Context, source string) (*Plugin, error) {
	// Determine source type
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		return l.loadFromURL(ctx, source)
	} else if strings.HasSuffix(source, ".zip") || strings.HasSuffix(source, ".tar.gz") || strings.HasSuffix(source, ".tgz") {
		return l.loadFromArchive(source)
	} else {
		return l.loadFromDirectory(source)
	}
}

// loadFromURL loads a plugin from a URL
func (l *Loader) loadFromURL(ctx context.Context, url string) (*Plugin, error) {
	// Create temporary file for download
	tmpFile, err := ioutil.TempFile("", "plugin-*.zip")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Download plugin
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download plugin: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download plugin: HTTP %d", resp.StatusCode)
	}

	// Write to temp file
	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return nil, fmt.Errorf("failed to save plugin: %w", err)
	}

	// Extract and load
	return l.loadFromArchive(tmpFile.Name())
}

// loadFromArchive loads a plugin from an archive file
func (l *Loader) loadFromArchive(archivePath string) (*Plugin, error) {
	// Create temporary extraction directory
	extractDir := filepath.Join(l.pluginsDir, ".tmp", uuid.New().String())
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create extraction directory: %w", err)
	}

	// Extract based on archive type
	var err error
	if strings.HasSuffix(archivePath, ".zip") {
		err = l.extractZip(archivePath, extractDir)
	} else if strings.HasSuffix(archivePath, ".tar.gz") || strings.HasSuffix(archivePath, ".tgz") {
		err = l.extractTarGz(archivePath, extractDir)
	} else {
		err = fmt.Errorf("unsupported archive format")
	}

	if err != nil {
		os.RemoveAll(extractDir)
		return nil, fmt.Errorf("failed to extract archive: %w", err)
	}

	// Find plugin directory (might be in a subdirectory)
	pluginDir, err := l.findPluginDirectory(extractDir)
	if err != nil {
		os.RemoveAll(extractDir)
		return nil, err
	}

	// Load plugin from extracted directory
	plugin, err := l.loadFromDirectory(pluginDir)
	if err != nil {
		os.RemoveAll(extractDir)
		return nil, err
	}

	// Move to final location
	finalDir := filepath.Join(l.pluginsDir, plugin.ID)
	if err := os.Rename(pluginDir, finalDir); err != nil {
		// If rename fails (cross-device), try copy
		if err := l.copyDirectory(pluginDir, finalDir); err != nil {
			os.RemoveAll(extractDir)
			return nil, fmt.Errorf("failed to move plugin: %w", err)
		}
	}

	// Clean up temp directory
	os.RemoveAll(extractDir)

	// Update plugin path
	plugin.Path = finalDir

	return plugin, nil
}

// loadFromDirectory loads a plugin from a local directory
func (l *Loader) loadFromDirectory(dir string) (*Plugin, error) {
	// Read plugin manifest
	manifestPath := filepath.Join(dir, "plugin.json")
	manifestData, err := ioutil.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read plugin manifest: %w", err)
	}

	// Parse manifest
	var manifest PluginManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse plugin manifest: %w", err)
	}

	// Validate manifest
	if err := l.validateManifest(&manifest); err != nil {
		return nil, fmt.Errorf("invalid plugin manifest: %w", err)
	}

	// Create plugin instance
	plugin := &Plugin{
		ID:          l.generatePluginID(&manifest),
		Name:        manifest.Name,
		Version:     manifest.Version,
		Description: manifest.Description,
		Type:        manifest.Type,
		Status:      StatusInstalled,
		Enabled:     false,
		Manifest:    &manifest,
		Config:      manifest.Config.Default,
		Permissions: manifest.Permissions,
		Dependencies: l.extractDependencies(&manifest),
		Path:        dir,
		Metadata:    make(map[string]interface{}),
	}

	return plugin, nil
}

// LoadPlugin loads and initializes a plugin instance
func (l *Loader) LoadPlugin(ctx context.Context, plugin *Plugin) (PluginInterface, error) {
	if plugin.Manifest == nil {
		return nil, fmt.Errorf("plugin manifest is required")
	}

	// Load plugin binary/module based on type
	// This is a simplified implementation
	// In production, you would use plugin system or external processes

	// For now, create a basic plugin instance
	instance := &BasePlugin{
		id:      plugin.ID,
		name:    plugin.Name,
		version: plugin.Version,
		ptype:   plugin.Type,
		config:  plugin.Config,
	}

	return instance, nil
}

// extractZip extracts a zip archive
func (l *Loader) extractZip(archivePath, destDir string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		path := filepath.Join(destDir, file.Name)

		if file.FileInfo().IsDir() {
			os.MkdirAll(path, file.Mode())
			continue
		}

		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}

		destFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			return err
		}

		srcFile, err := file.Open()
		if err != nil {
			destFile.Close()
			return err
		}

		_, err = io.Copy(destFile, srcFile)
		srcFile.Close()
		destFile.Close()

		if err != nil {
			return err
		}
	}

	return nil
}

// extractTarGz extracts a tar.gz archive
func (l *Loader) extractTarGz(archivePath, destDir string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		path := filepath.Join(destDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return err
			}

			destFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}

			if _, err := io.Copy(destFile, tr); err != nil {
				destFile.Close()
				return err
			}
			destFile.Close()
		}
	}

	return nil
}

// findPluginDirectory finds the plugin directory containing plugin.json
func (l *Loader) findPluginDirectory(dir string) (string, error) {
	// Check if current directory has plugin.json
	if _, err := os.Stat(filepath.Join(dir, "plugin.json")); err == nil {
		return dir, nil
	}

	// Look in subdirectories
	entries, err := ioutil.ReadDir(dir)
	if err != nil {
		return "", err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			subdir := filepath.Join(dir, entry.Name())
			if _, err := os.Stat(filepath.Join(subdir, "plugin.json")); err == nil {
				return subdir, nil
			}
		}
	}

	return "", fmt.Errorf("plugin manifest not found in %s", dir)
}

// validateManifest validates a plugin manifest
func (l *Loader) validateManifest(manifest *PluginManifest) error {
	if manifest.APIVersion == "" {
		return fmt.Errorf("API version is required")
	}

	if manifest.Name == "" {
		return fmt.Errorf("plugin name is required")
	}

	if manifest.Version == "" {
		return fmt.Errorf("plugin version is required")
	}

	if manifest.Type == "" {
		return fmt.Errorf("plugin type is required")
	}

	// Validate type
	validTypes := map[PluginType]bool{
		PluginTypeModel:       true,
		PluginTypeInference:   true,
		PluginTypeStorage:     true,
		PluginTypeAuth:        true,
		PluginTypeMonitoring:  true,
		PluginTypeIntegration: true,
		PluginTypeCLI:         true,
		PluginTypeCustom:      true,
	}

	if !validTypes[manifest.Type] {
		return fmt.Errorf("invalid plugin type: %s", manifest.Type)
	}

	return nil
}

// generatePluginID generates a unique plugin ID
func (l *Loader) generatePluginID(manifest *PluginManifest) string {
	return fmt.Sprintf("%s@%s", manifest.Name, manifest.Version)
}

// extractDependencies extracts dependencies from manifest
func (l *Loader) extractDependencies(manifest *PluginManifest) []string {
	var deps []string
	for _, req := range manifest.Requires {
		deps = append(deps, req.Name)
	}
	return deps
}

// copyDirectory copies a directory recursively
func (l *Loader) copyDirectory(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		data, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}

		return ioutil.WriteFile(dstPath, data, info.Mode())
	})
}

// BasePlugin is a basic plugin implementation
type BasePlugin struct {
	id      string
	name    string
	version string
	ptype   PluginType
	config  map[string]interface{}
}

func (p *BasePlugin) Initialize(ctx context.Context, config map[string]interface{}) error {
	p.config = config
	return nil
}

func (p *BasePlugin) Start(ctx context.Context) error {
	return nil
}

func (p *BasePlugin) Stop(ctx context.Context) error {
	return nil
}

func (p *BasePlugin) Configure(ctx context.Context, config map[string]interface{}) error {
	for k, v := range config {
		p.config[k] = v
	}
	return nil
}

func (p *BasePlugin) GetInfo() *PluginInfo {
	return &PluginInfo{
		ID:      p.id,
		Name:    p.name,
		Version: p.version,
		Type:    p.ptype,
		Status:  StatusRunning,
	}
}

func (p *BasePlugin) HealthCheck(ctx context.Context) error {
	return nil
}

func (p *BasePlugin) Cleanup(ctx context.Context) error {
	return nil
}
