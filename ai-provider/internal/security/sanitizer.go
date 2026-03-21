package security

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"html"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Sanitization errors
var (
	ErrInvalidInput        = errors.New("invalid input")
	ErrInputTooLong        = errors.New("input exceeds maximum length")
	ErrInvalidCharacters   = errors.New("input contains invalid characters")
	ErrPotentialXSS        = errors.New("potential XSS detected")
	ErrPotentialSQLi       = errors.New("potential SQL injection detected")
	ErrInvalidEncoding     = errors.New("invalid encoding detected")
)

// SanitizerConfig holds configuration for the sanitizer
type SanitizerConfig struct {
	// MaxInputLength is the maximum allowed input length
	MaxInputLength int

	// AllowHTML allows HTML tags in input
	AllowHTML bool

	// AllowedHTMLTags is a list of allowed HTML tags
	AllowedHTMLTags []string

	// AllowedHTMLAttributes is a map of tag -> allowed attributes
	AllowedHTMLAttributes map[string][]string

	// StrictMode enables strict sanitization
	StrictMode bool

	// TrimWhitespace trims leading/trailing whitespace
	TrimWhitespace bool

	// NormalizeUnicode normalizes unicode characters
	NormalizeUnicode bool

	// RemoveNullBytes removes null bytes from input
	RemoveNullBytes bool

	// MaxJSONDepth is the maximum nesting depth for JSON
	MaxJSONDepth int
}

// DefaultSanitizerConfig returns default sanitizer configuration
func DefaultSanitizerConfig() *SanitizerConfig {
	return &SanitizerConfig{
		MaxInputLength:     10000,
		AllowHTML:          false,
		AllowedHTMLTags:    []string{"b", "i", "u", "strong", "em", "p", "br", "ul", "ol", "li", "a"},
		AllowedHTMLAttributes: map[string][]string{
			"a": {"href", "title"},
		},
		StrictMode:       true,
		TrimWhitespace:   true,
		NormalizeUnicode: true,
		RemoveNullBytes:  true,
		MaxJSONDepth:     10,
	}
}

// Sanitizer handles input/output sanitization
type Sanitizer struct {
	config *SanitizerConfig

	// XSS patterns
	xssPatterns []*regexp.Regexp

	// SQL injection patterns
	sqlPatterns []*regexp.Regexp
}

// NewSanitizer creates a new sanitizer
func NewSanitizer(config *SanitizerConfig) *Sanitizer {
	if config == nil {
		config = DefaultSanitizerConfig()
	}

	s := &Sanitizer{
		config: config,
	}

	s.compilePatterns()

	return s
}

// compilePatterns compiles regex patterns for XSS and SQL injection detection
func (s *Sanitizer) compilePatterns() {
	// XSS detection patterns
	s.xssPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)<\s*script[^>]*>.*?<\s*/\s*script\s*>`),
		regexp.MustCompile(`(?i)<\s*iframe[^>]*>.*?<\s*/\s*iframe\s*>`),
		regexp.MustCompile(`(?i)<\s*object[^>]*>.*?<\s*/\s*object\s*>`),
		regexp.MustCompile(`(?i)<\s*embed[^>]*>`),
		regexp.MustCompile(`(?i)<\s*form[^>]*>`),
		regexp.MustCompile(`(?i)javascript\s*:`),
		regexp.MustCompile(`(?i)vbscript\s*:`),
		regexp.MustCompile(`(?i)on\w+\s*=`), // Event handlers like onclick=
		regexp.MustCompile(`(?i)data\s*:\s*text/html`),
		regexp.MustCompile(`(?i)expression\s*\(`),
		regexp.MustCompile(`(?i)url\s*\(`),
		regexp.MustCompile(`(?i)@import`),
		regexp.MustCompile(`(?i)behavior\s*:`),
		regexp.MustCompile(`(?i)-moz-binding`),
		regexp.MustCompile(`(?i)<\s*link[^>]*>`),
		regexp.MustCompile(`(?i)<\s*style[^>]*>.*?<\s*/\s*style\s*>`),
		regexp.MustCompile(`(?i)<\s*base[^>]*>`),
		regexp.MustCompile(`(?i)<\s*meta[^>]*>`),
		regexp.MustCompile(`(?i)<\s*svg[^>]*>`),
		regexp.MustCompile(`(?i)<\s*math[^>]*>`),
	}

	// SQL injection detection patterns
	s.sqlPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)('\s*(or|and)\s*'?\d*\s*[=<>])`),
		regexp.MustCompile(`(?i)(union\s+(all\s+)?select)`),
		regexp.MustCompile(`(?i)(;\s*(drop|delete|truncate|update|insert|alter)\s+)`),
		regexp.MustCompile(`(?i)(--\s*$)`),
		regexp.MustCompile(`(?i)(/\*.*\*/)`),
		regexp.MustCompile(`(?i)(xp_cmdshell)`),
		regexp.MustCompile(`(?i)(exec\s+xp_)`),
		regexp.MustCompile(`(?i)(waitfor\s+delay)`),
		regexp.MustCompile(`(?i)(benchmark\s*\()`),
		regexp.MustCompile(`(?i)(sleep\s*\()`),
		regexp.MustCompile(`(?i)(load_file\s*\()`),
		regexp.MustCompile(`(?i)(into\s+outfile)`),
		regexp.MustCompile(`(?i)(into\s+dumpfile)`),
		regexp.MustCompile(`(?i)(information_schema)`),
		regexp.MustCompile(`(?i)(sys\.tables)`),
		regexp.MustCompile(`(?i)(sysobjects)`),
		regexp.MustCompile(`(?i)(syscolumns)`),
	}
}

