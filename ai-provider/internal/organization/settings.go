// internal/organization/settings.go
// Organization settings and preferences management
// Handles organization-level configuration, branding, and preferences

package organization

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	ErrSettingsNotFound    = errors.New("settings not found")
	ErrInvalidSettings     = errors.New("invalid settings")
	ErrSettingsUpdateFailed = errors.New("failed to update settings")
)

// OrganizationSettings represents organization-level settings
type OrganizationSettings struct {
	// Organization Reference
	OrganizationID string `json:"organization_id" db:"organization_id"`

	// Branding Settings
	Branding BrandingSettings `json:"branding" db:"branding"`

	// Preferences
	Preferences PreferenceSettings `json:"preferences" db:"preferences"`

	// Notification Settings
	Notifications NotificationSettings `json:"notifications" db:"notifications"`

	// Feature Flags
	Features map[string]bool `json:"features" db:"features"`

	// Metadata
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
	UpdatedBy string    `json:"updated_by" db:"updated_by"`
}

// BrandingSettings represents organization branding configuration
type BrandingSettings struct {
	LogoURL       string `json:"logo_url"`
	FaviconURL    string `json:"favicon_url"`
	PrimaryColor  string `json:"primary_color"`
	SecondaryColor string `json:"secondary_color"`
	CustomDomain  string `json:"custom_domain"`
	CompanyName   string `json:"company_name"`
	SupportEmail  string `json:"support_email"`
	WebsiteURL    string `json:"website_url"`
}

// PreferenceSettings represents organization preferences
type PreferenceSettings struct {
	DefaultWorkspace string   `json:"default_workspace"`
	Timezone         string   `json:"timezone"`
	Language         string   `json:"language"`
	DateFormat       string   `json:"date_format"`
	Theme            string   `json:"theme"`
	AutoSave         bool     `json:"auto_save"`
	DefaultModel     string   `json:"default_model"`
	AllowedModels    []string `json:"allowed_models"`
}

// NotificationSettings represents notification preferences
type NotificationSettings struct {
	EmailEnabled        bool     `json:"email_enabled"`
	SlackEnabled        bool     `json:"slack_enabled"`
	WebhookEnabled      bool     `json:"webhook_enabled"`
	WebhookURL          string   `json:"webhook_url"`
	NotificationEmails  []string `json:"notification_emails"`
	AlertTypes          []string `json:"alert_types"`
	DigestFrequency     string   `json:"digest_frequency"` // daily, weekly, monthly
	QuietHours          QuietHours `json:"quiet_hours"`
}

// QuietHours represents quiet hours configuration
type QuietHours struct {
	Enabled  bool   `json:"enabled"`
	StartTime string `json:"start_time"` // HH:MM format
	EndTime   string `json:"end_time"`   // HH:MM format
	Timezone string `json:"timezone"`
}

// SettingsManager manages organization settings
type SettingsManager struct {
	db *sql.DB
}

// NewSettingsManager creates a new settings manager
func NewSettingsManager(db *sql.DB) *SettingsManager {
	return &SettingsManager{
		db: db,
	}
}

// CreateSettings creates default settings for a new organization
func (sm *SettingsManager) CreateSettings(ctx context.Context, organizationID, createdBy string) (*OrganizationSettings, error) {
	if organizationID == "" {
		return nil, ErrInvalidSettings
	}

	settings := sm.GetDefaultSettings()
	settings.OrganizationID = organizationID
	settings.CreatedAt = time.Now()
	settings.UpdatedAt = time.Now()
	settings.UpdatedBy = createdBy

	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal settings: %w", err)
	}

	query := `
		INSERT INTO organization_settings (organization_id, branding, preferences, notifications, features, created_at, updated_at, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING organization_id, created_at, updated_at
	`

	var created, updated time.Time
	err = sm.db.QueryRowContext(ctx, query,
		settings.OrganizationID,
		settingsJSON,
		settingsJSON,
		settingsJSON,
		settingsJSON,
		settings.CreatedAt,
		settings.UpdatedAt,
		settings.UpdatedBy,
	).Scan(&settings.OrganizationID, &created, &updated)

	if err != nil {
		return nil, fmt.Errorf("failed to create settings: %w", err)
	}

	settings.CreatedAt = created
	settings.UpdatedAt = updated

	return settings, nil
}

