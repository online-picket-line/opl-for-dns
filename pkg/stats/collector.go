// Package stats provides DNS query statistics collection and reporting.
package stats

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Collector tracks DNS query statistics.
type Collector struct {
	totalQueries     atomic.Int64
	queriesBlocked   atomic.Int64
	queriesForwarded atomic.Int64
	bypassesIssued   atomic.Int64

	// For delta calculation
	lastReportQueries   atomic.Int64
	lastReportBlocked   atomic.Int64
	lastReportForwarded atomic.Int64
	lastReportBypasses  atomic.Int64

	// Top blocked domains tracking
	mu             sync.Mutex
	blockedDomains map[string]int64

	startTime time.Time
}

// NewCollector creates a new stats collector.
func NewCollector() *Collector {
	return &Collector{
		blockedDomains: make(map[string]int64),
		startTime:      time.Now(),
	}
}

// RecordQuery records a DNS query that was forwarded to upstream.
func (c *Collector) RecordQuery() {
	c.totalQueries.Add(1)
	c.queriesForwarded.Add(1)
}

// RecordBlock records a DNS query that was blocked.
func (c *Collector) RecordBlock(domain string) {
	c.totalQueries.Add(1)
	c.queriesBlocked.Add(1)

	c.mu.Lock()
	c.blockedDomains[domain]++
	c.mu.Unlock()
}

// RecordBypass records a bypass being issued.
func (c *Collector) RecordBypass() {
	c.bypassesIssued.Add(1)
}

// DomainCount holds a domain and its block count.
type DomainCount struct {
	Domain string `json:"domain"`
	Count  int64  `json:"count"`
}

// TopBlockedDomains returns the top N blocked domains.
func (c *Collector) TopBlockedDomains(n int) []DomainCount {
	c.mu.Lock()
	defer c.mu.Unlock()

	domains := make([]DomainCount, 0, len(c.blockedDomains))
	for domain, count := range c.blockedDomains {
		domains = append(domains, DomainCount{Domain: domain, Count: count})
	}

	sort.Slice(domains, func(i, j int) bool {
		return domains[i].Count > domains[j].Count
	})

	if len(domains) > n {
		domains = domains[:n]
	}
	return domains
}

// Snapshot returns a point-in-time snapshot of all counters.
func (c *Collector) Snapshot() (totalQueries, blocked, forwarded, bypasses int64) {
	return c.totalQueries.Load(), c.queriesBlocked.Load(), c.queriesForwarded.Load(), c.bypassesIssued.Load()
}

// Uptime returns the duration since the collector was created.
func (c *Collector) Uptime() time.Duration {
	return time.Since(c.startTime)
}

// computeDeltas calculates the delta since last report and updates the baseline.
func (c *Collector) computeDeltas() (dQueries, dBlocked, dForwarded, dBypasses int64) {
	total := c.totalQueries.Load()
	blocked := c.queriesBlocked.Load()
	forwarded := c.queriesForwarded.Load()
	bypasses := c.bypassesIssued.Load()

	dQueries = total - c.lastReportQueries.Load()
	dBlocked = blocked - c.lastReportBlocked.Load()
	dForwarded = forwarded - c.lastReportForwarded.Load()
	dBypasses = bypasses - c.lastReportBypasses.Load()

	c.lastReportQueries.Store(total)
	c.lastReportBlocked.Store(blocked)
	c.lastReportForwarded.Store(forwarded)
	c.lastReportBypasses.Store(bypasses)

	return
}

// StatsReport is the payload sent to the OPL backend.
type StatsReport struct {
	InstanceID           string        `json:"instanceId"`
	Version              string        `json:"version"`
	Uptime               int64         `json:"uptime"` // seconds
	TotalQueries         int64         `json:"totalQueries"`
	QueriesBlocked       int64         `json:"queriesBlocked"`
	QueriesForwarded     int64         `json:"queriesForwarded"`
	BypassesIssued       int64         `json:"bypassesIssued"`
	ActiveSessions       int           `json:"activeSessions"`
	BlocklistSize        int           `json:"blocklistSize"`
	BlocklistEmployers   int           `json:"blocklistEmployers"`
	LastBlocklistRefresh string        `json:"lastBlocklistRefresh,omitempty"`
	TopBlockedDomains    []DomainCount `json:"topBlockedDomains"`

	// Deltas since last report
	QueriesSinceLastReport   int64 `json:"queriesSinceLastReport"`
	BlockedSinceLastReport   int64 `json:"blockedSinceLastReport"`
	ForwardedSinceLastReport int64 `json:"forwardedSinceLastReport"`
	BypassesSinceLastReport  int64 `json:"bypassesSinceLastReport"`
}

