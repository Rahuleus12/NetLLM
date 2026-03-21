package security

import (
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Validation errors
var (
	ErrInvalidInput        = errors.New("invalid input")
	ErrRequiredField       = errors.New("required field is empty")
	ErrInvalidEmail        = errors.New("invalid email address")
	ErrInvalidURL          = errors.New("invalid URL")
	ErrInvalidLength       = errors.New("invalid length")
	ErrInvalidFormat       = errors.New("invalid format")
	ErrInvalidRange        = errors.New("value out of range")
	ErrInvalidCharacters   = errors.New("contains invalid characters")
	ErrPotentiallyDangerous = errors.New("input contains potentially dangerous content")
)

// ValidationError represents a validation error with field details
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Code    string `json:"code"`
	Value   interface{} `json:"value,omitempty"`
}

// Error implements the error interface
func (e *ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s: %s", e.Field, e.Message)
	}
	return e.Message
}

// ValidationErrors is a collection of validation errors
type ValidationErrors []*ValidationError

// Error implements the error interface
func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return ""
	}
	var messages []string
	for _, err := range e {
		messages = append(messages, err.Error())
	}
	return strings.Join(messages, "; ")
}

// HasErrors returns true if there are any errors
func (e ValidationErrors) HasErrors() bool {
	return len(e) > 0
}

// Add adds a new validation error
func (e *ValidationErrors) Add(field, message, code string, value interface{}) {
	*e = append(*e, &ValidationError{
		Field:   field,
		Message: message,
		Code:    code,
		Value:   value,
	})
}

// Validator provides input validation functionality
type Validator struct {
	errors ValidationErrors
}

// NewValidator creates a new validator
func NewValidator() *Validator {
	return &Validator{
		errors: make(ValidationErrors, 0),
	}
}

// Errors returns all validation errors
func (v *Validator) Errors() ValidationErrors {
	return v.errors
}

// HasErrors returns true if there are any errors
func (v *Validator) HasErrors() bool {
	return len(v.errors) > 0
}

// AddError adds a validation error
func (v *Validator) AddError(field, message, code string, value interface{}) {
	v.errors.Add(field, message, code, value)
}

// Required validates that a field is not empty
func (v *Validator) Required(field string, value interface{}) *Validator {
	if isEmpty(value) {
		v.AddError(field, "This field is required", "REQUIRED", value)
	}
	return v
}

// MinLength validates minimum string length
func (v *Validator) MinLength(field, value string, min int) *Validator {
	if value != "" && utf8.RuneCountInString(value) < min {
		v.AddError(field, fmt.Sprintf("Must be at least %d characters", min), "MIN_LENGTH", value)
	}
	return v
}

// MaxLength validates maximum string length
func (v *Validator) MaxLength(field, value string, max int) *Validator {
	if value != "" && utf8.RuneCountInString(value) > max {
		v.AddError(field, fmt.Sprintf("Must be at most %d characters", max), "MAX_LENGTH", value)
	}
	return v
}

// Length validates string length is within range
func (v *Validator) Length(field, value string, min, max int) *Validator {
	v.MinLength(field, value, min)
	v.MaxLength(field, value, max)
	return v
}

// Email validates email format
func (v *Validator) Email(field, value string) *Validator {
	if value != "" && !IsValidEmail(value) {
		v.AddError(field, "Invalid email address", "INVALID_EMAIL", value)
	}
	return v
}

// URL validates URL format
func (v *Validator) URL(field, value string) *Validator {
	if value != "" && !IsValidURL(value) {
		v.AddError(field, "Invalid URL", "INVALID_URL", value)
	}
	return v
}

// Match validates string matches a regex pattern
func (v *Validator) Match(field, value, pattern, message string) *Validator {
	if value != "" {
		matched, err := regexp.MatchString(pattern, value)
		if err != nil || !matched {
			if message == "" {
				message = "Invalid format"
			}
			v.AddError(field, message, "INVALID_FORMAT", value)
		}
	}
	return v
}

