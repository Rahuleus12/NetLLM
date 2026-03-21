package crypto

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Vault errors
var (
	ErrVaultNotConfigured   = errors.New("vault not configured")
	ErrVaultSealed          = errors.New("vault is sealed")
	ErrVaultUnauthorized    = errors.New("vault unauthorized")
	ErrSecretNotFound       = errors.New("secret not found")
	ErrInvalidVaultResponse = errors.New("invalid vault response")
	ErrVaultConnection      = errors.New("vault connection error")
	ErrSecretLeaseExpired   = errors.New("secret lease has expired")
	ErrInvalidToken         = errors.New("invalid vault token")
)

// VaultConfig holds Vault client configuration
type VaultConfig struct {
	// Address is the Vault server address
	Address string `json:"address" mapstructure:"address"`

	// Token is the Vault token for authentication
	Token string `json:"token" mapstructure:"token"`

	// Namespace is the Vault namespace (Enterprise only)
	Namespace string `json:"namespace" mapstructure:"namespace"`

	// RoleID for AppRole authentication
	RoleID string `json:"role_id" mapstructure:"role_id"`

	// SecretID for AppRole authentication
	SecretID string `json:"secret_id" mapstructure:"secret_id"`

	// AppRolePath is the path to the AppRole auth method
	AppRolePath string `json:"approle_path" mapstructure:"approle_path"`

	// KubernetesRole for Kubernetes authentication
	KubernetesRole string `json:"kubernetes_role" mapstructure:"kubernetes_role"`

	// KubernetesPath is the path to the Kubernetes auth method
	KubernetesPath string `json:"kubernetes_path" mapstructure:"kubernetes_path"`

	// TLSCACert is the path to the CA certificate
	TLSCACert string `json:"tls_ca_cert" mapstructure:"tls_ca_cert"`

	// TLSClientCert is the path to the client certificate
	TLSClientCert string `json:"tls_client_cert" mapstructure:"tls_client_cert"`

	// TLSClientKey is the path to the client key
	TLSClientKey string `json:"tls_client_key" mapstructure:"tls_client_key"`

	// TLSSkipVerify skips TLS verification
	TLSSkipVerify bool `json:"tls_skip_verify" mapstructure:"tls_skip_verify"`

	// MaxRetries is the maximum number of retries for API calls
	MaxRetries int `json:"max_retries" mapstructure:"max_retries"`

	// Timeout is the HTTP client timeout
	Timeout time.Duration `json:"timeout" mapstructure:"timeout"`

	// SecretEnginePath is the path to the secrets engine
	SecretEnginePath string `json:"secret_engine_path" mapstructure:"secret_engine_path"`
}

// DefaultVaultConfig returns default Vault configuration
func DefaultVaultConfig() *VaultConfig {
	return &VaultConfig{
		Address:          "http://127.0.0.1:8200",
		AppRolePath:      "approle",
		KubernetesPath:   "kubernetes",
		SecretEnginePath: "secret",
		MaxRetries:       3,
		Timeout:          30 * time.Second,
	}
}

// VaultSecret represents a secret from Vault
type VaultSecret struct {
	Path      string                 `json:"path"`
	Data      map[string]interface{} `json:"data"`
	Metadata  *SecretMetadata        `json:"metadata,omitempty"`
	LeaseID   string                 `json:"lease_id,omitempty"`
	LeaseTime int                    `json:"lease_time,omitempty"`
	Renewable bool                   `json:"renewable,omitempty"`
}

// SecretMetadata contains metadata about a secret
type SecretMetadata struct {
	Version      int       `json:"version"`
	CreatedTime  time.Time `json:"created_time"`
	UpdatedTime  time.Time `json:"updated_time"`
	Destroyed    bool      `json:"destroyed"`
	DeletionTime time.Time `json:"deletion_time,omitempty"`
}

// VaultHealthStatus represents Vault health status
type VaultHealthStatus struct {
	Initialized   bool   `json:"initialized"`
	Sealed        bool   `json:"sealed"`
	Standby       bool   `json:"standby"`
	ServerTime    int64  `json:"server_time"`
	Version       string `json:"version"`
	ClusterName   string `json:"cluster_name,omitempty"`
	ClusterID     string `json:"cluster_id,omitempty"`
	ReplicaCount  int    `json:"replica_count,omitempty"`
}

// VaultClient provides a client for interacting with HashiCorp Vault
type VaultClient struct {
	config     *VaultConfig
	httpClient *http.Client
	token      string
	tokenMu    sync.RWMutex
	mountPath  string
}

