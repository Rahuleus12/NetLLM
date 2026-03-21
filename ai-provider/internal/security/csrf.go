package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
)

// CSRF errors
var (
	ErrCSRFTokenInvalid     = errors.New("csrf token is invalid")
	ErrCSRFTokenMissing     = errors.New("csrf token is missing")
	ErrCSRFTokenExpired     = errors.New("csrf token has expired")
	ErrCSRFTokenMismatch    = errors.New("csrf token mismatch")
	ErrCSRFRefererMismatch  = errors.New("referer header mismatch")
	ErrCSRFOriginMismatch   = errors.New("origin header mismatch")
	ErrCSRFDoubleCookieMismatch = errors.New("double cookie csrf mismatch")
)

// CSRFConfig holds CSRF protection configuration
type CSRFConfig struct {
	// Cookie settings
	CookieName     string
	CookieDomain   string
	CookiePath     string
	CookieSecure   bool
	CookieHTTPOnly bool
	CookieSameSite http.SameSite
	CookieMaxAge   int

	// Header settings
	HeaderName string

	// Form field name
	FormFieldName string

	// Token settings
	TokenLength    int
	TokenLifetime  time.Duration

	// Security settings
	CheckReferer   bool
	CheckOrigin    bool
	TrustedOrigins []string
	DoubleCookie   bool // Enable double submit cookie pattern

	// Safe methods that don't require CSRF protection
	SafeMethods []string
}

// DefaultCSRFConfig returns default CSRF configuration
func DefaultCSRFConfig() *CSRFConfig {
	return &CSRFConfig{
		CookieName:     "_csrf",
		CookiePath:     "/",
		CookieSecure:   true,
		CookieHTTPOnly: true,
		CookieSameSite: http.SameSiteStrictMode,
		CookieMaxAge:   int(24 * time.Hour / time.Second),

		HeaderName:     "X-CSRF-Token",
		FormFieldName:  "_csrf",

		TokenLength:    32,
		TokenLifetime:  24 * time.Hour,

		CheckReferer:   true,
		CheckOrigin:    true,
		TrustedOrigins: []string{},
		DoubleCookie:   true,

		SafeMethods: []string{http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace},
	}
}

// CSRFToken represents a CSRF token with metadata
type CSRFToken struct {
	Value     string
	ExpiresAt time.Time
	CreatedAt time.Time
}

// IsExpired checks if the token has expired
func (t *CSRFToken) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

// CSRFTokenStore defines the interface for CSRF token storage
type CSRFTokenStore interface {
	// Get retrieves a token for a session
	Get(sessionID string) (*CSRFToken, error)
	// Set stores a token for a session
	Set(sessionID string, token *CSRFToken) error
	// Delete removes a token
	Delete(sessionID string) error
	// Exists checks if a token exists
	Exists(sessionID string) (bool, error)
}

// MemoryCSRFTokenStore is an in-memory implementation of CSRFTokenStore
type MemoryCSRFTokenStore struct {
	tokens map[string]*CSRFToken
	mu     sync.RWMutex
}

// NewMemoryCSRFTokenStore creates a new in-memory token store
func NewMemoryCSRFTokenStore() *MemoryCSRFTokenStore {
	return &MemoryCSRFTokenStore{
		tokens: make(map[string]*CSRFToken),
	}
}

// Get retrieves a token for a session
func (s *MemoryCSRFTokenStore) Get(sessionID string) (*CSRFToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	token, exists := s.tokens[sessionID]
	if !exists {
		return nil, ErrCSRFTokenMissing
	}

	if token.IsExpired() {
		delete(s.tokens, sessionID)
		return nil, ErrCSRFTokenExpired
	}

	return token, nil
}

// Set stores a token for a session
func (s *MemoryCSRFTokenStore) Set(sessionID string, token *CSRFToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tokens[sessionID] = token
	return nil
}

// Delete removes a token
func (s *MemoryCSRFTokenStore) Delete(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.tokens, sessionID)
	return nil
}

// Exists checks if a token exists
func (s *MemoryCSRFTokenStore) Exists(sessionID string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	token, exists := s.tokens[sessionID]
	if !exists {
		return false, nil
	}

	return !token.IsExpired(), nil
}

// CSRFProtection provides CSRF protection functionality
type CSRFProtection struct {
	config *CSRFConfig
	store  CSRFTokenStore
}

