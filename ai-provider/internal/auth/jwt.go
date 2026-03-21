package auth

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidToken        = errors.New("invalid token")
	ErrExpiredToken        = errors.New("token has expired")
	ErrInvalidSigningMethod = errors.New("invalid signing method")
	ErrTokenNotValidYet    = errors.New("token is not valid yet")
	ErrTokenBlacklisted    = errors.New("token has been blacklisted")
	ErrInvalidKey          = errors.New("invalid key")
	ErrMissingClaims       = errors.New("missing required claims")
)

// TokenType represents the type of JWT token
type TokenType string

const (
	AccessToken  TokenType = "access"
	RefreshToken TokenType = "refresh"
)

// JWTConfig holds configuration for JWT operations
type JWTConfig struct {
	// SigningMethod can be "HS256", "HS384", "HS512", "RS256", "RS384", "RS512"
	SigningMethod string `mapstructure:"signing_method"`

	// Secret key for HMAC algorithms
	SecretKey string `mapstructure:"secret_key"`

	// Private key file path for RSA algorithms
	PrivateKeyPath string `mapstructure:"private_key_path"`

	// Public key file path for RSA algorithms
	PublicKeyPath string `mapstructure:"public_key_path"`

	// AccessTokenTTL is the duration access tokens are valid
	AccessTokenTTL time.Duration `mapstructure:"access_token_ttl"`

	// RefreshTokenTTL is the duration refresh tokens are valid
	RefreshTokenTTL time.Duration `mapstructure:"refresh_token_ttl"`

	// Issuer identifies the principal that issued the JWT
	Issuer string `mapstructure:"issuer"`

	// Audience identifies the recipients that the JWT is intended for
	Audience []string `mapstructure:"audience"`
}

// Claims represents the custom JWT claims
type Claims struct {
	UserID      string            `json:"user_id"`
	Email       string            `json:"email"`
	Username    string            `json:"username"`
	Roles       []string          `json:"roles,omitempty"`
	Permissions []string          `json:"permissions,omitempty"`
	TokenType   TokenType         `json:"token_type"`
	SessionID   string            `json:"session_id"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// TokenPair represents an access and refresh token pair
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	TokenType    string    `json:"token_type"`
}

// BlacklistEntry represents a blacklisted token
type BlacklistEntry struct {
	TokenID    string    `json:"token_id"`
	UserID     string    `json:"user_id"`
	Reason     string    `json:"reason"`
	BlacklistedAt time.Time `json:"blacklisted_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// JWTManager handles JWT token operations
type JWTManager struct {
	config        *JWTConfig
	privateKey    *rsa.PrivateKey
	publicKey     *rsa.PublicKey
	blacklist     map[string]*BlacklistEntry
	blacklistLock sync.RWMutex
}

// NewJWTManager creates a new JWT manager
func NewJWTManager(config *JWTConfig) (*JWTManager, error) {
	if config == nil {
		return nil, errors.New("jwt config is required")
	}

	// Set defaults
	if config.SigningMethod == "" {
		config.SigningMethod = "HS256"
	}
	if config.AccessTokenTTL == 0 {
		config.AccessTokenTTL = 15 * time.Minute
	}
	if config.RefreshTokenTTL == 0 {
		config.RefreshTokenTTL = 7 * 24 * time.Hour
	}

	jm := &JWTManager{
		config:    config,
		blacklist: make(map[string]*BlacklistEntry),
	}

	// Load RSA keys if using RSA signing method
	if strings.HasPrefix(strings.ToUpper(config.SigningMethod), "RS") {
		if err := jm.loadRSAKeys(); err != nil {
			return nil, fmt.Errorf("failed to load RSA keys: %w", err)
		}
	}

	// Start cleanup goroutine for blacklist
	go jm.cleanupBlacklist()

	return jm, nil
}

// loadRSAKeys loads RSA private and public keys from files
func (jm *JWTManager) loadRSAKeys() error {
	// Load private key
	if jm.config.PrivateKeyPath != "" {
		keyData, err := os.ReadFile(jm.config.PrivateKeyPath)
		if err != nil {
			return fmt.Errorf("failed to read private key file: %w", err)
		}

		block, _ := pem.Decode(keyData)
		if block == nil {
			return errors.New("failed to decode private key PEM")
		}

		privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			pkcs8Key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
			if err != nil {
				return fmt.Errorf("failed to parse private key: %w", err)
			}
			var ok bool
			privateKey, ok = pkcs8Key.(*rsa.PrivateKey)
			if !ok {
				return errors.New("private key is not RSA")
			}
		}
		jm.privateKey = privateKey
	}

	// Load public key
	if jm.config.PublicKeyPath != "" {
		keyData, err := os.ReadFile(jm.config.PublicKeyPath)
		if err != nil {
			return fmt.Errorf("failed to read public key file: %w", err)
		}

		block, _ := pem.Decode(keyData)
		if block == nil {
			return errors.New("failed to decode public key PEM")
		}

		pub, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return fmt.Errorf("failed to parse public key: %w", err)
		}

		publicKey, ok := pub.(*rsa.PublicKey)
		if !ok {
			return errors.New("public key is not RSA")
		}
		jm.publicKey = publicKey
	} else if jm.privateKey != nil {
		jm.publicKey = &jm.privateKey.PublicKey
	}

	return nil
}

