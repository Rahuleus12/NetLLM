package crypto

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

// Secrets errors
var (
	ErrSecretNotFound      = errors.New("secret not found")
	ErrSecretExpired       = errors.New("secret has expired")
	ErrSecretDecryption    = errors.New("failed to decrypt secret")
	ErrSecretEncryption    = errors.New("failed to encrypt secret")
	ErrInvalidSecretKey    = errors.New("invalid secret key")
	ErrSecretKeyTooShort   = errors.New("secret key too short")
	ErrSecretVersionNotFound = errors.New("secret version not found")
	ErrSecretAccessDenied  = errors.New("secret access denied")
)

// SecretType represents the type of secret
type SecretType string

const (
	SecretTypePassword   SecretType = "password"
	SecretTypeAPIKey     SecretType = "api_key"
	SecretTypeToken      SecretType = "token"
	SecretTypeCertificate SecretType = "certificate"
	SecretTypeKey        SecretType = "key"
	SecretTypeConnection  SecretType = "connection_string"
	SecretTypeGeneric    SecretType = "generic"
)

// Secret represents a stored secret
type Secret struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Type        SecretType        `json:"type"`
	Value       string            `json:"value,omitempty"` // Encrypted value
	Version     int               `json:"version"`
	Tags        []string          `json:"tags,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
	CreatedBy   string            `json:"created_by"`
	LastAccessed *time.Time       `json:"last_accessed,omitempty"`
	RotationPolicy *RotationPolicy `json:"rotation_policy,omitempty"`
}

// IsExpired checks if the secret has expired
func (s *Secret) IsExpired() bool {
	if s.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*s.ExpiresAt)
}

// SecretVersion represents a specific version of a secret
type SecretVersion struct {
	ID        string    `json:"id"`
	SecretID  string    `json:"secret_id"`
	Version   int       `json:"version"`
	Value     string    `json:"value"` // Encrypted value
	CreatedAt time.Time `json:"created_at"`
	CreatedBy string    `json:"created_by"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Active    bool      `json:"active"`
}

// RotationPolicy defines how a secret should be rotated
type RotationPolicy struct {
	Enabled       bool          `json:"enabled"`
	Interval      time.Duration `json:"interval"`
	MaxVersions   int           `json:"max_versions"`
	AutoRotate    bool          `json:"auto_rotate"`
	LastRotated   *time.Time    `json:"last_rotated,omitempty"`
	NextRotation  *time.Time    `json:"next_rotation,omitempty"`
}

// SecretAccessLog represents an access log entry for a secret
type SecretAccessLog struct {
	ID        string    `json:"id"`
	SecretID  string    `json:"secret_id"`
	UserID    string    `json:"user_id"`
	Action    string    `json:"action"` // read, write, delete, rotate
	Timestamp time.Time `json:"timestamp"`
	IPAddress string    `json:"ip_address,omitempty"`
	Success   bool      `json:"success"`
}

// SecretStore defines the interface for secret storage
type SecretStore interface {
	// Secret operations
	CreateSecret(ctx context.Context, secret *Secret) error
	GetSecret(ctx context.Context, id string) (*Secret, error)
	GetSecretByName(ctx context.Context, name string) (*Secret, error)
	UpdateSecret(ctx context.Context, secret *Secret) error
	DeleteSecret(ctx context.Context, id string) error
	ListSecrets(ctx context.Context, filter SecretFilter) ([]*Secret, error)

	// Version operations
	GetSecretVersion(ctx context.Context, secretID string, version int) (*SecretVersion, error)
	ListSecretVersions(ctx context.Context, secretID string) ([]*SecretVersion, error)

	// Access logs
	LogAccess(ctx context.Context, log *SecretAccessLog) error
	GetAccessLogs(ctx context.Context, secretID string, limit int) ([]*SecretAccessLog, error)
}

// SecretFilter defines filters for listing secrets
type SecretFilter struct {
	Type     SecretType `json:"type,omitempty"`
	Tags     []string   `json:"tags,omitempty"`
	Name     string     `json:"name,omitempty"`
	Expiered *bool      `json:"expired,omitempty"`
}

// MemorySecretStore implements SecretStore using in-memory storage
type MemorySecretStore struct {
	mu         sync.RWMutex
	secrets    map[string]*Secret
	byName     map[string]string // name -> id
	versions   map[string][]*SecretVersion // secretID -> versions
	accessLogs map[string][]*SecretAccessLog // secretID -> logs
}