// NewVaultClient creates a new Vault client
func NewVaultClient(config *VaultConfig) (*VaultClient, error) {
	if config == nil {
		config = DefaultVaultConfig()
	}

	if config.Address == "" {
		return nil, ErrVaultNotConfigured
	}

	// Create HTTP client with TLS configuration
	tlsConfig := &tls.Config{
		InsecureSkipVerify: config.TLSSkipVerify,
	}

	if config.TLSCACert != "" {
		caPool := x509.NewCertPool()
		// In production, load the CA cert from file
		tlsConfig.RootCAs = caPool
	}

	if config.TLSClientCert != "" && config.TLSClientKey != "" {
		// In production, load client cert from files
	}

	transport := &http.Transport{
		TLSClientConfig:     tlsConfig,
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
		DisableCompression:  false,
	}

	client := &VaultClient{
		config: config,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   config.Timeout,
		},
		mountPath: config.SecretEnginePath,
	}

	// Set initial token if provided
	if config.Token != "" {
		client.token = config.Token
	}

	return client, nil
}

// Authenticate authenticates with Vault using the configured method
func (c *VaultClient) Authenticate(ctx context.Context) error {
	// Try AppRole authentication if configured
	if c.config.RoleID != "" && c.config.SecretID != "" {
		return c.authenticateAppRole(ctx)
	}

	// Try Kubernetes authentication if configured
	if c.config.KubernetesRole != "" {
		return c.authenticateKubernetes(ctx)
	}

	// Use provided token
	if c.config.Token != "" {
		c.tokenMu.Lock()
		c.token = c.config.Token
		c.tokenMu.Unlock()
		return c.validateToken(ctx)
	}

	return ErrVaultNotConfigured
}

// authenticateAppRole authenticates using AppRole
func (c *VaultClient) authenticateAppRole(ctx context.Context) error {
	path := fmt.Sprintf("/v1/auth/%s/login", c.config.AppRolePath)

	data := map[string]interface{}{
		"role_id":   c.config.RoleID,
		"secret_id": c.config.SecretID,
	}

	resp, err := c.doRequest(ctx, "POST", path, data, false)
	if err != nil {
		return fmt.Errorf("approle authentication failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Auth struct {
			ClientToken string `json:"client_token"`
			LeaseTime   int    `json:"lease_duration"`
			Renewable   bool   `json:"renewable"`
		} `json:"auth"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode vault response: %w", err)
	}

	if result.Auth.ClientToken == "" {
		return ErrVaultUnauthorized
	}

	c.tokenMu.Lock()
	c.token = result.Auth.ClientToken
	c.tokenMu.Unlock()

	return nil
}

// authenticateKubernetes authenticates using Kubernetes service account
func (c *VaultClient) authenticateKubernetes(ctx context.Context) error {
	// Read the Kubernetes service account token
	jwt, err := c.readKubernetesJWT()
	if err != nil {
		return fmt.Errorf("failed to read kubernetes jwt: %w", err)
	}

	path := fmt.Sprintf("/v1/auth/%s/login", c.config.KubernetesPath)

	data := map[string]interface{}{
		"role": c.config.KubernetesRole,
		"jwt":  jwt,
	}

	resp, err := c.doRequest(ctx, "POST", path, data, false)
	if err != nil {
		return fmt.Errorf("kubernetes authentication failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Auth struct {
			ClientToken string `json:"client_token"`
			LeaseTime   int    `json:"lease_duration"`
			Renewable   bool   `json:"renewable"`
		} `json:"auth"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode vault response: %w", err)
	}

	if result.Auth.ClientToken == "" {
		return ErrVaultUnauthorized
	}

	c.tokenMu.Lock()
	c.token = result.Auth.ClientToken
	c.tokenMu.Unlock()

	return nil
}

// readKubernetesJWT reads the Kubernetes service account JWT
func (c *VaultClient) readKubernetesJWT() (string, error) {
	// In production, read from /var/run/secrets/kubernetes.io/serviceaccount/token
	return "", errors.New("kubernetes jwt reading not implemented")
}

// validateToken validates the current token
func (c *VaultClient) validateToken(ctx context.Context) error {
	resp, err := c.doRequest(ctx, "GET", "/v1/auth/token/lookup-self", nil, true)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ErrInvalidToken
	}

	return nil
}

