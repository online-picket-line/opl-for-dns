package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	// Check DNS defaults
	if cfg.DNS.ListenAddr != "0.0.0.0:53" {
		t.Errorf("Expected DNS listen addr '0.0.0.0:53', got '%s'", cfg.DNS.ListenAddr)
	}
	if len(cfg.DNS.UpstreamDNS) != 2 {
		t.Errorf("Expected 2 upstream DNS servers, got %d", len(cfg.DNS.UpstreamDNS))
	}
	if cfg.DNS.BlockPageIP != "127.0.0.1" {
		t.Errorf("Expected block page IP '127.0.0.1', got '%s'", cfg.DNS.BlockPageIP)
	}

	// Check API defaults
	if cfg.API.BaseURL != "https://onlinepicketline.com/api" {
		t.Errorf("Expected API base URL 'https://onlinepicketline.com/api', got '%s'", cfg.API.BaseURL)
	}
	if cfg.API.RefreshInterval.Duration != 15*time.Minute {
		t.Errorf("Expected refresh interval 15m, got %v", cfg.API.RefreshInterval.Duration)
	}

	// Check Web defaults
	if cfg.Web.ListenAddr != "0.0.0.0:8080" {
		t.Errorf("Expected web listen addr '0.0.0.0:8080', got '%s'", cfg.Web.ListenAddr)
	}
	if cfg.Web.DisplayMode != "block" {
		t.Errorf("Expected display mode 'block', got '%s'", cfg.Web.DisplayMode)
	}

	// Check Session defaults
	if cfg.Session.TokenTTL.Duration != 24*time.Hour {
		t.Errorf("Expected token TTL 24h, got %v", cfg.Session.TokenTTL.Duration)
	}
}

func TestDurationMarshalJSON(t *testing.T) {
	d := Duration{5 * time.Minute}
	data, err := d.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}
	if string(data) != `"5m0s"` {
		t.Errorf("Expected '5m0s', got %s", string(data))
	}
}

func TestDurationUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{
			name:     "string duration",
			input:    `"5m"`,
			expected: 5 * time.Minute,
		},
		{
			name:     "string duration with seconds",
			input:    `"30s"`,
			expected: 30 * time.Second,
		},
		{
			name:     "numeric duration",
			input:    `300000000000`,
			expected: 5 * time.Minute,
		},
		{
			name:    "invalid string",
			input:   `"invalid"`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var d Duration
			err := d.UnmarshalJSON([]byte(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalJSON failed: %v", err)
			}
			if d.Duration != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, d.Duration)
			}
		})
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr string
	}{
		{
			name:    "valid config",
			modify:  func(c *Config) { c.Session.Secret = "test-secret" },
			wantErr: "",
		},
		{
			name:    "missing DNS listen addr",
			modify:  func(c *Config) { c.DNS.ListenAddr = "" },
			wantErr: "dns.listen_addr",
		},
		{
			name:    "missing upstream DNS",
			modify:  func(c *Config) { c.DNS.UpstreamDNS = nil },
			wantErr: "dns.upstream_dns",
		},
		{
			name:    "missing block page IP",
			modify:  func(c *Config) { c.DNS.BlockPageIP = "" },
			wantErr: "dns.block_page_ip",
		},
		{
			name:    "missing API base URL",
			modify:  func(c *Config) { c.API.BaseURL = "" },
			wantErr: "api.base_url",
		},
		{
			name:    "missing web listen addr",
			modify:  func(c *Config) { c.Web.ListenAddr = "" },
			wantErr: "web.listen_addr",
		},
		{
			name:    "missing session secret",
			modify:  func(c *Config) {},
			wantErr: "session.secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.modify(cfg)
			err := cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("Expected error containing '%s', got nil", tt.wantErr)
				} else if !contains(err.Error(), tt.wantErr) {
					t.Errorf("Expected error containing '%s', got: %v", tt.wantErr, err)
				}
			}
		})
	}
}

func TestConfigSaveLoad(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	// Create and save config
	cfg := DefaultConfig()
	cfg.Session.Secret = "test-secret"
	cfg.DNS.ListenAddr = "0.0.0.0:5353"

	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load config
	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify values
	if loaded.DNS.ListenAddr != cfg.DNS.ListenAddr {
		t.Errorf("Expected DNS listen addr '%s', got '%s'", cfg.DNS.ListenAddr, loaded.DNS.ListenAddr)
	}
	if loaded.Session.Secret != cfg.Session.Secret {
		t.Errorf("Expected session secret '%s', got '%s'", cfg.Session.Secret, loaded.Session.Secret)
	}
}

func TestLoadNonExistentConfig(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.json")
	if err != nil {
		t.Fatalf("Load should not fail for non-existent file: %v", err)
	}

	// Should return defaults
	defaults := DefaultConfig()
	if cfg.DNS.ListenAddr != defaults.DNS.ListenAddr {
		t.Errorf("Expected default DNS listen addr")
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	// Create temp file with invalid JSON
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	if err := os.WriteFile(configPath, []byte("not valid json"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestEnvOverrides(t *testing.T) {
	// Create a minimal config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	configContent := `{
		"dns": {"listen_addr": "0.0.0.0:53", "upstream_dns": ["8.8.8.8:53"], "block_page_ip": "127.0.0.1"},
		"api": {"base_url": "http://original.com/api"},
		"web": {"listen_addr": "0.0.0.0:8080", "external_url": "http://original.com"},
		"session": {"secret": "original-secret"},
		"logging": {"level": "info", "format": "text"}
	}`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Set environment variables
	os.Setenv("BLOCK_PAGE_IP", "192.168.1.100")
	os.Setenv("DNS_SESSION_SECRET", "env-secret")
	os.Setenv("BLOCK_PAGE_EXTERNAL_URL", "https://test.com/block")
	os.Setenv("OPL_API_KEY", "test-api-key")
	defer func() {
		os.Unsetenv("BLOCK_PAGE_IP")
		os.Unsetenv("DNS_SESSION_SECRET")
		os.Unsetenv("BLOCK_PAGE_EXTERNAL_URL")
		os.Unsetenv("OPL_API_KEY")
	}()

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify env overrides were applied
	if cfg.DNS.BlockPageIP != "192.168.1.100" {
		t.Errorf("Expected block page IP '192.168.1.100', got '%s'", cfg.DNS.BlockPageIP)
	}
	if cfg.Session.Secret != "env-secret" {
		t.Errorf("Expected session secret 'env-secret', got '%s'", cfg.Session.Secret)
	}
	if cfg.Web.ExternalURL != "https://test.com/block" {
		t.Errorf("Expected external URL 'https://test.com/block', got '%s'", cfg.Web.ExternalURL)
	}
	if cfg.API.APIKey != "test-api-key" {
		t.Errorf("Expected API key 'test-api-key', got '%s'", cfg.API.APIKey)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
