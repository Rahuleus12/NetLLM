package crypto

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"math/big"
	"net"
	"os"
	"sync"
	"time"
)

// TLS errors
var (
	ErrTLSCertNotFound      = errors.New("tls certificate not found")
	ErrTLSCertExpired       = errors.New("tls certificate has expired")
	ErrTLSCertInvalid       = errors.New("tls certificate is invalid")
	ErrTLSKeyInvalid        = errors.New("tls private key is invalid")
	ErrTLSCertGenFailed     = errors.New("failed to generate tls certificate")
	ErrTLSConfigInvalid     = errors.New("tls configuration is invalid")
	ErrCertParseFailed      = errors.New("failed to parse certificate")
	ErrKeyParseFailed       = errors.New("failed to parse private key")
)

// TLSVersion represents a TLS version
type TLSVersion string

const (
	TLSVersion10 TLSVersion = "1.0"
	TLSVersion11 TLSVersion = "1.1"
	TLSVersion12 TLSVersion = "1.2"
	TLSVersion13 TLSVersion = "1.3"
)

// TLSConfig holds TLS configuration
type TLSConfig struct {
	// Enabled determines if TLS is enabled
	Enabled bool `json:"enabled"`

	// CertFile is the path to the certificate file
	CertFile string `json:"cert_file"`

	// KeyFile is the path to the private key file
	KeyFile string `json:"key_file"`

	// CAFile is the path to the CA certificate file
	CAFile string `json:"ca_file,omitempty"`

	// MinVersion is the minimum TLS version
	MinVersion TLSVersion `json:"min_version"`

	// MaxVersion is the maximum TLS version
	MaxVersion TLSVersion `json:"max_version"`

	// CipherSuites is a list of cipher suites
	CipherSuites []string `json:"cipher_suites,omitempty"`

	// InsecureSkipVerify skips certificate verification (for development only)
	InsecureSkipVerify bool `json:"insecure_skip_verify"`

	// ServerName is used for SNI
	ServerName string `json:"server_name,omitempty"`

	// ClientAuth specifies the client authentication type
	ClientAuth string `json:"client_auth,omitempty"`

	// ClientCAs is the path to client CA certificates
	ClientCAs string `json:"client_cas,omitempty"`

	// PreferServerCipherSuites prefers server cipher suites
	PreferServerCipherSuites bool `json:"prefer_server_cipher_suites"`

	// SessionTicketsDisabled disables session tickets
	SessionTicketsDisabled bool `json:"session_tickets_disabled"`

	// SessionTicketKey is the session ticket key (32 bytes)
	SessionTicketKey []byte `json:"-"`

	// CurvePreferences is a list of curve preferences
	CurvePreferences []string `json:"curve_preferences,omitempty"`

	// HandshakeTimeout is the handshake timeout
	HandshakeTimeout time.Duration `json:"handshake_timeout"`

	// AutoCert enables automatic certificate management
	AutoCert bool `json:"auto_cert"`

	// AutoCertHosts is a list of hosts for auto certificate
	AutoCertHosts []string `json:"auto_cert_hosts,omitempty"`

	// CertRefreshInterval is the interval for checking certificate updates
	CertRefreshInterval time.Duration `json:"cert_refresh_interval"`
}

// DefaultTLSConfig returns default TLS configuration
func DefaultTLSConfig() *TLSConfig {
	return &TLSConfig{
		Enabled:                   false,
		MinVersion:                TLSVersion12,
		MaxVersion:                TLSVersion13,
		PreferServerCipherSuites:  true,
		SessionTicketsDisabled:    false,
		HandshakeTimeout:          10 * time.Second,
		CertRefreshInterval:       1 * time.Hour,
		AutoCert:                  false,
	}
}

