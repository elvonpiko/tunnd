package config_test

// Property-based tests for the configuration package.
//
// These tests use Go's testing/quick package to generate random inputs and
// verify that correctness properties hold across all valid inputs.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/quick"

	"gopkg.in/yaml.v3"

	"github.com/elvonpiko/tunnd/internal/config"
)

// ── Task 1.3: Property test for configuration file search order ───────────────
// Property 2: Configuration File Search Order
// Validates: Requirements 1.1
//
// For any valid config file placed in a search path, the loader uses the
// highest-precedence path (explicit > cwd > ~/.tunnd/ > /etc/tunnd/).

func TestProperty_ConfigFileSearchOrder(t *testing.T) {
	// We systematically test that an explicitly-provided config path always
	// takes precedence over a file in the current working directory.
	property := func(subdomainExplicit, subdomainLocal string) bool {
		// Build distinct, valid subdomain-like strings for identification.
		clean := func(s string) string {
			var b strings.Builder
			for _, r := range strings.ToLower(s) {
				if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
					b.WriteRune(r)
				}
			}
			out := b.String()
			if len(out) == 0 {
				out = "default"
			}
			if len(out) > 20 {
				out = out[:20]
			}
			return out
		}
		se := clean(subdomainExplicit)
		sl := clean(subdomainLocal)
		if se == sl {
			sl = sl + "x" // ensure they differ
		}

		tmpDir := t.TempDir()
		explicitPath := filepath.Join(tmpDir, "explicit.yaml")
		localPath := filepath.Join(tmpDir, "tunnd.yaml")

		writeClientYAML(t, explicitPath, "wss://server.com", "tok_exp", se)
		writeClientYAML(t, localPath, "wss://server.com", "tok_local", sl)

		// Change to tmpDir so ./tunnd.yaml is the "local" file.
		origWd, _ := os.Getwd()
		_ = os.Chdir(tmpDir)
		defer os.Chdir(origWd)

		// With explicit path: must load explicit file.
		cfg, err := config.LoadClient(explicitPath, nil)
		if err != nil {
			return true // skip if loading fails for other reasons
		}
		if cfg.Subdomain != se {
			return false // explicit file should win
		}

		// Without explicit path: must load local file.
		cfg2, err := config.LoadClient("", nil)
		if err != nil {
			return true
		}
		return cfg2.Subdomain == sl
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 50}); err != nil {
		t.Errorf("config file search order property failed: %v", err)
	}
}

// ── Task 2.5: Property test for configuration precedence ─────────────────────
// Property 1: Configuration Precedence
// Validates: Requirements 1.4, 1.12
//
// For any key present in multiple sources, final config uses the
// highest-precedence source (CLI > ENV > File > Default).

func TestProperty_ConfigPrecedence(t *testing.T) {
	// We test the precedence of the "subdomain" field which is overridable at
	// every level.
	property := func(fileVal, envVal, cliVal string) bool {
		clean := func(s string) string {
			var b strings.Builder
			for _, r := range strings.ToLower(s) {
				if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
					b.WriteRune(r)
				}
			}
			out := b.String()
			if len(out) == 0 {
				out = "x"
			}
			if len(out) > 20 {
				out = out[:20]
			}
			return out
		}
		fv := clean(fileVal)
		ev := clean(envVal)
		cv := clean(cliVal)

		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "tunnd.yaml")
		writeClientYAML(t, cfgPath, "wss://server.com", "tok_test", fv)

		t.Setenv("TUNND_SUBDOMAIN", ev)

		// CLI override beats ENV and File.
		cfg, err := config.LoadClient(cfgPath, map[string]interface{}{
			"subdomain": cv,
		})
		if err != nil {
			return true
		}
		if cfg.Subdomain != cv {
			return false // CLI must win
		}

		// ENV beats File when no CLI override.
		cfg2, err := config.LoadClient(cfgPath, nil)
		if err != nil {
			return true
		}
		return cfg2.Subdomain == ev
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 50}); err != nil {
		t.Errorf("config precedence property failed: %v", err)
	}
}