// NewCSRFProtection creates a new CSRF protection instance
func NewCSRFProtection(config *CSRFConfig, store CSRFTokenStore) *CSRFProtection {
	if config == nil {
		config = DefaultCSRFConfig()
	}
	if store == nil {
		store = NewMemoryCSRFTokenStore()
	}

	return &CSRFProtection{
		config: config,
		store:  store,
	}
}

// GenerateToken generates a new CSRF token
func (cp *CSRFProtection) GenerateToken(sessionID string) (string, error) {
	token, err := cp.generateRandomToken()
	if err != nil {
		return "", err
	}

	csrfToken := &CSRFToken{
		Value:     token,
		ExpiresAt: time.Now().Add(cp.config.TokenLifetime),
		CreatedAt: time.Now(),
	}

	if err := cp.store.Set(sessionID, csrfToken); err != nil {
		return "", err
	}

	return token, nil
}

// generateRandomToken generates a cryptographically secure random token
func (cp *CSRFProtection) generateRandomToken() (string, error) {
	bytes := make([]byte, cp.config.TokenLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// ValidateToken validates a CSRF token
func (cp *CSRFProtection) ValidateToken(sessionID, token string) error {
	if token == "" {
		return ErrCSRFTokenMissing
	}

	storedToken, err := cp.store.Get(sessionID)
	if err != nil {
		return err
	}

	// Use constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare([]byte(storedToken.Value), []byte(token)) != 1 {
		return ErrCSRFTokenMismatch
	}

	return nil
}

// ValidateRequest validates CSRF protection for an HTTP request
func (cp *CSRFProtection) ValidateRequest(r *http.Request, sessionID string) error {
	// Skip safe methods
	if cp.isSafeMethod(r.Method) {
		return nil
	}

	// Get token from header or form
	token := cp.extractToken(r)
	if token == "" {
		return ErrCSRFTokenMissing
	}

	// Check double cookie if enabled
	if cp.config.DoubleCookie {
		cookieToken, err := cp.getCookieToken(r)
		if err != nil {
			return err
		}

		// Compare header/form token with cookie token
		if subtle.ConstantTimeCompare([]byte(cookieToken), []byte(token)) != 1 {
			return ErrCSRFDoubleCookieMismatch
		}
	}

	// Validate against stored token
	if err := cp.ValidateToken(sessionID, token); err != nil {
		return err
	}

	// Check Origin header
	if cp.config.CheckOrigin {
		if err := cp.validateOrigin(r); err != nil {
			return err
		}
	}

	// Check Referer header
	if cp.config.CheckReferer {
		if err := cp.validateReferer(r); err != nil {
			return err
		}
	}

	return nil
}

// extractToken extracts the CSRF token from the request
func (cp *CSRFProtection) extractToken(r *http.Request) string {
	// First, check header
	token := r.Header.Get(cp.config.HeaderName)
	if token != "" {
		return token
	}

	// Check form field (for form submissions)
	if err := r.ParseForm(); err == nil {
		token = r.FormValue(cp.config.FormFieldName)
		if token != "" {
			return token
		}
	}

	// Check multipart form
	if err := r.ParseMultipartForm(32 << 20); err == nil {
		token = r.FormValue(cp.config.FormFieldName)
		if token != "" {
			return token
		}
	}

	return ""
}

// getCookieToken gets the CSRF token from the cookie
func (cp *CSRFProtection) getCookieToken(r *http.Request) (string, error) {
	cookie, err := r.Cookie(cp.config.CookieName)
	if err != nil {
		return "", ErrCSRFTokenMissing
	}
	return cookie.Value, nil
}

// isSafeMethod checks if the HTTP method is safe
func (cp *CSRFProtection) isSafeMethod(method string) bool {
	for _, safeMethod := range cp.config.SafeMethods {
		if strings.EqualFold(method, safeMethod) {
			return true
		}
	}
	return false
}

// validateOrigin validates the Origin header
func (cp *CSRFProtection) validateOrigin(r *http.Request) error {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// Origin might be missing for same-origin requests
		return nil
	}

	// Check against trusted origins
	for _, trusted := range cp.config.TrustedOrigins {
		if origin == trusted {
			return nil
		}
	}

	// Check if origin matches the host
	host := r.Host
	if strings.HasPrefix(origin, "http://") || strings.HasPrefix(origin, "https://") {
		originHost := strings.TrimPrefix(origin, "http://")
		originHost = strings.TrimPrefix(originHost, "https://")
		if originHost == host {
			return nil
		}
	}

	return ErrCSRFOriginMismatch
}

