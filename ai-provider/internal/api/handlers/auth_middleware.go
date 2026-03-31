package handlers

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// AuthConfig holds configuration for the authentication middleware
type AuthConfig struct {
	// Enabled controls whether authentication is required
	Enabled bool `json:"enabled" yaml:"enabled"`

	// JWTSecret is the secret key used to validate JWT tokens
	JWTSecret string `json:"jwt_secret" yaml:"jwt_secret"`

	// APIKeyHeader is the HTTP header name for API key authentication
	APIKeyHeader string `json:"api_key_header" yaml:"api_key_header"`

	// SkipPaths are URL paths that should skip authentication
	SkipPaths []string `json:"skip_paths" yaml:"skip_paths"`

	// BypassInternal bypasses auth for requests from internal IPs
	BypassInternal bool `json:"bypass_internal" yaml:"bypass_internal"`

	// RateLimitPerKey sets per-key request rate limits (0 = unlimited)
	RateLimitPerKey int `json:"rate_limit_per_key" yaml:"rate_limit_per_key"`
}

// DefaultAuthConfig returns a default authentication configuration
func DefaultAuthConfig() *AuthConfig {
	return &AuthConfig{
		Enabled:       false,
		JWTSecret:     "",
		APIKeyHeader:  "X-API-Key",
		SkipPaths:     []string{"/health", "/ready", "/version", "/ping", "/metrics"},
		BypassInternal: false,
		RateLimitPerKey: 0,
	}
}

