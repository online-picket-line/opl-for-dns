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
	// Check API defaults
	if cfg.API.BaseURL != "https://onlinepicketline.com/api" {
		t.Errorf("Expected API base URL 'https://onlinepicketline.com/api', got '%s'", cfg.API.BaseURL)
	}
	if cfg.API.RefreshInterval.Duration != 15*time.Minute {
		t.Errorf("Expected refresh interval 15m, got %v", cfg.API.RefreshInterval.Duration)
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
			modify:  func(c *Config) {},
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
			name:    "missing API base URL",
			modify:  func(c *Config) { c.API.BaseURL = "" },
			wantErr: "api.base_url",
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
		"dns": {"listen_addr": "0.0.0.0:53", "upstream_dns": ["8.8.8.8:53"]},
		"api": {"base_url": "http://original.com/api"},
		"logging": {"level": "info", "format": "text"}
	}`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Set environment variables
	os.Setenv("OPL_API_KEY", "test-api-key")
	defer func() {
		os.Unsetenv("OPL_API_KEY")
	}()

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify env overrides were applied
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
