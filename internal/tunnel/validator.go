package tunnel

import (
	"fmt"
	"regexp"
	"strings"
)

// SubdomainValidator validates and sanitizes subdomain names according to DNS rules.
type SubdomainValidator struct {
	reservedNames map[string]bool
}

// NewSubdomainValidator creates a new SubdomainValidator with the given reserved subdomain names.
func NewSubdomainValidator(reserved []string) *SubdomainValidator {
	reservedMap := make(map[string]bool, len(reserved))
	for _, name := range reserved {
		// Store reserved names in lowercase for case-insensitive comparison
		reservedMap[strings.ToLower(name)] = true
	}
	return &SubdomainValidator{
		reservedNames: reservedMap,
	}
}

// ValidateAndSanitize performs sanitization and validation on a subdomain string.
// It returns the sanitized subdomain and an error if validation fails.
//
// Sanitization steps:
//   - Trim leading and trailing whitespace
//   - Convert to lowercase
//
// Validation rules:
//   - Must not be empty after sanitization
//   - Length must be between 3 and 63 characters
//   - Must contain only lowercase letters (a-z), digits (0-9), and hyphens (-)
//   - Must not start with a hyphen
//   - Must not end with a hyphen
//   - Must not contain consecutive hyphens
//   - Must not be in the reserved names list
func (v *SubdomainValidator) ValidateAndSanitize(subdomain string) (string, error) {
	// Sanitization: trim whitespace and convert to lowercase
	sanitized := strings.ToLower(strings.TrimSpace(subdomain))

	// Validation: check if empty
	if sanitized == "" {
		return "", &ValidationError{
			Code:    "invalid_subdomain",
			Message: "subdomain cannot be empty",
		}
	}

	// Validation: check length (3-63 characters)
	if len(sanitized) < 3 {
		return "", &ValidationError{
			Code:    "invalid_subdomain",
			Message: "subdomain must be between 3 and 63 characters",
		}
	}
	if len(sanitized) > 63 {
		return "", &ValidationError{
			Code:    "invalid_subdomain",
			Message: "subdomain must be between 3 and 63 characters",
		}
	}

	// Validation: check for valid characters (a-z, 0-9, -)
	validChars := regexp.MustCompile(`^[a-z0-9-]+$`)
	if !validChars.MatchString(sanitized) {
		return "", &ValidationError{
			Code:    "invalid_subdomain",
			Message: "subdomain contains invalid characters: only a-z, 0-9, and - are allowed",
		}
	}

	// Validation: must not start with hyphen
	if strings.HasPrefix(sanitized, "-") {
		return "", &ValidationError{
			Code:    "invalid_subdomain",
			Message: "subdomain cannot start with a hyphen",
		}
	}

	// Validation: must not end with hyphen
	if strings.HasSuffix(sanitized, "-") {
		return "", &ValidationError{
			Code:    "invalid_subdomain",
			Message: "subdomain cannot end with a hyphen",
		}
	}

	// Validation: must not contain consecutive hyphens
	if strings.Contains(sanitized, "--") {
		return "", &ValidationError{
			Code:    "invalid_subdomain",
			Message: "subdomain cannot contain consecutive hyphens",
		}
	}

	// Validation: must not be reserved
	if v.reservedNames[sanitized] {
		return "", &ValidationError{
			Code:    "invalid_subdomain",
			Message: fmt.Sprintf("subdomain '%s' is reserved", sanitized),
		}
	}

	return sanitized, nil
}

// ValidationError represents a subdomain validation error.
type ValidationError struct {
	Code    string
	Message string
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	return e.Message
}