// Certificate represents a TLS certificate
type Certificate struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Certificate  *x509.Certificate `json:"-"`
	PrivateKey   *rsa.PrivateKey  `json:"-"`
	CertPEM      []byte       `json:"cert_pem"`
	KeyPEM       []byte       `json:"-"` // Never expose key PEM in JSON
	NotBefore    time.Time    `json:"not_before"`
	NotAfter     time.Time    `json:"not_after"`
	Subject      string       `json:"subject"`
	Issuer       string       `json:"issuer"`
	DNSNames     []string     `json:"dns_names,omitempty"`
	IPAddresses  []net.IP     `json:"ip_addresses,omitempty"`
	IsCA         bool         `json:"is_ca"`
	SerialNumber string       `json:"serial_number"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
}

// IsExpired checks if the certificate is expired
func (c *Certificate) IsExpired() bool {
	return time.Now().After(c.NotAfter)
}

// ExpiresSoon checks if the certificate expires within the given duration
func (c *Certificate) ExpiresSoon(within time.Duration) bool {
	return time.Now().Add(within).After(c.NotAfter)
}

// IsValid checks if the certificate is currently valid
func (c *Certificate) IsValid() bool {
	now := time.Now()
	return now.After(c.NotBefore) && now.Before(c.NotAfter)
}

// TLSCertificate returns a tls.Certificate for use with crypto/tls
func (c *Certificate) TLSCertificate() (tls.Certificate, error) {
	return tls.X509KeyPair(c.CertPEM, c.KeyPEM)
}

// TLSManager manages TLS certificates and configuration
type TLSManager struct {
	config       *TLSConfig
	certificates map[string]*Certificate
	certCache    *tls.Certificate
	certMutex    sync.RWMutex
	certWatcher  *CertWatcher
	stopCh       chan struct{}
}

// NewTLSManager creates a new TLS manager
func NewTLSManager(config *TLSConfig) (*TLSManager, error) {
	if config == nil {
		config = DefaultTLSConfig()
	}

	manager := &TLSManager{
		config:       config,
		certificates: make(map[string]*Certificate),
		stopCh:       make(chan struct{}),
	}

	// Load initial certificate if configured
	if config.Enabled && config.CertFile != "" && config.KeyFile != "" {
		if err := manager.LoadCertificate(config.CertFile, config.KeyFile); err != nil {
			return nil, fmt.Errorf("failed to load certificate: %w", err)
		}
	}

	// Start certificate refresh routine if enabled
	if config.Enabled && config.CertRefreshInterval > 0 {
		go manager.certRefreshRoutine()
	}

	return manager, nil
}

// LoadCertificate loads a certificate from files
func (m *TLSManager) LoadCertificate(certFile, keyFile string) error {
	certPEM, err := ioutil.ReadFile(certFile)
	if err != nil {
		return fmt.Errorf("failed to read certificate file: %w", err)
	}

	keyPEM, err := ioutil.ReadFile(keyFile)
	if err != nil {
		return fmt.Errorf("failed to read key file: %w", err)
	}

	return m.LoadCertificatePEM(certPEM, keyPEM)
}

// LoadCertificatePEM loads a certificate from PEM data
func (m *TLSManager) LoadCertificatePEM(certPEM, keyPEM []byte) error {
	// Parse the certificate
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return ErrCertParseFailed
	}

	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCertParseFailed, err)
	}

	// Parse the private key
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return ErrKeyParseFailed
	}

	var privateKey *rsa.PrivateKey
	if key, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes); err == nil {
		privateKey = key
	} else if key, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes); err == nil {
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			privateKey = rsaKey
		} else {
			return ErrTLSKeyInvalid
		}
	} else {
		return fmt.Errorf("%w: %v", ErrKeyParseFailed, err)
	}

	// Create tls.Certificate for caching
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return fmt.Errorf("failed to create tls certificate: %w", err)
	}

	certificate := &Certificate{
		ID:           generateCertID(),
		Certificate:  cert,
		PrivateKey:   privateKey,
		CertPEM:      certPEM,
		KeyPEM:       keyPEM,
		NotBefore:    cert.NotBefore,
		NotAfter:     cert.NotAfter,
		Subject:      cert.Subject.CommonName,
		Issuer:       cert.Issuer.CommonName,
		DNSNames:     cert.DNSNames,
		IPAddresses:  cert.IPAddresses,
		IsCA:         cert.IsCA,
		SerialNumber: cert.SerialNumber.String(),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	m.certMutex.Lock()
	m.certificates[certificate.ID] = certificate
	m.certCache = &tlsCert
	m.certMutex.Unlock()

	return nil
}

// GetCertificate returns a tls.Certificate for the current certificate
func (m *TLSManager) GetCertificate() (*tls.Certificate, error) {
	m.certMutex.RLock()
	defer m.certMutex.RUnlock()

	if m.certCache == nil {
		return nil, ErrTLSCertNotFound
	}

	return m.certCache, nil
}

// GetConfigForClient returns a TLS config for server use with client hello support
func (m *TLSManager) GetConfigForClient(hello *tls.ClientHelloInfo) (*tls.Config, error) {
	m.certMutex.RLock()
	defer m.certMutex.RUnlock()

	if m.certCache == nil {
		return nil, ErrTLSCertNotFound
	}

	config, err := m.buildTLSConfig()
	if err != nil {
		return nil, err
	}

	config.Certificates = []tls.Certificate{*m.certCache}
	config.GetCertificate = nil // Prevent recursion

	return config, nil
}

// BuildTLSConfig builds a tls.Config from the manager configuration
func (m *TLSManager) BuildTLSConfig() (*tls.Config, error) {
	m.certMutex.RLock()
	defer m.certMutex.RUnlock()

	return m.buildTLSConfig()
}

// buildTLSConfig builds a tls.Config (must be called with lock held)
func (m *TLSManager) buildTLSConfig() (*tls.Config, error) {
	if !m.config.Enabled {
		return nil, nil
	}

	config := &tls.Config{
		MinVersion:               m.getTLSVersion(m.config.MinVersion),
		MaxVersion:               m.getTLSVersion(m.config.MaxVersion),
		InsecureSkipVerify:       m.config.InsecureSkipVerify,
		ServerName:               m.config.ServerName,
		PreferServerCipherSuites: m.config.PreferServerCipherSuites,
		SessionTicketsDisabled:   m.config.SessionTicketsDisabled,
	}

	// Set cipher suites
	if len(m.config.CipherSuites) > 0 {
		config.CipherSuites = m.getCipherSuites()
	}

	// Set curve preferences
	if len(m.config.CurvePreferences) > 0 {
		config.CurvePreferences = m.getCurvePreferences()
	}

	// Set session ticket key
	if len(m.config.SessionTicketKey) == 32 {
		config.SessionTicketKey = [32]byte(m.config.SessionTicketKey)
	}

	// Set client authentication
	if m.config.ClientAuth != "" {
		config.ClientAuth = m.getClientAuthType()
	}

	// Load client CA certificates
	if m.config.ClientCAs != "" {
		caPEM, err := ioutil.ReadFile(m.config.ClientCAs)
		if err != nil {
			return nil, fmt.Errorf("failed to read client CA file: %w", err)
		}

		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(caPEM) {
			return nil, ErrCertParseFailed
		}
		config.ClientCAs = certPool
	}

	// Load CA certificate
	if m.config.CAFile != "" {
		caPEM, err := ioutil.ReadFile(m.config.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA file: %w", err)
		}

		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(caPEM) {
			return nil, ErrCertParseFailed
		}
		config.RootCAs = certPool
	}

	// Set certificate
	if m.certCache != nil {
		config.Certificates = []tls.Certificate{*m.certCache}
		config.GetCertificate = m.GetCertificate
	}

	return config, nil
}

// getTLSVersion converts TLSVersion string to uint16
func (m *TLSManager) getTLSVersion(version TLSVersion) uint16 {
	switch version {
	case TLSVersion10:
		return tls.VersionTLS10
	case TLSVersion11:
		return tls.VersionTLS11
	case TLSVersion12:
		return tls.VersionTLS12
	case TLSVersion13:
		return tls.VersionTLS13
	default:
		return tls.VersionTLS12
	}
}

// getCipherSuites converts cipher suite names to IDs
func (m *TLSManager) getCipherSuites() []uint16 {
	cipherMap := map[string]uint16{
		"TLS_RSA_WITH_RC4_128_SHA":                tls.TLS_RSA_WITH_RC4_128_SHA,
		"TLS_RSA_WITH_3DES_EDE_CBC_SHA":           tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
		"TLS_RSA_WITH_AES_128_CBC_SHA":            tls.TLS_RSA_WITH_AES_128_CBC_SHA,
		"TLS_RSA_WITH_AES_256_CBC_SHA":            tls.TLS_RSA_WITH_AES_256_CBC_SHA,
		"TLS_RSA_WITH_AES_128_CBC_SHA256":         tls.TLS_RSA_WITH_AES_128_CBC_SHA256,
		"TLS_RSA_WITH_AES_128_GCM_SHA256":         tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
		"TLS_RSA_WITH_AES_256_GCM_SHA384":         tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		"TLS_ECDHE_ECDSA_WITH_RC4_128_SHA":        tls.TLS_ECDHE_ECDSA_WITH_RC4_128_SHA,
		"TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA":    tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
		"TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA":    tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
		"TLS_ECDHE_RSA_WITH_RC4_128_SHA":          tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA,
		"TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA":     tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
		"TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA":      tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
		"TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA":      tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
		"TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256": tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,
		"TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256":   tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,
		"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256":   tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256": tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384":   tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		"TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384": tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		"TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305":    tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		"TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305":  tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		"TLS_AES_128_GCM_SHA256":                  tls.TLS_AES_128_GCM_SHA256,
		"TLS_AES_256_GCM_SHA384":                  tls.TLS_AES_256_GCM_SHA384,
		"TLS_CHACHA20_POLY1305_SHA256":            tls.TLS_CHACHA20_POLY1305_SHA256,
	}

	var suites []uint16
	for _, name := range m.config.CipherSuites {
		if id, ok := cipherMap[name]; ok {
			suites = append(suites, id)
		}
	}
	return suites
}

// getCurvePreferences converts curve names to CurveIDs
func (m *TLSManager) getCurvePreferences() []tls.CurveID {
	curveMap := map[string]tls.CurveID{
		"P256":   tls.CurveP256,
		"P384":   tls.CurveP384,
		"P521":   tls.CurveP521,
		"X25519": tls.X25519,
	}

	var curves []tls.CurveID
	for _, name := range m.config.CurvePreferences {
		if id, ok := curveMap[name]; ok {
			curves = append(curves, id)
		}
	}
	return curves
}

// getClientAuthType converts client auth type string
func (m *TLSManager) getClientAuthType() tls.ClientAuthType {
	switch m.config.ClientAuth {
	case "request":
		return tls.RequestClientCert
	case "require":
		return tls.RequireAnyClientCert
	case "verify":
		return tls.VerifyClientCertIfGiven
	case "require-verify":
		return tls.RequireAndVerifyClientCert
	default:
		return tls.NoClientCert
	}
}

// GenerateSelfSignedCertificate generates a self-signed certificate
func (m *TLSManager) GenerateSelfSignedCertificate(hosts []string, validFor time.Duration) (*Certificate, error) {
	if validFor <= 0 {
		validFor = 365 * 24 * time.Hour // Default: 1 year
	}

	// Generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTLSCertGenFailed, err)
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().Unix()),
		Subject: pkix.Name{
			Organization: []string{"AI-Provider"},
			CommonName:   hosts[0],
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(validFor),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}

	// Add hosts to certificate
	for _, host := range hosts {
		if ip := net.ParseIP(host); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, host)
		}
	}

	// Generate certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTLSCertGenFailed, err)
	}

	// Encode to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	// Parse the generated certificate
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCertParseFailed, err)
	}

	certificate := &Certificate{
		ID:           generateCertID(),
		Certificate:  cert,
		PrivateKey:   privateKey,
		CertPEM:      certPEM,
		KeyPEM:       keyPEM,
		NotBefore:    cert.NotBefore,
		NotAfter:     cert.NotAfter,
		Subject:      cert.Subject.CommonName,
		Issuer:       cert.Issuer.CommonName,
		DNSNames:     cert.DNSNames,
		IPAddresses:  cert.IPAddresses,
		IsCA:         cert.IsCA,
		SerialNumber: cert.SerialNumber.String(),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	// Store the certificate
	m.certMutex.Lock()
	m.certificates[certificate.ID] = certificate
	tlsCert, _ := tls.X509KeyPair(certPEM, keyPEM)
	m.certCache = &tlsCert
	m.certMutex.Unlock()

	return certificate, nil
}

// SaveCertificate saves a certificate to files
func (m *TLSManager) SaveCertificate(certID, certFile, keyFile string) error {
	m.certMutex.RLock()
	defer m.certMutex.RUnlock()

	cert, exists := m.certificates[certID]
	if !exists {
		return ErrTLSCertNotFound
	}

	// Write certificate file
	if err := ioutil.WriteFile(certFile, cert.CertPEM, 0644); err != nil {
		return fmt.Errorf("failed to write certificate file: %w", err)
	}

	// Write key file
	if err := ioutil.WriteFile(keyFile, cert.KeyPEM, 0600); err != nil {
		return fmt.Errorf("failed to write key file: %w", err)
	}

	return nil
}

// GetCertificateInfo returns information about a certificate
func (m *TLSManager) GetCertificateInfo(certID string) (*Certificate, error) {
	m.certMutex.RLock()
	defer m.certMutex.RUnlock()

	cert, exists := m.certificates[certID]
	if !exists {
		return nil, ErrTLSCertNotFound
	}

	return cert, nil
}

// ListCertificates lists all certificates
func (m *TLSManager) ListCertificates() []*Certificate {
	m.certMutex.RLock()
	defer m.certMutex.RUnlock()

	certs := make([]*Certificate, 0, len(m.certificates))
	for _, cert := range m.certificates {
		certs = append(certs, cert)
	}
	return certs
}

// DeleteCertificate removes a certificate
func (m *TLSManager) DeleteCertificate(certID string) error {
	m.certMutex.Lock()
	defer m.certMutex.Unlock()

	if _, exists := m.certificates[certID]; !exists {
		return ErrTLSCertNotFound
	}

	delete(m.certificates, certID)
	return nil
}

// certRefreshRoutine periodically checks for certificate updates
func (m *TLSManager) certRefreshRoutine() {
	ticker := time.NewTicker(m.config.CertRefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.checkCertificateExpiry()
		}
	}
}

// checkCertificateExpiry checks if certificates need renewal
func (m *TLSManager) checkCertificateExpiry() {
	m.certMutex.RLock()
	defer m.certMutex.RUnlock()

	for id, cert := range m.certificates {
		if cert.ExpiresSoon(30 * 24 * time.Hour) { // 30 days
			// In production, this would trigger a renewal process
			// or send an alert
			_ = id
		}
	}
}

// Stop stops the TLS manager
func (m *TLSManager) Stop() {
	close(m.stopCh)
}

// VerifyCertificate verifies a certificate
func VerifyCertificate(certPEM []byte, roots *x509.CertPool) error {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return ErrCertParseFailed
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCertParseFailed, err)
	}

	// Check validity period
	if time.Now().Before(cert.NotBefore) {
		return ErrTLSCertInvalid
	}
	if time.Now().After(cert.NotAfter) {
		return ErrTLSCertExpired
	}

	// Verify certificate chain
	opts := x509.VerifyOptions{
		Roots: roots,
	}

	if _, err := cert.Verify(opts); err != nil {
		return fmt.Errorf("%w: %v", ErrTLSCertInvalid, err)
	}

	return nil
}

// ParseCertificate parses a PEM-encoded certificate
func ParseCertificate(certPEM []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, ErrCertParseFailed
	}

	return x509.ParseCertificate(block.Bytes)
}

// ParsePrivateKey parses a PEM-encoded private key
func ParsePrivateKey(keyPEM []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, ErrKeyParseFailed
	}

	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
	}

	return nil, ErrTLSKeyInvalid
}

// CertWatcher watches certificate files for changes
type CertWatcher struct {
	certFile string
	keyFile  string
	onChange func()
	stopCh   chan struct{}
}

// NewCertWatcher creates a new certificate watcher
func NewCertWatcher(certFile, keyFile string, onChange func()) *CertWatcher {
	return &CertWatcher{
		certFile: certFile,
		keyFile:  keyFile,
		onChange: onChange,
		stopCh:   make(chan struct{}),
	}
}

// Watch starts watching for certificate changes
func (w *CertWatcher) Watch(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var lastCertMod, lastKeyMod time.Time

	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			certInfo, certErr := os.Stat(w.certFile)
			keyInfo, keyErr := os.Stat(w.keyFile)

			if certErr != nil || keyErr != nil {
				continue
			}

			if lastCertMod.IsZero() || lastKeyMod.IsZero() {
				lastCertMod = certInfo.ModTime()
				lastKeyMod = keyInfo.ModTime()
				continue
			}

			if certInfo.ModTime().After(lastCertMod) || keyInfo.ModTime().After(lastKeyMod) {
				lastCertMod = certInfo.ModTime()
				lastKeyMod = keyInfo.ModTime()
				if w.onChange != nil {
					w.onChange()
				}
			}
		}
	}
}

// Stop stops the certificate watcher
func (w *CertWatcher) Stop() {
	close(w.stopCh)
}

// generateCertID generates a unique certificate ID
func generateCertID() string {
	return fmt.Sprintf("cert_%d", time.Now().UnixNano())
}

// CreateClientTLSConfig creates a TLS config for client use
func CreateClientTLSConfig(config *TLSConfig) (*tls.Config, error) {
	if config == nil || !config.Enabled {
		return &tls.Config{
			InsecureSkipVerify: false,
			MinVersion:         tls.VersionTLS12,
		}, nil
	}

	manager, err := NewTLSManager(config)
	if err != nil {
		return nil, err
	}

	return manager.BuildTLSConfig()
}

// CreateServerTLSConfig creates a TLS config for server use
func CreateServerTLSConfig(config *TLSConfig) (*tls.Config, error) {
	if config == nil || !config.Enabled {
		return nil, nil
	}

	manager, err := NewTLSManager(config)
	if err != nil {
		return nil, err
	}

	return manager.BuildTLSConfig()
}

import (
	"crypto/x509/pkix"
)
