package dns

import (
	"log/slog"
	"net"
	"os"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/online-picket-line/opl-for-dns/pkg/api"
	"github.com/online-picket-line/opl-for-dns/pkg/session"
)

func TestNewServer(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	apiClient := api.NewClient("https://api.example.com", "", 10*time.Second)
	sessionMgr := session.NewManager("test-secret", 24*time.Hour)

	server, err := NewServer(
		"127.0.0.1:5353",
		"192.168.1.100",
		[]string{"8.8.8.8:53"},
		5*time.Second,
		apiClient,
		sessionMgr,
		nil,
		logger,
	)

	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	if server == nil {
		t.Fatal("NewServer returned nil")
	}
}

func TestNewServerInvalidBlockPageIP(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	apiClient := api.NewClient("https://api.example.com", "", 10*time.Second)
	sessionMgr := session.NewManager("test-secret", 24*time.Hour)

	_, err := NewServer(
		"127.0.0.1:5353",
		"invalid-ip",
		[]string{"8.8.8.8:53"},
		5*time.Second,
		apiClient,
		sessionMgr,
		nil,
		logger,
	)

	if err == nil {
		t.Error("Expected error for invalid block page IP")
	}
}

func TestGetBlockedDomainInfo(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	apiClient := api.NewClient("https://api.example.com", "", 10*time.Second)
	sessionMgr := session.NewManager("test-secret", 24*time.Hour)

	// Setup blocklist
	apiClient.SetBlocklistForTesting(&api.Blocklist{
		BlockList: []api.BlockListItem{
			{
				URL:      "https://example.com",
				Employer: "Test Corp",
				Location: "Test City",
				ActionDetails: api.ActionDetails{
					ActionType:   "strike",
					Description:  "Test strike",
					Organization: "Test Union",
				},
			},
		},
	})

	server, _ := NewServer(
		"127.0.0.1:5353",
		"192.168.1.100",
		[]string{"8.8.8.8:53"},
		5*time.Second,
		apiClient,
		sessionMgr,
		nil,
		logger,
	)

	// Test blocked domain
	info, blocked := server.GetBlockedDomainInfo("example.com")
	if !blocked {
		t.Error("Expected domain to be blocked")
	}
	if info.Employer != "Test Corp" {
		t.Errorf("Expected employer 'Test Corp', got '%s'", info.Employer)
	}
	if info.ActionType != "strike" {
		t.Errorf("Expected action type 'strike', got '%s'", info.ActionType)
	}

	// Test non-blocked domain
	_, blocked = server.GetBlockedDomainInfo("notblocked.com")
	if blocked {
		t.Error("Expected domain to not be blocked")
	}
}

func TestServeDNSEmptyQuestion(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	apiClient := api.NewClient("https://api.example.com", "", 10*time.Second)
	sessionMgr := session.NewManager("test-secret", 24*time.Hour)

	server, _ := NewServer(
		"127.0.0.1:5353",
		"192.168.1.100",
		[]string{"8.8.8.8:53"},
		5*time.Second,
		apiClient,
		sessionMgr,
		nil,
		logger,
	)

	// Create request with no questions
	r := new(dns.Msg)
	r.SetQuestion("", dns.TypeA)
	r.Question = nil // Remove questions

	w := &mockDNSWriter{}
	server.ServeDNS(w, r)

	if w.msg == nil {
		t.Error("Expected response message")
	}
}

// mockDNSWriter is a mock implementation of dns.ResponseWriter
type mockDNSWriter struct {
	msg *dns.Msg
}

func (m *mockDNSWriter) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 53}
}

func (m *mockDNSWriter) RemoteAddr() net.Addr {
	return &net.UDPAddr{IP: net.ParseIP("192.168.1.50"), Port: 12345}
}

func (m *mockDNSWriter) WriteMsg(msg *dns.Msg) error {
	m.msg = msg
	return nil
}

func (m *mockDNSWriter) Write([]byte) (int, error) {
	return 0, nil
}

func (m *mockDNSWriter) Close() error {
	return nil
}

func (m *mockDNSWriter) TsigStatus() error {
	return nil
}

func (m *mockDNSWriter) TsigTimersOnly(bool) {
}

func (m *mockDNSWriter) Hijack() {
}

type mockUDPAddr struct{}

func (m *mockUDPAddr) Network() string {
	return "udp"
}

func (m *mockUDPAddr) String() string {
	return "192.168.1.50:12345"
}
