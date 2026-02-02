package session

import (
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	m := NewManager("test-secret", 24*time.Hour)
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if m.tokenTTL != 24*time.Hour {
		t.Errorf("Expected token TTL 24h, got %v", m.tokenTTL)
	}
}

func TestCreateBypassToken(t *testing.T) {
	m := NewManager("test-secret", 24*time.Hour)

	token, err := m.CreateBypassToken("192.168.1.100", "example.com")
	if err != nil {
		t.Fatalf("CreateBypassToken failed: %v", err)
	}
	if token == "" {
		t.Error("Expected non-empty token")
	}
}

func TestValidateBypassToken(t *testing.T) {
	m := NewManager("test-secret", 24*time.Hour)

	clientIP := "192.168.1.100"
	domain := "example.com"

	// Create token
	token, err := m.CreateBypassToken(clientIP, domain)
	if err != nil {
		t.Fatalf("CreateBypassToken failed: %v", err)
	}

	// Validate token
	session, err := m.ValidateBypassToken(token, clientIP, domain)
	if err != nil {
		t.Fatalf("ValidateBypassToken failed: %v", err)
	}
	if session.ClientIP != clientIP {
		t.Errorf("Expected client IP '%s', got '%s'", clientIP, session.ClientIP)
	}
	if session.Domain != domain {
		t.Errorf("Expected domain '%s', got '%s'", domain, session.Domain)
	}
}

func TestValidateBypassTokenInvalid(t *testing.T) {
	m := NewManager("test-secret", 24*time.Hour)

	// Invalid token format
	_, err := m.ValidateBypassToken("not-a-valid-token", "192.168.1.100", "example.com")
	if err != ErrInvalidToken {
		t.Errorf("Expected ErrInvalidToken, got %v", err)
	}
}

func TestValidateBypassTokenDomainMismatch(t *testing.T) {
	m := NewManager("test-secret", 24*time.Hour)

	token, _ := m.CreateBypassToken("192.168.1.100", "example.com")

	// Try to validate with different domain
	_, err := m.ValidateBypassToken(token, "192.168.1.100", "other.com")
	if err != ErrDomainMismatch {
		t.Errorf("Expected ErrDomainMismatch, got %v", err)
	}
}

func TestValidateBypassTokenExpired(t *testing.T) {
	// Create manager with very short TTL
	m := NewManager("test-secret", 1*time.Millisecond)

	token, _ := m.CreateBypassToken("192.168.1.100", "example.com")

	// Wait for token to expire
	time.Sleep(10 * time.Millisecond)

	// Validate should fail
	_, err := m.ValidateBypassToken(token, "192.168.1.100", "example.com")
	if err != ErrExpiredToken {
		t.Errorf("Expected ErrExpiredToken, got %v", err)
	}
}

func TestHasBypass(t *testing.T) {
	m := NewManager("test-secret", 24*time.Hour)

	clientIP := "192.168.1.100"
	domain := "example.com"

	// Initially no bypass
	if m.HasBypass(clientIP, domain) {
		t.Error("Expected no bypass initially")
	}

	// Create bypass
	m.CreateBypassToken(clientIP, domain)

	// Now should have bypass
	if !m.HasBypass(clientIP, domain) {
		t.Error("Expected bypass after creating token")
	}

	// Different domain should not have bypass
	if m.HasBypass(clientIP, "other.com") {
		t.Error("Expected no bypass for different domain")
	}
}

func TestRevokeBypass(t *testing.T) {
	m := NewManager("test-secret", 24*time.Hour)

	clientIP := "192.168.1.100"
	domain := "example.com"

	// Create bypass
	m.CreateBypassToken(clientIP, domain)
	if !m.HasBypass(clientIP, domain) {
		t.Fatal("Expected bypass after creating token")
	}

	// Revoke bypass
	m.RevokeBypass(clientIP, domain)

	// Should no longer have bypass
	if m.HasBypass(clientIP, domain) {
		t.Error("Expected no bypass after revocation")
	}
}

func TestCleanupExpired(t *testing.T) {
	m := NewManager("test-secret", 1*time.Millisecond)

	// Create multiple sessions
	m.CreateBypassToken("192.168.1.1", "example1.com")
	m.CreateBypassToken("192.168.1.2", "example2.com")
	m.CreateBypassToken("192.168.1.3", "example3.com")

	// Wait for tokens to expire
	time.Sleep(10 * time.Millisecond)

	// Cleanup
	cleaned := m.CleanupExpired()
	if cleaned != 3 {
		t.Errorf("Expected 3 cleaned sessions, got %d", cleaned)
	}

	// Should be no active sessions
	if m.GetActiveSessionCount() != 0 {
		t.Errorf("Expected 0 active sessions, got %d", m.GetActiveSessionCount())
	}
}

func TestGetActiveSessionCount(t *testing.T) {
	m := NewManager("test-secret", 24*time.Hour)

	// Initially 0
	if m.GetActiveSessionCount() != 0 {
		t.Errorf("Expected 0 active sessions initially")
	}

	// Add sessions
	m.CreateBypassToken("192.168.1.1", "example1.com")
	m.CreateBypassToken("192.168.1.2", "example2.com")

	if m.GetActiveSessionCount() != 2 {
		t.Errorf("Expected 2 active sessions, got %d", m.GetActiveSessionCount())
	}
}

func TestNormalizeIP(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"192.168.1.100", "192.168.1.100"},
		{"192.168.1.100:8080", "192.168.1.100"},
		{"::1", "::1"},
		{"[::1]:8080", "::1"},
	}

	for _, tt := range tests {
		result := normalizeIP(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeIP(%s): expected '%s', got '%s'", tt.input, tt.expected, result)
		}
	}
}

func TestNormalizeDomain(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"example.com", "example.com"},
		{"Example.COM", "example.com"},
		{"example.com.", "example.com"},
		{"EXAMPLE.COM.", "example.com"},
	}

	for _, tt := range tests {
		result := normalizeDomain(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeDomain(%s): expected '%s', got '%s'", tt.input, tt.expected, result)
		}
	}
}

func TestIPMatches(t *testing.T) {
	tests := []struct {
		ip1      string
		ip2      string
		expected bool
	}{
		{"192.168.1.100", "192.168.1.100", true},
		{"192.168.1.100", "192.168.1.101", false},
		{"::1", "::1", true},
		{"::1", "::2", false},
		{"invalid", "192.168.1.100", false},
	}

	for _, tt := range tests {
		result := ipMatches(tt.ip1, tt.ip2)
		if result != tt.expected {
			t.Errorf("ipMatches(%s, %s): expected %v, got %v", tt.ip1, tt.ip2, tt.expected, result)
		}
	}
}

func TestConcurrentAccess(t *testing.T) {
	m := NewManager("test-secret", 24*time.Hour)

	// Concurrent creation
	done := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func(n int) {
			m.CreateBypassToken("192.168.1.1", "example.com")
			m.HasBypass("192.168.1.1", "example.com")
			done <- true
		}(i)
	}

	for i := 0; i < 100; i++ {
		<-done
	}

	// Should have exactly 1 session (same IP+domain overwrites)
	if m.GetActiveSessionCount() != 1 {
		t.Errorf("Expected 1 active session, got %d", m.GetActiveSessionCount())
	}
}