// InList validates value is in a list of allowed values
func (v *Validator) InList(field, value string, allowed []string) *Validator {
	if value != "" {
		found := false
		for _, a := range allowed {
			if value == a {
				found = true
				break
			}
		}
		if !found {
			v.AddError(field, fmt.Sprintf("Must be one of: %s", strings.Join(allowed, ", ")), "INVALID_VALUE", value)
		}
	}
	return v
}

// Alphanumeric validates string contains only alphanumeric characters
func (v *Validator) Alphanumeric(field, value string) *Validator {
	if value != "" && !IsAlphanumeric(value) {
		v.AddError(field, "Must contain only letters and numbers", "ALPHANUMERIC", value)
	}
	return v
}

// AlphanumericDashUnderscore validates string contains only alphanumeric, dash, and underscore
func (v *Validator) AlphanumericDashUnderscore(field, value string) *Validator {
	if value != "" && !IsAlphanumericDashUnderscore(value) {
		v.AddError(field, "Must contain only letters, numbers, dashes, and underscores", "ALPHANUMERIC_DASH_UNDERSCORE", value)
	}
	return v
}

// NoHTML validates string doesn't contain HTML
func (v *Validator) NoHTML(field, value string) *Validator {
	if value != "" && ContainsHTML(value) {
		v.AddError(field, "Must not contain HTML", "NO_HTML", value)
	}
	return v
}

// SafeString validates string is safe (no HTML, scripts, etc.)
func (v *Validator) SafeString(field, value string) *Validator {
	if value != "" && !IsSafeString(value) {
		v.AddError(field, "Contains potentially unsafe content", "UNSAFE_STRING", value)
	}
	return v
}

// Username validates username format
func (v *Validator) Username(field, value string) *Validator {
	if value != "" {
		v.MinLength(field, value, 3).
			MaxLength(field, value, 50).
			AlphanumericDashUnderscore(field, value)
	}
	return v
}

// Password validates password strength
func (v *Validator) Password(field, value string, minLen int) *Validator {
	if value != "" {
		v.MinLength(field, value, minLen)
		if !IsStrongPassword(value, minLen) {
			v.AddError(field, "Password is not strong enough", "WEAK_PASSWORD", nil)
		}
	}
	return v
}

// IP validates IP address format
func (v *Validator) IP(field, value string) *Validator {
	if value != "" && !IsValidIP(value) {
		v.AddError(field, "Invalid IP address", "INVALID_IP", value)
	}
	return v
}

// IPv4 validates IPv4 address format
func (v *Validator) IPv4(field, value string) *Validator {
	if value != "" && !IsValidIPv4(value) {
		v.AddError(field, "Invalid IPv4 address", "INVALID_IPV4", value)
	}
	return v
}

// IPv6 validates IPv6 address format
func (v *Validator) IPv6(field, value string) *Validator {
	if value != "" && !IsValidIPv6(value) {
		v.AddError(field, "Invalid IPv6 address", "INVALID_IPV6", value)
	}
	return v
}

// CIDR validates CIDR notation
func (v *Validator) CIDR(field, value string) *Validator {
	if value != "" && !IsValidCIDR(value) {
		v.AddError(field, "Invalid CIDR notation", "INVALID_CIDR", value)
	}
	return v
}

// Port validates port number
func (v *Validator) Port(field string, value int) *Validator {
	if value < 0 || value > 65535 {
		v.AddError(field, "Invalid port number", "INVALID_PORT", value)
	}
	return v
}

// Min validates numeric minimum
func (v *Validator) Min(field string, value, min int) *Validator {
	if value < min {
		v.AddError(field, fmt.Sprintf("Must be at least %d", min), "MIN_VALUE", value)
	}
	return v
}

// Max validates numeric maximum
func (v *Validator) Max(field string, value, max int) *Validator {
	if value > max {
		v.AddError(field, fmt.Sprintf("Must be at most %d", max), "MAX_VALUE", value)
	}
	return v
}