// GenerateTokenPair generates both access and refresh tokens
func (jm *JWTManager) GenerateTokenPair(claims *Claims) (*TokenPair, error) {
	if claims == nil || claims.UserID == "" {
		return nil, ErrMissingClaims
	}

	sessionID := claims.SessionID
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	// Generate access token
	accessClaims := &Claims{
		UserID:      claims.UserID,
		Email:       claims.Email,
		Username:    claims.Username,
		Roles:       claims.Roles,
		Permissions: claims.Permissions,
		TokenType:   AccessToken,
		SessionID:   sessionID,
		Metadata:    claims.Metadata,
	}

	accessToken, err := jm.GenerateToken(accessClaims, jm.config.AccessTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	// Generate refresh token
	refreshClaims := &Claims{
		UserID:    claims.UserID,
		TokenType: RefreshToken,
		SessionID: sessionID,
	}

	refreshToken, err := jm.GenerateToken(refreshClaims, jm.config.RefreshTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    time.Now().Add(jm.config.AccessTokenTTL),
		TokenType:    "Bearer",
	}, nil
}

// GenerateToken generates a JWT token with the given claims and expiration
func (jm *JWTManager) GenerateToken(claims *Claims, ttl time.Duration) (string, error) {
	if claims == nil || claims.UserID == "" {
		return "", ErrMissingClaims
	}

	now := time.Now()
	exp := now.Add(ttl)

	tokenID := uuid.New().String()

	// Build the token header and payload
	header := jm.buildHeader()
	payload := jm.buildPayload(claims, tokenID, now, exp)

	// Create the signing input
	signingInput := header + "." + payload

	// Sign the token
	signature, err := jm.sign(signingInput)
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return signingInput + "." + signature, nil
}

// buildHeader creates the JWT header
func (jm *JWTManager) buildHeader() string {
	alg := jm.config.SigningMethod
	return `{"alg":"` + alg + `","typ":"JWT"}`
}

// buildPayload creates the JWT payload
func (jm *JWTManager) buildPayload(claims *Claims, tokenID string, issuedAt, expiresAt time.Time) string {
	payload := map[string]interface{}{
		"sub": claims.UserID,
		"iat": issuedAt.Unix(),
		"exp": expiresAt.Unix(),
		"nbf": issuedAt.Unix(),
		"jti": tokenID,
		"typ": string(claims.TokenType),
		"sid": claims.SessionID,
	}

	if claims.Email != "" {
		payload["email"] = claims.Email
	}
	if claims.Username != "" {
		payload["username"] = claims.Username
	}
	if len(claims.Roles) > 0 {
		payload["roles"] = claims.Roles
	}
	if len(claims.Permissions) > 0 {
		payload["permissions"] = claims.Permissions
	}
	if jm.config.Issuer != "" {
		payload["iss"] = jm.config.Issuer
	}
	if len(jm.config.Audience) > 0 {
		payload["aud"] = jm.config.Audience
	}
	if claims.TokenType == AccessToken {
		payload["custom_claims"] = claims.Metadata
	}

	// JSON encode the payload
	jsonPayload, _ := jm.encodeJSON(payload)
	return jsonPayload
}

// sign creates the signature for the signing input
func (jm *JWTManager) sign(signingInput string) (string, error) {
	method := strings.ToUpper(jm.config.SigningMethod)

	switch method {
	case "HS256", "HS384", "HS512":
		return jm.signHMAC(signingInput, method)
	case "RS256", "RS384", "RS512":
		return jm.signRSA(signingInput, method)
	default:
		return "", ErrInvalidSigningMethod
	}
}

// ValidateToken validates a JWT token and returns the claims
func (jm *JWTManager) ValidateToken(tokenString string) (*Claims, error) {
	if tokenString == "" {
		return nil, ErrInvalidToken
	}

	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}

	// Verify the signature
	signingInput := parts[0] + "." + parts[1]
	signature := parts[2]

	if err := jm.verify(signingInput, signature); err != nil {
		return nil, fmt.Errorf("signature verification failed: %w", err)
	}

	// Parse the payload
	payload, err := jm.decodePayload(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode payload: %w", err)
	}

	// Validate standard claims
	if err := jm.validateStandardClaims(payload); err != nil {
		return nil, err
	}

	// Check if token is blacklisted
	tokenID, _ := payload["jti"].(string)
	if jm.IsBlacklisted(tokenID) {
		return nil, ErrTokenBlacklisted
	}

	// Extract custom claims
	claims := &Claims{
		UserID:    getString(payload, "sub"),
		Email:     getString(payload, "email"),
		Username:  getString(payload, "username"),
		TokenType: TokenType(getString(payload, "typ")),
		SessionID: getString(payload, "sid"),
	}

	if roles, ok := payload["roles"].([]interface{}); ok {
		claims.Roles = make([]string, len(roles))
		for i, r := range roles {
			claims.Roles[i] = fmt.Sprintf("%v", r)
		}
	}

	if perms, ok := payload["permissions"].([]interface{}); ok {
		claims.Permissions = make([]string, len(perms))
		for i, p := range perms {
			claims.Permissions[i] = fmt.Sprintf("%v", p)
		}
	}

	if customClaims, ok := payload["custom_claims"].(map[string]interface{}); ok {
		claims.Metadata = make(map[string]string)
		for k, v := range customClaims {
			claims.Metadata[k] = fmt.Sprintf("%v", v)
		}
	}

	return claims, nil
}

