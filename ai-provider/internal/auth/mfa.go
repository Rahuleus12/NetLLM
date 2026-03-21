package auth

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// MFAProvider defines the type of MFA provider
type MFAProvider string

const (
	MFAProviderTOTP MFAProvider = "totp"
	MFAProviderSMS  MFAProvider = "sms"
)

// MFAStatus represents the status of MFA for a user
type MFAStatus string

const (
	MFAStatusDisabled MFAStatus = "disabled"
	MFAStatusEnabled  MFAStatus = "enabled"
	MFAStatusPending  MFAStatus = "pending" // Setup initiated but not verified
)

// MFAConfig holds MFA configuration
type MFAConfig struct {
	Issuer        string
	IssuerLabel   string
	Digits        int
	Period        uint
	Algorithm     otp.Algorithm
	Skew          uint
	RecoveryCodes int
}

// DefaultMFAConfig returns the default MFA configuration
func DefaultMFAConfig() *MFAConfig {
	return &MFAConfig{
		Issuer:        "AI-Provider",
		IssuerLabel:   "AI-Provider",
		Digits:        6,
		Period:        30,
		Algorithm:     otp.AlgorithmSHA1,
		Skew:          1,
		RecoveryCodes: 8,
	}
}

// MFASetup represents the MFA setup response
type MFASetup struct {
	Secret        string   `json:"secret"`
	QRCodeURL     string   `json:"qr_code_url"`
	RecoveryCodes []string `json:"recovery_codes"`
	Provider      string   `json:"provider"`
}

// MFAVerifyRequest represents an MFA verification request
type MFAVerifyRequest struct {
	UserID   string `json:"user_id"`
	Code     string `json:"code"`
	Provider string `json:"provider,omitempty"`
}

// MFAVerifyResponse represents the MFA verification response
type MFAVerifyResponse struct {
	Success    bool   `json:"success"`
	Message    string `json:"message,omitempty"`
	RecoveryCodeUsed bool `json:"recovery_code_used,omitempty"`
}

// MFAService handles multi-factor authentication operations
type MFAService struct {
	config        *MFAConfig
	recoveryStore RecoveryCodeStore
}

// RecoveryCodeStore defines the interface for storing recovery codes
type RecoveryCodeStore interface {
	Store(userID string, codes []string) error
	Validate(userID string, code string) (bool, error)
	Delete(userID string) error
	List(userID string) ([]string, error)
	MarkUsed(userID string, code string) error
}

// NewMFAService creates a new MFA service
func NewMFAService(config *MFAConfig, recoveryStore RecoveryCodeStore) *MFAService {
	if config == nil {
		config = DefaultMFAConfig()
	}
	return &MFAService{
		config:        config,
		recoveryStore: recoveryStore,
	}
}

// GenerateSecret generates a new TOTP secret for a user
func (s *MFAService) GenerateSecret(userID, userEmail string) (*MFASetup, error) {
	// Generate a new TOTP key
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      s.config.Issuer,
		AccountName: userEmail,
		Period:      s.config.Period,
		SecretSize:  32,
		Secret:      nil,
		Digits:      otp.Digits(s.config.Digits),
		Algorithm:   s.config.Algorithm,
	})
	if err != nil {
		return nil, NewAuthError(ErrCodeInternalError, "failed to generate MFA secret").WithCause(err)
	}

	// Generate recovery codes
	recoveryCodes, err := s.generateRecoveryCodes()
	if err != nil {
		return nil, NewAuthError(ErrCodeInternalError, "failed to generate recovery codes").WithCause(err)
	}

	return &MFASetup{
		Secret:        key.Secret(),
		QRCodeURL:     key.URL(),
		RecoveryCodes: recoveryCodes,
		Provider:      string(MFAProviderTOTP),
	}, nil
}

// EnableMFA enables MFA for a user after verifying the setup
func (s *MFAService) EnableMFA(userID string, secret string, verificationCode string) error {
	// Verify the code before enabling
	valid, err := s.ValidateTOTP(secret, verificationCode)
	if err != nil {
		return err
	}
	if !valid {
		return NewAuthError(ErrCodeInvalidMFA, "invalid verification code")
	}
	return nil
}

// ValidateTOTP validates a TOTP code against a secret
func (s *MFAService) ValidateTOTP(secret string, code string) (bool, error) {
	// Clean the code (remove spaces, dashes)
	code = strings.ReplaceAll(code, " ", "")
	code = strings.ReplaceAll(code, "-", "")

	// Validate code format
	if len(code) != s.config.Digits {
		return false, nil
	}

	// Validate the code
	valid, err := totp.ValidateCustom(code, secret, time.Now().UTC(), totp.ValidateOpts{
		Period:    s.config.Period,
		Skew:      s.config.Skew,
		Digits:    otp.Digits(s.config.Digits),
		Algorithm: s.config.Algorithm,
	})
	if err != nil {
		return false, NewAuthError(ErrCodeInvalidMFA, "failed to validate TOTP code").WithCause(err)
	}

	return valid, nil
}