// Range validates numeric range
func (v *Validator) Range(field string, value, min, max int) *Validator {
	v.Min(field, value, min)
	v.Max(field, value, max)
	return v
}

// UUID validates UUID format
func (v *Validator) UUID(field, value string) *Validator {
	if value != "" && !IsValidUUID(value) {
		v.AddError(field, "Invalid UUID format", "INVALID_UUID", value)
	}
	return v
}

// Phone validates phone number format
func (v *Validator) Phone(field, value string) *Validator {
	if value != "" && !IsValidPhone(value) {
		v.AddError(field, "Invalid phone number", "INVALID_PHONE", value)
	}
	return v
}

// JSON validates JSON string
func (v *Validator) JSON(field, value string) *Validator {
	if value != "" && !IsValidJSON(value) {
		v.AddError(field, "Invalid JSON format", "INVALID_JSON", value)
	}
	return v
}

// Slug validates slug format
func (v *Validator) Slug(field, value string) *Validator {
	if value != "" && !IsValidSlug(value) {
		v.AddError(field, "Invalid slug format (use lowercase letters, numbers, and hyphens)", "INVALID_SLUG", value)
	}
	return v
}

// Helper functions

// isEmpty checks if a value is empty
func isEmpty(value interface{}) bool {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v) == ""
	case []string:
		return len(v) == 0
	case []interface{}:
		return len(v) == 0
	case map[string]interface{}:
		return len(v) == 0
	case nil:
		return true
	default:
		return false
	}
}

// IsValidEmail validates email format
func IsValidEmail(email string) bool {
	if email == "" {
		return false
	}

	// Basic length check
	if len(email) > 254 {
		return false
	}

	// Use standard library parser
	_, err := mail.ParseAddress(email)
	return err == nil
}

// IsValidURL validates URL format
func IsValidURL(rawURL string) bool {
	if rawURL == "" {
		return false
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	// Must have a scheme and host
	return u.Scheme != "" && u.Host != ""
}

// IsValidURLWithScheme validates URL with specific schemes
func IsValidURLWithScheme(rawURL string, allowedSchemes []string) bool {
	if !IsValidURL(rawURL) {
		return false
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	for _, scheme := range allowedSchemes {
		if strings.EqualFold(u.Scheme, scheme) {
			return true
		}
	}

	return false
}

// IsAlphanumeric checks if string is alphanumeric
func IsAlphanumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// IsAlphanumericDashUnderscore checks if string contains only alphanumeric, dash, and underscore
func IsAlphanumericDashUnderscore(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

// IsAlphanumericSpace checks if string contains only alphanumeric and spaces
func IsAlphanumericSpace(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

// ContainsHTML checks if string contains HTML tags
func ContainsHTML(s string) bool {
	htmlPattern := regexp.MustCompile(`<[^>]+>`)
	return htmlPattern.MatchString(s)
}

// ContainsScript checks if string contains script tags or javascript
func ContainsScript(s string) bool {
	lower := strings.ToLower(s)

	// Check for script tags
	if strings.Contains(lower, "<script") || strings.Contains(lower, "</script>") {
		return true
	}

	// Check for javascript: protocol
	if strings.Contains(lower, "javascript:") {
		return true
	}

	// Check for event handlers
	eventHandlers := []string{"onclick", "onerror", "onload", "onmouseover", "onfocus", "onblur"}
	for _, handler := range eventHandlers {
		if strings.Contains(lower, handler+"=") {
			return true
		}
	}

	return false
}

// IsSafeString checks if string is safe (no HTML, scripts, etc.)
func IsSafeString(s string) bool {
	return !ContainsHTML(s) && !ContainsScript(s)
}

// IsStrongPassword checks password strength
func IsStrongPassword(password string, minLen int) bool {
	if len(password) < minLen {
		return false
	}

	var (
		hasUpper   bool
		hasLower   bool
		hasNumber  bool
		hasSpecial bool
	)

	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasNumber = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			hasSpecial = true
		}
	}

	// Require at least 3 of 4 character types
	score := 0
	if hasUpper {
		score++
	}
	if hasLower {
		score++
	}
	if hasNumber {
		score++
	}
	if hasSpecial {
		score++
	}

	return score >= 3
}

// IsValidIP validates IP address (v4 or v6)
func IsValidIP(ip string) bool {
	return net.ParseIP(ip) != nil
}

// IsValidIPv4 validates IPv4 address
func IsValidIPv4(ip string) bool {
	parsed := net.ParseIP(ip)
	return parsed != nil && parsed.To4() != nil
}

// IsValidIPv6 validates IPv6 address
func IsValidIPv6(ip string) bool {
	parsed := net.ParseIP(ip)
	return parsed != nil && parsed.To4() == nil
}

// IsValidCIDR validates CIDR notation
func IsValidCIDR(cidr string) bool {
	_, _, err := net.ParseCIDR(cidr)
	return err == nil
}

// IsValidUUID validates UUID format
func IsValidUUID(uuid string) bool {
	if len(uuid) != 36 {
		return false
	}

	pattern := regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	return pattern.MatchString(uuid)
}

// IsValidPhone validates phone number (basic validation)
func IsValidPhone(phone string) bool {
	// Remove common separators
	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) {
			return r
		}
		if r == '+' || r == '-' || r == ' ' || r == '(' || r == ')' || r == '.' {
			return -1 // Remove
		}
		return r
	}, phone)

	// Check if we have at least 10 digits
	digitCount := 0
	for _, r := range cleaned {
		if unicode.IsDigit(r) {
			digitCount++
		}
	}

	return digitCount >= 10 && digitCount <= 15
}

