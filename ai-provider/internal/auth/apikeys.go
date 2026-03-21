// Package auth provides authentication and authorization functionality
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// APIKey-related errors
var (
	ErrAPIKeyNotFound      = errors.New("api key not found")
	ErrAPIKeyExpired       = errors.New("api key has expired")
	ErrAPIKeyRevoked       = errors.New("api key has been revoked")
	ErrInvalidAPIKey       = errors.New("invalid api key")
	ErrInvalidScope        = errors.New("invalid scope")
	ErrAPIKeyNameExists    = errors.New("api key with this name already exists")
	ErrAPIKeyLimitExceeded = errors.New("api key limit exceeded")
)

// Scope represents a permission scope for an API key
type Scope string

const (
	// ScopeRead allows read operations
	ScopeRead Scope = "read"
	// ScopeWrite allows write operations
	ScopeWrite Scope = "write"
	// ScopeDelete allows delete operations
	ScopeDelete Scope = "delete"
	// ScopeAdmin allows administrative operations
	ScopeAdmin Scope = "admin"
	// ScopeInference allows model inference operations
	ScopeInference Scope = "inference"
	// ScopeModels allows model management operations
	ScopeModels Scope = "models"
	// ScopeUsers allows user management operations
	ScopeUsers Scope = "users"
	// ScopeAudit allows audit log access
	ScopeAudit Scope = "audit"
	// ScopeBilling allows billing operations
	ScopeBilling Scope = "billing"
)

// ValidScopes contains all valid API key scopes
var ValidScopes = map[Scope]bool{
	ScopeRead:      true,
	ScopeWrite:     true,
	ScopeDelete:    true,
	ScopeAdmin:     true,
	ScopeInference: true,
	ScopeModels:    true,
	ScopeUsers:     true,
	ScopeAudit:     true,
	ScopeBilling:   true,
}

