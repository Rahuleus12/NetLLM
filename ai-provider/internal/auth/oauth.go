package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"golang.org/x/oauth2/google"
)

// OAuthProvider represents an OAuth2 provider
type OAuthProvider string

const (
	ProviderGoogle   OAuthProvider = "google"
	ProviderGitHub   OAuthProvider = "github"
	ProviderMicrosoft OAuthProvider = "microsoft"
)

// OAuthConfig holds OAuth2 configuration for a provider
type OAuthConfig struct {
	Provider        OAuthProvider
	ClientID        string
	ClientSecret    string
	RedirectURL     string
	Scopes          []string
	UserInfoURL     string
	AuthURL         string
	TokenURL        string
}

// OAuthUserInfo represents user information from OAuth provider
type OAuthUserInfo struct {
	Provider    OAuthProvider `json:"provider"`
	ProviderID  string        `json:"provider_id"`
	Email       string        `json:"email"`
	Name        string        `json:"name"`
	AvatarURL   string        `json:"avatar_url,omitempty"`
	Username    string        `json:"username,omitempty"`
	Verified    bool          `json:"verified,omitempty"`
	RawData     map[string]interface{} `json:"raw_data,omitempty"`
}

// OAuthState represents an OAuth state for CSRF protection
type OAuthState struct {
	State       string    `json:"state"`
	RedirectURL string    `json:"redirect_url"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// OAuthManager manages OAuth2 authentication
type OAuthManager struct {
	configs      map[OAuthProvider]*oauth2.Config
	userInfoURLs map[OAuthProvider]string
	stateStore   OAuthStateStore
	jwtManager   *JWTManager
	mu           sync.RWMutex
}

// OAuthStateStore defines the interface for OAuth state storage
type OAuthStateStore interface {
	Set(ctx context.Context, state string, oauthState *OAuthState) error
	Get(ctx context.Context, state string) (*OAuthState, error)
	Delete(ctx context.Context, state string) error
}

// MemoryOAuthStateStore is an in-memory implementation of OAuthStateStore
type MemoryOAuthStateStore struct {
	states map[string]*OAuthState
	mu     sync.RWMutex
}

// NewMemoryOAuthStateStore creates a new in-memory state store
func NewMemoryOAuthStateStore() *MemoryOAuthStateStore {
	return &MemoryOAuthStateStore{
		states: make(map[string]*OAuthState),
	}
}

// Set stores an OAuth state
func (s *MemoryOAuthStateStore) Set(ctx context.Context, state string, oauthState *OAuthState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[state] = oauthState
	return nil
}

// Get retrieves an OAuth state
func (s *MemoryOAuthStateStore) Get(ctx context.Context, state string) (*OAuthState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	oauthState, ok := s.states[state]
	if !ok {
		return nil, ErrInvalidOAuthState
	}
	if time.Now().After(oauthState.ExpiresAt) {
		delete(s.states, state)
		return nil, ErrOAuthStateExpired
	}
	return oauthState, nil
}

// Delete removes an OAuth state
func (s *MemoryOAuthStateStore) Delete(ctx context.Context, state string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.states, state)
	return nil
}

// NewOAuthManager creates a new OAuth manager
func NewOAuthManager(jwtManager *JWTManager, stateStore OAuthStateStore) *OAuthManager {
	return &OAuthManager{
		configs:      make(map[OAuthProvider]*oauth2.Config),
		userInfoURLs: make(map[OAuthProvider]string),
		stateStore:   stateStore,
		jwtManager:   jwtManager,
	}
}

// RegisterProvider registers an OAuth2 provider
func (m *OAuthManager) RegisterProvider(config OAuthConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	oauthConfig := &oauth2.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		RedirectURL:  config.RedirectURL,
		Scopes:       config.Scopes,
	}

	// Set provider-specific endpoints
	switch config.Provider {
	case ProviderGoogle:
		oauthConfig.Endpoint = google.Endpoint
		m.userInfoURLs[config.Provider] = "https://www.googleapis.com/oauth2/v3/userinfo"
	case ProviderGitHub:
		oauthConfig.Endpoint = github.Endpoint
		m.userInfoURLs[config.Provider] = "https://api.github.com/user"
	case ProviderMicrosoft:
		oauthConfig.Endpoint = oauth2.Endpoint{
			AuthURL:  "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
			TokenURL: "https://login.microsoftonline.com/common/oauth2/v2.0/token",
		}
		m.userInfoURLs[config.Provider] = "https://graph.microsoft.com/v1.0/me"
	default:
		// Custom provider
		if config.AuthURL != "" && config.TokenURL != "" {
			oauthConfig.Endpoint = oauth2.Endpoint{
				AuthURL:  config.AuthURL,
				TokenURL: config.TokenURL,
			}
		} else {
			return fmt.Errorf("%w: %s", ErrUnsupportedOAuthProvider, config.Provider)
		}
		if config.UserInfoURL != "" {
			m.userInfoURLs[config.Provider] = config.UserInfoURL
		}
	}

	m.configs[config.Provider] = oauthConfig
	return nil
}

// GetAuthURL generates an OAuth2 authorization URL
func (m *OAuthManager) GetAuthURL(ctx context.Context, provider OAuthProvider, redirectURL string) (string, error) {
	m.mu.RLock()
	config, ok := m.configs[provider]
	m.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("%w: %s", ErrUnsupportedOAuthProvider, provider)
	}

	// Generate random state
	state, err := generateOAuthState()
	if err != nil {
		return "", fmt.Errorf("failed to generate state: %w", err)
	}

	// Store state for validation
	oauthState := &OAuthState{
		State:       state,
		RedirectURL: redirectURL,
		CreatedAt:   time.Now(),
		ExpiresAt:   time.Now().Add(10 * time.Minute),
	}

	if err := m.stateStore.Set(ctx, state, oauthState); err != nil {
		return "", fmt.Errorf("failed to store state: %w", err)
	}

	return config.AuthCodeURL(state), nil
}

// HandleCallback handles OAuth2 callback
func (m *OAuthManager) HandleCallback(ctx context.Context, provider OAuthProvider, state, code string) (*OAuthUserInfo, string, error) {
	m.mu.RLock()
	config, ok := m.configs[provider]
	m.mu.RUnlock()

	if !ok {
		return nil, "", fmt.Errorf("%w: %s", ErrUnsupportedOAuthProvider, provider)
	}

	// Validate state
	oauthState, err := m.stateStore.Get(ctx, state)
	if err != nil {
		return nil, "", err
	}
	defer m.stateStore.Delete(ctx, state)

	// Exchange code for token
	token, err := config.Exchange(ctx, code)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrOAuthTokenExchange, err)
	}

	// Get user info
	userInfo, err := m.getUserInfo(ctx, provider, token.AccessToken)
	if err != nil {
		return nil, "", err
	}

	// Generate JWT token for our system
	jwtToken, err := m.jwtManager.GenerateToken(userInfo.ProviderID, userInfo.Email)
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate JWT: %w", err)
	}

	return userInfo, jwtToken, nil
}

// getUserInfo retrieves user information from the OAuth provider
func (m *OAuthManager) getUserInfo(ctx context.Context, provider OAuthProvider, accessToken string) (*OAuthUserInfo, error) {
	m.mu.RLock()
	userInfoURL, ok := m.userInfoURLs[provider]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("%w: no user info URL for provider %s", ErrOAuthUserInfo, provider)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", userInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: status %d, body: %s", ErrOAuthUserInfo, resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return m.parseUserInfo(provider, body)
}

// parseUserInfo parses user information based on the provider
func (m *OAuthManager) parseUserInfo(provider OAuthProvider, body []byte) (*OAuthUserInfo, error) {
	var rawData map[string]interface{}
	if err := json.Unmarshal(body, &rawData); err != nil {
		return nil, fmt.Errorf("failed to parse user info: %w", err)
	}

	userInfo := &OAuthUserInfo{
		Provider: provider,
		RawData:  rawData,
	}

	switch provider {
	case ProviderGoogle:
		return parseGoogleUserInfo(rawData, userInfo)
	case ProviderGitHub:
		return parseGitHubUserInfo(rawData, userInfo)
	case ProviderMicrosoft:
		return parseMicrosoftUserInfo(rawData, userInfo)
	default:
		// Generic parsing
		if email, ok := rawData["email"].(string); ok {
			userInfo.Email = email
		}
		if id, ok := rawData["id"].(string); ok {
			userInfo.ProviderID = id
		} else if id, ok := rawData["sub"].(string); ok {
			userInfo.ProviderID = id
		}
		if name, ok := rawData["name"].(string); ok {
			userInfo.Name = name
		}
	}

	if userInfo.ProviderID == "" {
		return nil, fmt.Errorf("%w: missing provider ID", ErrOAuthUserInfo)
	}

	return userInfo, nil
}

// parseGoogleUserInfo parses Google user info
func parseGoogleUserInfo(rawData map[string]interface{}, userInfo *OAuthUserInfo) (*OAuthUserInfo, error) {
	if sub, ok := rawData["sub"].(string); ok {
		userInfo.ProviderID = sub
	}
	if email, ok := rawData["email"].(string); ok {
		userInfo.Email = email
	}
	if name, ok := rawData["name"].(string); ok {
		userInfo.Name = name
	}
	if picture, ok := rawData["picture"].(string); ok {
		userInfo.AvatarURL = picture
	}
	if verified, ok := rawData["email_verified"].(bool); ok {
		userInfo.Verified = verified
	}
	return userInfo, nil
}

// parseGitHubUserInfo parses GitHub user info
func parseGitHubUserInfo(rawData map[string]interface{}, userInfo *OAuthUserInfo) (*OAuthUserInfo, error) {
	if id, ok := rawData["id"].(float64); ok {
		userInfo.ProviderID = fmt.Sprintf("%d", int64(id))
	}
	if email, ok := rawData["email"].(string); ok {
		userInfo.Email = email
	}
	if name, ok := rawData["name"].(string); ok {
		userInfo.Name = name
	}
	if login, ok := rawData["login"].(string); ok {
		userInfo.Username = login
	}
	if avatar, ok := rawData["avatar_url"].(string); ok {
		userInfo.AvatarURL = avatar
	}
	return userInfo, nil
}

// parseMicrosoftUserInfo parses Microsoft user info
func parseMicrosoftUserInfo(rawData map[string]interface{}, userInfo *OAuthUserInfo) (*OAuthUserInfo, error) {
	if id, ok := rawData["id"].(string); ok {
		userInfo.ProviderID = id
	}
	if email, ok := rawData["mail"].(string); ok {
		userInfo.Email = email
	} else if email, ok := rawData["userPrincipalName"].(string); ok {
		userInfo.Email = email
	}
	if name, ok := rawData["displayName"].(string); ok {
		userInfo.Name = name
	}
	return userInfo, nil
}

// RefreshToken refreshes an OAuth2 token
func (m *OAuthManager) RefreshToken(ctx context.Context, provider OAuthProvider, refreshToken string) (*oauth2.Token, error) {
	m.mu.RLock()
	config, ok := m.configs[provider]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedOAuthProvider, provider)
	}

	token := &oauth2.Token{
		RefreshToken: refreshToken,
	}

	tokenSource := config.TokenSource(ctx, token)
	newToken, err := tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrOAuthTokenRefresh, err)
	}

	return newToken, nil
}

// RevokeToken revokes an OAuth2 token
func (m *OAuthManager) RevokeToken(ctx context.Context, provider OAuthProvider, token string) error {
	var revokeURL string

	switch provider {
	case ProviderGoogle:
		revokeURL = "https://oauth2.googleapis.com/revoke?token=" + url.QueryEscape(token)
	case ProviderGitHub:
		// GitHub doesn't have a revoke endpoint, tokens are managed differently
		return nil
	case ProviderMicrosoft:
		// Microsoft revocation is done through the Microsoft Graph API
		return nil
	default:
		return ErrUnsupportedOAuthProvider
	}

	req, err := http.NewRequestWithContext(ctx, "POST", revokeURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create revoke request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to revoke token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: status %d", ErrOAuthTokenRevoke, resp.StatusCode)
	}

	return nil
}

// GetAvailableProviders returns list of registered providers
func (m *OAuthManager) GetAvailableProviders() []OAuthProvider {
	m.mu.RLock()
	defer m.mu.RUnlock()

	providers := make([]OAuthProvider, 0, len(m.configs))
	for p := range m.configs {
		providers = append(providers, p)
	}
	return providers
}

// generateOAuthState generates a random OAuth state string
func generateOAuthState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// OAuthProviderConfig is a configuration struct for initialization
type OAuthProviderConfig struct {
	Google struct {
		ClientID     string   `json:"client_id" yaml:"client_id"`
		ClientSecret string   `json:"client_secret" yaml:"client_secret"`
		RedirectURL  string   `json:"redirect_url" yaml:"redirect_url"`
		Scopes       []string `json:"scopes" yaml:"scopes"`
		Enabled      bool     `json:"enabled" yaml:"enabled"`
	} `json:"google" yaml:"google"`
	GitHub struct {
		ClientID     string   `json:"client_id" yaml:"client_id"`
		ClientSecret string   `json:"client_secret" yaml:"client_secret"`
		RedirectURL  string   `json:"redirect_url" yaml:"redirect_url"`
		Scopes       []string `json:"scopes" yaml:"scopes"`
		Enabled      bool     `json:"enabled" yaml:"enabled"`
	} `json:"github" yaml:"github"`
	Microsoft struct {
		ClientID     string   `json:"client_id" yaml:"client_id"`
		ClientSecret string   `json:"client_secret" yaml:"client_secret"`
		RedirectURL  string   `json:"redirect_url" yaml:"redirect_url"`
		Scopes       []string `json:"scopes" yaml:"scopes"`
		Enabled      bool     `json:"enabled" yaml:"enabled"`
	} `json:"microsoft" yaml:"microsoft"`
}

// DefaultScopes returns default scopes for each provider
func DefaultScopes(provider OAuthProvider) []string {
	switch provider {
	case ProviderGoogle:
		return []string{"openid", "email", "profile"}
	case ProviderGitHub:
		return []string{"user:email", "read:user"}
	case ProviderMicrosoft:
		return []string{"openid", "email", "profile"}
	default:
		return []string{}
	}
}

// ConfigureFromConfig configures the OAuth manager from a config struct
func (m *OAuthManager) ConfigureFromConfig(config OAuthProviderConfig) error {
	if config.Google.Enabled {
		scopes := config.Google.Scopes
		if len(scopes) == 0 {
			scopes = DefaultScopes(ProviderGoogle)
		}
		if err := m.RegisterProvider(OAuthConfig{
			Provider:     ProviderGoogle,
			ClientID:     config.Google.ClientID,
			ClientSecret: config.Google.ClientSecret,
			RedirectURL:  config.Google.RedirectURL,
			Scopes:       scopes,
		}); err != nil {
			return fmt.Errorf("failed to register Google provider: %w", err)
		}
	}

	if config.GitHub.Enabled {
		scopes := config.GitHub.Scopes
		if len(scopes) == 0 {
			scopes = DefaultScopes(ProviderGitHub)
		}
		if err := m.RegisterProvider(OAuthConfig{
			Provider:     ProviderGitHub,
			ClientID:     config.GitHub.ClientID,
			ClientSecret: config.GitHub.ClientSecret,
			RedirectURL:  config.GitHub.RedirectURL,
			Scopes:       scopes,
		}); err != nil {
			return fmt.Errorf("failed to register GitHub provider: %w", err)
		}
	}

	if config.Microsoft.Enabled {
		scopes := config.Microsoft.Scopes
		if len(scopes) == 0 {
			scopes = DefaultScopes(ProviderMicrosoft)
		}
		if err := m.RegisterProvider(OAuthConfig{
			Provider:     ProviderMicrosoft,
			ClientID:     config.Microsoft.ClientID,
			ClientSecret: config.Microsoft.ClientSecret,
			RedirectURL:  config.Microsoft.RedirectURL,
			Scopes:       scopes,
		}); err != nil {
			return fmt.Errorf("failed to register Microsoft provider: %w", err)
		}
	}

	return nil
}

// ParseProvider parses a string into an OAuthProvider
func ParseProvider(s string) (OAuthProvider, error) {
	switch strings.ToLower(s) {
	case "google":
		return ProviderGoogle, nil
	case "github":
		return ProviderGitHub, nil
	case "microsoft", "azure", "azuread":
		return ProviderMicrosoft, nil
	default:
		return "", fmt.Errorf("%w: %s", ErrUnsupportedOAuthProvider, s)
	}
}

// String returns the string representation of the provider
func (p OAuthProvider) String() string {
	return string(p)
}