// SanitizeString sanitizes a string input
func (s *Sanitizer) SanitizeString(input string) (string, error) {
	if input == "" {
		return "", nil
	}

	// Check length
	if s.config.MaxInputLength > 0 && len(input) > s.config.MaxInputLength {
		return "", ErrInputTooLong
	}

	result := input

	// Remove null bytes
	if s.config.RemoveNullBytes {
		result = strings.ReplaceAll(result, "\x00", "")
	}

	// Trim whitespace
	if s.config.TrimWhitespace {
		result = strings.TrimSpace(result)
	}

	// Check for potential XSS
	if s.config.StrictMode {
		if err := s.detectXSS(result); err != nil {
			return "", err
		}
	}

	// Escape or strip HTML
	if s.config.AllowHTML {
		result = s.sanitizeHTML(result)
	} else {
		result = html.EscapeString(result)
	}

	return result, nil
}

// SanitizeHTML sanitizes HTML content allowing only safe tags
func (s *Sanitizer) SanitizeHTML(input string) string {
	if input == "" {
		return ""
	}

	// First check for XSS
	if err := s.detectXSS(input); err != nil {
		return html.EscapeString(input)
	}

	return s.sanitizeHTML(input)
}

// sanitizeHTML performs HTML sanitization
func (s *Sanitizer) sanitizeHTML(input string) string {
	// Remove dangerous tags
	result := input

	// Remove script tags and content
	scriptRegex := regexp.MustCompile(`(?i)<\s*script[^>]*>.*?<\s*/\s*script\s*>`)
	result = scriptRegex.ReplaceAllString(result, "")

	// Remove style tags and content
	styleRegex := regexp.MustCompile(`(?i)<\s*style[^>]*>.*?<\s*/\s*style\s*>`)
	result = styleRegex.ReplaceAllString(result, "")

	// Remove event handlers
	eventHandlerRegex := regexp.MustCompile(`(?i)\s+on\w+\s*=\s*["'][^"']*["']`)
	result = eventHandlerRegex.ReplaceAllString(result, "")

	// Remove javascript: URLs
	jsURLRegex := regexp.MustCompile(`(?i)javascript\s*:[^"'\s]*`)
	result = jsURLRegex.ReplaceAllString(result, "")

	// Remove dangerous attributes
	dangerousAttrRegex := regexp.MustCompile(`(?i)\s+(srcdoc|data|formaction|action|background|dynsrc|lowsrc)\s*=\s*["'][^"']*["']`)
	result = dangerousAttrRegex.ReplaceAllString(result, "")

	// Filter to allowed tags only if configured
	if len(s.config.AllowedHTMLTags) > 0 {
		result = s.filterHTMLTags(result)
	}

	return result
}

// filterHTMLTags filters HTML to only allow specified tags
func (s *Sanitizer) filterHTMLTags(input string) string {
	allowedTags := make(map[string]bool)
	for _, tag := range s.config.AllowedHTMLTags {
		allowedTags[strings.ToLower(tag)] = true
	}

	// Match all HTML tags
	tagRegex := regexp.MustCompile(`</?([a-zA-Z][a-zA-Z0-9]*)[^>]*>`)

	result := tagRegex.ReplaceAllStringFunc(input, func(match string) string {
		// Extract tag name
		tagNameMatch := regexp.MustCompile(`</?([a-zA-Z][a-zA-Z0-9]*)`).FindStringSubmatch(match)
		if len(tagNameMatch) < 2 {
			return ""
		}

		tagName := strings.ToLower(tagNameMatch[1])
		if !allowedTags[tagName] {
			return ""
		}

		// Filter attributes
		return s.filterHTMLAttributes(match, tagName)
	})

	return result
}

