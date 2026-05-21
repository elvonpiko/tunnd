package tunnel_test

import (
	"strings"
	"testing"
	"testing/quick"
	"unicode"

	"github.com/elvonpiko/tunnd/internal/tunnel"
)

// ── Sanitization ──────────────────────────────────────────────────────────────

func TestValidateAndSanitize_TrimsWhitespace(t *testing.T) {
	v := tunnel.NewSubdomainValidator(nil)

	tests := []struct {
		input string
		want  string
	}{
		{"  myapp  ", "myapp"},
		{"\tmyapp\n", "myapp"},
		{"  my-app  ", "my-app"},
		{"myapp", "myapp"},
	}

	for _, tt := range tests {
		got, err := v.ValidateAndSanitize(tt.input)
		if err != nil {
			t.Errorf("ValidateAndSanitize(%q) error = %v, want nil", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ValidateAndSanitize(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestValidateAndSanitize_ConvertsToLowercase(t *testing.T) {
	v := tunnel.NewSubdomainValidator(nil)

	tests := []struct {
		input string
		want  string
	}{
		{"MyApp", "myapp"},
		{"MYAPP", "myapp"},
		{"MyApp123", "myapp123"},
		{"My-App", "my-app"},
	}

	for _, tt := range tests {
		got, err := v.ValidateAndSanitize(tt.input)
		if err != nil {
			t.Errorf("ValidateAndSanitize(%q) error = %v, want nil", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ValidateAndSanitize(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestValidateAndSanitize_SanitizationIsIdempotent(t *testing.T) {
	v := tunnel.NewSubdomainValidator(nil)

	input := "  MyApp  "
	first, err := v.ValidateAndSanitize(input)
	if err != nil {
		t.Fatalf("first ValidateAndSanitize(%q) error = %v", input, err)
	}

	second, err := v.ValidateAndSanitize(first)
	if err != nil {
		t.Fatalf("second ValidateAndSanitize(%q) error = %v", first, err)
	}

	if first != second {
		t.Errorf("sanitization not idempotent: first = %q, second = %q", first, second)
	}
}

// ── Empty validation ──────────────────────────────────────────────────────────

func TestValidateAndSanitize_RejectsEmptyString(t *testing.T) {
	v := tunnel.NewSubdomainValidator(nil)

	tests := []string{"", "   ", "\t\n"}

	for _, input := range tests {
		_, err := v.ValidateAndSanitize(input)
		if err == nil {
			t.Errorf("ValidateAndSanitize(%q) expected error, got nil", input)
			continue
		}
		if !strings.Contains(err.Error(), "cannot be empty") {
			t.Errorf("ValidateAndSanitize(%q) error = %v, want 'cannot be empty'", input, err)
		}
	}
}

// ── Length validation ─────────────────────────────────────────────────────────

func TestValidateAndSanitize_RejectsTooShort(t *testing.T) {
	v := tunnel.NewSubdomainValidator(nil)

	tests := []string{"ab", "a", "12"}

	for _, input := range tests {
		_, err := v.ValidateAndSanitize(input)
		if err == nil {
			t.Errorf("ValidateAndSanitize(%q) expected error, got nil", input)
			continue
		}
		if !strings.Contains(err.Error(), "between 3 and 63 characters") {
			t.Errorf("ValidateAndSanitize(%q) error = %v, want length error", input, err)
		}
	}
}

func TestValidateAndSanitize_RejectsTooLong(t *testing.T) {
	v := tunnel.NewSubdomainValidator(nil)

	// 64 characters (exceeds 63 limit)
	input := strings.Repeat("a", 64)

	_, err := v.ValidateAndSanitize(input)
	if err == nil {
		t.Errorf("ValidateAndSanitize(%q) expected error, got nil", input)
	}
	if err != nil && !strings.Contains(err.Error(), "between 3 and 63 characters") {
		t.Errorf("ValidateAndSanitize(%q) error = %v, want length error", input, err)
	}
}

func TestValidateAndSanitize_AcceptsValidLengths(t *testing.T) {
	v := tunnel.NewSubdomainValidator(nil)

	tests := []string{
		"abc",                   // 3 characters (minimum)
		"myapp",                 // typical length
		strings.Repeat("a", 63), // 63 characters (maximum)
	}

	for _, input := range tests {
		_, err := v.ValidateAndSanitize(input)
		if err != nil {
			t.Errorf("ValidateAndSanitize(%q) unexpected error = %v", input, err)
		}
	}
}

// ── Character validation ──────────────────────────────────────────────────────

func TestValidateAndSanitize_RejectsInvalidCharacters(t *testing.T) {
	v := tunnel.NewSubdomainValidator(nil)

	tests := []string{
		"my_app",    // underscore
		"my.app",    // dot
		"my app",    // space
		"my@app",    // at sign
		"my#app",    // hash
		"my$app",    // dollar
		"my%app",    // percent
		"my&app",    // ampersand
		"my*app",    // asterisk
		"my+app",    // plus
		"my=app",    // equals
		"my!app",    // exclamation
		"my?app",    // question mark
		"my/app",    // slash
		"my\\app",   // backslash
		"my|app",    // pipe
		"my[app]",   // brackets
		"my{app}",   // braces
		"my(app)",   // parentheses
		"my<app>",   // angle brackets
		"my\"app\"", // quotes
		"my'app'",   // single quotes
		"my`app`",   // backticks
		"my~app",    // tilde
		"my^app",    // caret
		"myapp;",    // semicolon
		"myapp:",    // colon
		"myapp,",    // comma
	}

	for _, input := range tests {
		_, err := v.ValidateAndSanitize(input)
		if err == nil {
			t.Errorf("ValidateAndSanitize(%q) expected error, got nil", input)
			continue
		}
		if !strings.Contains(err.Error(), "invalid characters") {
			t.Errorf("ValidateAndSanitize(%q) error = %v, want 'invalid characters'", input, err)
		}
	}
}

func TestValidateAndSanitize_AcceptsValidCharacters(t *testing.T) {
	v := tunnel.NewSubdomainValidator(nil)

	tests := []string{
		"abc",
		"myapp",
		"my-app",
		"my-app-123",
		"app123",
		"123app",
		"a1b2c3",
	}

	for _, input := range tests {
		_, err := v.ValidateAndSanitize(input)
		if err != nil {
			t.Errorf("ValidateAndSanitize(%q) unexpected error = %v", input, err)
		}
	}
}

// ── Hyphen position validation ────────────────────────────────────────────────

func TestValidateAndSanitize_RejectsStartingHyphen(t *testing.T) {
	v := tunnel.NewSubdomainValidator(nil)

	tests := []string{"-myapp", "-app", "-my-app"}

	for _, input := range tests {
		_, err := v.ValidateAndSanitize(input)
		if err == nil {
			t.Errorf("ValidateAndSanitize(%q) expected error, got nil", input)
			continue
		}
		if !strings.Contains(err.Error(), "cannot start with a hyphen") {
			t.Errorf("ValidateAndSanitize(%q) error = %v, want 'cannot start with a hyphen'", input, err)
		}
	}
}

func TestValidateAndSanitize_RejectsEndingHyphen(t *testing.T) {
	v := tunnel.NewSubdomainValidator(nil)

	tests := []string{"myapp-", "app-", "my-app-"}

	for _, input := range tests {
		_, err := v.ValidateAndSanitize(input)
		if err == nil {
			t.Errorf("ValidateAndSanitize(%q) expected error, got nil", input)
			continue
		}
		if !strings.Contains(err.Error(), "cannot end with a hyphen") {
			t.Errorf("ValidateAndSanitize(%q) error = %v, want 'cannot end with a hyphen'", input, err)
		}
	}
}

func TestValidateAndSanitize_RejectsConsecutiveHyphens(t *testing.T) {
	v := tunnel.NewSubdomainValidator(nil)

	tests := []string{"my--app", "app--123", "my---app", "a--b"}

	for _, input := range tests {
		_, err := v.ValidateAndSanitize(input)
		if err == nil {
			t.Errorf("ValidateAndSanitize(%q) expected error, got nil", input)
			continue
		}
		if !strings.Contains(err.Error(), "consecutive hyphens") {
			t.Errorf("ValidateAndSanitize(%q) error = %v, want 'consecutive hyphens'", input, err)
		}
	}
}

func TestValidateAndSanitize_AcceptsSingleHyphens(t *testing.T) {
	v := tunnel.NewSubdomainValidator(nil)

	tests := []string{"my-app", "my-app-123", "a-b-c", "app-1-2-3"}

	for _, input := range tests {
		_, err := v.ValidateAndSanitize(input)
		if err != nil {
			t.Errorf("ValidateAndSanitize(%q) unexpected error = %v", input, err)
		}
	}
}

// ── Reserved names validation ─────────────────────────────────────────────────

func TestValidateAndSanitize_RejectsReservedNames(t *testing.T) {
	reserved := []string{"www", "api", "admin", "mail", "ftp"}
	v := tunnel.NewSubdomainValidator(reserved)

	for _, name := range reserved {
		_, err := v.ValidateAndSanitize(name)
		if err == nil {
			t.Errorf("ValidateAndSanitize(%q) expected error, got nil", name)
			continue
		}
		if !strings.Contains(err.Error(), "reserved") {
			t.Errorf("ValidateAndSanitize(%q) error = %v, want 'reserved'", name, err)
		}
	}
}

func TestValidateAndSanitize_ReservedNamesCaseInsensitive(t *testing.T) {
	reserved := []string{"www", "api", "admin"}
	v := tunnel.NewSubdomainValidator(reserved)

	tests := []string{"WWW", "Api", "ADMIN", "AdMiN"}

	for _, input := range tests {
		_, err := v.ValidateAndSanitize(input)
		if err == nil {
			t.Errorf("ValidateAndSanitize(%q) expected error, got nil", input)
			continue
		}
		if !strings.Contains(err.Error(), "reserved") {
			t.Errorf("ValidateAndSanitize(%q) error = %v, want 'reserved'", input, err)
		}
	}
}

func TestValidateAndSanitize_AcceptsNonReservedNames(t *testing.T) {
	reserved := []string{"www", "api", "admin"}
	v := tunnel.NewSubdomainValidator(reserved)

	tests := []string{"myapp", "webapp", "api-v2", "admin-panel"}

	for _, input := range tests {
		_, err := v.ValidateAndSanitize(input)
		if err != nil {
			t.Errorf("ValidateAndSanitize(%q) unexpected error = %v", input, err)
		}
	}
}

// ── NewSubdomainValidator ─────────────────────────────────────────────────────

func TestNewSubdomainValidator_EmptyReservedList(t *testing.T) {
	v := tunnel.NewSubdomainValidator([]string{})
	if v == nil {
		t.Fatal("NewSubdomainValidator returned nil")
	}

	// Should accept any valid subdomain
	_, err := v.ValidateAndSanitize("www")
	if err != nil {
		t.Errorf("ValidateAndSanitize with empty reserved list: unexpected error = %v", err)
	}
}

func TestNewSubdomainValidator_NilReservedList(t *testing.T) {
	v := tunnel.NewSubdomainValidator(nil)
	if v == nil {
		t.Fatal("NewSubdomainValidator returned nil")
	}

	// Should accept any valid subdomain
	_, err := v.ValidateAndSanitize("www")
	if err != nil {
		t.Errorf("ValidateAndSanitize with nil reserved list: unexpected error = %v", err)
	}
}

// ── ValidationError ───────────────────────────────────────────────────────────

func TestValidationError_ImplementsError(t *testing.T) {
	v := tunnel.NewSubdomainValidator(nil)

	_, err := v.ValidateAndSanitize("")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Verify error message is accessible
	msg := err.Error()
	if msg == "" {
		t.Error("error message is empty")
	}
}

// ── Property-based tests ──────────────────────────────────────────────────────

// Task 3.5: Property test for sanitization idempotence
// Property 7: Subdomain Sanitization Idempotence
// Validates: Requirements 10.1, 10.2
func TestProperty_SanitizationIdempotence(t *testing.T) {
	v := tunnel.NewSubdomainValidator(nil)

	// sanitize applies trim+lowercase exactly as ValidateAndSanitize does
	sanitize := func(s string) string {
		return strings.ToLower(strings.TrimSpace(s))
	}

	property := func(input string) bool {
		first := sanitize(input)
		second := sanitize(first)
		return first == second
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 1000}); err != nil {
		t.Errorf("sanitization idempotence failed: %v", err)
	}

	// Also verify through the actual validator for strings that pass validation
	propertyViaValidator := func(input string) bool {
		first, err := v.ValidateAndSanitize(input)
		if err != nil {
			// Invalid input — idempotence only concerns the sanitization step,
			// which is the trim+lowercase applied before any validation check.
			return true
		}
		second, err := v.ValidateAndSanitize(first)
		if err != nil {
			return false // sanitized output should itself be valid
		}
		return first == second
	}

	if err := quick.Check(propertyViaValidator, &quick.Config{MaxCount: 1000}); err != nil {
		t.Errorf("sanitization idempotence via validator failed: %v", err)
	}
}

// Task 3.6: Property test for character validation
// Property 8: Subdomain Character Validation
// Validates: Requirements 4.5, 10.4
func TestProperty_CharacterValidation(t *testing.T) {
	v := tunnel.NewSubdomainValidator(nil)

	// Generate subdomains containing at least one invalid character (not a-z, 0-9, -)
	// and verify they are rejected with "invalid_subdomain".
	property := func(prefix, suffix string, invalid rune) bool {
		// Only test invalid runes that are not whitespace
		// (whitespace is handled by sanitization / empty check, not char validation).
		if unicode.IsSpace(invalid) {
			return true
		}
		// Build a 3-63 char subdomain with an invalid character in the middle.
		// Keep prefix/suffix to lowercase+digits to avoid other validation triggers.
		clean := func(s string) string {
			var b strings.Builder
			for _, r := range strings.ToLower(s) {
				if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
					b.WriteRune(r)
				}
			}
			return b.String()
		}
		p := clean(prefix)
		if len(p) == 0 {
			p = "a"
		}
		s := clean(suffix)
		if len(s) == 0 {
			s = "b"
		}
		// The invalid rune must be outside [a-z0-9-]
		if (invalid >= 'a' && invalid <= 'z') || (invalid >= '0' && invalid <= '9') || invalid == '-' {
			return true
		}
		candidate := p + string(invalid) + s
		// Must be 3-63 chars to avoid length errors.
		if len(candidate) < 3 || len(candidate) > 63 {
			return true
		}
		_, err := v.ValidateAndSanitize(candidate)
		if err == nil {
			return false // must be rejected
		}
		ve, ok := err.(*tunnel.ValidationError)
		return ok && ve.Code == "invalid_subdomain"
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 500}); err != nil {
		t.Errorf("character validation property failed: %v", err)
	}
}

// Task 3.7: Property test for length validation
// Property 9: Subdomain Length Validation
// Validates: Requirements 4.6, 10.8
func TestProperty_LengthValidation(t *testing.T) {
	v := tunnel.NewSubdomainValidator(nil)

	// Subdomains shorter than 3 chars (non-empty) must be rejected.
	tooShort := func(n uint8) bool {
		length := int(n%2) + 1 // 1 or 2
		s := strings.Repeat("a", length)
		_, err := v.ValidateAndSanitize(s)
		if err == nil {
			return false
		}
		return strings.Contains(err.Error(), "between 3 and 63 characters")
	}
	if err := quick.Check(tooShort, &quick.Config{MaxCount: 200}); err != nil {
		t.Errorf("too-short length validation failed: %v", err)
	}

	// Subdomains longer than 63 chars must be rejected.
	tooLong := func(n uint8) bool {
		length := 64 + int(n) // 64-319 chars
		s := strings.Repeat("a", length)
		_, err := v.ValidateAndSanitize(s)
		if err == nil {
			return false
		}
		return strings.Contains(err.Error(), "between 3 and 63 characters")
	}
	if err := quick.Check(tooLong, &quick.Config{MaxCount: 200}); err != nil {
		t.Errorf("too-long length validation failed: %v", err)
	}

	// Valid lengths (3-63) should not fail with a length error.
	validLength := func(n uint8) bool {
		length := 3 + int(n%61) // 3-63
		s := strings.Repeat("a", length)
		_, err := v.ValidateAndSanitize(s)
		if err != nil && strings.Contains(err.Error(), "between 3 and 63 characters") {
			return false
		}
		return true
	}
	if err := quick.Check(validLength, &quick.Config{MaxCount: 200}); err != nil {
		t.Errorf("valid-length should not get length error: %v", err)
	}
}

// Task 3.8: Property test for hyphen position validation
// Property 10: Subdomain Hyphen Position Validation
// Validates: Requirements 10.5, 10.6, 10.7
func TestProperty_HyphenPositionValidation(t *testing.T) {
	v := tunnel.NewSubdomainValidator(nil)

	// Starts-with-hyphen must be rejected.
	startHyphen := func(body string) bool {
		clean := func(s string) string {
			var b strings.Builder
			for _, r := range strings.ToLower(s) {
				if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
					b.WriteRune(r)
				}
			}
			return b.String()
		}
		b := clean(body)
		if len(b) == 0 {
			b = "ab"
		}
		candidate := "-" + b
		if len(candidate) < 3 || len(candidate) > 63 {
			return true
		}
		_, err := v.ValidateAndSanitize(candidate)
		if err == nil {
			return false
		}
		return strings.Contains(err.Error(), "cannot start with a hyphen")
	}
	if err := quick.Check(startHyphen, &quick.Config{MaxCount: 500}); err != nil {
		t.Errorf("starts-with-hyphen property failed: %v", err)
	}

	// Ends-with-hyphen must be rejected.
	endHyphen := func(body string) bool {
		clean := func(s string) string {
			var b strings.Builder
			for _, r := range strings.ToLower(s) {
				if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
					b.WriteRune(r)
				}
			}
			return b.String()
		}
		b := clean(body)
		if len(b) == 0 {
			b = "ab"
		}
		candidate := b + "-"
		if len(candidate) < 3 || len(candidate) > 63 {
			return true
		}
		_, err := v.ValidateAndSanitize(candidate)
		if err == nil {
			return false
		}
		return strings.Contains(err.Error(), "cannot end with a hyphen")
	}
	if err := quick.Check(endHyphen, &quick.Config{MaxCount: 500}); err != nil {
		t.Errorf("ends-with-hyphen property failed: %v", err)
	}

	// Consecutive hyphens must be rejected.
	consecHyphens := func(left, right string) bool {
		clean := func(s string) string {
			var b strings.Builder
			for _, r := range strings.ToLower(s) {
				if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
					b.WriteRune(r)
				}
			}
			return b.String()
		}
		l := clean(left)
		if len(l) == 0 {
			l = "a"
		}
		r := clean(right)
		if len(r) == 0 {
			r = "b"
		}
		candidate := l + "--" + r
		if len(candidate) < 3 || len(candidate) > 63 {
			return true
		}
		_, err := v.ValidateAndSanitize(candidate)
		if err == nil {
			return false
		}
		return strings.Contains(err.Error(), "consecutive hyphens")
	}
	if err := quick.Check(consecHyphens, &quick.Config{MaxCount: 500}); err != nil {
		t.Errorf("consecutive-hyphens property failed: %v", err)
	}
}

// Task 3.9: Property test for reserved subdomain rejection
// Property 11: Reserved Subdomain Rejection
// Validates: Requirements 10.9
func TestProperty_ReservedSubdomainRejection(t *testing.T) {
	reserved := []string{"www", "api", "admin", "mail", "ftp"}
	v := tunnel.NewSubdomainValidator(reserved)

	// Every reserved name (and its uppercase variants) must be rejected with "invalid_subdomain".
	for _, name := range reserved {
		name := name // capture
		_, err := v.ValidateAndSanitize(name)
		if err == nil {
			t.Errorf("reserved name %q was accepted, want rejection", name)
			continue
		}
		ve, ok := err.(*tunnel.ValidationError)
		if !ok || ve.Code != "invalid_subdomain" {
			t.Errorf("reserved name %q: got error %v, want ValidationError with code 'invalid_subdomain'", name, err)
		}
		if !strings.Contains(err.Error(), "reserved") {
			t.Errorf("reserved name %q: error %q does not mention 'reserved'", name, err.Error())
		}
	}

	// Property: any reserved name, regardless of case, must still be rejected.
	caseVariants := func(idx uint8, upper bool) bool {
		name := reserved[int(idx)%len(reserved)]
		var candidate string
		if upper {
			candidate = strings.ToUpper(name)
		} else {
			candidate = name
		}
		_, err := v.ValidateAndSanitize(candidate)
		if err == nil {
			return false
		}
		ve, ok := err.(*tunnel.ValidationError)
		return ok && ve.Code == "invalid_subdomain"
	}
	if err := quick.Check(caseVariants, &quick.Config{MaxCount: 200}); err != nil {
		t.Errorf("reserved name case variant property failed: %v", err)
	}
}