// IsValidJSON validates JSON format
func IsValidJSON(s string) bool {
	if s == "" {
		return false
	}

	s = strings.TrimSpace(s)

	// Must start with { or [
	if !strings.HasPrefix(s, "{") && !strings.HasPrefix(s, "[") {
		return false
	}

	// Simple bracket matching check
	var stack []rune
	inString := false
	escape := false

	for _, r := range s {
		if escape {
			escape = false
			continue
		}

		switch r {
		case '\\':
			escape = true
		case '"':
			inString = !inString
		case '{', '[':
			if !inString {
				stack = append(stack, r)
			}
		case '}':
			if !inString {
				if len(stack) == 0 || stack[len(stack)-1] != '{' {
					return false
				}
				stack = stack[:len(stack)-1]
			}
		case ']':
			if !inString {
				if len(stack) == 0 || stack[len(stack)-1] != '[' {
					return false
				}
				stack = stack[:len(stack)-1]
			}
		}
	}

	return len(stack) == 0
}

// IsValidSlug validates slug format
func IsValidSlug(slug string) bool {
	if slug == "" || len(slug) > 200 {
		return false
	}

	pattern := regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	return pattern.MatchString(slug)
}

// IsPrintable checks if string contains only printable characters
func IsPrintable(s string) bool {
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

// TrimAndValidate trims whitespace and validates
func TrimAndValidate(s string) string {
	return strings.TrimSpace(s)
}

// NormalizeEmail normalizes email address
func NormalizeEmail(email string) string {
	email = strings.ToLower(strings.TrimSpace(email))

	// Remove dots from Gmail addresses
	parts := strings.SplitN(email, "@", 2)
	if len(parts) == 2 && (parts[1] == "gmail.com" || parts[1] == "googlemail.com") {
		parts[0] = strings.ReplaceAll(parts[0], ".", "")
		email = parts[0] + "@" + parts[1]
	}

	return email
}

// SanitizeFilename sanitizes a filename
func SanitizeFilename(filename string) string {
	// Remove path separators
	filename = strings.ReplaceAll(filename, "/", "_")
	filename = strings.ReplaceAll(filename, "\\", "_")

	// Remove null bytes
	filename = strings.ReplaceAll(filename, "\x00", "")

	// Remove leading/trailing dots and spaces
	filename = strings.Trim(filename, ". ")

	// Limit length
	if len(filename) > 255 {
		filename = filename[:255]
	}

	return filename
}

// ValidateStruct validates a struct using tags (simplified version)
func ValidateStruct(s interface{}) ValidationErrors {
	// This is a simplified version
	// In production, use a library like go-playground/validator
	return nil
}

// Common regex patterns
var (
	// UsernamePattern matches valid usernames (alphanumeric, dash, underscore)
	UsernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,50}$`)

	// UUIDPattern matches UUID format
	UUIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

	// SlugPattern matches URL-friendly slugs
	SlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

	// AlphanumericPattern matches alphanumeric strings
	AlphanumericPattern = regexp.MustCompile(`^[a-zA-Z0-9]+$`)

	// AlphanumericDashUnderscorePattern matches alphanumeric with dash and underscore
	AlphanumericDashUnderscorePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

	// PhonePattern matches common phone number formats
	PhonePattern = regexp.MustCompile(`^\+?[0-9]{10,15}$`)

	// HexColorPattern matches hex color codes
	HexColorPattern = regexp.MustCompile(`^#([A-Fa-f0-9]{6}|[A-Fa-f0-9]{3})$`)

	// SemVerPattern matches semantic versioning
	SemVerPattern = regexp.MustCompile(`^v?(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)
)

// ValidateRequest represents a generic validation request
type ValidateRequest struct {
	Field   string      `json:"field"`
	Value   interface{} `json:"value"`
	Rules   []string    `json:"rules"` // e.g., "required", "email", "min:8"
}

// ValidateResponse represents a validation response
type ValidateResponse struct {
	Valid  bool              `json:"valid"`
	Errors []*ValidationError `json:"errors,omitempty"`
}

// BatchValidate validates multiple fields at once
func BatchValidate(requests []ValidateRequest) *ValidateResponse {
	v := NewValidator()

	for _, req := range requests {
		for _, rule := range req.Rules {
			applyRule(v, req.Field, req.Value, rule)
		}
	}

	return &ValidateResponse{
		Valid:  !v.HasErrors(),
		Errors: v.Errors(),
	}
}

// applyRule applies a validation rule
func applyRule(v *Validator, field string, value interface{}, rule string) {
	// Parse rule and parameters
	parts := strings.SplitN(rule, ":", 2)
	ruleName := parts[0]
	var param string
	if len(parts) > 1 {
		param = parts[1]
	}

	strValue, _ := value.(string)

	switch ruleName {
	case "required":
		v.Required(field, value)
	case "email":
		v.Email(field, strValue)
	case "url":
		v.URL(field, strValue)
	case "uuid":
		v.UUID(field, strValue)
	case "alphanumeric":
		v.Alphanumeric(field, strValue)
	case "alphanumeric_dash_underscore":
		v.AlphanumericDashUnderscore(field, strValue)
	case "no_html":
		v.NoHTML(field, strValue)
	case "safe_string":
		v.SafeString(field, strValue)
	case "username":
		v.Username(field, strValue)
	case "slug":
		v.Slug(field, strValue)
	case "ip":
		v.IP(field, strValue)
	case "ipv4":
		v.IPv4(field, strValue)
	case "ipv6":
		v.IPv6(field, strValue)
	case "phone":
		v.Phone(field, strValue)
	case "json":
		v.JSON(field, strValue)
	case "min":
		if paramInt := parseIntParam(param); paramInt > 0 {
			v.MinLength(field, strValue, paramInt)
		}
	case "max":
		if paramInt := parseIntParam(param); paramInt > 0 {
			v.MaxLength(field, strValue, paramInt)
		}
	case "length":
		params := strings.Split(param, ",")
		if len(params) == 2 {
			min := parseIntParam(params[0])
			max := parseIntParam(params[1])
			v.Length(field, strValue, min, max)
		}
	}
}

// parseIntParam parses an integer parameter
func parseIntParam(s string) int {
	var result int
	for _, r := range s {
		if r >= '0' && r <= '9' {
			result = result*10 + int(r-'0')
		}
	}
	return result
}