// Health returns the health status of the Vault server
func (c *VaultClient) Health(ctx context.Context) (*VaultHealthStatus, error) {
	resp, err := c.doRequest(ctx, "GET", "/v1/sys/health", nil, false)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Vault returns non-200 status codes for health issues
	// 200 = initialized, unsealed, active
	// 429 = unsealed, standby
	// 472 = disaster recovery mode
	// 473 = performance standby
	// 501 = not initialized
	// 503 = sealed

	status := &VaultHealthStatus{}
	if err := json.NewDecoder(resp.Body).Decode(status); err != nil {
		return nil, fmt.Errorf("failed to decode health response: %w", err)
	}

	return status, nil
}

// GetSecret retrieves a secret from Vault
func (c *VaultClient) GetSecret(ctx context.Context, path string) (*VaultSecret, error) {
	fullPath := fmt.Sprintf("/v1/%s/data/%s", c.mountPath, path)

	resp, err := c.doRequest(ctx, "GET", fullPath, nil, true)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrSecretNotFound
	}

	var result struct {
		Data struct {
			Data     map[string]interface{} `json:"data"`
			Metadata struct {
				Version      int       `json:"version"`
				CreatedTime  time.Time `json:"created_time"`
				UpdatedTime  time.Time `json:"updated_time"`
				Destroyed    bool      `json:"destroyed"`
				DeletionTime time.Time `json:"deletion_time"`
			} `json:"metadata"`
		} `json:"data"`
		LeaseID   string `json:"lease_id"`
		LeaseTime int    `json:"lease_duration"`
		Renewable bool   `json:"renewable"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode secret response: %w", err)
	}

	return &VaultSecret{
		Path: path,
		Data: result.Data.Data,
		Metadata: &SecretMetadata{
			Version:      result.Data.Metadata.Version,
			CreatedTime:  result.Data.Metadata.CreatedTime,
			UpdatedTime:  result.Data.Metadata.UpdatedTime,
			Destroyed:    result.Data.Metadata.Destroyed,
			DeletionTime: result.Data.Metadata.DeletionTime,
		},
		LeaseID:   result.LeaseID,
		LeaseTime: result.LeaseTime,
		Renewable: result.Renewable,
	}, nil
}

// GetSecretVersion retrieves a specific version of a secret
func (c *VaultClient) GetSecretVersion(ctx context.Context, path string, version int) (*VaultSecret, error) {
	fullPath := fmt.Sprintf("/v1/%s/data/%s?version=%d", c.mountPath, path, version)

	resp, err := c.doRequest(ctx, "GET", fullPath, nil, true)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrSecretNotFound
	}

	var result struct {
		Data struct {
			Data     map[string]interface{} `json:"data"`
			Metadata struct {
				Version      int       `json:"version"`
				CreatedTime  time.Time `json:"created_time"`
				UpdatedTime  time.Time `json:"updated_time"`
				Destroyed    bool      `json:"destroyed"`
				DeletionTime time.Time `json:"deletion_time"`
			} `json:"metadata"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode secret response: %w", err)
	}

	return &VaultSecret{
		Path: path,
		Data: result.Data.Data,
		Metadata: &SecretMetadata{
			Version:      result.Data.Metadata.Version,
			CreatedTime:  result.Data.Metadata.CreatedTime,
			UpdatedTime:  result.Data.Metadata.UpdatedTime,
			Destroyed:    result.Data.Metadata.Destroyed,
			DeletionTime: result.Data.Metadata.DeletionTime,
		},
	}, nil
}

// SetSecret stores a secret in Vault
func (c *VaultClient) SetSecret(ctx context.Context, path string, data map[string]interface{}) error {
	fullPath := fmt.Sprintf("/v1/%s/data/%s", c.mountPath, path)

	payload := map[string]interface{}{
		"data": data,
	}

	resp, err := c.doRequest(ctx, "POST", fullPath, payload, true)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("failed to store secret: status %d", resp.StatusCode)
	}

	return nil
}

// DeleteSecret deletes a secret from Vault
func (c *VaultClient) DeleteSecret(ctx context.Context, path string) error {
	fullPath := fmt.Sprintf("/v1/%s/data/%s", c.mountPath, path)

	resp, err := c.doRequest(ctx, "DELETE", fullPath, nil, true)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("failed to delete secret: status %d", resp.StatusCode)
	}

	return nil
}