// AuthenticatedUser represents an authenticated user/client extracted from auth credentials
type AuthenticatedUser struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"` // "api_key", "jwt", "internal", "anonymous"
	Name       string                 `json:"name,omitempty"`
	Roles      []string               `json:"roles,omitempty"`
	Scopes     []string               `json:"scopes,omitempty"`
	TenantID   string                 `json:"tenant_id,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	AuthoredAt time.Time              `json:"authored_at"`
}

// contextKey is an unexported type for context keys defined in this package
type contextKey string

const (
	// userContextKey is the context key for the authenticated user
	userContextKey contextKey = "authenticated_user"

	// requestIDContextKey is the context key for the request ID
	requestIDContextKey contextKey = "request_id"
)

// APIKeyStore defines the interface for API key validation and lookup.
// Implementations may use a database, file, or in-memory store.
type APIKeyStore interface {
	// Validate checks if an API key is valid and returns the associated user
	Validate(ctx context.Context, apiKey string) (*AuthenticatedUser, error)
}

// JWTValidator defines the interface for JWT token validation.
type JWTValidator interface {
	// ValidateToken validates a JWT token and returns the associated user
	ValidateToken(ctx context.Context, token string) (*AuthenticatedUser, error)
}

// AuthMiddleware provides authentication middleware for HTTP handlers
type AuthMiddleware struct {
	config       *AuthConfig
	apiKeyStore  APIKeyStore
	jwtValidator JWTValidator

	// In-memory API keys for simple setups (key -> user mapping)
	staticKeys map[string]*AuthenticatedUser
	keysMu     sync.RWMutex

	// Rate limiting per key
	rateCounters map[string]*rateCounter
	rateMu       sync.RWMutex
}

// rateCounter tracks request counts for rate limiting
type rateCounter struct {
	count    int
	window   time.Time
}

// NewAuthMiddleware creates a new authentication middleware
func NewAuthMiddleware(config *AuthConfig) *AuthMiddleware {
	if config == nil {
		config = DefaultAuthConfig()
	}
	if config.APIKeyHeader == "" {
		config.APIKeyHeader = "X-API-Key"
	}
	return &AuthMiddleware{
		config:        config,
		staticKeys:    make(map[string]*AuthenticatedUser),
		rateCounters:  make(map[string]*rateCounter),
	}
}

// WithAPIKeyStore sets the API key store for dynamic key validation
func (am *AuthMiddleware) WithAPIKeyStore(store APIKeyStore) *AuthMiddleware {
	am.apiKeyStore = store
	return am
}

// WithJWTValidator sets the JWT validator for token-based authentication
func (am *AuthMiddleware) WithJWTValidator(validator JWTValidator) *AuthMiddleware {
	am.jwtValidator = validator
	return am
}

// RegisterAPIKey adds a static API key for authentication.
// This is useful for development, testing, or simple deployments.
func (am *AuthMiddleware) RegisterAPIKey(key string, user *AuthenticatedUser) {
	am.keysMu.Lock()
	defer am.keysMu.Unlock()
	if user.AuthoredAt.IsZero() {
		user.AuthoredAt = time.Now()
	}
	if user.Type == "" {
		user.Type = "api_key"
	}
	am.staticKeys[key] = user
}

// RevokeAPIKey removes a static API key
func (am *AuthMiddleware) RevokeAPIKey(key string) {
	am.keysMu.Lock()
	defer am.keysMu.Unlock()
	delete(am.staticKeys, key)
}

// Middleware returns the HTTP middleware function for authentication
func (am *AuthMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Generate request ID for tracing
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}
		ctx := context.WithValue(r.Context(), requestIDContextKey, requestID)

		// If auth is disabled, pass through with anonymous user
		if !am.config.Enabled {
			ctx = context.WithValue(ctx, userContextKey, &AuthenticatedUser{
				ID:         "anonymous",
				Type:       "anonymous",
				AuthoredAt: time.Now(),
			})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Skip authentication for whitelisted paths
		for _, path := range am.config.SkipPaths {
			if r.URL.Path == path || strings.HasPrefix(r.URL.Path+"/", path+"/") {
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		// Attempt authentication
		user, err := am.authenticate(r)
		if err != nil {
			log.Printf("[AUTH] Authentication failed for %s %s: %v (request_id: %s)",
				r.Method, r.URL.Path, err, requestID)
			am.sendAuthError(w, err)
			return
		}

		if user == nil {
			log.Printf("[AUTH] No credentials provided for %s %s (request_id: %s)",
				r.Method, r.URL.Path, requestID)
			am.sendAuthError(w, fmt.Errorf("authentication required"))
			return
		}

		// Rate limit check
		if am.config.RateLimitPerKey > 0 {
			if err := am.checkRateLimit(user.ID); err != nil {
				log.Printf("[AUTH] Rate limit exceeded for user %s (request_id: %s)",
					user.ID, requestID)
				am.sendRateLimitError(w)
				return
			}
		}

		// Add user to context
		ctx = context.WithValue(ctx, userContextKey, user)

		log.Printf("[AUTH] User %s (%s) authenticated for %s %s (request_id: %s)",
			user.ID, user.Type, r.Method, r.URL.Path, requestID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// authenticate attempts to authenticate the request using all available methods.
// It tries in order: API key header, Bearer token (JWT), Basic auth (future).
func (am *AuthMiddleware) authenticate(r *http.Request) (*AuthenticatedUser, error) {
	// 1. Try API key from configured header
	if user, err := am.authenticateAPIKey(r); err != nil {
		return nil, err
	} else if user != nil {
		return user, nil
	}

	// 2. Try Bearer token (JWT)
	if user, err := am.authenticateBearerToken(r); err != nil {
		return nil, err
	} else if user != nil {
		return user, nil
	}

	// 3. Try Authorization header with API key
	if user, err := am.authenticateAuthorizationHeader(r); err != nil {
		return nil, err
	} else if user != nil {
		return user, nil
	}

	return nil, fmt.Errorf("no valid credentials provided")
}

// authenticateAPIKey attempts to authenticate using the API key header
func (am *AuthMiddleware) authenticateAPIKey(r *http.Request) (*AuthenticatedUser, error) {
	apiKey := r.Header.Get(am.config.APIKeyHeader)
	if apiKey == "" {
		return nil, nil
	}

	// Try static keys first (fast path)
	am.keysMu.RLock()
	if user, ok := am.staticKeys[apiKey]; ok {
		am.keysMu.RUnlock()
		return user, nil
	}
	am.keysMu.RUnlock()

	// Try dynamic API key store
	if am.apiKeyStore != nil {
		user, err := am.apiKeyStore.Validate(r.Context(), apiKey)
		if err != nil {
			return nil, fmt.Errorf("invalid API key: %w", err)
		}
		return user, nil
	}

	return nil, nil
}

// authenticateBearerToken attempts to authenticate using a JWT Bearer token
func (am *AuthMiddleware) authenticateBearerToken(r *http.Request) (*AuthenticatedUser, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, nil
	}

	// Must be "Bearer <token>"
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return nil, nil
	}

	token := strings.TrimSpace(parts[1])
	if token == "" {
		return nil, nil
	}

	// Use JWT validator if available
	if am.jwtValidator != nil {
		user, err := am.jwtValidator.ValidateToken(r.Context(), token)
		if err != nil {
			return nil, fmt.Errorf("invalid JWT token: %w", err)
		}
		return user, nil
	}

	// Fallback: simple static token validation if no JWT validator is configured
	am.keysMu.RLock()
	if user, ok := am.staticKeys[token]; ok {
		am.keysMu.RUnlock()
		return user, nil
	}
	am.keysMu.RUnlock()

	return nil, nil
}

// authenticateAuthorizationHeader attempts to authenticate using a key in the Authorization header
func (am *AuthMiddleware) authenticateAuthorizationHeader(r *http.Request) (*AuthenticatedUser, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, nil
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "ApiKey") {
		apiKey := strings.TrimSpace(parts[1])
		if apiKey == "" {
			return nil, nil
		}

		am.keysMu.RLock()
		if user, ok := am.staticKeys[apiKey]; ok {
			am.keysMu.RUnlock()
			return user, nil
		}
		am.keysMu.RUnlock()

		if am.apiKeyStore != nil {
			user, err := am.apiKeyStore.Validate(r.Context(), apiKey)
			if err != nil {
				return nil, fmt.Errorf("invalid API key: %w", err)
			}
			return user, nil
		}
	}

	return nil, nil
}

// checkRateLimit performs a simple sliding window rate limit check per user
func (am *AuthMiddleware) checkRateLimit(userID string) error {
	am.rateMu.Lock()
	defer am.rateMu.Unlock()

	counter, exists := am.rateCounters[userID]
	if !exists {
		am.rateCounters[userID] = &rateCounter{
			count:  1,
			window: time.Now(),
		}
		return nil
	}

	// Reset window every minute
	if time.Since(counter.window) > time.Minute {
		counter.count = 1
		counter.window = time.Now()
		return nil
	}

	counter.count++
	if counter.count > am.config.RateLimitPerKey {
		return fmt.Errorf("rate limit exceeded: %d requests per minute", am.config.RateLimitPerKey)
	}

	return nil
}

// sendAuthError sends an authentication error response in OpenAI-compatible format
func (am *AuthMiddleware) sendAuthError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="Netllm API", error="invalid_token", error_description="%s"`, err.Error()))
	w.WriteHeader(http.StatusUnauthorized)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"message": err.Error(),
			"type":    "authentication_error",
			"code":    "401",
		},
	})
}

