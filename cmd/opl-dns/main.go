// Package main provides the entry point for the OPL DNS server.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/online-picket-line/opl-for-dns/pkg/api"
	"github.com/online-picket-line/opl-for-dns/pkg/config"
	"github.com/online-picket-line/opl-for-dns/pkg/dns"
	"github.com/online-picket-line/opl-for-dns/pkg/stats"
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

	// Create stats collector
	statsCollector := stats.NewCollector()

	// Create DNS server
	dnsServer, err := dns.NewServer(
		cfg.DNS.ListenAddr,
		cfg.DNS.UpstreamDNS,
		cfg.DNS.QueryTimeout.Duration,
		apiClient,
		statsCollector,
		logger.With("component", "dns"),
	)
	if err != nil {
		logger.Error("Error creating DNS server", "error", err)
		os.Exit(1)
	}

	// Context for shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initial blocklist fetch with retries
	logger.Info("Fetching initial blocklist...")
	for attempt := 1; attempt <= 10; attempt++ {
		if _, err := apiClient.FetchBlocklist(ctx); err != nil {
			logger.Warn("Error fetching initial blocklist", "error", err, "attempt", attempt, "maxAttempts", 10)
			if attempt < 10 {
				delay := time.Duration(attempt) * 3 * time.Second
				if delay > 30*time.Second {
					delay = 30 * time.Second
				}
				logger.Info("Retrying blocklist fetch...", "delay", delay)
				select {
				case <-time.After(delay):
				case <-ctx.Done():
					break
				}
			}
		} else {
			blocklist := apiClient.GetCachedBlocklist()
			if blocklist != nil {
				logger.Info("Blocklist loaded", "urls", blocklist.TotalURLs, "employers", len(blocklist.Employers))
			}
			break
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

	// Start stats reporter goroutine if enabled
	if cfg.Stats.Enabled {
		// Determine instance ID
		instanceID := cfg.Stats.InstanceID
		if instanceID == "" {
			if hostname, err := os.Hostname(); err == nil {
				instanceID = hostname
			} else {
				instanceID = "opl-dns-unknown"
			}
		}

		// Determine report URL
		reportURL := cfg.Stats.ReportURL
		if reportURL == "" {
			reportURL = strings.TrimSuffix(cfg.API.BaseURL, "/") + "/dns-stats/report"
		}

		reporter := stats.NewReporter(stats.ReporterConfig{
			Collector:  statsCollector,
			InstanceID: instanceID,
			Version:    version,
			ReportURL:  reportURL,
			APIKey:     cfg.API.APIKey,
			Interval:   cfg.Stats.ReportInterval.Duration,
			Logger:     logger.With("component", "stats"),
			GetBlocklistSize: func() (int, int) {
				blocklist := apiClient.GetCachedBlocklist()
				if blocklist == nil {
					return 0, 0
				}
				return blocklist.TotalURLs, len(blocklist.Employers)
			},
			GetLastRefresh: func() time.Time {
				return apiClient.LastFetchTime()
			},
		})

		go reporter.Start(ctx)
		logger.Info("Stats reporting enabled", "instanceId", instanceID, "interval", cfg.Stats.ReportInterval.Duration)
	}

	// Start servers
	errChan := make(chan error, 2)

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

	logger.Info("Shutdown complete")
}
