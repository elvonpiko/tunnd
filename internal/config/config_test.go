package config_test

import (
	"os"
	"testing"

	"github.com/elvonpiko/tunnd/internal/config"
)

func TestLoadServer_FailsWithNoDomain(t *testing.T) {
	os.Unsetenv("TUNND_DOMAIN")
	os.Unsetenv("TUNND_TLS_EMAIL")

	_, err := config.LoadServer("")
	if err == nil {
		t.Error("expected error when domain is missing, got nil")
	}
}

func TestLoadServer_DefaultsApplied(t *testing.T) {
	t.Setenv("TUNND_DOMAIN", "tunnel.test")
	// Not on port 443, so no TLS required
	t.Setenv("TUNND_HTTP_PORT", "8080")
	t.Setenv("TUNND_ADMIN_PASSWORD", "supersecretpassword")

	cfg, err := config.LoadServer("")
	if err != nil {
		t.Fatalf("LoadServer: %v", err)
	}
	if cfg.AdminPort != 9091 {
		t.Errorf("AdminPort = %d, want 9091", cfg.AdminPort)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", cfg.LogLevel)
	}
	if cfg.DBPath == "" {
		t.Error("DBPath should have a default")
	}
}

func TestLoadServer_EnvOverridesDefaults(t *testing.T) {
	t.Setenv("TUNND_DOMAIN", "my.tunnel.com")
	t.Setenv("TUNND_HTTP_PORT", "8443")
	t.Setenv("TUNND_LOG_LEVEL", "debug")
	t.Setenv("TUNND_TLS_EMAIL", "me@example.com")
	t.Setenv("TUNND_ADMIN_PASSWORD", "supersecretpassword")

	cfg, err := config.LoadServer("")
	if err != nil {
		t.Fatalf("LoadServer: %v", err)
	}
	if cfg.Domain != "my.tunnel.com" {
		t.Errorf("Domain = %q, want my.tunnel.com", cfg.Domain)
	}
	if cfg.HTTPPort != 8443 {
		t.Errorf("HTTPPort = %d, want 8443", cfg.HTTPPort)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
	}
}

func TestLoadServer_Port443RequiresTLS(t *testing.T) {
	t.Setenv("TUNND_DOMAIN", "tunnel.test")
	t.Setenv("TUNND_HTTP_PORT", "443")
	os.Unsetenv("TUNND_TLS_EMAIL")
	os.Unsetenv("TUNND_TLS_CERT_FILE")

	_, err := config.LoadServer("")
	if err == nil {
		t.Error("expected TLS error on port 443 without cert config")
	}
}

func TestLoadClient_Defaults(t *testing.T) {
	os.Unsetenv("TUNND_SERVER_ADDR")
	os.Unsetenv("TUNND_TOKEN")

	// LoadClient now validates required fields; missing server_addr should produce an error.
	_, err := config.LoadClient("", nil)
	if err == nil {
		t.Error("expected error when server_addr and token are missing, got nil")
	}
}

func TestLoadClient_DefaultOptionalFields(t *testing.T) {
	// With required fields set, optional fields should use defaults.
	t.Setenv("TUNND_SERVER_ADDR", "wss://tunnel.example.com")
	t.Setenv("TUNND_TOKEN", "tnnd_testtoken")

	cfg, err := config.LoadClient("", nil)
	if err != nil {
		t.Fatalf("LoadClient: %v", err)
	}
	if cfg.Protocol != "http" {
		t.Errorf("Protocol = %q, want http", cfg.Protocol)
	}
	if cfg.InspectorPort != 4040 {
		t.Errorf("InspectorPort = %d, want 4040", cfg.InspectorPort)
	}
}