// DestroySecret permanently destroys a secret version
func (c *VaultClient) DestroySecret(ctx context.Context, path string, versions []int) error {
	fullPath := fmt.Sprintf("/v1/%s/destroy/%s", c.mountPath, path)

	payload := map[string]interface{}{
		"versions": versions,
	}

	resp, err := c.doRequest(ctx, "POST", fullPath, payload, true)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("failed to destroy secret: status %d", resp.StatusCode)
	}

	return nil
}

// ListSecrets lists secrets at a path
func (c *VaultClient) ListSecrets(ctx context.Context, path string) ([]string, error) {
	fullPath := fmt.Sprintf("/v1/%s/metadata/%s", c.mountPath, path)

	resp, err := c.doRequest(ctx, "LIST", fullPath, nil, true)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return []string{}, nil
	}

	var result struct {
		Data struct {
			Keys []string `json:"keys"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode list response: %w", err)
	}

	return result.Data.Keys, nil
}

// RenewLease renews a secret lease
func (c *VaultClient) RenewLease(ctx context.Context, leaseID string, increment int) error {
	path := "/v1/sys/leases/renew"

	payload := map[string]interface{}{
		"lease_id":  leaseID,
		"increment": increment,
	}

	resp, err := c.doRequest(ctx, "PUT", path, payload, true)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ErrSecretLeaseExpired
	}

	return nil
}

// RevokeLease revokes a secret lease
func (c *VaultClient) RevokeLease(ctx context.Context, leaseID string) error {
	path := "/v1/sys/leases/revoke"

	payload := map[string]interface{}{
		"lease_id": leaseID,
	}

	resp, err := c.doRequest(ctx, "PUT", path, payload, true)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

// Encrypt encrypts data using Vault's transit secrets engine
func (c *VaultClient) Encrypt(ctx context.Context, keyName string, plaintext []byte) (string, error) {
	path := fmt.Sprintf("/v1/transit/encrypt/%s", keyName)

	payload := map[string]interface{}{
		"plaintext": plaintext,
	}

	resp, err := c.doRequest(ctx, "POST", path, payload, true)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			Ciphertext string `json:"ciphertext"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode encrypt response: %w", err)
	}

	return result.Data.Ciphertext, nil
}

// Decrypt decrypts data using Vault's transit secrets engine
func (c *VaultClient) Decrypt(ctx context.Context, keyName, ciphertext string) ([]byte, error) {
	path := fmt.Sprintf("/v1/transit/decrypt/%s", keyName)

	payload := map[string]interface{}{
		"ciphertext": ciphertext,
	}

	resp, err := c.doRequest(ctx, "POST", path, payload, true)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			Plaintext []byte `json:"plaintext"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode decrypt response: %w", err)
	}

	return result.Data.Plaintext, nil
}

// GenerateRandomBytes generates random bytes using Vault
func (c *VaultClient) GenerateRandomBytes(ctx context.Context, numBytes int) ([]byte, error) {
	path := fmt.Sprintf("/v1/sys/tools/random/%d", numBytes)

	resp, err := c.doRequest(ctx, "GET", path, nil, true)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			RandomBytes []byte `json:"random_bytes"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode random response: %w", err)
	}

	return result.Data.RandomBytes, nil
}