// ValidateWithRecovery validates using either TOTP or a recovery code
func (s *MFAService) ValidateWithRecovery(userID string, secret string, code string) (*MFAVerifyResponse, error) {
	// First try TOTP validation
	valid, err := s.ValidateTOTP(secret, code)
	if err != nil {
		return nil, err
	}
	if valid {
		return &MFAVerifyResponse{
			Success: true,
			Message: "MFA verification successful",
		}, nil
	}

	// If TOTP fails and recovery store is available, try recovery code
	if s.recoveryStore != nil {
		recoveryValid, err := s.recoveryStore.Validate(userID, code)
		if err != nil {
			return nil, NewAuthError(ErrCodeInternalError, "failed to validate recovery code").WithCause(err)
		}
		if recoveryValid {
			// Mark the recovery code as used
			if err := s.recoveryStore.MarkUsed(userID, code); err != nil {
				return nil, NewAuthError(ErrCodeInternalError, "failed to mark recovery code as used").WithCause(err)
			}
			return &MFAVerifyResponse{
				Success:          true,
				Message:          "Recovery code used successfully",
				RecoveryCodeUsed: true,
			}, nil
		}
	}

	return &MFAVerifyResponse{
		Success: false,
		Message: "invalid MFA code",
	}, nil
}

// StoreRecoveryCodes stores recovery codes for a user
func (s *MFAService) StoreRecoveryCodes(userID string, codes []string) error {
	if s.recoveryStore == nil {
		return NewAuthError(ErrCodeInternalError, "recovery code store not configured")
	}
	return s.recoveryStore.Store(userID, codes)
}

// GetRecoveryCodes retrieves remaining recovery codes for a user
func (s *MFAService) GetRecoveryCodes(userID string) ([]string, error) {
	if s.recoveryStore == nil {
		return nil, NewAuthError(ErrCodeInternalError, "recovery code store not configured")
	}
	return s.recoveryStore.List(userID)
}

// RegenerateRecoveryCodes generates new recovery codes for a user
func (s *MFAService) RegenerateRecoveryCodes(userID string) ([]string, error) {
	codes, err := s.generateRecoveryCodes()
	if err != nil {
		return nil, err
	}

	if s.recoveryStore != nil {
		if err := s.recoveryStore.Delete(userID); err != nil {
			return nil, NewAuthError(ErrCodeInternalError, "failed to delete old recovery codes").WithCause(err)
		}
		if err := s.recoveryStore.Store(userID, codes); err != nil {
			return nil, NewAuthError(ErrCodeInternalError, "failed to store new recovery codes").WithCause(err)
		}
	}

	return codes, nil
}

// DisableMFA disables MFA for a user
func (s *MFAService) DisableMFA(userID string) error {
	if s.recoveryStore != nil {
		if err := s.recoveryStore.Delete(userID); err != nil {
			return NewAuthError(ErrCodeInternalError, "failed to delete recovery codes").WithCause(err)
		}
	}
	return nil
}

// generateRecoveryCodes generates a set of recovery codes
func (s *MFAService) generateRecoveryCodes() ([]string, error) {
	codes := make([]string, s.config.RecoveryCodes)
	for i := 0; i < s.config.RecoveryCodes; i++ {
		code, err := s.generateRecoveryCode()
		if err != nil {
			return nil, err
		}
		codes[i] = code
	}
	return codes, nil
}

// generateRecoveryCode generates a single recovery code
// Format: XXXX-XXXX (8 alphanumeric characters)
func (s *MFAService) generateRecoveryCode() (string, error) {
	bytes := make([]byte, 5) // 5 bytes = 8 base32 characters
	if _, err := rand.Read(bytes); err != nil {
		return "", NewAuthError(ErrCodeInternalError, "failed to generate random bytes").WithCause(err)
	}

	// Encode to base32 and take first 8 characters
	encoded := base32.StdEncoding.EncodeToString(bytes)
	code := encoded[:8]

	// Format as XXXX-XXXX
	return fmt.Sprintf("%s-%s", code[:4], code[4:]), nil
}

// MFAStats represents MFA statistics for a user
type MFAStats struct {
	Enabled            bool      `json:"enabled"`
	Provider           string    `json:"provider"`
	RecoveryCodesLeft  int       `json:"recovery_codes_left"`
	LastUsed           time.Time `json:"last_used,omitempty"`
	SetupAt            time.Time `json:"setup_at,omitempty"`
}

// GetMFAStats returns MFA statistics for a user
func (s *MFAService) GetMFAStats(userID string, mfaEnabled bool, setupAt time.Time) (*MFAStats, error) {
	stats := &MFAStats{
		Enabled:  mfaEnabled,
		Provider: string(MFAProviderTOTP),
		SetupAt:  setupAt,
	}

	if s.recoveryStore != nil && mfaEnabled {
		codes, err := s.recoveryStore.List(userID)
		if err != nil {
			return nil, NewAuthError(ErrCodeInternalError, "failed to get recovery codes").WithCause(err)
		}
		stats.RecoveryCodesLeft = len(codes)
	}

	return stats, nil
}

// ValidateCodeFormat validates the format of an MFA code
func ValidateCodeFormat(code string) bool {
	// Remove spaces and dashes
	code = strings.ReplaceAll(code, " ", "")
	code = strings.ReplaceAll(code, "-", "")

	// Should be 6-8 digits
	if len(code) < 6 || len(code) > 8 {
		return false
	}

	// Should be all digits
	for _, c := range code {
		if c < '0' || c > '9' {
			return false
		}
	}

	return true
}

// ValidateRecoveryCodeFormat validates the format of a recovery code
func ValidateRecoveryCodeFormat(code string) bool {
	// Remove spaces
	code = strings.ReplaceAll(code, " ", "")

	// Format should be XXXX-XXXX or XXXXXXXX
	if len(code) == 9 && code[4] == '-' {
		code = code[:4] + code[5:]
	}

	if len(code) != 8 {
		return false
	}

	// Should be all alphanumeric
	for _, c := range code {
		if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')) {
			return false
		}
	}

	return true
}