// validateStandardClaims validates the standard JWT claims
func (jm *JWTManager) validateStandardClaims(payload map[string]interface{}) error {
	now := time.Now().Unix()

	// Check expiration
	if exp, ok := payload["exp"].(float64); ok {
		if int64(exp) < now {
			return ErrExpiredToken
		}
	}

	// Check not before
	if nbf, ok := payload["nbf"].(float64); ok {
		if int64(nbf) > now {
			return ErrTokenNotValidYet
		}
	}

	// Check issuer
	if jm.config.Issuer != "" {
		if iss, ok := payload["iss"].(string); ok {
			if iss != jm.config.Issuer {
				return ErrInvalidToken
			}
		}
	}

	// Check audience
	if len(jm.config.Audience) > 0 {
		aud, ok := payload["aud"]
		if !ok {
			return ErrInvalidToken
		}

		switch v := aud.(type) {
		case string:
			if !jm.containsAudience(v) {
				return ErrInvalidToken
			}
		case []interface{}:
			found := false
			for _, a := range v {
				if jm.containsAudience(fmt.Sprintf("%v", a)) {
					found = true
					break
				}
			}
			if !found {
				return ErrInvalidToken
			}
		}
	}

	return nil
}

// containsAudience checks if the audience is in the configured audience list
func (jm *JWTManager) containsAudience(aud string) bool {
	for _, a := range jm.config.Audience {
		if a == aud {
			return true
		}
	}
	return false
}

// RefreshAccessToken refreshes an access token using a valid refresh token
func (jm *JWTManager) RefreshAccessToken(refreshToken string) (*TokenPair, error) {
	claims, err := jm.ValidateToken(refreshToken)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}

	if claims.TokenType != RefreshToken {
		return nil, errors.New("token is not a refresh token")
	}

	// Blacklist the old refresh token
	tokenID := jm.extractTokenID(refreshToken)
	if tokenID != "" {
		jm.BlacklistToken(tokenID, claims.UserID, "token_refreshed")
	}

	// Generate new token pair
	newClaims := &Claims{
		UserID:    claims.UserID,
		SessionID: claims.SessionID,
	}

	return jm.GenerateTokenPair(newClaims)
}