// validateReferer validates the Referer header
func (cp *CSRFProtection) validateReferer(r *http.Request) error {
	referer := r.Header.Get("Referer")
	if referer == "" {
		// Referer might be missing, rely on Origin check
		return nil
	}

	// Check against trusted origins
	for _, trusted := range cp.config.TrustedOrigins {
		if strings.HasPrefix(referer, trusted) {
			return nil
		}
	}

	// Check if referer matches the host
	host := r.Host
	if strings.Contains(referer, "://") {
		// Extract host from URL
		refererPart := strings.SplitN(referer, "://", 2)
		if len(refererPart) == 2 {
			pathPart := strings.SplitN(refererPart[1], "/", 2)
			if pathPart[0] == host {
				return nil
			}
		}
	}

	return ErrCSRFRefererMismatch
}

// SetCookie sets the CSRF token cookie on the response
func (cp *CSRFProtection) SetCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     cp.config.CookieName,
		Value:    token,
		Domain:   cp.config.CookieDomain,
		Path:     cp.config.CookiePath,
		Secure:   cp.config.CookieSecure,
		HttpOnly: cp.config.CookieHTTPOnly,
		SameSite: cp.config.CookieSameSite,
		MaxAge:   cp.config.CookieMaxAge,
	})
}

// ClearCookie clears the CSRF token cookie
func (cp *CSRFProtection) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     cp.config.CookieName,
		Value:    "",
		Domain:   cp.config.CookieDomain,
		Path:     cp.config.CookiePath,
		Secure:   cp.config.CookieSecure,
		HttpOnly: cp.config.CookieHTTPOnly,
		SameSite: cp.config.CookieSameSite,
		MaxAge:   -1,
	})
}

