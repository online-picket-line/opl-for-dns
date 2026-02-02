package blockpage

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/online-picket-line/opl-for-dns/pkg/api"
	"github.com/online-picket-line/opl-for-dns/pkg/session"
)

func setupTestServer(t *testing.T) (*Server, *api.Client, *session.Manager) {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	apiClient := api.NewClient("https://api.example.com", "", 10*time.Second)
	sessionMgr := session.NewManager("test-secret", 24*time.Hour)

	// Setup blocklist
	apiClient.SetBlocklistForTesting(&api.Blocklist{
		BlockList: []api.BlockListItem{
			{
				URL:         "https://blocked.com",
				Employer:    "Test Corp",
				MoreInfoURL: "https://example.com/info",
				Location:    "Test City",
				ActionDetails: api.ActionDetails{
					ActionType:   "strike",
					Description:  "Workers on strike for better wages",
					Demands:      "15% wage increase",
					Organization: "Test Union",
				},
			},
		},
	})

	server, err := NewServer(
		"127.0.0.1:8080",
		"http://localhost:8080",
		"block",
		apiClient,
		sessionMgr,
		logger,
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	return server, apiClient, sessionMgr
}

func TestHandleHealth(t *testing.T) {
	server, _, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Status != "ok" {
		t.Errorf("Expected status 'ok', got '%s'", resp.Status)
	}
	if !resp.BlocklistLoaded {
		t.Error("Expected blocklist to be loaded")
	}
}

func TestHandleCheck(t *testing.T) {
	server, _, _ := setupTestServer(t)

	tests := []struct {
		name         string
		domain       string
		expectedCode int
		blocked      bool
	}{
		{
			name:         "blocked domain",
			domain:       "blocked.com",
			expectedCode: http.StatusOK,
			blocked:      true,
		},
		{
			name:         "non-blocked domain",
			domain:       "notblocked.com",
			expectedCode: http.StatusOK,
			blocked:      false,
		},
		{
			name:         "missing domain",
			domain:       "",
			expectedCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/api/check"
			if tt.domain != "" {
				url += "?domain=" + tt.domain
			}

			req := httptest.NewRequest(http.MethodGet, url, nil)
			w := httptest.NewRecorder()

			server.handleCheck(w, req)

			if w.Code != tt.expectedCode {
				t.Errorf("Expected status %d, got %d", tt.expectedCode, w.Code)
			}

			if tt.expectedCode == http.StatusOK {
				var resp CheckResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}

				if resp.Blocked != tt.blocked {
					t.Errorf("Expected blocked=%v, got %v", tt.blocked, resp.Blocked)
				}

				if tt.blocked && resp.Employer == "" {
					t.Error("Expected employer for blocked domain")
				}
			}
		})
	}
}

func TestHandleBypassGet(t *testing.T) {
	server, _, sessionMgr := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/bypass?domain=blocked.com", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	w := httptest.NewRecorder()

	server.handleBypass(w, req)

	// Should redirect
	if w.Code != http.StatusTemporaryRedirect {
		t.Errorf("Expected redirect, got %d", w.Code)
	}

	// Should have bypass
	if !sessionMgr.HasBypass("192.168.1.100", "blocked.com") {
		t.Error("Expected bypass to be created")
	}
}

func TestHandleBypassPost(t *testing.T) {
	server, _, sessionMgr := setupTestServer(t)

	body := strings.NewReader(`{"domain": "blocked.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/bypass", body)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.168.1.100:12345"
	w := httptest.NewRecorder()

	server.handleBypass(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var resp BypassResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !resp.Success {
		t.Errorf("Expected success, got error: %s", resp.Error)
	}
	if resp.Token == "" {
		t.Error("Expected token in response")
	}
	if resp.RedirectURL != "https://blocked.com" {
		t.Errorf("Expected redirect URL 'https://blocked.com', got '%s'", resp.RedirectURL)
	}

	// Should have bypass
	if !sessionMgr.HasBypass("192.168.1.100", "blocked.com") {
		t.Error("Expected bypass to be created")
	}
}

func TestHandleBypassNotBlockedDomain(t *testing.T) {
	server, _, _ := setupTestServer(t)

	body := strings.NewReader(`{"domain": "notblocked.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/bypass", body)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.168.1.100:12345"
	w := httptest.NewRecorder()

	server.handleBypass(w, req)

	var resp BypassResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Success {
		t.Error("Expected failure for non-blocked domain")
	}
}

func TestHandleBlockPageBlocked(t *testing.T) {
	server, _, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/?domain=blocked.com", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	w := httptest.NewRecorder()

	server.handleBlockPage(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Test Corp") {
		t.Error("Expected employer name in block page")
	}
	if !strings.Contains(body, "strike") {
		t.Error("Expected action type in block page")
	}
}

func TestHandleBlockPageNotBlocked(t *testing.T) {
	server, _, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/?domain=notblocked.com", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	w := httptest.NewRecorder()

	server.handleBlockPage(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 for non-blocked domain, got %d", w.Code)
	}
}

func TestHandleBlockPageWithBypass(t *testing.T) {
	server, _, sessionMgr := setupTestServer(t)

	// Create bypass first
	sessionMgr.CreateBypassToken("192.168.1.100", "blocked.com")

	req := httptest.NewRequest(http.MethodGet, "/?domain=blocked.com", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	w := httptest.NewRecorder()

	server.handleBlockPage(w, req)

	// Should redirect to original site
	if w.Code != http.StatusTemporaryRedirect {
		t.Errorf("Expected redirect for bypassed domain, got %d", w.Code)
	}
}

func TestHandleBlockPageOverlayMode(t *testing.T) {
	server, _, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/?domain=blocked.com&mode=overlay", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	w := httptest.NewRecorder()

	server.handleBlockPage(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	// Overlay template should have the overlay class
	if !strings.Contains(body, "overlay") {
		t.Error("Expected overlay class in response")
	}
}

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string]string
		remoteAddr string
		expected   string
	}{
		{
			name:       "from RemoteAddr",
			headers:    nil,
			remoteAddr: "192.168.1.100:12345",
			expected:   "192.168.1.100",
		},
		{
			name: "from X-Forwarded-For",
			headers: map[string]string{
				"X-Forwarded-For": "10.0.0.1, 192.168.1.1",
			},
			remoteAddr: "192.168.1.100:12345",
			expected:   "10.0.0.1",
		},
		{
			name: "from X-Real-IP",
			headers: map[string]string{
				"X-Real-IP": "10.0.0.2",
			},
			remoteAddr: "192.168.1.100:12345",
			expected:   "10.0.0.2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			result := getClientIP(req)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}