// extractTokenID extracts the token ID (jti) from a token
func (jm *JWTManager) extractTokenID(tokenString string) string {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return ""
	}

	payload, err := jm.decodePayload(parts[1])
	if err != nil {
		return ""
	}

	if jti, ok := payload["jti"].(string); ok {
		return jti
	}
	return ""
}

// BlacklistToken adds a token to the blacklist
func (jm *JWTManager) BlacklistToken(tokenID, userID, reason string) {
	jm.blacklistLock.Lock()
	defer jm.blacklistLock.Unlock()

	jm.blacklist[tokenID] = &BlacklistEntry{
		TokenID:       tokenID,
		UserID:        userID,
		Reason:        reason,
		BlacklistedAt: time.Now(),
		ExpiresAt:     time.Now().Add(jm.config.RefreshTokenTTL),
	}
}

// IsBlacklisted checks if a token is blacklisted
func (jm *JWTManager) IsBlacklisted(tokenID string) bool {
	jm.blacklistLock.RLock()
	defer jm.blacklistLock.RUnlock()

	entry, exists := jm.blacklist[tokenID]
	if !exists {
		return false
	}

	// Check if the entry has expired
	if time.Now().After(entry.ExpiresAt) {
		return false
	}

	return true
}

// RemoveFromBlacklist removes a token from the blacklist
func (jm *JWTManager) RemoveFromBlacklist(tokenID string) {
	jm.blacklistLock.Lock()
	defer jm.blacklistLock.Unlock()

	delete(jm.blacklist, tokenID)
}

// cleanupBlacklist periodically removes expired entries from the blacklist
func (jm *JWTManager) cleanupBlacklist() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		jm.blacklistLock.Lock()
		now := time.Now()
		for tokenID, entry := range jm.blacklist {
			if now.After(entry.ExpiresAt) {
				delete(jm.blacklist, tokenID)
			}
		}
		jm.blacklistLock.Unlock()
	}
}

// GetBlacklistedTokens returns all blacklisted tokens for a user
func (jm *JWTManager) GetBlacklistedTokens(userID string) []*BlacklistEntry {
	jm.blacklistLock.RLock()
	defer jm.blacklistLock.RUnlock()

	var entries []*BlacklistEntry
	for _, entry := range jm.blacklist {
		if entry.UserID == userID {
			entries = append(entries, entry)
		}
	}
	return entries
}

// RevokeAllUserTokens revokes all tokens for a user by blacklisting their session
func (jm *JWTManager) RevokeAllUserTokens(userID, reason string) {
	// This is a simplified implementation
	// In production, you would track all active sessions/tokens per user
	// and blacklist them all here
}

// Helper functions

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func (jm *JWTManager) encodeJSON(v interface{}) (string, error) {
	// Simple JSON encoding for JWT payload
	// In production, use encoding/json
	switch val := v.(type) {
	case map[string]interface{}:
		parts := make([]string, 0, len(val))
		for k, v := range val {
			var strVal string
			switch vv := v.(type) {
			case string:
				strVal = `"` + vv + `"`
			case float64:
				strVal = fmt.Sprintf("%v", vv)
			case int64:
				strVal = fmt.Sprintf("%v", vv)
			case []string:
				parts2 := make([]string, len(vv))
				for i, s := range vv {
					parts2[i] = `"` + s + `"`
				}
				strVal = "[" + strings.Join(parts2, ",") + "]"
			case []interface{}:
				parts2 := make([]string, len(vv))
				for i, item := range vv {
					parts2[i] = fmt.Sprintf(`"%v"`, item)
				}
				strVal = "[" + strings.Join(parts2, ",") + "]"
			default:
				strVal = fmt.Sprintf(`"%v"`, v)
			}
			parts = append(parts, fmt.Sprintf(`"%s":%s`, k, strVal))
		}
		return "{" + strings.Join(parts, ",") + "}", nil
	default:
		return "", fmt.Errorf("unsupported type: %T", val)
	}
}

func (jm *JWTManager) decodePayload(payload string) (map[string]interface{}, error) {
	// Decode base64url
	decoded, err := jm.decodeBase64(payload)
	if err != nil {
		return nil, err
	}

	// Parse JSON
	result := make(map[string]interface{})
	// Simple JSON parsing - in production use encoding/json
	// This is a simplified implementation
	str := string(decoded)
	str = strings.Trim(str, "{}")

	parts := jm.parseJSONObject(str)
	for k, v := range parts {
		result[k] = v
	}

	return result, nil
}