// filterHTMLAttributes filters attributes for a specific tag
func (s *Sanitizer) filterHTMLAttributes(tag, tagName string) string {
	allowedAttrs, ok := s.config.AllowedHTMLAttributes[tagName]
	if !ok || len(allowedAttrs) == 0 {
		// Remove all attributes
		if strings.HasPrefix(tag, "</") {
			return tag
		}
		isSelfClosing := strings.HasSuffix(tag, "/>")
		if isSelfClosing {
			return "<" + tagName + "/>"
		}
		return "<" + tagName + ">"
	}

	allowedAttrMap := make(map[string]bool)
	for _, attr := range allowedAttrs {
		allowedAttrMap[strings.ToLower(attr)] = true
	}

	// Parse and filter attributes
	attrRegex := regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9_-]*)\s*=\s*["']([^"']*)["']`)

	filteredAttrs := attrRegex.ReplaceAllStringFunc(tag, func(attrMatch string) string {
		parts := attrRegex.FindStringSubmatch(attrMatch)
		if len(parts) < 3 {
			return ""
		}

		attrName := strings.ToLower(parts[1])
		attrValue := parts[2]

		if !allowedAttrMap[attrName] {
			return ""
		}

		// Sanitize attribute value
		if attrName == "href" {
			// Only allow safe protocols
			if !s.isSafeURL(attrValue) {
				return ""
			}
		}

		return attrName + `="` + html.EscapeString(attrValue) + `"`
	})

	// Reconstruct the tag
	isClosing := strings.HasPrefix(tag, "</")
	isSelfClosing := strings.HasSuffix(tag, "/>")

	if isClosing {
		return "</" + tagName + ">"
	}

	result := "<" + tagName
	// Extract filtered attributes from the result
	attrs := strings.TrimSpace(filteredAttrs[strings.Index(filteredAttrs, " "):])
	if attrs != "" && attrs != tagName {
		result += " " + attrs
	}

	if isSelfClosing {
		result += "/>"
	} else {
		result += ">"
	}

	return result
}

// isSafeURL checks if a URL uses a safe protocol
func (s *Sanitizer) isSafeURL(url string) bool {
	url = strings.TrimSpace(url)
	url = strings.ToLower(url)

	safeProtocols := []string{"http://", "https://", "mailto:", "tel:", "/", "#"}
	for _, proto := range safeProtocols {
		if strings.HasPrefix(url, proto) {
			return true
		}
	}

	// Allow relative URLs without protocol
	if !strings.Contains(url, ":") {
		return true
	}

	return false
}

// detectXSS checks for potential XSS patterns
func (s *Sanitizer) detectXSS(input string) error {
	for _, pattern := range s.xssPatterns {
		if pattern.MatchString(input) {
			return ErrPotentialXSS
		}
	}
	return nil
}

// DetectSQLInjection checks for potential SQL injection patterns
func (s *Sanitizer) DetectSQLInjection(input string) error {
	for _, pattern := range s.sqlPatterns {
		if pattern.MatchString(input) {
			return ErrPotentialSQLi
		}
	}
	return nil
}

// SanitizeSQLInput sanitizes input for SQL queries (use parameterized queries instead!)
// This is a defense-in-depth measure, not a replacement for parameterized queries
func (s *Sanitizer) SanitizeSQLInput(input string) (string, error) {
	if input == "" {
		return "", nil
	}

	// Check for SQL injection patterns
	if err := s.DetectSQLInjection(input); err != nil {
		return "", err
	}

	// Escape single quotes
	result := strings.ReplaceAll(input, "'", "''")

	// Remove null bytes
	result = strings.ReplaceAll(result, "\x00", "")

	return result, nil
}

// SanitizeJSON sanitizes JSON input
func (s *Sanitizer) SanitizeJSON(input string) (string, error) {
	if input == "" {
		return "", nil
	}

	// Parse JSON to validate structure
	var data interface{}
	decoder := json.NewDecoder(strings.NewReader(input))

	// Limit depth
	if s.config.MaxJSONDepth > 0 {
		// We'll check depth during recursive sanitization
	}

	if err := decoder.Decode(&data); err != nil {
		return "", err
	}

	// Recursively sanitize string values
	sanitizedData := s.sanitizeJSONValue(data, 0)

	// Re-encode to JSON
	result, err := json.Marshal(sanitizedData)
	if err != nil {
		return "", err
	}

	return string(result), nil
}

// sanitizeJSONValue recursively sanitizes JSON values
func (s *Sanitizer) sanitizeJSONValue(value interface{}, depth int) interface{} {
	if depth > s.config.MaxJSONDepth {
		return nil
	}

	switch v := value.(type) {
	case string:
		sanitized, err := s.SanitizeString(v)
		if err != nil {
			return v // Return original if sanitization fails
		}
		return sanitized

	case map[string]interface{}:
		result := make(map[string]interface{})
		for key, val := range v {
			// Sanitize key
			sanitizedKey, err := s.SanitizeString(key)
			if err != nil {
				sanitizedKey = key
			}
			result[sanitizedKey] = s.sanitizeJSONValue(val, depth+1)
		}
		return result

	case []interface{}:
		result := make([]interface{}, len(v))
		for i, val := range v {
			result[i] = s.sanitizeJSONValue(val, depth+1)
		}
		return result

	default:
		return v
	}
}

// SanitizeFilename sanitizes a filename
func (s *Sanitizer) SanitizeFilename(filename string) string {
	if filename == "" {
		return ""
	}

	// Remove path separators
	result := strings.ReplaceAll(filename, "/", "_")
	result = strings.ReplaceAll(result, "\\", "_")

	// Remove null bytes
	result = strings.ReplaceAll(result, "\x00", "")

	// Remove dangerous characters
	dangerousChars := []string{"..", "<", ">", ":", "\"", "|", "?", "*"}
	for _, char := range dangerousChars {
		result = strings.ReplaceAll(result, char, "_")
	}

	// Trim whitespace
	result = strings.TrimSpace(result)

	// Limit length
	if len(result) > 255 {
		result = result[:255]
	}

	return result
}

// SanitizeEmail sanitizes an email address
func (s *Sanitizer) SanitizeEmail(email string) string {
	if email == "" {
		return ""
	}

	// Trim whitespace
	result := strings.TrimSpace(email)

	// Convert to lowercase
	result = strings.ToLower(result)

	// Remove control characters
	result = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, result)

	return result
}

// SanitizeURL sanitizes a URL
func (s *Sanitizer) SanitizeURL(urlStr string) (string, error) {
	if urlStr == "" {
		return "", nil
	}

	// Trim whitespace
	result := strings.TrimSpace(urlStr)

	// Check for javascript: protocol
	if strings.HasPrefix(strings.ToLower(result), "javascript:") {
		return "", ErrInvalidInput
	}

	// Check for data: protocol (except images)
	lowerResult := strings.ToLower(result)
	if strings.HasPrefix(lowerResult, "data:") && !strings.HasPrefix(lowerResult, "data:image/") {
		return "", ErrInvalidInput
	}

	// Remove null bytes
	result = strings.ReplaceAll(result, "\x00", "")

	return result, nil
}

// SanitizeBase64 sanitizes base64 encoded input
func (s *Sanitizer) SanitizeBase64(input string) (string, error) {
	if input == "" {
		return "", nil
	}

	// Remove whitespace
	result := strings.ReplaceAll(input, " ", "")
	result = strings.ReplaceAll(result, "\n", "")
	result = strings.ReplaceAll(result, "\r", "")
	result = strings.ReplaceAll(result, "\t", "")

	// Validate and decode
	decoded, err := base64.StdEncoding.DecodeString(result)
	if err != nil {
		return "", ErrInvalidEncoding
	}

	// Check decoded length
	if s.config.MaxInputLength > 0 && len(decoded) > s.config.MaxInputLength {
		return "", ErrInputTooLong
	}

	// Re-encode
	return base64.StdEncoding.EncodeToString(decoded), nil
}

// StripControlCharacters removes control characters from input
func (s *Sanitizer) StripControlCharacters(input string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return -1
		}
		return r
	}, input)
}

// NormalizeWhitespace normalizes whitespace in input
func (s *Sanitizer) NormalizeWhitespace(input string) string {
	// Replace multiple spaces with single space
	spaceRegex := regexp.MustCompile(`\s+`)
	result := spaceRegex.ReplaceAllString(input, " ")

	// Trim
	result = strings.TrimSpace(result)

	return result
}

// Truncate truncates input to a maximum length
func (s *Sanitizer) Truncate(input string, maxLen int) string {
	if maxLen <= 0 {
		return input
	}

	if len(input) <= maxLen {
		return input
	}

	// Try to truncate at UTF-8 boundary
	for i := maxLen; i >= 0; i-- {
		if utf8.RuneStart(input[i]) {
			return input[:i]
		}
	}

	return input[:maxLen]
}

// EscapeString escapes special characters for safe output
func (s *Sanitizer) EscapeString(input string, contentType ContentType) string {
	switch contentType {
	case ContentTypeHTML:
		return html.EscapeString(input)
	case ContentTypeJSON:
		var buf strings.Builder
		json.HTMLEscape(&buf, []byte(input))
		return buf.String()
	case ContentTypeXML:
		return s.escapeXML(input)
	case ContentTypeJavaScript:
		return s.escapeJavaScript(input)
	case ContentTypeURL:
		return s.escapeURL(input)
	default:
		return html.EscapeString(input)
	}
}

// ContentType represents the type of content being escaped
type ContentType string

const (
	ContentTypeHTML      ContentType = "html"
	ContentTypeJSON      ContentType = "json"
	ContentTypeXML       ContentType = "xml"
	ContentTypeJavaScript ContentType = "javascript"
	ContentTypeURL       ContentType = "url"
)

// escapeXML escapes special characters for XML
func (s *Sanitizer) escapeXML(input string) string {
	result := input
	result = strings.ReplaceAll(result, "&", "&amp;")
	result = strings.ReplaceAll(result, "<", "&lt;")
	result = strings.ReplaceAll(result, ">", "&gt;")
	result = strings.ReplaceAll(result, "\"", "&quot;")
	result = strings.ReplaceAll(result, "'", "&apos;")
	return result
}

// escapeJavaScript escapes special characters for JavaScript strings
func (s *Sanitizer) escapeJavaScript(input string) string {
	result := input
	result = strings.ReplaceAll(result, "\\", "\\\\")
	result = strings.ReplaceAll(result, "\"", "\\\"")
	result = strings.ReplaceAll(result, "'", "\\'")
	result = strings.ReplaceAll(result, "\n", "\\n")
	result = strings.ReplaceAll(result, "\r", "\\r")
	result = strings.ReplaceAll(result, "\t", "\\t")
	result = strings.ReplaceAll(result, "</", "<\\/")
	return result
}

// escapeURL escapes special characters for URL
func (s *Sanitizer) escapeURL(input string) string {
	var result strings.Builder
	for _, r := range input {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' || r == '~' {
			result.WriteRune(r)
		} else {
			result.WriteString("%" + strings.ToUpper(fmt.Sprintf("%02X", r)))
		}
	}
	return result.String()
}

import "fmt"

// SanitizeMap sanitizes all string values in a map
func (s *Sanitizer) SanitizeMap(data map[string]interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	for key, value := range data {
		// Sanitize key
		sanitizedKey, err := s.SanitizeString(key)
		if err != nil {
			return nil, err
		}

		// Sanitize value based on type
		switch v := value.(type) {
		case string:
			sanitized, err := s.SanitizeString(v)
			if err != nil {
				return nil, err
			}
			result[sanitizedKey] = sanitized
		case map[string]interface{}:
			sanitized, err := s.SanitizeMap(v)
			if err != nil {
				return nil, err
			}
			result[sanitizedKey] = sanitized
		case []interface{}:
			sanitized, err := s.SanitizeSlice(v)
			if err != nil {
				return nil, err
			}
			result[sanitizedKey] = sanitized
		default:
			result[sanitizedKey] = v
		}
	}
	return result, nil
}

// SanitizeSlice sanitizes all string values in a slice
func (s *Sanitizer) SanitizeSlice(data []interface{}) ([]interface{}, error) {
	result := make([]interface{}, len(data))
	for i, value := range data {
		switch v := value.(type) {
		case string:
			sanitized, err := s.SanitizeString(v)
			if err != nil {
				return nil, err
			}
			result[i] = sanitized
		case map[string]interface{}:
			sanitized, err := s.SanitizeMap(v)
			if err != nil {
				return nil, err
			}
			result[i] = sanitized
		case []interface{}:
			sanitized, err := s.SanitizeSlice(v)
			if err != nil {
				return nil, err
			}
			result[i] = sanitized
		default:
			result[i] = v
		}
	}
	return result, nil
}
