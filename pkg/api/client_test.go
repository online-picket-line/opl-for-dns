package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	client := NewClient("https://api.example.com", "test-key", 10*time.Second)
	if client == nil {
		t.Fatal("NewClient returned nil")
	}
	if client.baseURL != "https://api.example.com" {
		t.Errorf("Expected base URL 'https://api.example.com', got '%s'", client.baseURL)
	}
	if client.apiKey != "test-key" {
		t.Errorf("Expected API key 'test-key', got '%s'", client.apiKey)
	}
}

func TestNewClientTrimsTrailingSlash(t *testing.T) {
	client := NewClient("https://api.example.com/", "test-key", 10*time.Second)
	if client.baseURL != "https://api.example.com" {
		t.Errorf("Expected base URL without trailing slash, got '%s'", client.baseURL)
	}
}

func TestFetchBlocklist(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.URL.Path != "/blocklist.json" {
			t.Errorf("Expected path '/blocklist.json', got '%s'", r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Errorf("Expected API key header 'test-key', got '%s'", r.Header.Get("X-API-Key"))
		}
		if r.Header.Get("User-Agent") != "OPL-DNS-Server/1.0.0" {
			t.Errorf("Expected User-Agent 'OPL-DNS-Server/1.0.0', got '%s'", r.Header.Get("User-Agent"))
		}

		// Return mock response
		resp := Blocklist{
			Version:     "1.0",
			GeneratedAt: "2024-01-15T10:30:00Z",
			TotalURLs:   2,
			Employers: []Employer{
				{ID: "emp-1", Name: "Test Corp", URLCount: 2},
			},
			BlockList: []BlockListItem{
				{
					URL:        "https://example.com",
					Employer:   "Test Corp",
					EmployerID: "emp-1",
					ActionDetails: ActionDetails{
						ActionType:  "strike",
						Description: "Workers on strike",
					},
				},
				{
					URL:        "https://test.example.com",
					Employer:   "Test Corp",
					EmployerID: "emp-1",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Content-Hash", "abc123")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", 10*time.Second)
	blocklist, err := client.FetchBlocklist(context.Background())
	if err != nil {
		t.Fatalf("FetchBlocklist failed: %v", err)
	}

	if blocklist.Version != "1.0" {
		t.Errorf("Expected version '1.0', got '%s'", blocklist.Version)
	}
	if blocklist.TotalURLs != 2 {
		t.Errorf("Expected 2 total URLs, got %d", blocklist.TotalURLs)
	}
	if len(blocklist.BlockList) != 2 {
		t.Errorf("Expected 2 blocklist items, got %d", len(blocklist.BlockList))
	}
}

func TestFetchBlocklistNotModified(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// First call - return data
			resp := Blocklist{
				Version:   "1.0",
				TotalURLs: 1,
				BlockList: []BlockListItem{
					{URL: "https://example.com", Employer: "Test"},
				},
			}
			w.Header().Set("X-Content-Hash", "hash123")
			json.NewEncoder(w).Encode(resp)
		} else {
			// Subsequent calls - return 304
			w.WriteHeader(http.StatusNotModified)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 10*time.Second)

	// First fetch
	blocklist1, err := client.FetchBlocklist(context.Background())
	if err != nil {
		t.Fatalf("First fetch failed: %v", err)
	}

	// Second fetch should return cached data
	blocklist2, err := client.FetchBlocklist(context.Background())
	if err != nil {
		t.Fatalf("Second fetch failed: %v", err)
	}

	if blocklist2.Version != blocklist1.Version {
		t.Error("Expected same blocklist from cache")
	}
}

func TestFetchBlocklistError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 10*time.Second)
	_, err := client.FetchBlocklist(context.Background())
	if err == nil {
		t.Error("Expected error for 500 response")
	}
}

func TestCheckDomain(t *testing.T) {
	// Setup client with mock blocklist
	client := NewClient("https://api.example.com", "", 10*time.Second)
	client.blocklist = &Blocklist{
		BlockList: []BlockListItem{
			{URL: "https://example.com", Employer: "Test Corp"},
			{URL: "https://www.blocked.com", Employer: "Another Corp"},
			{URL: "facebook.com/testcorp", Employer: "Test Corp"},
		},
	}
	// Build domain map
	client.blocklist.domainMap = make(map[string]*BlockListItem)
	for i := range client.blocklist.BlockList {
		item := &client.blocklist.BlockList[i]
		domain := extractDomain(item.URL)
		if domain != "" {
			client.blocklist.domainMap[domain] = item
		}
	}

	tests := []struct {
		domain   string
		blocked  bool
		employer string
	}{
		{"example.com", true, "Test Corp"},
		{"EXAMPLE.COM", true, "Test Corp"},
		{"www.example.com", true, "Test Corp"},     // Should match parent domain
		{"sub.example.com", true, "Test Corp"},     // Should match parent domain
		{"www.blocked.com", true, "Another Corp"},
		{"notblocked.com", false, ""},
		{"facebook.com", true, "Test Corp"},
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			item, blocked := client.CheckDomain(tt.domain)
			if blocked != tt.blocked {
				t.Errorf("CheckDomain(%s): expected blocked=%v, got %v", tt.domain, tt.blocked, blocked)
			}
			if blocked && item.Employer != tt.employer {
				t.Errorf("CheckDomain(%s): expected employer '%s', got '%s'", tt.domain, tt.employer, item.Employer)
			}
		})
	}
}

func TestCheckDomainNoBlocklist(t *testing.T) {
	client := NewClient("https://api.example.com", "", 10*time.Second)
	// No blocklist loaded

	_, blocked := client.CheckDomain("example.com")
	if blocked {
		t.Error("Expected not blocked when no blocklist loaded")
	}
}

func TestExtractDomain(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"https://example.com", "example.com"},
		{"https://example.com/path", "example.com"},
		{"http://www.example.com:8080/path", "www.example.com"},
		{"example.com", "example.com"},
		{"example.com/path", "example.com"},
		{"facebook.com/testpage", "facebook.com"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			result := extractDomain(tt.url)
			if result != tt.expected {
				t.Errorf("extractDomain(%s): expected '%s', got '%s'", tt.url, tt.expected, result)
			}
		})
	}
}

func TestGetCachedBlocklist(t *testing.T) {
	client := NewClient("https://api.example.com", "", 10*time.Second)

	// Initially nil
	if client.GetCachedBlocklist() != nil {
		t.Error("Expected nil blocklist initially")
	}

	// Set blocklist
	client.blocklist = &Blocklist{Version: "1.0"}

	// Should return cached
	if client.GetCachedBlocklist() == nil {
		t.Error("Expected non-nil blocklist after setting")
	}
	if client.GetCachedBlocklist().Version != "1.0" {
		t.Error("Expected cached blocklist version '1.0'")
	}
}

func TestLastFetchTime(t *testing.T) {
	client := NewClient("https://api.example.com", "", 10*time.Second)

	// Initially zero
	if !client.LastFetchTime().IsZero() {
		t.Error("Expected zero time initially")
	}

	// Set last fetch time
	now := time.Now()
	client.lastFetch = now

	if !client.LastFetchTime().Equal(now) {
		t.Error("Expected last fetch time to match")
	}
}