func TestLoadClient_EnvOverrides(t *testing.T) {
	t.Setenv("TUNND_SERVER_ADDR", "wss://my.server.com")
	t.Setenv("TUNND_TOKEN", "tnnd_abc123")
	t.Setenv("TUNND_SUBDOMAIN", "myapp")

	cfg, err := config.LoadClient("", nil)
	if err != nil {
		t.Fatalf("LoadClient: %v", err)
	}
	if cfg.ServerAddr != "wss://my.server.com" {
		t.Errorf("ServerAddr = %q", cfg.ServerAddr)
	}
	if cfg.Token != "tnnd_abc123" {
		t.Errorf("Token = %q", cfg.Token)
	}
	if cfg.Subdomain != "myapp" {
		t.Errorf("Subdomain = %q", cfg.Subdomain)
	}
}

func TestLoadClient_CLIOverrides(t *testing.T) {
	t.Setenv("TUNND_SERVER_ADDR", "wss://env.server.com")
	t.Setenv("TUNND_TOKEN", "tnnd_env_token")
	t.Setenv("TUNND_SUBDOMAIN", "env-subdomain")

	// CLI overrides should take precedence over environment variables
	cliOverrides := map[string]interface{}{
		"subdomain": "cli-subdomain",
		"protocol":  "tcp",
	}

	cfg, err := config.LoadClient("", cliOverrides)
	if err != nil {
		t.Fatalf("LoadClient: %v", err)
	}
	if cfg.Subdomain != "cli-subdomain" {
		t.Errorf("Subdomain = %q, want cli-subdomain (CLI should override ENV)", cfg.Subdomain)
	}
	if cfg.Protocol != "tcp" {
		t.Errorf("Protocol = %q, want tcp (CLI should override default)", cfg.Protocol)
	}
	// ENV values should still be used for non-overridden fields
	if cfg.ServerAddr != "wss://env.server.com" {
		t.Errorf("ServerAddr = %q, want wss://env.server.com", cfg.ServerAddr)
	}
	if cfg.Token != "tnnd_env_token" {
		t.Errorf("Token = %q, want tnnd_env_token", cfg.Token)
	}
}

func TestLoadClient_FileSearchOrder(t *testing.T) {
	// Create a temporary directory structure for testing
	tmpDir := t.TempDir()

	// Create config files in different locations with different values
	localConfig := tmpDir + "/tunnd.yaml"
	homeDir := tmpDir + "/home"
	homeConfig := homeDir + "/.tunnd/tunnd.yaml"
	etcDir := tmpDir + "/etc/tunnd"
	etcConfig := etcDir + "/tunnd.yaml"

	// Create directories
	os.MkdirAll(homeDir+"/.tunnd", 0755)
	os.MkdirAll(etcDir, 0755)

	// Write config files with different subdomain values to identify which one is loaded
	localYAML := `server_addr: wss://server.com
token: tnnd_token
subdomain: local-config`
	homeYAML := `server_addr: wss://server.com
token: tnnd_token
subdomain: home-config`
	etcYAML := `server_addr: wss://server.com
token: tnnd_token
subdomain: etc-config`

	os.WriteFile(localConfig, []byte(localYAML), 0644)
	os.WriteFile(homeConfig, []byte(homeYAML), 0644)
	os.WriteFile(etcConfig, []byte(etcYAML), 0644)

	// Test 1: Explicit config file takes highest precedence
	cfg, err := config.LoadClient(homeConfig, nil)
	if err != nil {
		t.Fatalf("LoadClient with explicit config: %v", err)
	}
	if cfg.Subdomain != "home-config" {
		t.Errorf("With explicit config file, Subdomain = %q, want home-config", cfg.Subdomain)
	}

	// Test 2: Local config file is used when no explicit config specified
	// Change to tmpDir to make it the "current working directory" conceptually
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	cfg, err = config.LoadClient("", nil)
	if err != nil {
		t.Fatalf("LoadClient with local config: %v", err)
	}
	if cfg.Subdomain != "local-config" {
		t.Errorf("With local config file, Subdomain = %q, want local-config", cfg.Subdomain)
	}
}

