package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Session errors
var (
	ErrSessionNotFound      = errors.New("session not found")
	ErrSessionExpired       = errors.New("session expired")
	ErrSessionInvalid       = errors.New("invalid session")
	ErrSessionStoreFailed   = errors.New("failed to store session")
	ErrSessionDeleteFailed  = errors.New("failed to delete session")
)

// Session represents a user session
type Session struct {
	ID           string            `json:"id"`
	UserID       string            `json:"user_id"`
	Email        string            `json:"email"`
	Username     string            `json:"username"`
	Roles        []string          `json:"roles,omitempty"`
	Permissions  []string          `json:"permissions,omitempty"`
	Attributes   map[string]any    `json:"attributes,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	ExpiresAt    time.Time         `json:"expires_at"`
	LastActivity time.Time         `json:"last_activity"`
	IPAddress    string            `json:"ip_address,omitempty"`
	UserAgent    string            `json:"user_agent,omitempty"`
	IsMFA        bool              `json:"is_mfa"`
	Provider     string            `json:"provider,omitempty"` // local, google, github, etc.
}

// IsExpired checks if the session has expired
func (s *Session) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// IsInactive checks if the session has been inactive for too long
func (s *Session) IsInactive(maxInactiveDuration time.Duration) bool {
	return time.Since(s.LastActivity) > maxInactiveDuration
}

// Touch updates the last activity time
func (s *Session) Touch() {
	s.LastActivity = time.Now()
	s.UpdatedAt = time.Now()
}

// HasRole checks if the session has a specific role
func (s *Session) HasRole(role string) bool {
	for _, r := range s.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// HasPermission checks if the session has a specific permission
func (s *Session) HasPermission(permission string) bool {
	for _, p := range s.Permissions {
		if p == permission {
			return true
		}
	}
	return false
}

// ToJSON serializes the session to JSON
func (s *Session) ToJSON() ([]byte, error) {
	return json.Marshal(s)
}

// FromJSON deserializes the session from JSON
func (s *Session) FromJSON(data []byte) error {
	return json.Unmarshal(data, s)
}

// SessionConfig holds session configuration
type SessionConfig struct {
	// Cookie settings
	CookieName     string
	CookieDomain   string
	CookiePath     string
	CookieSecure   bool
	CookieHTTPOnly bool
	CookieSameSite http.SameSite

	// Session settings
	SessionDuration    time.Duration
	MaxInactiveDuration time.Duration
	RefreshThreshold   time.Duration // When to refresh session

	// Security settings
	EnableSessionBinding bool // Bind session to IP/User-Agent
}

// DefaultSessionConfig returns default session configuration
func DefaultSessionConfig() *SessionConfig {
	return &SessionConfig{
		CookieName:          "session_id",
		CookiePath:          "/",
		CookieSecure:        true,
		CookieHTTPOnly:      true,
		CookieSameSite:      http.SameSiteLaxMode,
		SessionDuration:     24 * time.Hour,
		MaxInactiveDuration: 30 * time.Minute,
		RefreshThreshold:    1 * time.Hour,
		EnableSessionBinding: true,
	}
}

// SessionStore defines the interface for session storage
type SessionStore interface {
	// Get retrieves a session by ID
	Get(ctx context.Context, sessionID string) (*Session, error)

	// Set stores a session
	Set(ctx context.Context, session *Session) error

	// Delete removes a session
	Delete(ctx context.Context, sessionID string) error

	// DeleteByUserID removes all sessions for a user
	DeleteByUserID(ctx context.Context, userID string) error

	// Exists checks if a session exists
	Exists(ctx context.Context, sessionID string) (bool, error)

	// Count returns the number of active sessions
	Count(ctx context.Context) (int64, error)

	// Cleanup removes expired sessions
	Cleanup(ctx context.Context) error
}

// MemorySessionStore implements SessionStore using in-memory storage
type MemorySessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	byUser   map[string]map[string]struct{} // userID -> sessionIDs
}

// NewMemorySessionStore creates a new in-memory session store
func NewMemorySessionStore() *MemorySessionStore {
	return &MemorySessionStore{
		sessions: make(map[string]*Session),
		byUser:   make(map[string]map[string]struct{}),
	}
}

// Get retrieves a session by ID
func (s *MemorySessionStore) Get(ctx context.Context, sessionID string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}

	if session.IsExpired() {
		return nil, ErrSessionExpired
	}

	return session, nil
}

// Set stores a session
func (s *MemorySessionStore) Set(ctx context.Context, session *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessions[session.ID] = session

	if _, exists := s.byUser[session.UserID]; !exists {
		s.byUser[session.UserID] = make(map[string]struct{})
	}
	s.byUser[session.UserID][session.ID] = struct{}{}

	return nil
}

// Delete removes a session
func (s *MemorySessionStore) Delete(ctx context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return nil
	}

	delete(s.sessions, sessionID)

	if userSessions, exists := s.byUser[session.UserID]; exists {
		delete(userSessions, sessionID)
		if len(userSessions) == 0 {
			delete(s.byUser, session.UserID)
		}
	}

	return nil
}

// DeleteByUserID removes all sessions for a user
func (s *MemorySessionStore) DeleteByUserID(ctx context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	userSessions, exists := s.byUser[userID]
	if !exists {
		return nil
	}

	for sessionID := range userSessions {
		delete(s.sessions, sessionID)
	}

	delete(s.byUser, userID)

	return nil
}

// Exists checks if a session exists
func (s *MemorySessionStore) Exists(ctx context.Context, sessionID string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return false, nil
	}

	return !session.IsExpired(), nil
}

// Count returns the number of active sessions
func (s *MemorySessionStore) Count(ctx context.Context) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int64
	for _, session := range s.sessions {
		if !session.IsExpired() {
			count++
		}
	}

	return count, nil
}

// Cleanup removes expired sessions
func (s *MemorySessionStore) Cleanup(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for id, session := range s.sessions {
		if now.After(session.ExpiresAt) {
			delete(s.sessions, id)

			if userSessions, exists := s.byUser[session.UserID]; exists {
				delete(userSessions, id)
				if len(userSessions) == 0 {
					delete(s.byUser, session.UserID)
				}
			}
		}
	}

	return nil
}

// SessionManager manages sessions
type SessionManager struct {
	store  SessionStore
	config *SessionConfig
}

// NewSessionManager creates a new session manager
func NewSessionManager(store SessionStore, config *SessionConfig) *SessionManager {
	if config == nil {
		config = DefaultSessionConfig()
	}

	return &SessionManager{
		store:  store,
		config: config,
	}
}

// generateSessionID generates a secure random session ID
func generateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate session ID: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// CreateSession creates a new session for a user
func (m *SessionManager) CreateSession(ctx context.Context, userID, email, username string, opts ...SessionOption) (*Session, error) {
	sessionID, err := generateSessionID()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	session := &Session{
		ID:           sessionID,
		UserID:       userID,
		Email:        email,
		Username:     username,
		Roles:        []string{},
		Permissions:  []string{},
		Attributes:   make(map[string]any),
		CreatedAt:    now,
		UpdatedAt:    now,
		ExpiresAt:    now.Add(m.config.SessionDuration),
		LastActivity: now,
		Provider:     "local",
	}

	// Apply options
	for _, opt := range opts {
		opt(session)
	}

	if err := m.store.Set(ctx, session); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSessionStoreFailed, err)
	}

	return session, nil
}

// GetSession retrieves a session by ID
func (m *SessionManager) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	session, err := m.store.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	if session.IsExpired() {
		_ = m.store.Delete(ctx, sessionID)
		return nil, ErrSessionExpired
	}

	if m.config.MaxInactiveDuration > 0 && session.IsInactive(m.config.MaxInactiveDuration) {
		_ = m.store.Delete(ctx, sessionID)
		return nil, ErrSessionExpired
	}

	return session, nil
}

// ValidateSession validates a session and optionally refreshes it
func (m *SessionManager) ValidateSession(ctx context.Context, sessionID string) (*Session, error) {
	session, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	// Touch the session to update activity
	session.Touch()

	// Refresh session if approaching expiration
	if time.Until(session.ExpiresAt) < m.config.RefreshThreshold {
		session.ExpiresAt = time.Now().Add(m.config.SessionDuration)
	}

	if err := m.store.Set(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to refresh session: %w", err)
	}

	return session, nil
}

// DestroySession destroys a session
func (m *SessionManager) DestroySession(ctx context.Context, sessionID string) error {
	return m.store.Delete(ctx, sessionID)
}

// DestroyUserSessions destroys all sessions for a user
func (m *SessionManager) DestroyUserSessions(ctx context.Context, userID string) error {
	return m.store.DeleteByUserID(ctx, userID)
}

// UpdateSession updates a session
func (m *SessionManager) UpdateSession(ctx context.Context, session *Session) error {
	session.UpdatedAt = time.Now()
	return m.store.Set(ctx, session)
}

// SetSessionCookie sets the session cookie on the response
func (m *SessionManager) SetSessionCookie(w http.ResponseWriter, sessionID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     m.config.CookieName,
		Value:    sessionID,
		Domain:   m.config.CookieDomain,
		Path:     m.config.CookiePath,
		Secure:   m.config.CookieSecure,
		HttpOnly: m.config.CookieHTTPOnly,
		SameSite: m.config.CookieSameSite,
		Expires:  time.Now().Add(m.config.SessionDuration),
	})
}

// ClearSessionCookie clears the session cookie
func (m *SessionManager) ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     m.config.CookieName,
		Value:    "",
		Domain:   m.config.CookieDomain,
		Path:     m.config.CookiePath,
		Secure:   m.config.CookieSecure,
		HttpOnly: m.config.CookieHTTPOnly,
		SameSite: m.config.CookieSameSite,
		MaxAge:   -1,
	})
}

// GetSessionIDFromRequest extracts the session ID from the request
func (m *SessionManager) GetSessionIDFromRequest(r *http.Request) (string, error) {
	cookie, err := r.Cookie(m.config.CookieName)
	if err != nil {
		if errors.Is(err, http.ErrNoCookie) {
			return "", ErrSessionNotFound
		}
		return "", fmt.Errorf("failed to get session cookie: %w", err)
	}

	sessionID, err := url.QueryUnescape(cookie.Value)
	if err != nil {
		return "", ErrSessionInvalid
	}

	return sessionID, nil
}

// GetSessionFromRequest extracts and validates the session from the request
func (m *SessionManager) GetSessionFromRequest(r *http.Request) (*Session, error) {
	sessionID, err := m.GetSessionIDFromRequest(r)
	if err != nil {
		return nil, err
	}

	session, err := m.GetSession(r.Context(), sessionID)
	if err != nil {
		return nil, err
	}

	// Validate session binding if enabled
	if m.config.EnableSessionBinding {
		if session.IPAddress != "" && session.IPAddress != GetClientIP(r) {
			return nil, ErrSessionInvalid
		}
	}

	return session, nil
}

// SessionOption is a function that modifies a session
type SessionOption func(*Session)

// WithRoles sets the roles for the session
func WithRoles(roles []string) SessionOption {
	return func(s *Session) {
		s.Roles = roles
	}
}

// WithPermissions sets the permissions for the session
func WithPermissions(permissions []string) SessionOption {
	return func(s *Session) {
		s.Permissions = permissions
	}
}

// WithProvider sets the authentication provider
func WithProvider(provider string) SessionOption {
	return func(s *Session) {
		s.Provider = provider
	}
}

// WithMFA marks the session as MFA-authenticated
func WithMFA(enabled bool) SessionOption {
	return func(s *Session) {
		s.IsMFA = enabled
	}
}

// WithRequestInfo sets IP and User-Agent from the request
func WithRequestInfo(r *http.Request) SessionOption {
	return func(s *Session) {
		s.IPAddress = GetClientIP(r)
		s.UserAgent = r.UserAgent()
	}
}

// WithAttributes sets additional attributes
func WithAttributes(attrs map[string]any) SessionOption {
	return func(s *Session) {
		for k, v := range attrs {
			s.Attributes[k] = v
		}
	}
}

// WithExpiration sets a custom expiration time
func WithExpiration(expiresAt time.Time) SessionOption {
	return func(s *Session) {
		s.ExpiresAt = expiresAt
	}
}

// GetClientIP extracts the client IP from the request
func GetClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For may contain multiple IPs, the first is the original client
		if idx := len(xff); idx > 0 {
			ips := splitIPs(xff)
			if len(ips) > 0 {
				return ips[0]
			}
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	return r.RemoteAddr
}

// splitIPs splits a comma-separated list of IPs
func splitIPs(s string) []string {
	var ips []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			ip := trimSpace(s[start:i])
			if ip != "" {
				ips = append(ips, ip)
			}
			start = i + 1
		}
	}
	if start < len(s) {
		ip := trimSpace(s[start:])
		if ip != "" {
			ips = append(ips, ip)
		}
	}
	return ips
}

// trimSpace trims whitespace from a string
func trimSpace(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

// CleanupRoutine starts a background goroutine to clean up expired sessions
func (m *SessionManager) CleanupRoutine(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = m.store.Cleanup(ctx)
			}
		}
	}()
}

// GetStore returns the underlying session store
func (m *SessionManager) GetStore() SessionStore {
	return m.store
}

// GetConfig returns the session configuration
func (m *SessionManager) GetConfig() *SessionConfig {
	return m.config
}
