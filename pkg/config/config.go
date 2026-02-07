// Package config provides configuration management for the OPL DNS server.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Config holds the configuration for the OPL DNS server.
type Config struct {
	// DNS server configuration
	DNS DNSConfig `json:"dns"`

	// API configuration for Online Picketline
	API APIConfig `json:"api"`

	// Web server configuration for block page
	Web WebConfig `json:"web"`

	// Session configuration for bypass tokens
	Session SessionConfig `json:"session"`

	// Stats reporting configuration
	Stats StatsConfig `json:"stats"`

	// Logging configuration
	Logging LoggingConfig `json:"logging"`
}

// DNSConfig holds DNS server settings.
type DNSConfig struct {
	// ListenAddr is the address to listen on (e.g., "0.0.0.0:53")
	ListenAddr string `json:"listen_addr"`

	// UpstreamDNS is the list of upstream DNS servers
	UpstreamDNS []string `json:"upstream_dns"`

	// BlockPageIP is the IP address to return for blocked domains
	BlockPageIP string `json:"block_page_ip"`

	// CacheTTL is how long to cache DNS responses
	CacheTTL Duration `json:"cache_ttl"`

	// QueryTimeout is the timeout for upstream DNS queries
	QueryTimeout Duration `json:"query_timeout"`
}

// APIConfig holds Online Picketline API settings.
type APIConfig struct {
	// BaseURL is the base URL for the Online Picketline API
	BaseURL string `json:"base_url"`

	// APIKey is the API key for authentication
	APIKey string `json:"api_key"`

	// RefreshInterval is how often to refresh the blocklist
	RefreshInterval Duration `json:"refresh_interval"`

	// Timeout is the HTTP request timeout
	Timeout Duration `json:"timeout"`
}

// WebConfig holds web server settings for the block page.
type WebConfig struct {
	// ListenAddr is the address to listen on (e.g., "0.0.0.0:80")
	ListenAddr string `json:"listen_addr"`

	// ExternalURL is the external URL for the block page (used in redirects)
	ExternalURL string `json:"external_url"`

	// DisplayMode is the default display mode ("block" or "overlay")
	DisplayMode string `json:"display_mode"`

	// StaticDir is the directory for static assets
	StaticDir string `json:"static_dir"`
}

// SessionConfig holds session management settings.
type SessionConfig struct {
	// TokenTTL is how long bypass tokens are valid
	TokenTTL Duration `json:"token_ttl"`

	// Secret is the secret key for signing tokens
	Secret string `json:"secret"`

	// CleanupInterval is how often to cleanup expired sessions
	CleanupInterval Duration `json:"cleanup_interval"`
}

// LoggingConfig holds logging settings.
type LoggingConfig struct {
	// Level is the log level (debug, info, warn, error)
	Level string `json:"level"`

	// Format is the log format (json, text)
	Format string `json:"format"`
}

// StatsConfig holds stats reporting settings.
type StatsConfig struct {
	// Enabled controls whether stats reporting is active
	Enabled bool `json:"enabled"`

	// ReportInterval is how often to send stats reports
	ReportInterval Duration `json:"report_interval"`

	// InstanceID is a unique identifier for this DNS server instance.
	// If empty, the hostname will be used.
	InstanceID string `json:"instance_id"`

	// ReportURL is the URL to POST stats reports to.
	// Defaults to {api.base_url}/dns-stats/report
	ReportURL string `json:"report_url"`
}

// Duration is a wrapper for time.Duration that supports JSON marshaling.
type Duration struct {
	time.Duration
}