// Middleware returns a middleware function for CSRF protection
func (cp *CSRFProtection) Middleware(sessionIDFunc func(r *http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get session ID
			sessionID := sessionIDFunc(r)
			if sessionID == "" {
				http.Error(w, "Session required for CSRF protection", http.StatusUnauthorized)
				return
			}

			// For safe methods, generate or refresh token
			if cp.isSafeMethod(r.Method) {
				// Check if token exists
				exists, _ := cp.store.Exists(sessionID)
				if !exists {
					token, err := cp.GenerateToken(sessionID)
					if err != nil {
						http.Error(w, "Failed to generate CSRF token", http.StatusInternalServerError)
						return
					}
					cp.SetCookie(w, token)
					w.Header().Set(cp.config.HeaderName, token)
				}
				next.ServeHTTP(w, r)
				return
			}

			// Validate CSRF token for unsafe methods
			if err := cp.ValidateRequest(r, sessionID); err != nil {
				switch err {
				case ErrCSRFTokenMissing:
					http.Error(w, "CSRF token is missing", http.StatusForbidden)
				case ErrCSRFTokenMismatch, ErrCSRFDoubleCookieMismatch:
					http.Error(w, "CSRF token is invalid", http.StatusForbidden)
				case ErrCSRFTokenExpired:
					http.Error(w, "CSRF token has expired", http.StatusForbidden)
				case ErrCSRFOriginMismatch, ErrCSRFRefererMismatch:
					http.Error(w, "Request origin validation failed", http.StatusForbidden)
				default:
					http.Error(w, "CSRF validation failed", http.StatusForbidden)
				}
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// GetToken returns the CSRF token for a session
func (cp *CSRFProtection) GetToken(sessionID string) (string, error) {
	token, err := cp.store.Get(sessionID)
	if err != nil {
		return "", err
	}
	return token.Value, nil
}

// RefreshToken generates a new token for a session
func (cp *CSRFProtection) RefreshToken(sessionID string) (string, error) {
	return cp.GenerateToken(sessionID)
}

// AddTrustedOrigin adds a trusted origin
func (cp *CSRFProtection) AddTrustedOrigin(origin string) {
	cp.config.TrustedOrigins = append(cp.config.TrustedOrigins, origin)
}

// RemoveTrustedOrigin removes a trusted origin
func (cp *CSRFProtection) RemoveTrustedOrigin(origin string) {
	for i, trusted := range cp.config.TrustedOrigins {
		if trusted == origin {
			cp.config.TrustedOrigins = append(
				cp.config.TrustedOrigins[:i],
				cp.config.TrustedOrigins[i+1:]...,
			)
			break
		}
	}
}

// CSRFMiddleware creates a simple CSRF middleware with default configuration
func CSRFMiddleware(sessionIDFunc func(r *http.Request) string) func(http.Handler) http.Handler {
	protection := NewCSRFProtection(nil, nil)
	return protection.Middleware(sessionIDFunc)
}

// CSRFOption is a functional option for CSRF configuration
type CSRFOption func(*CSRFConfig)

// WithCSRFCookieName sets the cookie name
func WithCSRFCookieName(name string) CSRFOption {
	return func(c *CSRFConfig) {
		c.CookieName = name
	}
}

// WithCSRFHeaderName sets the header name
func WithCSRFHeaderName(name string) CSRFOption {
	return func(c *CSRFConfig) {
		c.HeaderName = name
	}
}

// WithCSRFFormFieldName sets the form field name
func WithCSRFFormFieldName(name string) CSRFOption {
	return func(c *CSRFConfig) {
		c.FormFieldName = name
	}
}

// WithCSRFTokenLength sets the token length
func WithCSRFTokenLength(length int) CSRFOption {
	return func(c *CSRFConfig) {
		c.TokenLength = length
	}
}

// WithCSRFTokenLifetime sets the token lifetime
func WithCSRFTokenLifetime(lifetime time.Duration) CSRFOption {
	return func(c *CSRFConfig) {
		c.TokenLifetime = lifetime
		c.CookieMaxAge = int(lifetime / time.Second)
	}
}

// WithCSRFSecureCookie sets whether the cookie should be secure
func WithCSRFSecureCookie(secure bool) CSRFOption {
	return func(c *CSRFConfig) {
		c.CookieSecure = secure
	}
}

// WithCSRFSameSite sets the SameSite attribute
func WithCSRFSameSite(sameSite http.SameSite) CSRFOption {
	return func(c *CSRFConfig) {
		c.CookieSameSite = sameSite
	}
}

// WithCSRFTrustedOrigins sets the trusted origins
func WithCSRFTrustedOrigins(origins []string) CSRFOption {
	return func(c *CSRFConfig) {
		c.TrustedOrigins = origins
	}
}

// WithCSRFDoubleCookie enables or disables double submit cookie
func WithCSRFDoubleCookie(enabled bool) CSRFOption {
	return func(c *CSRFConfig) {
		c.DoubleCookie = enabled
	}
}

// WithCSRFCheckOrigin enables or disables origin checking
func WithCSRFCheckOrigin(enabled bool) CSRFOption {
	return func(c *CSRFConfig) {
		c.CheckOrigin = enabled
	}
}

// WithCSRFCheckReferer enables or disables referer checking
func WithCSRFCheckReferer(enabled bool) CSRFOption {
	return func(c *CSRFConfig) {
		c.CheckReferer = enabled
	}
}

// NewCSRFProtectionWithOptions creates a new CSRF protection with options
func NewCSRFProtectionWithOptions(store CSRFTokenStore, opts ...CSRFOption) *CSRFProtection {
	config := DefaultCSRFConfig()
	for _, opt := range opts {
		opt(config)
	}
	return NewCSRFProtection(config, store)
}

// TokenResponse contains CSRF token information for the client
type TokenResponse struct {
	Token     string `json:"token"`
	HeaderName string `json:"header_name"`
	FieldName  string `json:"field_name"`
}

// GetTokenResponse returns a token response for API clients
func (cp *CSRFProtection) GetTokenResponse(sessionID string) (*TokenResponse, error) {
	token, err := cp.GetToken(sessionID)
	if err != nil {
		// Generate a new token if one doesn't exist
		token, err = cp.GenerateToken(sessionID)
		if err != nil {
			return nil, err
		}
	}

	return &TokenResponse{
		Token:      token,
		HeaderName: cp.config.HeaderName,
		FieldName:  cp.config.FormFieldName,
	}, nil
}

// CleanupExpiredTokens removes expired tokens from the store
func (cp *CSRFProtection) CleanupExpiredTokens() {
	if memoryStore, ok := cp.store.(*MemoryCSRFTokenStore); ok {
		memoryStore.mu.Lock()
		defer memoryStore.mu.Unlock()

		now := time.Now()
		for sessionID, token := range memoryStore.tokens {
			if now.After(token.ExpiresAt) {
				delete(memoryStore.tokens, sessionID)
			}
		}
	}
}

// StartCleanupRoutine starts a background routine to clean up expired tokens
func (cp *CSRFProtection) StartCleanupRoutine(interval time.Duration) chan struct{} {
	stop := make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				cp.CleanupExpiredTokens()
			case <-stop:
				return
			}
		}
	}()

	return stop
}