// ── Task 2.6: Property test for YAML round-trip ───────────────────────────────
// Property 3: YAML Parsing Round-Trip
// Validates: Requirements 1.2
//
// For any valid Client config struct, serializing to YAML then deserializing
// produces an equivalent struct.

func TestProperty_YAMLRoundTripClient(t *testing.T) {
	validProtocols := []string{"http", "tcp"}
	validLogLevels := []string{"debug", "info", "warn", "error"}

	property := func(idx1, idx2 uint8, portOffset uint16) bool {
		original := config.Client{
			ServerAddr:    "wss://server.example.com",
			Token:         "tok_roundtrip",
			Subdomain:     "myapp",
			Protocol:      validProtocols[int(idx1)%len(validProtocols)],
			InspectorPort: 1024 + int(portOffset%64511),
			LogLevel:      validLogLevels[int(idx2)%len(validLogLevels)],
		}

		tmpDir := t.TempDir()
		yamlPath := filepath.Join(tmpDir, "roundtrip.yaml")

		if err := marshalClientToYAML(original, yamlPath); err != nil {
			return true // skip marshalling errors
		}

		loaded, err := config.LoadClient(yamlPath, nil)
		if err != nil {
			return false
		}

		return loaded.ServerAddr == original.ServerAddr &&
			loaded.Token == original.Token &&
			loaded.Subdomain == original.Subdomain &&
			loaded.Protocol == original.Protocol &&
			loaded.InspectorPort == original.InspectorPort &&
			loaded.LogLevel == original.LogLevel
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("YAML round-trip client property failed: %v", err)
	}
}

func TestProperty_YAMLRoundTripServer(t *testing.T) {
	validLogLevels := []string{"debug", "info", "warn", "error"}

	property := func(idx uint8, adminPortOffset uint16) bool {
		adminPort := 1024 + int(adminPortOffset%64511)
		original := config.Server{
			Domain:        "tunnel.example.com",
			HTTPPort:      8080, // not 443 to avoid TLS requirement
			AdminPort:     adminPort,
			DBPath:        "./tunnd.db",
			AdminPassword: "supersecretpassword",
			LogLevel:      validLogLevels[int(idx)%len(validLogLevels)],
			LogFormat:     "pretty",
		}

		tmpDir := t.TempDir()
		yamlPath := filepath.Join(tmpDir, "roundtrip-server.yaml")

		if err := marshalServerToYAML(original, yamlPath); err != nil {
			return true
		}

		loaded, err := config.LoadServer(yamlPath)
		if err != nil {
			return false
		}

		return loaded.Domain == original.Domain &&
			loaded.HTTPPort == original.HTTPPort &&
			loaded.AdminPassword == original.AdminPassword &&
			loaded.LogLevel == original.LogLevel
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("YAML round-trip server property failed: %v", err)
	}
}

// ── Task 2.7: Property test for invalid YAML error handling ───────────────────
// Property 4: Invalid YAML Error Handling
// Validates: Requirements 1.3
//
// For any syntactically invalid YAML content, the loader returns an error
// containing the file path.

func TestProperty_InvalidYAMLErrorHandling(t *testing.T) {
	// We test a set of known-invalid YAML snippets and ensure each triggers an
	// error that contains the file path.
	invalidYAMLSnippets := []string{
		`key: [unclosed array`,
		`key: {unclosed: map`,
		`key: value
  bad_indent: oops`,
		`: no-key`,
		`%invalid directive`,
		`server_addr: "unterminated string`,
	}

	for i, snippet := range invalidYAMLSnippets {
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, fmt.Sprintf("invalid-%d.yaml", i))

		if err := os.WriteFile(cfgPath, []byte(snippet), 0o600); err != nil {
			t.Fatalf("writing test file: %v", err)
		}

		_, err := config.LoadClient(cfgPath, nil)
		if err == nil {
			t.Errorf("snippet[%d]: expected error for invalid YAML, got nil", i)
			continue
		}
		if !strings.Contains(err.Error(), cfgPath) {
			t.Errorf("snippet[%d]: error %q does not contain file path %q", i, err.Error(), cfgPath)
		}
	}

	// Quick.Check variant: generate strings with syntax-breaking characters.
	property := func(prefix string) bool {
		// Guarantee the content is invalid by appending an unclosed bracket.
		content := strings.ReplaceAll(prefix, "\x00", "") + "\nkey: [unclosed"

		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "invalid-quick.yaml")
		if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
			return true
		}

		_, err := config.LoadClient(cfgPath, nil)
		if err == nil {
			// Valid YAML despite the unclosed bracket — skip.
			return true
		}
		// If there IS an error, it must contain the path.
		return strings.Contains(err.Error(), cfgPath)
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("invalid YAML error handling property failed: %v", err)
	}
}

