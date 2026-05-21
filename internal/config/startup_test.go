package config_test

// Integration tests for server startup validation.
//
// These tests exercise the validation path that config.LoadServer performs,
// mirroring the checks that cmd/server/main.go performs before accepting
// connections.  They validate Requirements 2.5, 2.6, 2.7, 2.13, 2.14.
//
// Note: probePort (in cmd/server/main.go) is an unexported function in package
// main; it cannot be called directly from an external test package.  We test
// the equivalent port-conflict detection using standard library primitives, and
// we test that config.LoadServer rejects bad configurations before a port-probe
// would even be reached.

import (
	"fmt"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/elvonpiko/tunnd/internal/config"
)

// ── Startup validation via config.LoadServer ──────────────────────────────────

// TestStartup_FailsWithoutDomain verifies Requirements 2.14:
// If TUNND_DOMAIN is not set, the server must fail to start with a descriptive
// error mentioning "domain".
func TestStartup_FailsWithoutDomain(t *testing.T) {
	os.Unsetenv("TUNND_DOMAIN")
	os.Unsetenv("TUNND_ADMIN_PASSWORD")

	_, err := config.LoadServer("")
	if err == nil {
		t.Fatal("expected error when domain is not configured, got nil")
	}
	if !contains(err.Error(), "domain") {
		t.Errorf("error message should mention 'domain', got: %v", err)
	}
}

// TestStartup_AdminPasswordOptional verifies that admin_password is no longer
// a hard requirement at startup — operators can leave it empty and use the
// first-run bootstrap dashboard to set one. The legacy "fails without
// admin_password" assertion was tightened during the bootstrap-flow redesign.
func TestStartup_AdminPasswordOptional(t *testing.T) {
	t.Setenv("TUNND_DOMAIN", "tunnel.test")
	t.Setenv("TUNND_HTTP_PORT", "8080")
	os.Unsetenv("TUNND_ADMIN_PASSWORD")

	cfg, err := config.LoadServer("")
	if err != nil {
		t.Fatalf("LoadServer should succeed without admin_password, got: %v", err)
	}
	if cfg.AdminPassword != "" {
		t.Errorf("expected empty AdminPassword, got %q", cfg.AdminPassword)
	}
}

// TestStartup_FailsOnPort443WithoutTLS verifies Requirement 2.5:
// Running on port 443 without TLS configuration is invalid — the server must
// reject this before attempting to bind.
func TestStartup_FailsOnPort443WithoutTLS(t *testing.T) {
	t.Setenv("TUNND_DOMAIN", "tunnel.test")
	t.Setenv("TUNND_HTTP_PORT", "443")
	t.Setenv("TUNND_ADMIN_PASSWORD", "supersecretpass1")
	os.Unsetenv("TUNND_TLS_EMAIL")
	os.Unsetenv("TUNND_TLS_CERT_FILE")
	os.Unsetenv("TUNND_TLS_KEY_FILE")

	_, err := config.LoadServer("")
	if err == nil {
		t.Fatal("expected TLS error on port 443 without cert config, got nil")
	}
	// Error should mention TLS or port 443.
	if !contains(err.Error(), "443") && !contains(err.Error(), "TLS") && !contains(err.Error(), "tls") {
		t.Errorf("error should mention port 443 or TLS, got: %v", err)
	}
}

// TestStartup_SucceedsWithMinimalValidConfig verifies that a minimal but valid
// configuration loads without error (non-443 port, domain + admin_password set).
func TestStartup_SucceedsWithMinimalValidConfig(t *testing.T) {
	t.Setenv("TUNND_DOMAIN", "tunnel.test")
	t.Setenv("TUNND_HTTP_PORT", "8080")
	t.Setenv("TUNND_ADMIN_PASSWORD", "supersecretpass1")

	cfg, err := config.LoadServer("")
	if err != nil {
		t.Fatalf("expected successful load with valid config, got: %v", err)
	}
	if cfg.Domain != "tunnel.test" {
		t.Errorf("Domain = %q, want tunnel.test", cfg.Domain)
	}
}

// TestStartup_ErrorMessageIsDescriptive_MissingDomain verifies Requirement 2.14:
// Error messages must clearly indicate what is wrong AND how to fix it.
func TestStartup_ErrorMessageIsDescriptive_MissingDomain(t *testing.T) {
	os.Unsetenv("TUNND_DOMAIN")
	os.Unsetenv("TUNND_ADMIN_PASSWORD")

	_, err := config.LoadServer("")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	// Should mention how to set the domain (env var or config file).
	hasResolution := contains(msg, "TUNND_DOMAIN") || contains(msg, "domain:")
	if !hasResolution {
		t.Errorf("error message should include resolution hint (e.g. TUNND_DOMAIN), got: %v", err)
	}
}

// TestStartup_ErrorMessageIsDescriptive_MissingPassword previously asserted
// that a missing admin_password produced an actionable error. The bootstrap
// redesign made admin_password optional, so the test no longer applies — see
// TestStartup_AdminPasswordOptional for the current contract.

