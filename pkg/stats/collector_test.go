package stats

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCollector_RecordQuery(t *testing.T) {
	c := NewCollector()

	c.RecordQuery()
	c.RecordQuery()
	c.RecordQuery()

	total, _, forwarded, _ := c.Snapshot()
	if total != 3 {
		t.Errorf("expected 3 total queries, got %d", total)
	}
	if forwarded != 3 {
		t.Errorf("expected 3 forwarded queries, got %d", forwarded)
	}
}

func TestCollector_RecordBlock(t *testing.T) {
	c := NewCollector()

	c.RecordBlock("example.com")
	c.RecordBlock("example.com")
	c.RecordBlock("test.org")

	total, blocked, forwarded, _ := c.Snapshot()
	if total != 3 {
		t.Errorf("expected 3 total queries, got %d", total)
	}
	if blocked != 3 {
		t.Errorf("expected 3 blocked queries, got %d", blocked)
	}
	if forwarded != 0 {
		t.Errorf("expected 0 forwarded queries, got %d", forwarded)
	}
}

func TestCollector_RecordBypass(t *testing.T) {
	c := NewCollector()

	c.RecordBypass()
	c.RecordBypass()

	_, _, _, bypasses := c.Snapshot()
	if bypasses != 2 {
		t.Errorf("expected 2 bypasses, got %d", bypasses)
	}
}

func TestCollector_TopBlockedDomains(t *testing.T) {
	c := NewCollector()

	c.RecordBlock("aaa.com")
	c.RecordBlock("bbb.com")
	c.RecordBlock("bbb.com")
	c.RecordBlock("ccc.com")
	c.RecordBlock("ccc.com")
	c.RecordBlock("ccc.com")

	top := c.TopBlockedDomains(2)
	if len(top) != 2 {
		t.Fatalf("expected 2 top domains, got %d", len(top))
	}
	if top[0].Domain != "ccc.com" {
		t.Errorf("expected top domain to be ccc.com, got %s", top[0].Domain)
	}
	if top[0].Count != 3 {
		t.Errorf("expected top domain count 3, got %d", top[0].Count)
	}
	if top[1].Domain != "bbb.com" {
		t.Errorf("expected second domain to be bbb.com, got %s", top[1].Domain)
	}
}

func TestCollector_ComputeDeltas(t *testing.T) {
	c := NewCollector()

	// First batch
	c.RecordBlock("a.com")
	c.RecordQuery()
	c.RecordBypass()

	dQ, dB, dF, dBp := c.computeDeltas()
	if dQ != 2 {
		t.Errorf("expected delta queries 2, got %d", dQ)
	}
	if dB != 1 {
		t.Errorf("expected delta blocked 1, got %d", dB)
	}
	if dF != 1 {
		t.Errorf("expected delta forwarded 1, got %d", dF)
	}
	if dBp != 1 {
		t.Errorf("expected delta bypasses 1, got %d", dBp)
	}

	// Second batch
	c.RecordQuery()
	c.RecordQuery()

	dQ, dB, dF, dBp = c.computeDeltas()
	if dQ != 2 {
		t.Errorf("expected delta queries 2, got %d", dQ)
	}
	if dB != 0 {
		t.Errorf("expected delta blocked 0, got %d", dB)
	}
	if dF != 2 {
		t.Errorf("expected delta forwarded 2, got %d", dF)
	}
	if dBp != 0 {
		t.Errorf("expected delta bypasses 0, got %d", dBp)
	}
}

func TestCollector_Uptime(t *testing.T) {
	c := NewCollector()
	time.Sleep(10 * time.Millisecond)

	up := c.Uptime()
	if up < 10*time.Millisecond {
		t.Errorf("expected uptime >= 10ms, got %v", up)
	}
}

func TestReporter_SendReport(t *testing.T) {
	var receivedReport StatsReport

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Errorf("expected API key 'test-key', got %s", r.Header.Get("X-API-Key"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		if err := json.NewDecoder(r.Body).Decode(&receivedReport); err != nil {
			t.Errorf("failed to decode report: %v", err)
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"message": "ok"})
	}))
	defer server.Close()

	c := NewCollector()
	c.RecordBlock("test.com")
	c.RecordBlock("test.com")
	c.RecordQuery()

	reporter := NewReporter(ReporterConfig{
		Collector:         c,
		InstanceID:        "test-instance",
		Version:           "1.0.0-test",
		ReportURL:         server.URL,
		APIKey:            "test-key",
		Interval:          1 * time.Second,
		Logger:            slog.Default(),
		GetActiveSessions: func() int { return 5 },
		GetBlocklistSize:  func() (int, int) { return 42, 3 },
		GetLastRefresh:    func() time.Time { return time.Now() },
	})

	reporter.sendReport(context.Background())

	if receivedReport.InstanceID != "test-instance" {
		t.Errorf("expected instanceId 'test-instance', got %s", receivedReport.InstanceID)
	}
	if receivedReport.TotalQueries != 3 {
		t.Errorf("expected 3 total queries, got %d", receivedReport.TotalQueries)
	}
	if receivedReport.QueriesBlocked != 2 {
		t.Errorf("expected 2 blocked, got %d", receivedReport.QueriesBlocked)
	}
	if receivedReport.ActiveSessions != 5 {
		t.Errorf("expected 5 active sessions, got %d", receivedReport.ActiveSessions)
	}
	if receivedReport.BlocklistSize != 42 {
		t.Errorf("expected blocklist size 42, got %d", receivedReport.BlocklistSize)
	}
}