func TestLoadServer_FileSearchOrder(t *testing.T) {
	// Create a temporary directory structure for testing
	tmpDir := t.TempDir()

	// Create config files in different locations with different values
	localConfig := tmpDir + "/tunnd-server.yaml"
	explicitConfig := tmpDir + "/custom-server.yaml"

	// Write config files with different log levels to identify which one is loaded.
	// admin_password is required for validation to pass.
	localYAML := `domain: tunnel.local.com
http_port: 8080
admin_password: localsecretpassword
log_level: debug`
	explicitYAML := `domain: tunnel.explicit.com
http_port: 8080
admin_password: explicitsecretpassword
log_level: error`

	os.WriteFile(localConfig, []byte(localYAML), 0644)
	os.WriteFile(explicitConfig, []byte(explicitYAML), 0644)

	// Test 1: Explicit config file takes highest precedence
	cfg, err := config.LoadServer(explicitConfig)
	if err != nil {
		t.Fatalf("LoadServer with explicit config: %v", err)
	}
	if cfg.Domain != "tunnel.explicit.com" {
		t.Errorf("With explicit config file, Domain = %q, want tunnel.explicit.com", cfg.Domain)
	}
	if cfg.LogLevel != "error" {
		t.Errorf("With explicit config file, LogLevel = %q, want error", cfg.LogLevel)
	}

	// Test 2: Local config file is used when no explicit config specified
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	cfg, err = config.LoadServer("")
	if err != nil {
		t.Fatalf("LoadServer with local config: %v", err)
	}
	if cfg.Domain != "tunnel.local.com" {
		t.Errorf("With local config file, Domain = %q, want tunnel.local.com", cfg.Domain)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("With local config file, LogLevel = %q, want debug", cfg.LogLevel)
	}
}

func TestLoadClient_InvalidYAMLError(t *testing.T) {
	tmpDir := t.TempDir()
	invalidConfig := tmpDir + "/invalid.yaml"

	// Write syntactically invalid YAML
	invalidYAML := `server_addr: wss://server.com
token: tnnd_token
subdomain: [unclosed array`

	os.WriteFile(invalidConfig, []byte(invalidYAML), 0644)

	_, err := config.LoadClient(invalidConfig, nil)
	if err == nil {
		t.Error("Expected error for invalid YAML, got nil")
	}

	// Error message should contain the file path
	if !contains(err.Error(), invalidConfig) {
		t.Errorf("Error message should contain file path %q, got: %v", invalidConfig, err)
	}
}

func TestLoadServer_InvalidYAMLError(t *testing.T) {
	tmpDir := t.TempDir()
	invalidConfig := tmpDir + "/invalid-server.yaml"

	// Write syntactically invalid YAML (malformed structure)
	invalidYAML := `domain: tunnel.com
http_port: 8080
[invalid yaml structure`

	os.WriteFile(invalidConfig, []byte(invalidYAML), 0644)

	_, err := config.LoadServer(invalidConfig)
	if err == nil {
		t.Error("Expected error for invalid YAML, got nil")
	}

	// Error message should indicate parsing failure
	errMsg := err.Error()
	if !contains(errMsg, "failed to") && !contains(errMsg, "parsing") && !contains(errMsg, "error") {
		t.Errorf("Error message should indicate parsing failure, got: %v", err)
	}
}

func TestLoadClient_ConfigFilePrecedence(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := tmpDir + "/tunnd.yaml"

	// Write config file
	configYAML := `server_addr: wss://file.server.com
token: tnnd_file_token
subdomain: file-subdomain
protocol: tcp
inspector_port: 5050
log_level: debug`

	os.WriteFile(configFile, []byte(configYAML), 0644)

	// Set environment variables that should be overridden by config file
	t.Setenv("TUNND_SUBDOMAIN", "env-subdomain")
	t.Setenv("TUNND_PROTOCOL", "http")

	// Load config - environment variables should override file values
	cfg, err := config.LoadClient(configFile, nil)
	if err != nil {
		t.Fatalf("LoadClient: %v", err)
	}

	// Environment variables should override config file
	if cfg.Subdomain != "env-subdomain" {
		t.Errorf("Subdomain = %q, want env-subdomain (ENV should override file)", cfg.Subdomain)
	}
	if cfg.Protocol != "http" {
		t.Errorf("Protocol = %q, want http (ENV should override file)", cfg.Protocol)
	}

	// Values not set in ENV should come from file
	if cfg.ServerAddr != "wss://file.server.com" {
		t.Errorf("ServerAddr = %q, want wss://file.server.com", cfg.ServerAddr)
	}
	if cfg.Token != "tnnd_file_token" {
		t.Errorf("Token = %q, want tnnd_file_token", cfg.Token)
	}
	if cfg.InspectorPort != 5050 {
		t.Errorf("InspectorPort = %d, want 5050", cfg.InspectorPort)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
	}
}