// TestStartup_FailsWithInvalidYAMLConfigFile verifies Requirement 2.5:
// A syntactically invalid configuration file must cause startup to fail with a
// descriptive error containing the file path.
func TestStartup_FailsWithInvalidYAMLConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	badConfig := tmpDir + "/bad-server.yaml"

	os.WriteFile(badConfig, []byte(`domain: tunnel.test
http_port: [not_an_integer`), 0o600)

	_, err := config.LoadServer(badConfig)
	if err == nil {
		t.Fatal("expected error for invalid YAML config, got nil")
	}
	if !contains(err.Error(), badConfig) {
		t.Errorf("error should contain config file path %q, got: %v", badConfig, err)
	}
}

// TestStartup_FailsWithMalformedPortValue verifies that an invalid (non-integer)
// port value causes LoadServer to fail.  The config library will reject it
// during unmarshalling.
func TestStartup_FailsWithMalformedPortValue(t *testing.T) {
	tmpDir := t.TempDir()
	badConfig := tmpDir + "/bad-port.yaml"

	os.WriteFile(badConfig, []byte(`domain: tunnel.test
http_port: "not-a-port"
admin_password: supersecretpass1`), 0o600)

	// LoadServer may succeed (viper silently uses default for unparseable int) or
	// fail — either outcome is acceptable; what matters is it does not panic and
	// any error is descriptive.
	cfg, err := config.LoadServer(badConfig)
	if err != nil {
		// Error path: message should be non-empty.
		if strings.TrimSpace(err.Error()) == "" {
			t.Error("error message should not be empty")
		}
		return
	}
	// Success path: port must have fallen back to a valid value.
	if cfg.HTTPPort <= 0 {
		t.Errorf("HTTPPort = %d after malformed input, expected a positive default", cfg.HTTPPort)
	}
}

// ── Port conflict detection (probePort equivalent) ────────────────────────────

// TestProbePort_DetectsConflict verifies Requirement 2.13:
// When a port is already in use, binding should fail with a descriptive error
// indicating the port number.
//
// We test the underlying OS behavior that probePort (in cmd/server/main.go)
// relies on — namely that net.Listen returns an error for an already-bound port.
func TestProbePort_DetectsConflict(t *testing.T) {
	// Occupy a random free port.
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("could not bind to free port: %v", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port

	// Attempt to bind again to the same port.
	_, conflictErr := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if conflictErr == nil {
		t.Fatalf("expected conflict error for port %d, got nil", port)
	}

	// The error message must mention the port in some form (address already in
	// use, bind: address already in use, etc.).
	errMsg := conflictErr.Error()
	if !contains(errMsg, fmt.Sprintf("%d", port)) && !contains(errMsg, "address already in use") && !contains(errMsg, "bind") {
		t.Errorf("conflict error for port %d should be descriptive, got: %v", port, conflictErr)
	}
}

// TestProbePort_SucceedsForFreePort verifies that probing a free port does not
// return an error (no false positives).
func TestProbePort_SucceedsForFreePort(t *testing.T) {
	// Find a free port, then release it — the probe should succeed for it.
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("could not bind to free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	// Now probe: should not error.
	ln2, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		// On some systems the port may be reused immediately — skip rather than fail.
		t.Skipf("port %d not available for probe test: %v", port, err)
	}
	ln2.Close()
}

// TestStartup_LoadServerFromFile_ValidConfig verifies that a complete, valid
// server config file loads correctly (integration of file + validation).
func TestStartup_LoadServerFromFile_ValidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := tmpDir + "/tunnd-server.yaml"

	configYAML := `domain: tunnel.example.com
http_port: 8080
admin_port: 9091
admin_password: supersecretpassword
log_level: info
log_format: pretty`

	os.WriteFile(configFile, []byte(configYAML), 0o600)

	cfg, err := config.LoadServer(configFile)
	if err != nil {
		t.Fatalf("LoadServer with valid config file: %v", err)
	}
	if cfg.Domain != "tunnel.example.com" {
		t.Errorf("Domain = %q, want tunnel.example.com", cfg.Domain)
	}
	if cfg.HTTPPort != 8080 {
		t.Errorf("HTTPPort = %d, want 8080", cfg.HTTPPort)
	}
	if cfg.AdminPort != 9091 {
		t.Errorf("AdminPort = %d, want 9091", cfg.AdminPort)
	}
	if cfg.AdminPassword != "supersecretpassword" {
		t.Errorf("AdminPassword not loaded from file")
	}
}

// TestStartup_LoadServerFromFile_MissingDomain verifies Requirement 2.14 via file:
// A config file without domain must still fail validation.
func TestStartup_LoadServerFromFile_MissingDomain(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := tmpDir + "/tunnd-server.yaml"

	// No domain field.
	configYAML := `http_port: 8080
admin_password: supersecretpassword`

	os.WriteFile(configFile, []byte(configYAML), 0o600)

	// Ensure env var is not set to avoid interference.
	os.Unsetenv("TUNND_DOMAIN")

	_, err := config.LoadServer(configFile)
	if err == nil {
		t.Fatal("expected error for missing domain in config file, got nil")
	}
	if !contains(err.Error(), "domain") {
		t.Errorf("error should mention 'domain', got: %v", err)
	}
}