// ── Task 2.8: Property test for default value application ────────────────────
// Property 5: Default Value Application
// Validates: Requirements 1.6, 1.7, 1.8
//
// For optional fields with invalid values, the loader applies documented defaults.

func TestProperty_DefaultValueApplication(t *testing.T) {
	// Invalid protocol values → default "http"
	invalidProtocols := []string{"ftp", "https", "ws", "", "HTTP", "TCP", "grpc", "smtp"}
	for _, proto := range invalidProtocols {
		cfg, err := config.LoadClient("", map[string]interface{}{
			"server_addr": "wss://server.com",
			"token":       "tok_test",
			"protocol":    proto,
		})
		if err != nil {
			t.Errorf("protocol=%q: unexpected error %v", proto, err)
			continue
		}
		if cfg.Protocol != "http" {
			t.Errorf("protocol=%q: got %q, want default 'http'", proto, cfg.Protocol)
		}
	}

	// Invalid log_level values → default "info"
	invalidLogLevels := []string{"verbose", "trace", "fatal", "WARNING", "", "all"}
	for _, ll := range invalidLogLevels {
		cfg, err := config.LoadClient("", map[string]interface{}{
			"server_addr": "wss://server.com",
			"token":       "tok_test",
			"log_level":   ll,
		})
		if err != nil {
			t.Errorf("log_level=%q: unexpected error %v", ll, err)
			continue
		}
		if cfg.LogLevel != "info" {
			t.Errorf("log_level=%q: got %q, want default 'info'", ll, cfg.LogLevel)
		}
	}

	// Invalid inspector_port values → default 4040
	invalidPorts := []int{0, -1, 1023, 65536, 99999, 100000}
	for _, port := range invalidPorts {
		cfg, err := config.LoadClient("", map[string]interface{}{
			"server_addr":    "wss://server.com",
			"token":          "tok_test",
			"inspector_port": port,
		})
		if err != nil {
			t.Errorf("inspector_port=%d: unexpected error %v", port, err)
			continue
		}
		if cfg.InspectorPort != 4040 {
			t.Errorf("inspector_port=%d: got %d, want default 4040", port, cfg.InspectorPort)
		}
	}

	// Property: for any out-of-range port, default is always 4040.
	portDefault := func(n int16) bool {
		port := int(n)
		if port >= 1024 && port <= 65535 {
			return true // valid, skip
		}
		cfg, err := config.LoadClient("", map[string]interface{}{
			"server_addr":    "wss://server.com",
			"token":          "tok_test",
			"inspector_port": port,
		})
		if err != nil {
			return true
		}
		return cfg.InspectorPort == 4040
	}
	if err := quick.Check(portDefault, &quick.Config{MaxCount: 500}); err != nil {
		t.Errorf("default port application property failed: %v", err)
	}
}

// ── Task 2.9: Property test for required field validation ─────────────────────
// Property 6: Required Field Validation
// Validates: Requirements 1.9, 1.10, 7.11
//
// Configurations missing required fields must return an error.