func TestLoadClient_CompletePrecedenceOrder(t *testing.T) {
	// This test verifies the complete precedence order: CLI > ENV > File > Defaults
	tmpDir := t.TempDir()
	configFile := tmpDir + "/tunnd.yaml"

	// Write config file with specific values
	configYAML := `server_addr: wss://file.server.com
token: tnnd_file_token
subdomain: file-subdomain
protocol: tcp
inspector_port: 5050
log_level: debug`

	os.WriteFile(configFile, []byte(configYAML), 0644)

	// Set environment variables that should override file values
	t.Setenv("TUNND_SERVER_ADDR", "wss://env.server.com")
	t.Setenv("TUNND_SUBDOMAIN", "env-subdomain")
	t.Setenv("TUNND_INSPECTOR_PORT", "6060")

	// CLI overrides should take highest precedence
	cliOverrides := map[string]interface{}{
		"subdomain":      "cli-subdomain",
		"inspector_port": 7070,
	}

	cfg, err := config.LoadClient(configFile, cliOverrides)
	if err != nil {
		t.Fatalf("LoadClient: %v", err)
	}

	// Verify precedence order:
	// 1. CLI overrides take highest precedence
	if cfg.Subdomain != "cli-subdomain" {
		t.Errorf("Subdomain = %q, want cli-subdomain (CLI should override ENV and file)", cfg.Subdomain)
	}
	if cfg.InspectorPort != 7070 {
		t.Errorf("InspectorPort = %d, want 7070 (CLI should override ENV and file)", cfg.InspectorPort)
	}

	// 2. ENV overrides file (for fields not in CLI overrides)
	if cfg.ServerAddr != "wss://env.server.com" {
		t.Errorf("ServerAddr = %q, want wss://env.server.com (ENV should override file)", cfg.ServerAddr)
	}

	// 3. File values used when not in CLI or ENV
	if cfg.Token != "tnnd_file_token" {
		t.Errorf("Token = %q, want tnnd_file_token (should come from file)", cfg.Token)
	}
	if cfg.Protocol != "tcp" {
		t.Errorf("Protocol = %q, want tcp (should come from file)", cfg.Protocol)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug (should come from file)", cfg.LogLevel)
	}
}

func TestLoadClient_CLIOverridesEmptyValues(t *testing.T) {
	// Test that CLI overrides work even with empty string values
	t.Setenv("TUNND_SERVER_ADDR", "wss://server.com")
	t.Setenv("TUNND_TOKEN", "tnnd_token")
	t.Setenv("TUNND_SUBDOMAIN", "env-subdomain")

	// CLI override with empty string should still take precedence
	cliOverrides := map[string]interface{}{
		"subdomain": "",
	}

	cfg, err := config.LoadClient("", cliOverrides)
	if err != nil {
		t.Fatalf("LoadClient: %v", err)
	}

	// Empty string from CLI should override ENV value
	if cfg.Subdomain != "" {
		t.Errorf("Subdomain = %q, want empty string (CLI override should work with empty values)", cfg.Subdomain)
	}
}