// GetSettings retrieves settings for an organization
func (sm *SettingsManager) GetSettings(ctx context.Context, organizationID string) (*OrganizationSettings, error) {
	var settings OrganizationSettings
	var brandingJSON, preferencesJSON, notificationsJSON, featuresJSON []byte

	query := `
		SELECT organization_id, branding, preferences, notifications, features, created_at, updated_at, updated_by
		FROM organization_settings
		WHERE organization_id = $1
	`

	err := sm.db.QueryRowContext(ctx, query, organizationID).Scan(
		&settings.OrganizationID,
		&brandingJSON,
		&preferencesJSON,
		&notificationsJSON,
		&featuresJSON,
		&settings.CreatedAt,
		&settings.UpdatedAt,
		&settings.UpdatedBy,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrSettingsNotFound
		}
		return nil, fmt.Errorf("failed to get settings: %w", err)
	}

	// Unmarshal JSON fields
	if err := json.Unmarshal(brandingJSON, &settings.Branding); err != nil {
		return nil, fmt.Errorf("failed to unmarshal branding: %w", err)
	}
	if err := json.Unmarshal(preferencesJSON, &settings.Preferences); err != nil {
		return nil, fmt.Errorf("failed to unmarshal preferences: %w", err)
	}
	if err := json.Unmarshal(notificationsJSON, &settings.Notifications); err != nil {
		return nil, fmt.Errorf("failed to unmarshal notifications: %w", err)
	}
	if err := json.Unmarshal(featuresJSON, &settings.Features); err != nil {
		return nil, fmt.Errorf("failed to unmarshal features: %w", err)
	}

	return &settings, nil
}

// UpdateSettings updates organization settings
func (sm *SettingsManager) UpdateSettings(ctx context.Context, organizationID string, updates map[string]interface{}, updatedBy string) (*OrganizationSettings, error) {
	settings, err := sm.GetSettings(ctx, organizationID)
	if err != nil {
		return nil, err
	}

	// Apply updates
	if branding, ok := updates["branding"].(map[string]interface{}); ok {
		// Merge branding settings
		brandingJSON, _ := json.Marshal(branding)
		json.Unmarshal(brandingJSON, &settings.Branding)
	}

	if preferences, ok := updates["preferences"].(map[string]interface{}); ok {
		preferencesJSON, _ := json.Marshal(preferences)
		json.Unmarshal(preferencesJSON, &settings.Preferences)
	}

	if notifications, ok := updates["notifications"].(map[string]interface{}); ok {
		notificationsJSON, _ := json.Marshal(notifications)
		json.Unmarshal(notificationsJSON, &settings.Notifications)
	}

	if features, ok := updates["features"].(map[string]bool); ok {
		settings.Features = features
	}

	settings.UpdatedAt = time.Now()
	settings.UpdatedBy = updatedBy

	// Validate settings
	if err := sm.ValidateSettings(settings); err != nil {
		return nil, err
	}

	// Serialize settings
	brandingJSON, err := json.Marshal(settings.Branding)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal branding: %w", err)
	}

	preferencesJSON, err := json.Marshal(settings.Preferences)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal preferences: %w", err)
	}

	notificationsJSON, err := json.Marshal(settings.Notifications)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal notifications: %w", err)
	}

	featuresJSON, err := json.Marshal(settings.Features)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal features: %w", err)
	}

	// Update in database
	query := `
		UPDATE organization_settings
		SET branding = $1, preferences = $2, notifications = $3, features = $4, updated_at = $5, updated_by = $6
		WHERE organization_id = $7
		RETURNING organization_id, created_at, updated_at
	`

	var created, updated time.Time
	err = sm.db.QueryRowContext(ctx, query,
		brandingJSON,
		preferencesJSON,
		notificationsJSON,
		featuresJSON,
		settings.UpdatedAt,
		settings.UpdatedBy,
		organizationID,
	).Scan(&settings.OrganizationID, &created, &updated)

	if err != nil {
		return nil, fmt.Errorf("failed to update settings: %w", err)
	}

	settings.CreatedAt = created
	settings.UpdatedAt = updated

	return settings, nil
}

// UpdateBranding updates branding settings
func (sm *SettingsManager) UpdateBranding(ctx context.Context, organizationID string, branding *BrandingSettings, updatedBy string) error {
	settings, err := sm.GetSettings(ctx, organizationID)
	if err != nil {
		return err
	}

	settings.Branding = *branding
	settings.UpdatedAt = time.Now()
	settings.UpdatedBy = updatedBy

	brandingJSON, err := json.Marshal(settings.Branding)
	if err != nil {
		return fmt.Errorf("failed to marshal branding: %w", err)
	}

	query := `
		UPDATE organization_settings
		SET branding = $1, updated_at = $2, updated_by = $3
		WHERE organization_id = $4
	`

	_, err = sm.db.ExecContext(ctx, query, brandingJSON, settings.UpdatedAt, updatedBy, organizationID)
	if err != nil {
		return fmt.Errorf("failed to update branding: %w", err)
	}

	return nil
}