// Hash computes a hash using Vault
func (c *VaultClient) Hash(ctx context.Context, algorithm string, input []byte) (string, error) {
	path := "/v1/sys/tools/hash"

	payload := map[string]interface{}{
		"algorithm": algorithm,
		"input":     input,
	}

	resp, err := c.doRequest(ctx, "POST", path, payload, true)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			Sum string `json:"sum"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode hash response: %w", err)
	}

	return result.Data.Sum, nil
}

// doRequest performs an HTTP request to Vault
func (c *VaultClient) doRequest(ctx context.Context, method, path string, data interface{}, auth bool) (*http.Response, error) {
	var body io.Reader
	if data != nil {
		jsonData, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		body = strings.NewReader(string(jsonData))
	}

	url := c.config.Address + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Add namespace header if configured
	if c.config.Namespace != "" {
		req.Header.Set("X-Vault-Namespace", c.config.Namespace)
	}

	// Add token for authenticated requests
	if auth {
		c.tokenMu.RLock()
		token := c.token
		c.tokenMu.RUnlock()

		if token == "" {
			return nil, ErrVaultUnauthorized
		}
		req.Header.Set("X-Vault-Token", token)
	}

	var lastErr error
	maxRetries := c.config.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	for i := 0; i < maxRetries; i++ {
		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			if isRetryableError(err) {
				time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
				continue
			}
			return nil, fmt.Errorf("vault request failed: %w", err)
		}

		// Handle authentication errors
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
			resp.Body.Close()
			return nil, ErrVaultUnauthorized
		}

		return resp, nil
	}

	return nil, fmt.Errorf("vault request failed after %d retries: %w", maxRetries, lastErr)
}

// isRetryableError checks if an error is retryable
func isRetryableError(err error) bool {
	if urlErr, ok := err.(*url.Error); ok {
		return urlErr.Timeout() || urlErr.Temporary()
	}
	return false
}

// GetToken returns the current token
func (c *VaultClient) GetToken() string {
	c.tokenMu.RLock()
	defer c.tokenMu.RUnlock()
	return c.token
}

// SetToken sets the token
func (c *VaultClient) SetToken(token string) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	c.token = token
}

// RevokeToken revokes the current token
func (c *VaultClient) RevokeToken(ctx context.Context) error {
	resp, err := c.doRequest(ctx, "POST", "/v1/auth/token/revoke-self", nil, true)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	c.tokenMu.Lock()
	c.token = ""
	c.tokenMu.Unlock()

	return nil
}

// Close closes the Vault client
func (c *VaultClient) Close() error {
	// Revoke token on close if needed
	return nil
}

// VaultSecretCache provides a caching layer for Vault secrets
type VaultSecretCache struct {
	client   *VaultClient
	cache    map[string]*cachedSecret
	mu       sync.RWMutex
	ttl      time.Duration
}

type cachedSecret struct {
	secret   *VaultSecret
	cachedAt time.Time
}

// NewVaultSecretCache creates a new secret cache
func NewVaultSecretCache(client *VaultClient, ttl time.Duration) *VaultSecretCache {
	return &VaultSecretCache{
		client: client,
		cache:  make(map[string]*cachedSecret),
		ttl:    ttl,
	}
}

// GetSecret retrieves a secret from cache or Vault
func (c *VaultSecretCache) GetSecret(ctx context.Context, path string) (*VaultSecret, error) {
	// Check cache first
	c.mu.RLock()
	if cached, ok := c.cache[path]; ok {
		if time.Since(cached.cachedAt) < c.ttl {
			c.mu.RUnlock()
			return cached.secret, nil
		}
	}
	c.mu.RUnlock()

	// Fetch from Vault
	secret, err := c.client.GetSecret(ctx, path)
	if err != nil {
		return nil, err
	}

	// Update cache
	c.mu.Lock()
	c.cache[path] = &cachedSecret{
		secret:   secret,
		cachedAt: time.Now(),
	}
	c.mu.Unlock()

	return secret, nil
}

// Invalidate invalidates a cached secret
func (c *VaultSecretCache) Invalidate(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cache, path)
}

// InvalidateAll invalidates all cached secrets
func (c *VaultSecretCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[string]*cachedSecret)
}

// VaultSecretProvider implements SecretProvider using Vault
type VaultSecretProvider struct {
	client *VaultClient
	cache  *VaultSecretCache
}

// NewVaultSecretProvider creates a new Vault secret provider
func NewVaultSecretProvider(config *VaultConfig) (*VaultSecretProvider, error) {
	client, err := NewVaultClient(config)
	if err != nil {
		return nil, err
	}

	return &VaultSecretProvider{
		client: client,
		cache:  NewVaultSecretCache(client, 5*time.Minute),
	}, nil
}

// GetSecret retrieves a secret
func (p *VaultSecretProvider) GetSecret(ctx context.Context, key string) (string, error) {
	secret, err := p.cache.GetSecret(ctx, key)
	if err != nil {
		return "", err
	}

	if value, ok := secret.Data["value"].(string); ok {
		return value, nil
	}

	return "", ErrSecretNotFound
}

// GetSecretMap retrieves a secret as a map
func (p *VaultSecretProvider) GetSecretMap(ctx context.Context, key string) (map[string]interface{}, error) {
	secret, err := p.cache.GetSecret(ctx, key)
	if err != nil {
		return nil, err
	}
	return secret.Data, nil
}

// SetSecret stores a secret
func (p *VaultSecretProvider) SetSecret(ctx context.Context, key, value string) error {
	return p.client.SetSecret(ctx, key, map[string]interface{}{
		"value": value,
	})
}

// DeleteSecret deletes a secret
func (p *VaultSecretProvider) DeleteSecret(ctx context.Context, key string) error {
	return p.client.DeleteSecret(ctx, key)
}

// Close closes the provider
func (p *VaultSecretProvider) Close() error {
	return p.client.Close()
}