func TestLoadClient_CLIOverridesAllFields(t *testing.T) {
	// Test that all client config fields can be overridden via CLI
	cliOverrides := map[string]interface{}{
		"server_addr":    "wss://cli.server.com",
		"token":          "tnnd_cli_token",
		"subdomain":      "cli-subdomain",
		"protocol":       "tcp",
		"inspector_port": 8080,
		"log_level":      "error",
	}

	cfg, err := config.LoadClient("", cliOverrides)
	if err != nil {
		t.Fatalf("LoadClient: %v", err)
	}

	// Verify all CLI overrides are applied
	if cfg.ServerAddr != "wss://cli.server.com" {
		t.Errorf("ServerAddr = %q, want wss://cli.server.com", cfg.ServerAddr)
	}
	if cfg.Token != "tnnd_cli_token" {
		t.Errorf("Token = %q, want tnnd_cli_token", cfg.Token)
	}
	if cfg.Subdomain != "cli-subdomain" {
		t.Errorf("Subdomain = %q, want cli-subdomain", cfg.Subdomain)
	}
	if cfg.Protocol != "tcp" {
		t.Errorf("Protocol = %q, want tcp", cfg.Protocol)
	}
	if cfg.InspectorPort != 8080 {
		t.Errorf("InspectorPort = %d, want 8080", cfg.InspectorPort)
	}
	if cfg.LogLevel != "error" {
		t.Errorf("LogLevel = %q, want error", cfg.LogLevel)
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ── Validation method tests (Task 2.2) ───────────────────────────────────────

func TestClientValidate_MissingServerAddr(t *testing.T) {
	os.Unsetenv("TUNND_SERVER_ADDR")
	os.Unsetenv("TUNND_TOKEN")

	_, err := config.LoadClient("", nil)
	if err == nil {
		t.Fatal("expected error for missing server_addr, got nil")
	}
	if !contains(err.Error(), "server_addr") {
		t.Errorf("error message should mention 'server_addr', got: %v", err)
	}
}

func TestClientValidate_MissingToken(t *testing.T) {
	t.Setenv("TUNND_SERVER_ADDR", "wss://tunnel.example.com")
	os.Unsetenv("TUNND_TOKEN")

	_, err := config.LoadClient("", nil)
	if err == nil {
		t.Fatal("expected error for missing token, got nil")
	}
	if !contains(err.Error(), "token") {
		t.Errorf("error message should mention 'token', got: %v", err)
	}
}

func TestClientValidate_InvalidProtocolDefaultsToHTTP(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := tmpDir + "/tunnd.yaml"

	// invalid protocol value should be replaced with default "http"
	configYAML := `server_addr: wss://tunnel.example.com
token: tnnd_testtoken
protocol: ftp`

	os.WriteFile(configFile, []byte(configYAML), 0644)

	cfg, err := config.LoadClient(configFile, nil)
	if err != nil {
		t.Fatalf("LoadClient: %v", err)
	}
	if cfg.Protocol != "http" {
		t.Errorf("Protocol = %q, want http (invalid value should default to http)", cfg.Protocol)
	}
}

func TestClientValidate_InvalidInspectorPortDefaultsTo4040(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := tmpDir + "/tunnd.yaml"

	// port 80 is outside 1024-65535 range, should default to 4040
	configYAML := `server_addr: wss://tunnel.example.com
token: tnnd_testtoken
inspector_port: 80`

	os.WriteFile(configFile, []byte(configYAML), 0644)

	cfg, err := config.LoadClient(configFile, nil)
	if err != nil {
		t.Fatalf("LoadClient: %v", err)
	}
	if cfg.InspectorPort != 4040 {
		t.Errorf("InspectorPort = %d, want 4040 (out-of-range value should default to 4040)", cfg.InspectorPort)
	}
}

func TestClientValidate_ValidInspectorPortKept(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := tmpDir + "/tunnd.yaml"

	configYAML := `server_addr: wss://tunnel.example.com
token: tnnd_testtoken
inspector_port: 5000`

	os.WriteFile(configFile, []byte(configYAML), 0644)

	cfg, err := config.LoadClient(configFile, nil)
	if err != nil {
		t.Fatalf("LoadClient: %v", err)
	}
	if cfg.InspectorPort != 5000 {
		t.Errorf("InspectorPort = %d, want 5000", cfg.InspectorPort)
	}
}

func TestClientValidate_InvalidLogLevelDefaultsToInfo(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := tmpDir + "/tunnd.yaml"

	configYAML := `server_addr: wss://tunnel.example.com
token: tnnd_testtoken
log_level: verbose`

	os.WriteFile(configFile, []byte(configYAML), 0644)

	cfg, err := config.LoadClient(configFile, nil)
	if err != nil {
		t.Fatalf("LoadClient: %v", err)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info (invalid value should default to info)", cfg.LogLevel)
	}
}

// TestServerValidate_AdminPasswordOptional documents that admin_password is
// no longer required at config time — operators can leave it empty and set
// it via the first-run bootstrap dashboard. The server still warns when a
// known-weak default is used, but doesn't refuse to start.
func TestServerValidate_AdminPasswordOptional(t *testing.T) {
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
}

func TestServerValidate_InvalidLogLevelDefaultsToInfo(t *testing.T) {
	t.Setenv("TUNND_DOMAIN", "tunnel.test")
	t.Setenv("TUNND_HTTP_PORT", "8080")
	t.Setenv("TUNND_ADMIN_PASSWORD", "supersecretpassword")
	t.Setenv("TUNND_LOG_LEVEL", "verbose")

	cfg, err := config.LoadServer("")
	if err != nil {
		t.Fatalf("LoadServer: %v", err)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info (invalid value should default to info)", cfg.LogLevel)
	}
}

func TestClientValidate_ValidProtocolTCP(t *testing.T) {
	t.Setenv("TUNND_SERVER_ADDR", "wss://tunnel.example.com")
	t.Setenv("TUNND_TOKEN", "tnnd_testtoken")
	t.Setenv("TUNND_PROTOCOL", "tcp")

	cfg, err := config.LoadClient("", nil)
	if err != nil {
		t.Fatalf("LoadClient: %v", err)
	}
	if cfg.Protocol != "tcp" {
		t.Errorf("Protocol = %q, want tcp", cfg.Protocol)
	}
}

func TestClientValidate_InspectorPortBoundary(t *testing.T) {
	// Test boundary values: 1024 and 65535 should be valid; 1023 and 65536 should default
	tests := []struct {
		name     string
		port     int
		wantPort int
	}{
		{"min valid", 1024, 1024},
		{"max valid", 65535, 65535},
		{"below min", 1023, 4040},
		{"above max", 65536, 4040},
		{"zero", 0, 4040},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := config.LoadClient("", map[string]interface{}{
				"server_addr":    "wss://tunnel.example.com",
				"token":          "tnnd_testtoken",
				"inspector_port": tc.port,
			})
			if err != nil {
				t.Fatalf("LoadClient: %v", err)
			}
			if cfg.InspectorPort != tc.wantPort {
				t.Errorf("port %d: InspectorPort = %d, want %d", tc.port, cfg.InspectorPort, tc.wantPort)
			}
		})
	}
}

// ── YAML error handling tests (Task 2.3) ─────────────────────────────────────

func TestLoadClient_YAMLErrorContainsFilePath(t *testing.T) {
	tmpDir := t.TempDir()
	invalidConfig := tmpDir + "/bad-client.yaml"

	os.WriteFile(invalidConfig, []byte(`server_addr: wss://server.com
token: tnnd_token
key: [unclosed`), 0644)

	_, err := config.LoadClient(invalidConfig, nil)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
	if !contains(err.Error(), invalidConfig) {
		t.Errorf("error should contain file path %q, got: %v", invalidConfig, err)
	}
}

func TestLoadServer_YAMLErrorContainsFilePath(t *testing.T) {
	tmpDir := t.TempDir()
	invalidConfig := tmpDir + "/bad-server.yaml"

	os.WriteFile(invalidConfig, []byte(`domain: tunnel.com
http_port: [not_an_int`), 0644)

	_, err := config.LoadServer(invalidConfig)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
	if !contains(err.Error(), invalidConfig) {
		t.Errorf("error should contain file path %q, got: %v", invalidConfig, err)
	}
}