func (jm *JWTManager) parseJSONObject(str string) map[string]interface{} {
	result := make(map[string]interface{})

	// Very simplified JSON parsing - use encoding/json in production
	// This handles basic key-value pairs
	inString := false
	depth := 0
	currentKey := ""
	currentValue := strings.Builder{}

	for i := 0; i < len(str); i++ {
		char := str[i]

		if char == '"' && (i == 0 || str[i-1] != '\\') {
			inString = !inString
			currentValue.WriteByte(char)
			continue
		}

		if !inString {
			if char == '{' || char == '[' {
				depth++
				currentValue.WriteByte(char)
			} else if char == '}' || char == ']' {
				depth--
				currentValue.WriteByte(char)
			} else if char == ':' && depth == 0 {
				currentKey = strings.Trim(currentValue.String(), `" `)
				currentValue.Reset()
			} else if char == ',' && depth == 0 {
				val := strings.TrimSpace(currentValue.String())
				result[currentKey] = jm.parseJSONValue(val)
				currentValue.Reset()
			} else {
				currentValue.WriteByte(char)
			}
		} else {
			currentValue.WriteByte(char)
		}
	}

	if currentKey != "" {
		val := strings.TrimSpace(currentValue.String())
		result[currentKey] = jm.parseJSONValue(val)
	}

	return result
}

func (jm *JWTManager) parseJSONValue(val string) interface{} {
	val = strings.TrimSpace(val)

	// String
	if strings.HasPrefix(val, `"`) && strings.HasSuffix(val, `"`) {
		return strings.Trim(val, `"`)
	}

	// Number
	if strings.Contains(val, ".") {
		var f float64
		if _, err := fmt.Sscanf(val, "%f", &f); err == nil {
			return f
		}
	}
	var i int64
	if _, err := fmt.Sscanf(val, "%d", &i); err == nil {
		return i
	}

	// Boolean
	if val == "true" {
		return true
	}
	if val == "false" {
		return false
	}

	// Null
	if val == "null" {
		return nil
	}

	// Array (simplified)
	if strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]") {
		inner := strings.Trim(val, "[]")
		items := strings.Split(inner, ",")
		result := make([]interface{}, len(items))
		for i, item := range items {
			result[i] = jm.parseJSONValue(item)
		}
		return result
	}

	return val
}

func (jm *JWTManager) decodeBase64(s string) ([]byte, error) {
	// Add padding if necessary
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}

	// Replace URL-safe characters
	s = strings.ReplaceAll(s, "-", "+")
	s = strings.ReplaceAll(s, "_", "/")

	// Base64 decode
	result := make([]byte, len(s)*3/4)
	n, err := jm.base64Decode(result, s)
	if err != nil {
		return nil, err
	}

	return result[:n], nil
}

// base64Decode performs base64 decoding
func (jm *JWTManager) base64Decode(dst []byte, src string) (n int, err error) {
	// Standard base64 alphabet
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	decodeMap := make([]byte, 256)
	for i := range decodeMap {
		decodeMap[i] = 0xFF
	}
	for i := range alphabet {
		decodeMap[alphabet[i]] = byte(i)
	}

	var di, si int
	for si < len(src) && src[si] != '=' {
		// Process 4 characters at a time
		if si+4 <= len(src) {
			b0 := decodeMap[src[si]]
			b1 := decodeMap[src[si+1]]
			b2 := decodeMap[src[si+2]]
			b3 := decodeMap[src[si+3]]

			if b0 == 0xFF || b1 == 0xFF {
				return 0, errors.New("invalid base64 character")
			}

			dst[di] = (b0 << 2) | (b1 >> 4)
			di++

			if src[si+2] != '=' {
				if b2 == 0xFF {
					return 0, errors.New("invalid base64 character")
				}
				dst[di] = (b1 << 4) | (b2 >> 2)
				di++
			}

			if src[si+3] != '=' {
				if b3 == 0xFF {
					return 0, errors.New("invalid base64 character")
				}
				dst[di] = (b2 << 6) | b3
				di++
			}

			si += 4
		} else {
			break
		}
	}

	return di, nil
}