// UpdatePreferences updates preference settings
func (sm *SettingsManager) UpdatePreferences(ctx context.Context, organizationID string, preferences *PreferenceSettings, updatedBy string) error {
	settings, err := sm.GetSettings(ctx, organizationID)
	if err != nil {
		return err
	}

	settings.Preferences = *preferences
	settings.UpdatedAt = time.Now()
	settings.UpdatedBy = updatedBy

	preferencesJSON, err := json.Marshal(settings.Preferences)
	if err != nil {
		return fmt.Errorf("failed to marshal preferences: %w", err)
	}

	query := `
		UPDATE organization_settings
		SET preferences = $1, updated_at = $2, updated_by = $3
		WHERE organization_id = $4
	`

	_, err = sm.db.ExecContext(ctx, query, preferencesJSON, settings.UpdatedAt, updatedBy, organizationID)
	if err != nil {
		return fmt.Errorf("failed to update preferences: %w", err)
	}

	return nil
}

// UpdateNotifications updates notification settings
func (sm *SettingsManager) UpdateNotifications(ctx context.Context, organizationID string, notifications *NotificationSettings, updatedBy string) error {
	settings, err := sm.GetSettings(ctx, organizationID)
	if err != nil {
		return err
	}

	settings.Notifications = *notifications
	settings.UpdatedAt = time.Now()
	settings.UpdatedBy = updatedBy

	notificationsJSON, err := json.Marshal(settings.Notifications)
	if err != nil {
		return fmt.Errorf("failed to marshal notifications: %w", err)
	}

	query := `
		UPDATE organization_settings
		SET notifications = $1, updated_at = $2, updated_by = $3
		WHERE organization_id = $4
	`

	_, err = sm.db.ExecContext(ctx, query, notificationsJSON, settings.UpdatedAt, updatedBy, organizationID)
	if err != nil {
		return fmt.Errorf("failed to update notifications: %w", err)
	}

	return nil
}

// ValidateSettings validates organization settings
func (sm *SettingsManager) ValidateSettings(settings *OrganizationSettings) error {
	if settings.OrganizationID == "" {
		return ErrInvalidSettings
	}

	// Validate branding
	if settings.Branding.CustomDomain != "" {
		// TODO: Add proper domain validation
	}

	// Validate preferences
	if settings.Preferences.Timezone == "" {
		settings.Preferences.Timezone = "UTC"
	}
	if settings.Preferences.Language == "" {
		settings.Preferences.Language = "en"
	}

	// Validate notifications
	if settings.Notifications.DigestFrequency == "" {
		settings.Notifications.DigestFrequency = "daily"
	}

	return nil
}

// GetDefaultSettings returns default organization settings
func (sm *SettingsManager) GetDefaultSettings() *OrganizationSettings {
	return &OrganizationSettings{
		Branding: BrandingSettings{
			LogoURL:        "",
			FaviconURL:     "",
			PrimaryColor:   "#007bff",
			SecondaryColor: "#6c757d",
			CustomDomain:   "",
			CompanyName:    "",
			SupportEmail:   "",
			WebsiteURL:     "",
		},
		Preferences: PreferenceSettings{
			DefaultWorkspace: "",
			Timezone:         "UTC",
			Language:         "en",
			DateFormat:       "YYYY-MM-DD",
			Theme:            "light",
			AutoSave:         true,
			DefaultModel:     "",
			AllowedModels:    []string{},
		},
		Notifications: NotificationSettings{
			EmailEnabled:       true,
			SlackEnabled:       false,
			WebhookEnabled:     false,
			WebhookURL:         "",
			NotificationEmails: []string{},
			AlertTypes:         []string{"quota_exceeded", "system_alert"},
			DigestFrequency:    "daily",
			QuietHours: QuietHours{
				Enabled:  false,
				StartTime: "22:00",
				EndTime:   "08:00",
				Timezone: "UTC",
			},
		},
		Features: map[string]bool{
			"advanced_analytics": false,
			"custom_models":      false,
			"priority_support":   false,
			"api_access":         true,
		},
	}
}

// DeleteSettings deletes organization settings
func (sm *SettingsManager) DeleteSettings(ctx context.Context, organizationID string) error {
	query := `DELETE FROM organization_settings WHERE organization_id = $1`

	result, err := sm.db.ExecContext(ctx, query, organizationID)
	if err != nil {
		return fmt.Errorf("failed to delete settings: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return ErrSettingsNotFound
	}

	return nil
}