// APIKey represents an API key entity
type APIKey struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	KeyHash     string    `json:"-"` // Never expose the hash
	Name        string    `json:"name"`
	Scopes      []Scope   `json:"scopes"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
	RevokedBy   string    `json:"revoked_by,omitempty"`
	Description string    `json:"description,omitempty"`
}

// APIKeyCreateRequest represents the request to create an API key
type APIKeyCreateRequest struct {
	UserID      string   `json:"user_id"`
	Name        string   `json:"name"`
	Scopes      []Scope  `json:"scopes"`
	ExpiresIn   *int64   `json:"expires_in,omitempty"` // Duration in seconds
	Description string   `json:"description,omitempty"`
}

// APIKeyCreateResponse contains the created API key and the raw key (only shown once)
type APIKeyCreateResponse struct {
	APIKey   *APIKey `json:"api_key"`
	RawKey   string  `json:"raw_key"` // Only shown once during creation
	KeyPrefix string `json:"key_prefix"`
}

// APIKeyRepository defines the interface for API key storage
type APIKeyRepository interface {
	Create(ctx context.Context, apiKey *APIKey) error
	GetByID(ctx context.Context, id string) (*APIKey, error)
	GetByKeyHash(ctx context.Context, keyHash string) (*APIKey, error)
	GetByUserID(ctx context.Context, userID string) ([]*APIKey, error)
	List(ctx context.Context, limit, offset int) ([]*APIKey, error)
	Update(ctx context.Context, apiKey *APIKey) error
	Delete(ctx context.Context, id string) error
	GetByName(ctx context.Context, userID, name string) (*APIKey, error)
}

// APIKeyService provides API key management functionality
type APIKeyService struct {
	repo        APIKeyRepository
	keyPrefix   string
	maxKeysPerUser int
}

// NewAPIKeyService creates a new API key service
func NewAPIKeyService(repo APIKeyRepository, opts ...APIKeyServiceOption) *APIKeyService {
	svc := &APIKeyService{
		repo:           repo,
		keyPrefix:      "netllm", // Default prefix
		maxKeysPerUser: 10,       // Default limit
	}

	for _, opt := range opts {
		opt(svc)
	}

	return svc
}

// APIKeyServiceOption is a functional option for APIKeyService
type APIKeyServiceOption func(*APIKeyService)

// WithKeyPrefix sets a custom key prefix
func WithKeyPrefix(prefix string) APIKeyServiceOption {
	return func(s *APIKeyService) {
		s.keyPrefix = prefix
	}
}

// WithMaxKeysPerUser sets the maximum keys per user
func WithMaxKeysPerUser(max int) APIKeyServiceOption {
	return func(s *APIKeyService) {
		s.maxKeysPerUser = max
	}
}

// generateAPIKey generates a new random API key
func generateAPIKey() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// hashAPIKey creates a SHA-256 hash of the API key
func hashAPIKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

// CreateAPIKey creates a new API key for a user
func (s *APIKeyService) CreateAPIKey(ctx context.Context, req *APIKeyCreateRequest) (*APIKeyCreateResponse, error) {
	// Validate scopes
	for _, scope := range req.Scopes {
		if !ValidScopes[scope] {
			return nil, fmt.Errorf("%w: %s", ErrInvalidScope, scope)
		}
	}

	// Check if name already exists for this user
	existing, err := s.repo.GetByName(ctx, req.UserID, req.Name)
	if err == nil && existing != nil {
		return nil, ErrAPIKeyNameExists
	}

	// Check if user has exceeded their API key limit
	userKeys, err := s.repo.GetByUserID(ctx, req.UserID)
	if err == nil && len(userKeys) >= s.maxKeysPerUser {
		return nil, ErrAPIKeyLimitExceeded
	}

	// Generate the raw API key
	rawKey, err := generateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate API key: %w", err)
	}

	// Create the full key with prefix
	fullKey := fmt.Sprintf("%s_%s", s.keyPrefix, rawKey)
	keyHash := hashAPIKey(fullKey)

	// Calculate expiration
	var expiresAt *time.Time
	if req.ExpiresIn != nil && *req.ExpiresIn > 0 {
		exp := time.Now().Add(time.Duration(*req.ExpiresIn) * time.Second)
		expiresAt = &exp
	}

	apiKey := &APIKey{
		ID:          uuid.New().String(),
		UserID:      req.UserID,
		KeyHash:     keyHash,
		Name:        req.Name,
		Scopes:      req.Scopes,
		ExpiresAt:   expiresAt,
		CreatedAt:   time.Now(),
		Description: req.Description,
	}

	if err := s.repo.Create(ctx, apiKey); err != nil {
		return nil, fmt.Errorf("failed to create API key: %w", err)
	}

	return &APIKeyCreateResponse{
		APIKey:    apiKey,
		RawKey:    fullKey,
		KeyPrefix: s.keyPrefix + "_" + apiKey.ID[:8],
	}, nil
}

// ValidateAPIKey validates an API key and returns the associated API key info
func (s *APIKeyService) ValidateAPIKey(ctx context.Context, key string) (*APIKey, error) {
	// Hash the provided key
	keyHash := hashAPIKey(key)

	// Look up the key
	apiKey, err := s.repo.GetByKeyHash(ctx, keyHash)
	if err != nil {
		return nil, ErrAPIKeyNotFound
	}

	// Check if revoked
	if apiKey.RevokedAt != nil {
		return nil, ErrAPIKeyRevoked
	}

	// Check expiration
	if apiKey.ExpiresAt != nil && time.Now().After(*apiKey.ExpiresAt) {
		return nil, ErrAPIKeyExpired
	}

	// Update last used timestamp
	now := time.Now()
	apiKey.LastUsedAt = &now
	if err := s.repo.Update(ctx, apiKey); err != nil {
		// Log error but don't fail validation
		// In production, use proper logging
	}

	return apiKey, nil
}

// RevokeAPIKey revokes an API key
func (s *APIKeyService) RevokeAPIKey(ctx context.Context, id, revokedBy string) error {
	apiKey, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return ErrAPIKeyNotFound
	}

	if apiKey.RevokedAt != nil {
		return ErrAPIKeyRevoked
	}

	now := time.Now()
	apiKey.RevokedAt = &now
	apiKey.RevokedBy = revokedBy

	if err := s.repo.Update(ctx, apiKey); err != nil {
		return fmt.Errorf("failed to revoke API key: %w", err)
	}

	return nil
}

// GetAPIKey retrieves an API key by ID
func (s *APIKeyService) GetAPIKey(ctx context.Context, id string) (*APIKey, error) {
	return s.repo.GetByID(ctx, id)
}

// ListAPIKeysByUser lists all API keys for a user
func (s *APIKeyService) ListAPIKeysByUser(ctx context.Context, userID string) ([]*APIKey, error) {
	return s.repo.GetByUserID(ctx, userID)
}

// ListAPIKeys lists all API keys with pagination
func (s *APIKeyService) ListAPIKeys(ctx context.Context, limit, offset int) ([]*APIKey, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	return s.repo.List(ctx, limit, offset)
}

// DeleteAPIKey permanently deletes an API key
func (s *APIKeyService) DeleteAPIKey(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

// HasScope checks if an API key has a specific scope
func (k *APIKey) HasScope(scope Scope) bool {
	for _, s := range k.Scopes {
		if s == scope || s == ScopeAdmin {
			return true
		}
	}
	return false
}

// HasAnyScope checks if an API key has any of the specified scopes
func (k *APIKey) HasAnyScope(scopes ...Scope) bool {
	for _, scope := range scopes {
		if k.HasScope(scope) {
			return true
		}
	}
	return false
}

// HasAllScopes checks if an API key has all of the specified scopes
func (k *APIKey) HasAllScopes(scopes ...Scope) bool {
	for _, scope := range scopes {
		if !k.HasScope(scope) {
			return false
		}
	}
	return true
}

// IsExpired checks if the API key is expired
func (k *APIKey) IsExpired() bool {
	if k.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*k.ExpiresAt)
}

// IsRevoked checks if the API key is revoked
func (k *APIKey) IsRevoked() bool {
	return k.RevokedAt != nil
}

// IsValid checks if the API key is valid (not expired, not revoked)
func (k *APIKey) IsValid() bool {
	return !k.IsExpired() && !k.IsRevoked()
}

// GetKeyPrefix extracts the prefix from a full API key
func GetKeyPrefix(key string) string {
	parts := strings.SplitN(key, "_", 2)
	if len(parts) < 2 {
		return ""
	}
	return parts[0]
}

// MaskKey returns a masked version of the API key for display
func MaskKey(key string) string {
	if len(key) <= 12 {
		return "****"
	}
	return key[:8] + "****" + key[len(key)-4:]
}