// NewMemorySecretStore creates a new in-memory secret store
func NewMemorySecretStore() *MemorySecretStore {
	return &MemorySecretStore{
		secrets:    make(map[string]*Secret),
		byName:     make(map[string]string),
		versions:   make(map[string][]*SecretVersion),
		accessLogs: make(map[string][]*SecretAccessLog),
	}
}

// CreateSecret creates a new secret
func (s *MemorySecretStore) CreateSecret(ctx context.Context, secret *Secret) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.byName[secret.Name]; exists {
		return fmt.Errorf("secret with name %s already exists", secret.Name)
	}

	s.secrets[secret.ID] = secret
	s.byName[secret.Name] = secret.ID
	s.versions[secret.ID] = []*SecretVersion{}

	return nil
}

// GetSecret retrieves a secret by ID
func (s *MemorySecretStore) GetSecret(ctx context.Context, id string) (*Secret, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	secret, exists := s.secrets[id]
	if !exists {
		return nil, ErrSecretNotFound
	}

	return secret, nil
}

// GetSecretByName retrieves a secret by name
func (s *MemorySecretStore) GetSecretByName(ctx context.Context, name string) (*Secret, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, exists := s.byName[name]
	if !exists {
		return nil, ErrSecretNotFound
	}

	return s.secrets[id], nil
}

// UpdateSecret updates a secret
func (s *MemorySecretStore) UpdateSecret(ctx context.Context, secret *Secret) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.secrets[secret.ID]; !exists {
		return ErrSecretNotFound
	}

	// Check if name is being changed
	existing := s.secrets[secret.ID]
	if existing.Name != secret.Name {
		if _, exists := s.byName[secret.Name]; exists {
			return fmt.Errorf("secret with name %s already exists", secret.Name)
		}
		delete(s.byName, existing.Name)
		s.byName[secret.Name] = secret.ID
	}

	secret.UpdatedAt = time.Now()
	s.secrets[secret.ID] = secret

	return nil
}

// DeleteSecret deletes a secret
func (s *MemorySecretStore) DeleteSecret(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	secret, exists := s.secrets[id]
	if !exists {
		return nil
	}

	delete(s.secrets, id)
	delete(s.byName, secret.Name)
	delete(s.versions, id)
	delete(s.accessLogs, id)

	return nil
}