// Reporter periodically sends stats reports to the OPL backend.
type Reporter struct {
	collector  *Collector
	instanceID string
	version    string
	reportURL  string
	apiKey     string
	interval   time.Duration
	httpClient *http.Client
	logger     *slog.Logger

	// Callbacks to get dynamic data
	getActiveSessions func() int
	getBlocklistSize  func() (domains int, employers int)
	getLastRefresh    func() time.Time
}

// ReporterConfig holds configuration for the stats reporter.
type ReporterConfig struct {
	Collector  *Collector
	InstanceID string
	Version    string
	ReportURL  string // e.g. "https://onlinepicketline.com/api/dns-stats/report"
	APIKey     string
	Interval   time.Duration
	Logger     *slog.Logger

	// Callbacks
	GetActiveSessions func() int
	GetBlocklistSize  func() (domains int, employers int)
	GetLastRefresh    func() time.Time
}

// NewReporter creates a stats reporter.
func NewReporter(cfg ReporterConfig) *Reporter {
	return &Reporter{
		collector:         cfg.Collector,
		instanceID:        cfg.InstanceID,
		version:           cfg.Version,
		reportURL:         cfg.ReportURL,
		apiKey:            cfg.APIKey,
		interval:          cfg.Interval,
		logger:            cfg.Logger,
		httpClient:        &http.Client{Timeout: 10 * time.Second},
		getActiveSessions: cfg.GetActiveSessions,
		getBlocklistSize:  cfg.GetBlocklistSize,
		getLastRefresh:    cfg.GetLastRefresh,
	}
}

// Start begins periodic reporting. It blocks until the context is cancelled.
func (r *Reporter) Start(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	r.logger.Info("Stats reporter started",
		"instanceId", r.instanceID,
		"interval", r.interval,
		"reportUrl", r.reportURL,
	)

	for {
		select {
		case <-ctx.Done():
			// Send final report before exiting
			r.sendReport(context.Background())
			return
		case <-ticker.C:
			r.sendReport(ctx)
		}
	}
}

func (r *Reporter) sendReport(ctx context.Context) {
	total, blocked, forwarded, bypasses := r.collector.Snapshot()
	dQueries, dBlocked, dForwarded, dBypasses := r.collector.computeDeltas()

	activeSessions := 0
	if r.getActiveSessions != nil {
		activeSessions = r.getActiveSessions()
	}

	blocklistDomains, blocklistEmployers := 0, 0
	if r.getBlocklistSize != nil {
		blocklistDomains, blocklistEmployers = r.getBlocklistSize()
	}

	var lastRefreshStr string
	if r.getLastRefresh != nil {
		lastRefresh := r.getLastRefresh()
		if !lastRefresh.IsZero() {
			lastRefreshStr = lastRefresh.Format(time.RFC3339)
		}
	}

	report := StatsReport{
		InstanceID:               r.instanceID,
		Version:                  r.version,
		Uptime:                   int64(r.collector.Uptime().Seconds()),
		TotalQueries:             total,
		QueriesBlocked:           blocked,
		QueriesForwarded:         forwarded,
		BypassesIssued:           bypasses,
		ActiveSessions:           activeSessions,
		BlocklistSize:            blocklistDomains,
		BlocklistEmployers:       blocklistEmployers,
		LastBlocklistRefresh:     lastRefreshStr,
		TopBlockedDomains:        r.collector.TopBlockedDomains(10),
		QueriesSinceLastReport:   dQueries,
		BlockedSinceLastReport:   dBlocked,
		ForwardedSinceLastReport: dForwarded,
		BypassesSinceLastReport:  dBypasses,
	}

	body, err := json.Marshal(report)
	if err != nil {
		r.logger.Error("Failed to marshal stats report", "error", err)
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.reportURL, bytes.NewReader(body))
	if err != nil {
		r.logger.Error("Failed to create stats report request", "error", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", r.apiKey)
	req.Header.Set("User-Agent", fmt.Sprintf("OPL-DNS-Server/%s", r.version))

	resp, err := r.httpClient.Do(req)
	if err != nil {
		r.logger.Warn("Failed to send stats report", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		r.logger.Warn("Stats report rejected by server",
			"status", resp.StatusCode,
			"instanceId", r.instanceID,
		)
		return
	}

	r.logger.Debug("Stats report sent",
		"totalQueries", total,
		"blocked", blocked,
		"deltaQueries", dQueries,
	)
}
