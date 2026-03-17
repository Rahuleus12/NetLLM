package models

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SemanticVersion represents a semantic version (major.minor.patch)
type SemanticVersion struct {
	Major      int    `json:"major"`
	Minor      int    `json:"minor"`
	Patch      int    `json:"patch"`
	PreRelease string `json:"prerelease,omitempty"`
	Build      string `json:"build,omitempty"`
	Raw        string `json:"raw"`
}

// VersionManager manages model versions
type VersionManager struct {
	db       *sql.DB
	registry ModelRegistry
	mu       sync.RWMutex
}

// NewVersionManager creates a new version manager
func NewVersionManager(db *sql.DB, registry ModelRegistry) *VersionManager {
	return &VersionManager{
		db:       db,
		registry: registry,
	}
}

// ParseVersion parses a version string into a SemanticVersion
func ParseVersion(version string) (*SemanticVersion, error) {
	if version == "" {
		return nil, errors.New("version string is empty")
	}

	// Remove leading 'v' if present
	version = strings.TrimPrefix(version, "v")

	// Regex for semantic versioning
	re := regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)(?:-([a-zA-Z0-9-]+))?(?:\+([a-zA-Z0-9-]+))?$`)
	matches := re.FindStringSubmatch(version)

	if matches == nil {
		return nil, fmt.Errorf("invalid semantic version format: %s", version)
	}

	major, _ := strconv.Atoi(matches[1])
	minor, _ := strconv.Atoi(matches[2])
	patch, _ := strconv.Atoi(matches[3])

	return &SemanticVersion{
		Major:      major,
		Minor:      minor,
		Patch:      patch,
		PreRelease: matches[4],
		Build:      matches[5],
		Raw:        version,
	}, nil
}

// Compare compares two semantic versions
func (v *SemanticVersion) Compare(other *SemanticVersion) int {
	if v.Major != other.Major {
		if v.Major > other.Major {
			return 1
		}
		return -1
	}

	if v.Minor != other.Minor {
		if v.Minor > other.Minor {
			return 1
		}
		return -1
	}

	if v.Patch != other.Patch {
		if v.Patch > other.Patch {
			return 1
		}
		return -1
	}

	// Compare pre-release versions
	if v.PreRelease != other.PreRelease {
		// No pre-release is greater than having one
		if v.PreRelease == "" {
			return 1
		}
		if other.PreRelease == "" {
			return -1
		}

		// Compare pre-release strings lexicographically
		if v.PreRelease > other.PreRelease {
			return 1
		}
		return -1
	}

	return 0
}

// String returns the string representation of the version
func (v *SemanticVersion) String() string {
	version := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.PreRelease != "" {
		version += "-" + v.PreRelease
	}
	if v.Build != "" {
		version += "+" + v.Build
	}
	return version
}

// IsGreaterThan checks if this version is greater than another
func (v *SemanticVersion) IsGreaterThan(other *SemanticVersion) bool {
	return v.Compare(other) > 0
}

// IsLessThan checks if this version is less than another
func (v *SemanticVersion) IsLessThan(other *SemanticVersion) bool {
	return v.Compare(other) < 0
}

// IsEqual checks if this version equals another
func (v *SemanticVersion) IsEqual(other *SemanticVersion) bool {
	return v.Compare(other) == 0
}

// IncrementMajor increments the major version
func (v *SemanticVersion) IncrementMajor() *SemanticVersion {
	return &SemanticVersion{
		Major: v.Major + 1,
		Minor: 0,
		Patch: 0,
	}
}

// IncrementMinor increments the minor version
func (v *SemanticVersion) IncrementMinor() *SemanticVersion {
	return &SemanticVersion{
		Major: v.Major,
		Minor: v.Minor + 1,
		Patch: 0,
	}
}

// IncrementPatch increments the patch version
func (v *SemanticVersion) IncrementPatch() *SemanticVersion {
	return &SemanticVersion{
		Major: v.Major,
		Minor: v.Minor,
		Patch: v.Patch + 1,
	}
}

// CreateVersion creates a new version for a model
func (vm *VersionManager) CreateVersion(ctx context.Context, modelID string, version *ModelVersion) error {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	// Validate version format
	semVer, err := ParseVersion(version.Version)
	if err != nil {
		return NewVersionError(modelID, version.Version, err, "invalid version format")
	}

	// Check if version already exists
	existing, err := vm.GetVersion(ctx, modelID, version.Version)
	if err == nil && existing != nil {
		return NewVersionError(modelID, version.Version, ErrVersionAlreadyExists, "version already exists")
	}

	// Get model to verify it exists
	model, err := vm.registry.Get(ctx, modelID)
	if err != nil {
		return NewVersionError(modelID, version.Version, err, "model not found")
	}

	// Set timestamps
	now := time.Now()
	version.CreatedAt = now
	version.UpdatedAt = now
	version.ModelID = modelID

	// If this is the first version, make it active
	versionCount, err := vm.getVersionCount(ctx, modelID)
	if err != nil {
		return err
	}
	if versionCount == 0 {
		version.IsActive = true
	}

	// Insert into database
	query := `
		INSERT INTO model_versions (
			id, model_id, version, source_url, source_checksum, changelog,
			status, is_active, is_deprecated, created_at, updated_at, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`

	_, err = vm.db.ExecContext(ctx, query,
		version.ID, version.ModelID, version.Version,
		version.Source.URL, version.Source.Checksum, version.Changelog,
		version.Status, version.IsActive, version.IsDeprecated,
		version.CreatedAt, version.UpdatedAt, version.CreatedBy,
	)

	if err != nil {
		return NewVersionError(modelID, version.Version, err, "failed to create version in database")
	}

	log.Printf("Created version %s for model %s", semVer.String(), model.Name)
	return nil
}

// GetVersion retrieves a specific version of a model
func (vm *VersionManager) GetVersion(ctx context.Context, modelID string, version string) (*ModelVersion, error) {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	query := `
		SELECT id, model_id, version, source_url, source_checksum, changelog,
			   status, is_active, is_deprecated, created_at, updated_at, created_by
		FROM model_versions
		WHERE model_id = $1 AND version = $2
	`

	versionModel := &ModelVersion{}
	err := vm.db.QueryRowContext(ctx, query, modelID, version).Scan(
		&versionModel.ID, &versionModel.ModelID, &versionModel.Version,
		&versionModel.Source.URL, &versionModel.Source.Checksum, &versionModel.Changelog,
		&versionModel.Status, &versionModel.IsActive, &versionModel.IsDeprecated,
		&versionModel.CreatedAt, &versionModel.UpdatedAt, &versionModel.CreatedBy,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, NewVersionError(modelID, version, ErrVersionNotFound, "version not found")
		}
		return nil, NewVersionError(modelID, version, err, "failed to get version")
	}

	return versionModel, nil
}

// ListVersions lists all versions of a model
func (vm *VersionManager) ListVersions(ctx context.Context, modelID string) ([]*ModelVersion, error) {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	query := `
		SELECT id, model_id, version, source_url, source_checksum, changelog,
			   status, is_active, is_deprecated, created_at, updated_at, created_by
		FROM model_versions
		WHERE model_id = $1
		ORDER BY created_at DESC
	`

	rows, err := vm.db.QueryContext(ctx, query, modelID)
	if err != nil {
		return nil, NewVersionError(modelID, "", err, "failed to list versions")
	}
	defer rows.Close()

	versions := []*ModelVersion{}
	for rows.Next() {
		version := &ModelVersion{}
		err := rows.Scan(
			&version.ID, &version.ModelID, &version.Version,
			&version.Source.URL, &version.Source.Checksum, &version.Changelog,
			&version.Status, &version.IsActive, &version.IsDeprecated,
			&version.CreatedAt, &version.UpdatedAt, &version.CreatedBy,
		)
		if err != nil {
			return nil, NewVersionError(modelID, "", err, "failed to scan version")
		}
		versions = append(versions, version)
	}

	return versions, nil
}

// SetActiveVersion sets a specific version as the active version
func (vm *VersionManager) SetActiveVersion(ctx context.Context, modelID string, version string) error {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	// Start transaction
	tx, err := vm.db.BeginTx(ctx, nil)
	if err != nil {
		return NewVersionError(modelID, version, err, "failed to begin transaction")
	}
	defer tx.Rollback()

	// Deactivate all versions for this model
	_, err = tx.ExecContext(ctx,
		"UPDATE model_versions SET is_active = false, updated_at = $1 WHERE model_id = $2",
		time.Now(), modelID,
	)
	if err != nil {
		return NewVersionError(modelID, version, err, "failed to deactivate versions")
	}

	// Activate the specified version
	result, err := tx.ExecContext(ctx,
		"UPDATE model_versions SET is_active = true, updated_at = $1 WHERE model_id = $2 AND version = $3",
		time.Now(), modelID, version,
	)
	if err != nil {
		return NewVersionError(modelID, version, err, "failed to activate version")
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return NewVersionError(modelID, version, err, "failed to get rows affected")
	}

	if rows == 0 {
		return NewVersionError(modelID, version, ErrVersionNotFound, "version not found")
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return NewVersionError(modelID, version, err, "failed to commit transaction")
	}

	log.Printf("Set active version %s for model %s", version, modelID)
	return nil
}

// DeprecateVersion marks a version as deprecated
func (vm *VersionManager) DeprecateVersion(ctx context.Context, modelID string, version string) error {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	// Check if version is active
	versionModel, err := vm.GetVersion(ctx, modelID, version)
	if err != nil {
		return err
	}

	if versionModel.IsActive {
		return NewVersionError(modelID, version, ErrVersionConflict, "cannot deprecate active version")
	}

	query := `
		UPDATE model_versions
		SET is_deprecated = true, updated_at = $1
		WHERE model_id = $2 AND version = $3
	`

	result, err := vm.db.ExecContext(ctx, query, time.Now(), modelID, version)
	if err != nil {
		return NewVersionError(modelID, version, err, "failed to deprecate version")
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return NewVersionError(modelID, version, err, "failed to get rows affected")
	}

	if rows == 0 {
		return NewVersionError(modelID, version, ErrVersionNotFound, "version not found")
	}

	log.Printf("Deprecated version %s for model %s", version, modelID)
	return nil
}

// CompareVersions compares two versions of a model
func (vm *VersionManager) CompareVersions(ctx context.Context, modelID string, v1, v2 string) (*VersionComparison, error) {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	// Get both versions
	version1, err := vm.GetVersion(ctx, modelID, v1)
	if err != nil {
		return nil, err
	}

	version2, err := vm.GetVersion(ctx, modelID, v2)
	if err != nil {
		return nil, err
	}

	// Parse versions
	semVer1, err := ParseVersion(v1)
	if err != nil {
		return nil, NewVersionError(modelID, v1, err, "invalid version format")
	}

	semVer2, err := ParseVersion(v2)
	if err != nil {
		return nil, NewVersionError(modelID, v2, err, "invalid version format")
	}

	// Compare versions
	comparison := &VersionComparison{
		ModelID:     modelID,
		FromVersion: v1,
		ToVersion:   v2,
		Differences: []VersionDiff{},
		UpgradePath: []string{},
	}

	// Determine if this is an upgrade or downgrade
	if semVer2.IsGreaterThan(semVer1) {
		comparison.UpgradePath = vm.generateUpgradePath(semVer1, semVer2)
	}

	// Compare version details
	if version1.Source.URL != version2.Source.URL {
		comparison.Differences = append(comparison.Differences, VersionDiff{
			Field:    "source_url",
			OldValue: version1.Source.URL,
			NewValue: version2.Source.URL,
			Type:     "changed",
		})
	}

	if version1.Changelog != version2.Changelog {
		comparison.Differences = append(comparison.Differences, VersionDiff{
			Field:    "changelog",
			OldValue: version1.Changelog,
			NewValue: version2.Changelog,
			Type:     "changed",
		})
	}

	if version1.Status != version2.Status {
		comparison.Differences = append(comparison.Differences, VersionDiff{
			Field:    "status",
			OldValue: version1.Status,
			NewValue: version2.Status,
			Type:     "changed",
		})
	}

	return comparison, nil
}

// generateUpgradePath generates the upgrade path between two versions
func (vm *VersionManager) generateUpgradePath(from, to *SemanticVersion) []string {
	path := []string{}

	// Simple implementation - in a real system, you'd query the database
	// for all versions between from and to
	current := from

	for current.IsLessThan(to) {
		if current.Major < to.Major {
			current = current.IncrementMajor()
		} else if current.Minor < to.Minor {
			current = current.IncrementMinor()
		} else {
			current = current.IncrementPatch()
		}

		if current.IsLessThan(to) || current.IsEqual(to) {
			path = append(path, current.String())
		}
	}

	return path
}

// GetActiveVersion gets the currently active version for a model
func (vm *VersionManager) GetActiveVersion(ctx context.Context, modelID string) (*ModelVersion, error) {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	query := `
		SELECT id, model_id, version, source_url, source_checksum, changelog,
			   status, is_active, is_deprecated, created_at, updated_at, created_by
		FROM model_versions
		WHERE model_id = $1 AND is_active = true
	`

	version := &ModelVersion{}
	err := vm.db.QueryRowContext(ctx, query, modelID).Scan(
		&version.ID, &version.ModelID, &version.Version,
		&version.Source.URL, &version.Source.Checksum, &version.Changelog,
		&version.Status, &version.IsActive, &version.IsDeprecated,
		&version.CreatedAt, &version.UpdatedAt, &version.CreatedBy,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, NewVersionError(modelID, "", ErrVersionNotFound, "no active version found")
		}
		return nil, NewVersionError(modelID, "", err, "failed to get active version")
	}

	return version, nil
}

// GetLatestVersion gets the latest version for a model
func (vm *VersionManager) GetLatestVersion(ctx context.Context, modelID string) (*ModelVersion, error) {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	versions, err := vm.ListVersions(ctx, modelID)
	if err != nil {
		return nil, err
	}

	if len(versions) == 0 {
		return nil, NewVersionError(modelID, "", ErrVersionNotFound, "no versions found")
	}

	// Parse and compare versions
	var latest *ModelVersion
	var latestSemVer *SemanticVersion

	for _, v := range versions {
		semVer, err := ParseVersion(v.Version)
		if err != nil {
			continue // Skip invalid versions
		}

		if latest == nil || semVer.IsGreaterThan(latestSemVer) {
			latest = v
			latestSemVer = semVer
		}
	}

	return latest, nil
}

// DeleteVersion deletes a specific version
func (vm *VersionManager) DeleteVersion(ctx context.Context, modelID string, version string) error {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	// Check if version is active
	versionModel, err := vm.GetVersion(ctx, modelID, version)
	if err != nil {
		return err
	}

	if versionModel.IsActive {
		return NewVersionError(modelID, version, ErrVersionConflict, "cannot delete active version")
	}

	query := `DELETE FROM model_versions WHERE model_id = $1 AND version = $2`
	result, err := vm.db.ExecContext(ctx, query, modelID, version)
	if err != nil {
		return NewVersionError(modelID, version, err, "failed to delete version")
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return NewVersionError(modelID, version, err, "failed to get rows affected")
	}

	if rows == 0 {
		return NewVersionError(modelID, version, ErrVersionNotFound, "version not found")
	}

	log.Printf("Deleted version %s for model %s", version, modelID)
	return nil
}

// getVersionCount gets the count of versions for a model
func (vm *VersionManager) getVersionCount(ctx context.Context, modelID string) (int, error) {
	query := `SELECT COUNT(*) FROM model_versions WHERE model_id = $1`
	var count int
	err := vm.db.QueryRowContext(ctx, query, modelID).Scan(&count)
	if err != nil {
		return 0, NewVersionError(modelID, "", err, "failed to count versions")
	}
	return count, nil
}

// RollbackVersion rolls back to a previous version
func (vm *VersionManager) RollbackVersion(ctx context.Context, modelID string, targetVersion string) error {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	// Get current active version
	currentVersion, err := vm.GetActiveVersion(ctx, modelID)
	if err != nil {
		return err
	}

	// Parse versions
	currentSemVer, err := ParseVersion(currentVersion.Version)
	if err != nil {
		return NewVersionError(modelID, currentVersion.Version, err, "invalid current version format")
	}

	targetSemVer, err := ParseVersion(targetVersion)
	if err != nil {
		return NewVersionError(modelID, targetVersion, err, "invalid target version format")
	}

	// Check if target is actually a rollback
	if targetSemVer.IsGreaterThan(currentSemVer) || targetSemVer.IsEqual(currentSemVer) {
		return NewVersionError(modelID, targetVersion, ErrVersionConflict, "target version must be lower than current version")
	}

	// Verify target version exists
	_, err = vm.GetVersion(ctx, modelID, targetVersion)
	if err != nil {
		return err
	}

	// Set target version as active
	return vm.SetActiveVersion(ctx, modelID, targetVersion)
}

// GetVersionHistory gets the version history for a model
func (vm *VersionManager) GetVersionHistory(ctx context.Context, modelID string) ([]*ModelEvent, error) {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	query := `
		SELECT id, model_id, type, message, details, timestamp, user_id
		FROM model_events
		WHERE model_id = $1 AND type LIKE 'version_%'
		ORDER BY timestamp DESC
		LIMIT 100
	`

	rows, err := vm.db.QueryContext(ctx, query, modelID)
	if err != nil {
		return nil, NewVersionError(modelID, "", err, "failed to get version history")
	}
	defer rows.Close()

	events := []*ModelEvent{}
	for rows.Next() {
		event := &ModelEvent{}
		err := rows.Scan(
			&event.ID, &event.ModelID, &event.Type, &event.Message,
			&event.Details, &event.Timestamp, &event.UserID,
		)
		if err != nil {
			return nil, NewVersionError(modelID, "", err, "failed to scan event")
		}
		events = append(events, event)
	}

	return events, nil
}

// SuggestNextVersion suggests the next version based on the current version and change type
func (vm *VersionManager) SuggestNextVersion(ctx context.Context, modelID string, changeType string) (string, error) {
	latest, err := vm.GetLatestVersion(ctx, modelID)
	if err != nil {
		// If no versions exist, start with 1.0.0
		return "1.0.0", nil
	}

	currentSemVer, err := ParseVersion(latest.Version)
	if err != nil {
		return "", NewVersionError(modelID, latest.Version, err, "invalid current version format")
	}

	var nextSemVer *SemanticVersion
	switch changeType {
	case "major":
		nextSemVer = currentSemVer.IncrementMajor()
	case "minor":
		nextSemVer = currentSemVer.IncrementMinor()
	case "patch":
		nextSemVer = currentSemVer.IncrementPatch()
	default:
		// Default to patch increment
		nextSemVer = currentSemVer.IncrementPatch()
	}

	return nextSemVer.String(), nil
}

// ValidateUpgradePath validates if an upgrade path is valid
func (vm *VersionManager) ValidateUpgradePath(ctx context.Context, modelID string, fromVersion, toVersion string) error {
	// Get both versions
	from, err := vm.GetVersion(ctx, modelID, fromVersion)
	if err != nil {
		return err
	}

	to, err := vm.GetVersion(ctx, modelID, toVersion)
	if err != nil {
		return err
	}

	// Parse versions
	fromSemVer, err := ParseVersion(from.Version)
	if err != nil {
		return NewVersionError(modelID, fromVersion, err, "invalid from version format")
	}

	toSemVer, err := ParseVersion(to.Version)
	if err != nil {
		return NewVersionError(modelID, toVersion, err, "invalid to version format")
	}

	// Check if to version is greater
	if !toSemVer.IsGreaterThan(fromSemVer) {
		return NewVersionError(modelID, toVersion, ErrVersionConflict, "target version must be greater than source version")
	}

	// Check if versions are deprecated
	if from.IsDeprecated {
		return NewVersionError(modelID, fromVersion, ErrVersionConflict, "source version is deprecated")
	}

	if to.IsDeprecated {
		return NewVersionError(modelID, toVersion, ErrVersionConflict, "target version is deprecated")
	}

	return nil
}

// GetVersionStats returns statistics about model versions
func (vm *VersionManager) GetVersionStats(ctx context.Context, modelID string) (map[string]interface{}, error) {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	versions, err := vm.ListVersions(ctx, modelID)
	if err != nil {
		return nil, err
	}

	stats := map[string]interface{}{
		"total_versions":   len(versions),
		"active_versions":  0,
		"deprecated_versions": 0,
		"latest_version":   "",
		"oldest_version":   "",
	}

	var latestSemVer, oldestSemVer *SemanticVersion

	for _, v := range versions {
		if v.IsActive {
			stats["active_versions"] = stats["active_versions"].(int) + 1
		}
		if v.IsDeprecated {
			stats["deprecated_versions"] = stats["deprecated_versions"].(int) + 1
		}

		semVer, err := ParseVersion(v.Version)
		if err != nil {
			continue
		}

		if latestSemVer == nil || semVer.IsGreaterThan(latestSemVer) {
			latestSemVer = semVer
			stats["latest_version"] = v.Version
		}

		if oldestSemVer == nil || semVer.IsLessThan(oldestSemVer) {
			oldestSemVer = semVer
			stats["oldest_version"] = v.Version
		}
	}

	return stats, nil
}