// ListSecrets lists secrets based on filter
func (s *MemorySecretStore) ListSecrets(ctx context.Context, filter SecretFilter) ([]*Secret, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*Secret

	for _, secret := range s.secrets {
		if filter.Type != "" && secret.Type != filter.Type {
			continue
		}

		if filter.Name != "" && secret.Name != filter.Name {
			continue
		}

		if filter.Expiered != nil {
			expired := secret.IsExpired()
			if *filter.Expiered != expired {
				continue
			}
		}

		if len(filter.Tags) > 0 {
			matched := false
			for _, tag := range filter.Tags {
				for _, sTag := range secret.Tags {
					if tag == sTag {
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
			if !matched {
				continue
			}
		}

		// Create a copy without the value
		secretCopy := *secret
		secretCopy.Value = ""
		results = append(results, &secretCopy)
	}

	return results, nil
}

// GetSecretVersion retrieves a specific version of a secret
func (s *MemorySecretStore) GetSecretVersion(ctx context.Context, secretID string, version int) (*SecretVersion, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	versions, exists := s.versions[secretID]
	if !exists {
		return nil, ErrSecretNotFound
	}

	for _, v := range versions {
		if v.Version == version {
			return v, nil
		}
	}

	return nil, ErrSecretVersionNotFound
}

// ListSecretVersions lists all versions of a secret
func (s *MemorySecretStore) ListSecretVersions(ctx context.Context, secretID string) ([]*SecretVersion, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	versions, exists := s.versions[secretID]
	if !exists {
		return nil, ErrSecretNotFound
	}

	return versions, nil
}

// AddSecretVersion adds a new version of a secret
func (s *MemorySecretStore) AddSecretVersion(ctx context.Context, version *SecretVersion) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.versions[version.SecretID] = append(s.versions[version.SecretID], version)

	return nil
}

// LogAccess logs access to a secret
func (s *MemorySecretStore) LogAccess(ctx context.Context, log *SecretAccessLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.accessLogs[log.SecretID] = append(s.accessLogs[log.SecretID], log)

	return nil
}

// GetAccessLogs retrieves access logs for a secret
func (s *MemorySecretStore) GetAccessLogs(ctx context.Context, secretID string, limit int) ([]*SecretAccessLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	logs, exists := s.accessLogs[secretID]
	if !exists {
		return []*SecretAccessLog{}, nil
	}

	if limit > 0 && len(logs) > limit {
		return logs[len(logs)-limit:], nil
	}

	return logs, nil
}

// SecretsManager handles secrets management
type SecretsManager struct {
	store        SecretStore
	encryptor    Encryptor
	keyProvider  KeyProvider
	auditLogger  AuditLogger
}

// Encryptor defines the interface for encryption
type Encryptor interface {
	Encrypt(plaintext []byte, key []byte) ([]byte, error)
	Decrypt(ciphertext []byte, key []byte) ([]byte, error)
}

// KeyProvider provides encryption keys
type KeyProvider interface {
	GetKey(ctx context.Context, keyID string) ([]byte, error)
	GetCurrentKey(ctx context.Context) ([]byte, error)
	RotateKey(ctx context.Context) ([]byte, error)
}

// AuditLogger logs secret access for audit purposes
type AuditLogger interface {
	LogSecretAccess(ctx context.Context, secretID, userID, action string, success bool)
}

// NewSecretsManager creates a new secrets manager
func NewSecretsManager(store SecretStore, encryptor Encryptor, keyProvider KeyProvider) *SecretsManager {
	return &SecretsManager{
		store:       store,
		encryptor:   encryptor,
		keyProvider: keyProvider,
	}
}

// SetAuditLogger sets the audit logger
func (m *SecretsManager) SetAuditLogger(logger AuditLogger) {
	m.auditLogger = logger
}

// CreateSecret creates a new secret
func (m *SecretsManager) CreateSecret(ctx context.Context, req *CreateSecretRequest) (*Secret, error) {
	// Get encryption key
	key, err := m.keyProvider.GetCurrentKey(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get encryption key: %w", err)
	}

	// Encrypt the secret value
	encryptedValue, err := m.encryptor.Encrypt([]byte(req.Value), key)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSecretEncryption, err)
	}

	now := time.Now()
	secret := &Secret{
		ID:          generateSecretID(),
		Name:        req.Name,
		Description: req.Description,
		Type:        req.Type,
		Value:       base64.StdEncoding.EncodeToString(encryptedValue),
		Version:     1,
		Tags:        req.Tags,
		Metadata:    req.Metadata,
		CreatedAt:   now,
		UpdatedAt:   now,
		ExpiresAt:   req.ExpiresAt,
		CreatedBy:   req.CreatedBy,
	}

	if req.RotationPolicy != nil {
		secret.RotationPolicy = req.RotationPolicy
		if req.RotationPolicy.Enabled {
			nextRotation := now.Add(req.RotationPolicy.Interval)
			secret.RotationPolicy.NextRotation = &nextRotation
		}
	}

	if err := m.store.CreateSecret(ctx, secret); err != nil {
		return nil, err
	}

	// Create initial version
	version := &SecretVersion{
		ID:        generateSecretID(),
		SecretID:  secret.ID,
		Version:   1,
		Value:     secret.Value,
		CreatedAt: now,
		CreatedBy: req.CreatedBy,
		ExpiresAt: req.ExpiresAt,
		Active:    true,
	}

	if ms, ok := m.store.(*MemorySecretStore); ok {
		ms.AddSecretVersion(ctx, version)
	}

	// Log access
	m.logAccess(ctx, secret.ID, req.CreatedBy, "create", true)

	// Return secret without value
	secret.Value = ""
	return secret, nil
}