// MarshalJSON implements json.Marshaler.
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *Duration) UnmarshalJSON(b []byte) error {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch value := v.(type) {
	case float64:
		d.Duration = time.Duration(value)
		return nil
	case string:
		var err error
		d.Duration, err = time.ParseDuration(value)
		if err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("invalid duration: %v", v)
	}
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		DNS: DNSConfig{
			ListenAddr:   "0.0.0.0:53",
			UpstreamDNS:  []string{"8.8.8.8:53", "8.8.4.4:53"},
			BlockPageIP:  "127.0.0.1",
			CacheTTL:     Duration{5 * time.Minute},
			QueryTimeout: Duration{5 * time.Second},
		},
		API: APIConfig{
			BaseURL:         "https://onlinepicketline.com/api",
			APIKey:          "",
			RefreshInterval: Duration{15 * time.Minute},
			Timeout:         Duration{10 * time.Second},
		},
		Web: WebConfig{
			ListenAddr:  "0.0.0.0:8080",
			ExternalURL: "http://localhost:8080",
			DisplayMode: "block",
			StaticDir:   "./static",
		},
		Session: SessionConfig{
			TokenTTL:        Duration{24 * time.Hour},
			Secret:          "",
			CleanupInterval: Duration{1 * time.Hour},
		},
		Stats: StatsConfig{
			Enabled:        false,
			ReportInterval: Duration{5 * time.Minute},
			InstanceID:     "",
			ReportURL:      "",
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
		},
	}
}

// Load loads configuration from a JSON file.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil // Return defaults if config doesn't exist
		}
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	// Apply environment variable overrides
	cfg.applyEnvOverrides()

	return cfg, nil
}

// applyEnvOverrides applies environment variable overrides to the config.
// This allows Docker deployments to configure settings via environment variables.
func (c *Config) applyEnvOverrides() {
	// DNS settings
	if v := os.Getenv("DNS_LISTEN_ADDR"); v != "" {
		c.DNS.ListenAddr = v
	}
	if v := os.Getenv("BLOCK_PAGE_IP"); v != "" {
		c.DNS.BlockPageIP = v
	}

	// API settings
	if v := os.Getenv("OPL_API_BASE_URL"); v != "" {
		c.API.BaseURL = v
	}
	if v := os.Getenv("OPL_API_KEY"); v != "" {
		c.API.APIKey = v
	}

	// Web settings
	if v := os.Getenv("WEB_LISTEN_ADDR"); v != "" {
		c.Web.ListenAddr = v
	}
	if v := os.Getenv("BLOCK_PAGE_EXTERNAL_URL"); v != "" {
		c.Web.ExternalURL = v
	}
	if v := os.Getenv("DISPLAY_MODE"); v != "" {
		c.Web.DisplayMode = v
	}
	if v := os.Getenv("STATIC_DIR"); v != "" {
		c.Web.StaticDir = v
	}

	// Session settings
	if v := os.Getenv("DNS_SESSION_SECRET"); v != "" {
		c.Session.Secret = v
	}

	// Logging settings
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		c.Logging.Level = v
	}
	if v := os.Getenv("LOG_FORMAT"); v != "" {
		c.Logging.Format = v
	}

	// Stats settings
	if v := os.Getenv("STATS_ENABLED"); v == "true" || v == "1" {
		c.Stats.Enabled = true
	}
	if v := os.Getenv("STATS_INSTANCE_ID"); v != "" {
		c.Stats.InstanceID = v
	}
	if v := os.Getenv("STATS_REPORT_URL"); v != "" {
		c.Stats.ReportURL = v
	}
}

// Save saves the configuration to a JSON file.
func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	return nil
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.DNS.ListenAddr == "" {
		return fmt.Errorf("dns.listen_addr is required")
	}
	if len(c.DNS.UpstreamDNS) == 0 {
		return fmt.Errorf("dns.upstream_dns is required")
	}
	if c.DNS.BlockPageIP == "" {
		return fmt.Errorf("dns.block_page_ip is required")
	}
	if c.API.BaseURL == "" {
		return fmt.Errorf("api.base_url is required")
	}
	if c.Web.ListenAddr == "" {
		return fmt.Errorf("web.listen_addr is required")
	}
	if c.Session.Secret == "" {
		return fmt.Errorf("session.secret is required for security")
	}
	return nil
}
