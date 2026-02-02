// Package session provides session management for bypass tokens.
package session

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

var (
	// ErrInvalidToken is returned when a token is invalid.
	ErrInvalidToken = errors.New("invalid token")

	// ErrExpiredToken is returned when a token has expired.
	ErrExpiredToken = errors.New("token expired")

	// ErrDomainMismatch is returned when the token was issued for a different domain.
	ErrDomainMismatch = errors.New("domain mismatch")
)

// Manager manages bypass sessions for users.
type Manager struct {
	secret   []byte
	tokenTTL time.Duration

	mu       sync.RWMutex
	sessions map[string]*Session // key is clientIP:domain
}

// Session represents a bypass session for a specific client and domain.
type Session struct {
	ClientIP  string
	Domain    string
	Token     string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// NewManager creates a new session manager.
func NewManager(secret string, tokenTTL time.Duration) *Manager {
	return &Manager{
		secret:   []byte(secret),
		tokenTTL: tokenTTL,
		sessions: make(map[string]*Session),
	}
}

// CreateBypassToken creates a new bypass token for a client and domain.
func (m *Manager) CreateBypassToken(clientIP, domain string) (string, error) {
	// Normalize inputs
	clientIP = normalizeIP(clientIP)
	domain = normalizeDomain(domain)

	// Generate random bytes for token uniqueness
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("generating random bytes: %w", err)
	}

	// Create token data
	timestamp := time.Now().Unix()
	tokenData := fmt.Sprintf("%s|%s|%d|%s", clientIP, domain, timestamp, hex.EncodeToString(randomBytes))

	// Sign the token
	signature := m.sign(tokenData)

	// Encode as base64
	token := base64.URLEncoding.EncodeToString([]byte(tokenData + "|" + signature))

	// Create session
	session := &Session{
		ClientIP:  clientIP,
		Domain:    domain,
		Token:     token,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(m.tokenTTL),
	}

	// Store session
	key := sessionKey(clientIP, domain)
	m.mu.Lock()
	m.sessions[key] = session
	m.mu.Unlock()

	return token, nil
}

// ValidateBypassToken validates a bypass token and returns the session if valid.
func (m *Manager) ValidateBypassToken(token, clientIP, domain string) (*Session, error) {
	// Normalize inputs
	clientIP = normalizeIP(clientIP)
	domain = normalizeDomain(domain)

	// Decode token
	decoded, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return nil, ErrInvalidToken
	}

	// Parse token data
	parts := strings.Split(string(decoded), "|")
	if len(parts) != 5 {
		return nil, ErrInvalidToken
	}

	tokenIP := parts[0]
	tokenDomain := parts[1]
	tokenData := strings.Join(parts[:4], "|")
	providedSignature := parts[4]

	// Verify signature
	expectedSignature := m.sign(tokenData)
	if !hmac.Equal([]byte(providedSignature), []byte(expectedSignature)) {
		return nil, ErrInvalidToken
	}

	// Check domain match
	if normalizeDomain(tokenDomain) != domain {
		return nil, ErrDomainMismatch
	}

	// Check session exists and is not expired
	key := sessionKey(tokenIP, tokenDomain)
	m.mu.RLock()
	session, exists := m.sessions[key]
	m.mu.RUnlock()

	if !exists {
		return nil, ErrInvalidToken
	}

	if time.Now().After(session.ExpiresAt) {
		// Clean up expired session
		m.mu.Lock()
		delete(m.sessions, key)
		m.mu.Unlock()
		return nil, ErrExpiredToken
	}

	// Verify client IP matches (with some flexibility for NAT)
	if !ipMatches(tokenIP, clientIP) {
		return nil, ErrInvalidToken
	}

	return session, nil
}

// HasBypass checks if a client has a valid bypass for a domain.
func (m *Manager) HasBypass(clientIP, domain string) bool {
	clientIP = normalizeIP(clientIP)
	domain = normalizeDomain(domain)

	key := sessionKey(clientIP, domain)
	m.mu.RLock()
	session, exists := m.sessions[key]
	m.mu.RUnlock()

	if !exists {
		return false
	}

	return time.Now().Before(session.ExpiresAt)
}

// RevokeBypass revokes a bypass for a client and domain.
func (m *Manager) RevokeBypass(clientIP, domain string) {
	clientIP = normalizeIP(clientIP)
	domain = normalizeDomain(domain)

	key := sessionKey(clientIP, domain)
	m.mu.Lock()
	delete(m.sessions, key)
	m.mu.Unlock()
}

// CleanupExpired removes all expired sessions.
func (m *Manager) CleanupExpired() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	now := time.Now()
	for key, session := range m.sessions {
		if now.After(session.ExpiresAt) {
			delete(m.sessions, key)
			count++
		}
	}
	return count
}

// GetActiveSessionCount returns the number of active sessions.
func (m *Manager) GetActiveSessionCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

// sign creates an HMAC signature for the given data.
func (m *Manager) sign(data string) string {
	h := hmac.New(sha256.New, m.secret)
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

// sessionKey creates a unique key for a client and domain combination.
func sessionKey(clientIP, domain string) string {
	return clientIP + ":" + domain
}

// normalizeIP normalizes an IP address, extracting it from port if needed.
func normalizeIP(ip string) string {
	// Handle IP:port format
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	return ip
}

// normalizeDomain normalizes a domain name.
func normalizeDomain(domain string) string {
	domain = strings.ToLower(domain)
	domain = strings.TrimSuffix(domain, ".")
	return domain
}

// ipMatches checks if two IP addresses match, allowing for subnet matching.
func ipMatches(ip1, ip2 string) bool {
	// Exact match
	if ip1 == ip2 {
		return true
	}

	// Parse IPs
	parsed1 := net.ParseIP(ip1)
	parsed2 := net.ParseIP(ip2)

	if parsed1 == nil || parsed2 == nil {
		return false
	}

	// Check if they're the same IP
	return parsed1.Equal(parsed2)
}