// CreateSecretRequest represents a request to create a secret
type CreateSecretRequest struct {
	Name           string            `json:"name"`
	Description    string            `json:"description,omitempty"`
	Type           SecretType        `json:"type"`
	Value          string            `json:"value"`
	Tags           []string          `json:"tags,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	ExpiresAt      *time.Time        `json:"expires_at,omitempty"`
	CreatedBy      string            `json:"created_by"`
	RotationPolicy *RotationPolicy   `json:"rotation_policy,omitempty"`
}

// GetSecret retrieves and decrypts a secret
func (m *SecretsManager) GetSecret(ctx context.Context, id, userID string) (*Secret, string, error) {
	secret, err := m.store.GetSecret(ctx, id)
	if err != nil {
		m.logAccess(ctx, id, userID, "read", false)
		return nil, "", err
	}

	if secret.IsExpired() {
		m.logAccess(ctx, id, userID, "read", false)
		return nil, "", ErrSecretExpired
	}

	// Get encryption key
	key, err := m.keyProvider.GetCurrentKey(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get encryption key: %w", err)
	}

	// Decrypt the secret value
	encryptedValue, err := base64.StdEncoding.DecodeString(secret.Value)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrSecretDecryption, err)
	}

	decryptedValue, err := m.encryptor.Decrypt(encryptedValue, key)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrSecretDecryption, err)
	}

	// Update last accessed time
	now := time.Now()
	secret.LastAccessed = &now
	_ = m.store.UpdateSecret(ctx, secret)

	// Log access
	m.logAccess(ctx, id, userID, "read", true)

	return secret, string(decryptedValue), nil
}

// GetSecretByName retrieves a secret by name
func (m *SecretsManager) GetSecretByName(ctx context.Context, name, userID string) (*Secret, string, error) {
	secret, err := m.store.GetSecretByName(ctx, name)
	if err != nil {
		return nil, "", err
	}

	return m.GetSecret(ctx, secret.ID, userID)
}

// UpdateSecret updates a secret's value
func (m *SecretsManager) UpdateSecret(ctx context.Context, id, value, userID string) (*Secret, error) {
	secret, err := m.store.GetSecret(ctx, id)
	if err != nil {
		return nil, err
	}

	// Get encryption key
	key, err := m.keyProvider.GetCurrentKey(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get encryption key: %w", err)
	}

	// Encrypt the new value
	encryptedValue, err := m.encryptor.Encrypt([]byte(value), key)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSecretEncryption, err)
	}

	// Update secret
	secret.Value = base64.StdEncoding.EncodeToString(encryptedValue)
	secret.Version++
	secret.UpdatedAt = time.Now()

	if err := m.store.UpdateSecret(ctx, secret); err != nil {
		return nil, err
	}

	// Create new version
	version := &SecretVersion{
		ID:        generateSecretID(),
		SecretID:  secret.ID,
		Version:   secret.Version,
		Value:     secret.Value,
		CreatedAt: time.Now(),
		CreatedBy: userID,
		Active:    true,
	}

	if ms, ok := m.store.(*MemorySecretStore); ok {
		ms.AddSecretVersion(ctx, version)
	}

	// Log access
	m.logAccess(ctx, id, userID, "update", true)

	// Return secret without value
	secret.Value = ""
	return secret, nil
}

// RotateSecret rotates a secret's value
func (m *SecretsManager) RotateSecret(ctx context.Context, id, newValue, userID string) (*Secret, error) {
	secret, err := m.store.GetSecret(ctx, id)
	if err != nil {
		return nil, err
	}

	// Update the secret
	updatedSecret, err := m.UpdateSecret(ctx, id, newValue, userID)
	if err != nil {
		return nil, err
	}

	// Update rotation policy
	if secret.RotationPolicy != nil && secret.RotationPolicy.Enabled {
		now := time.Now()
		secret.RotationPolicy.LastRotated = &now
		nextRotation := now.Add(secret.RotationPolicy.Interval)
		secret.RotationPolicy.NextRotation = &nextRotation
		_ = m.store.UpdateSecret(ctx, secret)
	}

	// Log access
	m.logAccess(ctx, id, userID, "rotate", true)

	return updatedSecret, nil
}

// DeleteSecret deletes a secret
func (m *SecretsManager) DeleteSecret(ctx context.Context, id, userID string) error {
	// Log access before deletion
	m.logAccess(ctx, id, userID, "delete", true)

	return m.store.DeleteSecret(ctx, id)
}

// ListSecrets lists all secrets (without values)
func (m *SecretsManager) ListSecrets(ctx context.Context, filter SecretFilter) ([]*Secret, error) {
	return m.store.ListSecrets(ctx, filter)
}

// GetSecretVersion retrieves a specific version of a secret
func (m *SecretsManager) GetSecretVersion(ctx context.Context, secretID string, version int, userID string) (*SecretVersion, string, error) {
	secret, err := m.store.GetSecret(ctx, secretID)
	if err != nil {
		return nil, "", err
	}

	v, err := m.store.GetSecretVersion(ctx, secretID, version)
	if err != nil {
		return nil, "", err
	}

	// Get encryption key
	key, err := m.keyProvider.GetCurrentKey(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get encryption key: %w", err)
	}

	// Decrypt the secret value
	encryptedValue, err := base64.StdEncoding.DecodeString(v.Value)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrSecretDecryption, err)
	}

	decryptedValue, err := m.encryptor.Decrypt(encryptedValue, key)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrSecretDecryption, err)
	}

	// Log access
	m.logAccess(ctx, secretID, userID, "read_version", true)

	// Return version without value
	vCopy := *v
	vCopy.Value = ""

	return &vCopy, string(decryptedValue), nil
}

// ListSecretVersions lists all versions of a secret
func (m *SecretsManager) ListSecretVersions(ctx context.Context, secretID string) ([]*SecretVersion, error) {
	versions, err := m.store.ListSecretVersions(ctx, secretID)
	if err != nil {
		return nil, err
	}

	// Return versions without values
	var results []*SecretVersion
	for _, v := range versions {
		vCopy := *v
		vCopy.Value = ""
		results = append(results, &vCopy)
	}

	return results, nil
}

// logAccess logs access to a secret
func (m *SecretsManager) logAccess(ctx context.Context, secretID, userID, action string, success bool) {
	if m.auditLogger != nil {
		m.auditLogger.LogSecretAccess(ctx, secretID, userID, action, success)
	}

	// Also log to store
	log := &SecretAccessLog{
		ID:        generateSecretID(),
		SecretID:  secretID,
		UserID:    userID,
		Action:    action,
		Timestamp: time.Now(),
		Success:   success,
	}
	_ = m.store.LogAccess(ctx, log)
}

// CheckRotation checks and performs automatic rotation for secrets
func (m *SecretsManager) CheckRotation(ctx context.Context) ([]string, error) {
	secrets, err := m.store.ListSecrets(ctx, SecretFilter{})
	if err != nil {
		return nil, err
	}

	var rotated []string
	now := time.Now()

	for _, secret := range secrets {
		if secret.RotationPolicy == nil || !secret.RotationPolicy.Enabled || !secret.RotationPolicy.AutoRotate {
			continue
		}

		if secret.RotationPolicy.NextRotation != nil && now.After(*secret.RotationPolicy.NextRotation) {
			// In a real implementation, this would generate a new value
			// For now, we just mark it as needing rotation
			rotated = append(rotated, secret.ID)
		}
	}

	return rotated, nil
}

// AESEncryptor implements Encryptor using AES-GCM
type AESEncryptor struct{}

// NewAESEncryptor creates a new AES encryptor
func NewAESEncryptor() *AESEncryptor {
	return &AESEncryptor{}
}

// Encrypt encrypts plaintext using AES-GCM
func (e *AESEncryptor) Encrypt(plaintext []byte, key []byte) ([]byte, error) {
	if len(key) < 16 {
		return nil, ErrSecretKeyTooShort
	}

	// Use first 32 bytes of key for AES-256
	if len(key) > 32 {
		key = key[:32]
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts ciphertext using AES-GCM
func (e *AESEncryptor) Decrypt(ciphertext []byte, key []byte) ([]byte, error) {
	if len(key) < 16 {
		return nil, ErrSecretKeyTooShort
	}

	// Use first 32 bytes of key for AES-256
	if len(key) > 32 {
		key = key[:32]
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, ErrSecretDecryption
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrSecretDecryption
	}

	return plaintext, nil
}

// StaticKeyProvider provides a static key for encryption
type StaticKeyProvider struct {
	key []byte
}

// NewStaticKeyProvider creates a new static key provider
func NewStaticKeyProvider(key []byte) (*StaticKeyProvider, error) {
	if len(key) < 16 {
		return nil, ErrSecretKeyTooShort
	}
	return &StaticKeyProvider{key: key}, nil
}

// GetKey returns the static key
func (p *StaticKeyProvider) GetKey(ctx context.Context, keyID string) ([]byte, error) {
	return p.key, nil
}

// GetCurrentKey returns the current key
func (p *StaticKeyProvider) GetCurrentKey(ctx context.Context) ([]byte, error) {
	return p.key, nil
}

// RotateKey rotates the key (not supported for static provider)
func (p *StaticKeyProvider) RotateKey(ctx context.Context) ([]byte, error) {
	return nil, errors.New("key rotation not supported for static key provider")
}

// generateSecretID generates a unique secret ID
func generateSecretID() string {
	return fmt.Sprintf("sec_%d", time.Now().UnixNano())
}

// SecretMetadata represents metadata about a secret without its value
type SecretMetadata struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Type        SecretType `json:"type"`
	Version     int       `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
}

// ToMetadata converts a secret to metadata (without value)
func (s *Secret) ToMetadata() *SecretMetadata {
	return &SecretMetadata{
		ID:        s.ID,
		Name:      s.Name,
		Type:      s.Type,
		Version:   s.Version,
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
		ExpiresAt: s.ExpiresAt,
		Tags:      s.Tags,
	}
}

// ToJSON serializes a secret to JSON
func (s *Secret) ToJSON() ([]byte, error) {
	return json.Marshal(s)
}

// FromJSON deserializes a secret from JSON
func (s *Secret) FromJSON(data []byte) error {
	return json.Unmarshal(data, s)
}

// GenerateRandomSecret generates a random secret value
func GenerateRandomSecret(length int) (string, error) {
	if length <= 0 {
		length = 32
	}

	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	return base64.URLEncoding.EncodeToString(bytes), nil
}