// sendRateLimitError sends a rate limit error response
func (am *AuthMiddleware) sendRateLimitError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", "60")
	w.WriteHeader(http.StatusTooManyRequests)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"message": fmt.Sprintf("Rate limit exceeded. Maximum %d requests per minute.", am.config.RateLimitPerKey),
			"type":    "rate_limit_error",
			"code":    "429",
		},
	})
}

// CleanupRateLimiters removes stale rate limit counters. Call periodically.
func (am *AuthMiddleware) CleanupRateLimiters() {
	am.rateMu.Lock()
	defer am.rateMu.Unlock()

	cutoff := time.Now().Add(-2 * time.Minute)
	for key, counter := range am.rateCounters {
		if counter.window.Before(cutoff) {
			delete(am.rateCounters, key)
		}
	}
}

// GetAuthenticatedUser extracts the authenticated user from the request context.
// Returns nil if no user is found (auth may be disabled).
func GetAuthenticatedUser(r *http.Request) *AuthenticatedUser {
	if user, ok := r.Context().Value(userContextKey).(*AuthenticatedUser); ok {
		return user
	}
	return nil
}

// GetRequestID extracts the request ID from the request context.
func GetRequestID(r *http.Request) string {
	if id, ok := r.Context().Value(requestIDContextKey).(string); ok {
		return id
	}
	return ""
}

// RequireScope creates middleware that requires a specific scope/permission
func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := GetAuthenticatedUser(r)
			if user == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error": map[string]interface{}{
						"message": "authentication required",
						"type":    "authentication_error",
						"code":    "401",
					},
				})
				return
			}

			hasScope := false
			for _, s := range user.Scopes {
				if subtle.ConstantTimeCompare([]byte(s), []byte(scope)) == 1 || s == "*" {
					hasScope = true
					break
				}
			}

			if !hasScope {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error": map[string]interface{}{
						"message": fmt.Sprintf("insufficient permissions: scope '%s' required", scope),
						"type":    "authorization_error",
						"code":    "403",
					},
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireRole creates middleware that requires a specific role
func RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := GetAuthenticatedUser(r)
			if user == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error": map[string]interface{}{
						"message": "authentication required",
						"type":    "authentication_error",
						"code":    "401",
					},
				})
				return
			}

			hasRole := false
			for _, r := range user.Roles {
				if subtle.ConstantTimeCompare([]byte(r), []byte(role)) == 1 || r == "admin" {
					hasRole = true
					break
				}
			}

			if !hasRole {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error": map[string]interface{}{
						"message": fmt.Sprintf("insufficient permissions: role '%s' required", role),
						"type":    "authorization_error",
						"code":    "403",
					},
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