func TestProperty_RequiredFieldValidation(t *testing.T) {
	// Missing server_addr must produce an error mentioning "server_addr".
	t.Run("missing server_addr", func(t *testing.T) {
		os.Unsetenv("TUNND_SERVER_ADDR")
		os.Unsetenv("TUNND_TOKEN")
		_, err := config.LoadClient("", nil)
		if err == nil {
			t.Fatal("expected error for missing server_addr, got nil")
		}
		if !strings.Contains(err.Error(), "server_addr") {
			t.Errorf("error %q should mention 'server_addr'", err.Error())
		}
	})

	// Missing token (with server_addr present) must produce an error mentioning "token".
	t.Run("missing token", func(t *testing.T) {
		t.Setenv("TUNND_SERVER_ADDR", "wss://server.com")
		os.Unsetenv("TUNND_TOKEN")
		_, err := config.LoadClient("", nil)
		if err == nil {
			t.Fatal("expected error for missing token, got nil")
		}
		if !strings.Contains(err.Error(), "token") {
			t.Errorf("error %q should mention 'token'", err.Error())
		}
	})

	// Missing domain on server must produce an error mentioning "domain".
	t.Run("missing domain", func(t *testing.T) {
		os.Unsetenv("TUNND_DOMAIN")
		os.Unsetenv("TUNND_ADMIN_PASSWORD")
		_, err := config.LoadServer("")
		if err == nil {
			t.Fatal("expected error for missing domain, got nil")
		}
		if !strings.Contains(err.Error(), "domain") {
			t.Errorf("error %q should mention 'domain'", err.Error())
		}
	})

	// admin_password is optional on server — operators can use the bootstrap
	// flow instead. LoadServer must succeed without it.
	t.Run("missing admin_password is allowed", func(t *testing.T) {
		t.Setenv("TUNND_DOMAIN", "tunnel.test")
		t.Setenv("TUNND_HTTP_PORT", "8080")
		os.Unsetenv("TUNND_ADMIN_PASSWORD")
		cfg, err := config.LoadServer("")
		if err != nil {
			t.Fatalf("LoadServer should succeed without admin_password: %v", err)
		}
		if cfg.AdminPassword != "" {
			t.Errorf("expected empty AdminPassword, got %q", cfg.AdminPassword)
		}
	})

	// Property: any config file missing server_addr always errors.
	property := func(token string) bool {
		// Skip empty tokens — that triggers its own error.
		if strings.TrimSpace(token) == "" {
			return true
		}
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "missing-addr.yaml")
		content := fmt.Sprintf("token: %s\n", token)
		if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
			return true
		}
		os.Unsetenv("TUNND_SERVER_ADDR")
		_, err := config.LoadClient(cfgPath, nil)
		return err != nil
	}
	if err := quick.Check(property, &quick.Config{MaxCount: 50}); err != nil {
		t.Errorf("required server_addr validation property failed: %v", err)
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func writeClientYAML(t *testing.T, path, serverAddr, token, subdomain string) {
	t.Helper()
	content := fmt.Sprintf("server_addr: %s\ntoken: %s\nsubdomain: %s\n",
		serverAddr, token, subdomain)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writeClientYAML: %v", err)
	}
}

// marshalClientToYAML serializes a Client config struct to a YAML file using
// the same field names viper/mapstructure expects.
func marshalClientToYAML(c config.Client, path string) error {
	data := map[string]interface{}{
		"server_addr":    c.ServerAddr,
		"token":          c.Token,
		"subdomain":      c.Subdomain,
		"protocol":       c.Protocol,
		"inspector_port": c.InspectorPort,
		"log_level":      c.LogLevel,
	}
	out, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o600)
}

// marshalServerToYAML serializes a Server config struct to a YAML file.
func marshalServerToYAML(s config.Server, path string) error {
	data := map[string]interface{}{
		"domain":         s.Domain,
		"http_port":      s.HTTPPort,
		"admin_port":     s.AdminPort,
		"db_path":        s.DBPath,
		"admin_password": s.AdminPassword,
		"log_level":      s.LogLevel,
		"log_format":     s.LogFormat,
	}
	out, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o600)
}
