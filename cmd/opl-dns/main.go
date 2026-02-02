// Package main provides the entry point for the OPL DNS server.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/online-picket-line/opl-for-dns/pkg/api"
	"github.com/online-picket-line/opl-for-dns/pkg/blockpage"
	"github.com/online-picket-line/opl-for-dns/pkg/config"
	"github.com/online-picket-line/opl-for-dns/pkg/dns"
	"github.com/online-picket-line/opl-for-dns/pkg/session"
)

var (
	version   = "1.0.0"
	buildTime = "unknown"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "config.json", "Path to configuration file")
	showVersion := flag.Bool("version", false, "Show version information")
	generateConfig := flag.Bool("generate-config", false, "Generate example configuration file")
	flag.Parse()

	if *showVersion {
		fmt.Printf("OPL DNS Server v%s (built %s)\n", version, buildTime)
		os.Exit(0)
	}

	if *generateConfig {
		cfg := config.DefaultConfig()
		cfg.Session.Secret = "change-this-to-a-secure-random-string"
		if err := cfg.Save("config.example.json"); err != nil {
			fmt.Fprintf(os.Stderr, "Error generating config: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Generated config.example.json")
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	// Setup logging
	var logLevel slog.Level
	switch cfg.Logging.Level {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	var handler slog.Handler
	if cfg.Logging.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	}
	logger := slog.New(handler)

	logger.Info("Starting OPL DNS Server", "version", version)

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		logger.Error("Invalid configuration", "error", err)
		os.Exit(1)
	}

	// Create API client
	apiClient := api.NewClient(
		cfg.API.BaseURL,
		cfg.API.APIKey,
		cfg.API.Timeout.Duration,
	)

	// Create session manager
	sessionManager := session.NewManager(
		cfg.Session.Secret,
		cfg.Session.TokenTTL.Duration,
	)

	// Create DNS server
	dnsServer, err := dns.NewServer(
		cfg.DNS.ListenAddr,
		cfg.DNS.BlockPageIP,
		cfg.DNS.UpstreamDNS,
		cfg.DNS.QueryTimeout.Duration,
		apiClient,
		sessionManager,
		logger.With("component", "dns"),
	)
	if err != nil {
		logger.Error("Error creating DNS server", "error", err)
		os.Exit(1)
	}

	// Create block page server
	blockPageServer, err := blockpage.NewServer(
		cfg.Web.ListenAddr,
		cfg.Web.ExternalURL,
		cfg.Web.DisplayMode,
		apiClient,
		sessionManager,
		logger.With("component", "web"),
	)
	if err != nil {
		logger.Error("Error creating block page server", "error", err)
		os.Exit(1)
	}

	// Context for shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initial blocklist fetch
	logger.Info("Fetching initial blocklist...")
	if _, err := apiClient.FetchBlocklist(ctx); err != nil {
		logger.Warn("Error fetching initial blocklist (will retry)", "error", err)
	} else {
		blocklist := apiClient.GetCachedBlocklist()
		if blocklist != nil {
			logger.Info("Blocklist loaded", "urls", blocklist.TotalURLs, "employers", len(blocklist.Employers))
		}
	}

	// Start blocklist refresh goroutine
	go func() {
		ticker := time.NewTicker(cfg.API.RefreshInterval.Duration)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				logger.Debug("Refreshing blocklist...")
				if _, err := apiClient.FetchBlocklist(ctx); err != nil {
					logger.Error("Error refreshing blocklist", "error", err)
				} else {
					blocklist := apiClient.GetCachedBlocklist()
					if blocklist != nil {
						logger.Debug("Blocklist refreshed", "urls", blocklist.TotalURLs)
					}
				}
			}
		}
	}()

	// Start session cleanup goroutine
	go func() {
		ticker := time.NewTicker(cfg.Session.CleanupInterval.Duration)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cleaned := sessionManager.CleanupExpired()
				if cleaned > 0 {
					logger.Debug("Cleaned up expired sessions", "count", cleaned)
				}
			}
		}
	}()

	// Start servers
	errChan := make(chan error, 3)

	// Start DNS server (UDP)
	go func() {
		if err := dnsServer.Start(); err != nil {
			errChan <- fmt.Errorf("DNS server (UDP): %w", err)
		}
	}()

	// Start DNS server (TCP)
	go func() {
		if err := dnsServer.StartTCP(); err != nil {
			errChan <- fmt.Errorf("DNS server (TCP): %w", err)
		}
	}()

	// Start block page server
	go func() {
		if err := blockPageServer.Start(); err != nil {
			errChan <- fmt.Errorf("block page server: %w", err)
		}
	}()

	// Wait for signals or errors
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		logger.Info("Received signal, shutting down...", "signal", sig)
	case err := <-errChan:
		logger.Error("Server error", "error", err)
	}

	// Cancel context to stop background goroutines
	cancel()

	// Shutdown servers
	logger.Info("Stopping servers...")
	dnsServer.Stop()
	blockPageServer.Stop()

	logger.Info("Shutdown complete")
}
