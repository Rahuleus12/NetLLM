package auth

import (
	"errors"
	"fmt"
)

// Authentication error types
var (
	// ErrInvalidCredentials is returned when username/password is incorrect
	ErrInvalidCredentials = errors.New("invalid credentials")

	// ErrUserNotFound is returned when user does not exist
	ErrUserNotFound = errors.New("user not found")

	// ErrUserDisabled is returned when user account is disabled
	ErrUserDisabled = errors.New("user account is disabled")

	// ErrUserLocked is returned when user account is locked
	ErrUserLocked = errors.New("user account is locked")

	// ErrTokenExpired is returned when JWT token has expired
	ErrTokenExpired = errors.New("token has expired")

	// ErrTokenInvalid is returned when JWT token is invalid
	ErrTokenInvalid = errors.New("token is invalid")

	// ErrTokenMalformed is returned when JWT token is malformed
	ErrTokenMalformed = errors.New("token is malformed")

	// ErrTokenNotValidYet is returned when JWT token is not valid yet
	ErrTokenNotValidYet = errors.New("token is not valid yet")

	// ErrTokenSignatureInvalid is returned when JWT signature is invalid
	ErrTokenSignatureInvalid = errors.New("token signature is invalid")

	// ErrRefreshTokenExpired is returned when refresh token has expired
	ErrRefreshTokenExpired = errors.New("refresh token has expired")

	// ErrRefreshTokenInvalid is returned when refresh token is invalid
	ErrRefreshTokenInvalid = errors.New("refresh token is invalid")

	// ErrSessionExpired is returned when session has expired
	ErrSessionExpired = errors.New("session has expired")

	// ErrSessionInvalid is returned when session is invalid
	ErrSessionInvalid = errors.New("session is invalid")

	// ErrAPIKeyNotFound is returned when API key is not found
	ErrAPIKeyNotFound = errors.New("API key not found")

	// ErrAPIKeyExpired is returned when API key has expired
	ErrAPIKeyExpired = errors.New("API key has expired")

	// ErrAPIKeyRevoked is returned when API key has been revoked
	ErrAPIKeyRevoked = errors.New("API key has been revoked")

	// ErrAPIKeyInvalid is returned when API key is invalid
	ErrAPIKeyInvalid = errors.New("API key is invalid")

	// ErrMFANotEnabled is returned when MFA is not enabled for user
	ErrMFANotEnabled = errors.New("MFA is not enabled")

	// ErrMFAAlreadyEnabled is returned when MFA is already enabled
	ErrMFAAlreadyEnabled = errors.New("MFA is already enabled")

	// ErrMFAInvalidCode is returned when MFA code is invalid
	ErrMFAInvalidCode = errors.New("invalid MFA code")

	// ErrMFAMaxAttempts is returned when max MFA attempts exceeded
	ErrMFAMaxAttempts = errors.New("maximum MFA attempts exceeded")

	// ErrOAuthFailed is returned when OAuth authentication fails
	ErrOAuthFailed = errors.New("OAuth authentication failed")

	// ErrOAuthStateMismatch is returned when OAuth state does not match
	ErrOAuthStateMismatch = errors.New("OAuth state mismatch")

	// ErrOAuthProviderNotSupported is returned when OAuth provider is not supported
	ErrOAuthProviderNotSupported = errors.New("OAuth provider not supported")

	// ErrPasswordTooWeak is returned when password does not meet requirements
	ErrPasswordTooWeak = errors.New("password does not meet security requirements")

	// ErrPasswordReuse is returned when user reuses recent password
	ErrPasswordReuse = errors.New("cannot reuse recent password")

	// ErrEmailNotVerified is returned when email is not verified
	ErrEmailNotVerified = errors.New("email address not verified")
)

// AuthError represents a structured authentication error
type AuthError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Err     error  `json:"-"`
}

// Error implements the error interface
func (e *AuthError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

// Unwrap returns the underlying error
func (e *AuthError) Unwrap() error {
	return e.Err
}

// NewAuthError creates a new authentication error
func NewAuthError(code, message string, err error) *AuthError {
	return &AuthError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// Error codes for API responses
const (
	CodeInvalidCredentials    = "AUTH_001"
	CodeUserNotFound          = "AUTH_002"
	CodeUserDisabled          = "AUTH_003"
	CodeUserLocked            = "AUTH_004"
	CodeTokenExpired          = "AUTH_005"
	CodeTokenInvalid          = "AUTH_006"
	CodeRefreshTokenExpired   = "AUTH_007"
	CodeRefreshTokenInvalid   = "AUTH_008"
	CodeAPIKeyExpired         = "AUTH_009"
	CodeAPIKeyRevoked         = "AUTH_010"
	CodeAPIKeyInvalid         = "AUTH_011"
	CodeMFARequired           = "AUTH_012"
	CodeMFAInvalid            = "AUTH_013"
	CodeOAuthFailed           = "AUTH_014"
	CodePasswordTooWeak       = "AUTH_015"
	CodeEmailNotVerified      = "AUTH_016"
	CodeUnauthorized          = "AUTH_017"
	CodeForbidden             = "AUTH_018"
)

// IsAuthError checks if an error is an AuthError
func IsAuthError(err error) bool {
	var authErr *AuthError
	return errors.As(err, &authErr)
}

// GetErrorCode returns the error code for an error
func GetErrorCode(err error) string {
	var authErr *AuthError
	if errors.As(err, &authErr) {
		return authErr.Code
	}
	return "AUTH_000"
}
