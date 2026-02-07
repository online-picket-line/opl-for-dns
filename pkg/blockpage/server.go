// Package blockpage provides a web server for the block page.
package blockpage

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/online-picket-line/opl-for-dns/pkg/api"
	"github.com/online-picket-line/opl-for-dns/pkg/session"
	"github.com/online-picket-line/opl-for-dns/pkg/stats"
)

//go:embed templates/*
var templatesFS embed.FS

//go:embed static/*
var staticFS embed.FS

// Server is a web server that serves block pages.
type Server struct {
	listenAddr     string
	externalURL    string
	displayMode    string
	apiClient      *api.Client
	sessionManager *session.Manager
	statsCollector *stats.Collector
	logger         *slog.Logger

	templates *template.Template
	server    *http.Server
}

// NewServer creates a new block page server.
func NewServer(listenAddr, externalURL, displayMode string, apiClient *api.Client, sessionManager *session.Manager, statsCollector *stats.Collector, logger *slog.Logger) (*Server, error) {
	// Parse templates
	templates, err := template.ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parsing templates: %w", err)
	}

	return &Server{
		listenAddr:     listenAddr,
		externalURL:    strings.TrimSuffix(externalURL, "/"),
		displayMode:    displayMode,
		apiClient:      apiClient,
		sessionManager: sessionManager,
		statsCollector: statsCollector,
		logger:         logger,
		templates:      templates,
	}, nil
}

// Start starts the block page server.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Block page endpoints
	mux.HandleFunc("/", s.handleBlockPage)
	mux.HandleFunc("/api/bypass", s.handleBypass)
	mux.HandleFunc("/api/check", s.handleCheck)
	mux.HandleFunc("/health", s.handleHealth)

	// Serve static files
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServerFS(staticFS)))

	s.server = &http.Server{
		Addr:         s.listenAddr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	s.logger.Info("Starting block page server", "addr", s.listenAddr)
	return s.server.ListenAndServe()
}

// Stop stops the block page server.
func (s *Server) Stop() error {
	if s.server != nil {
		return s.server.Close()
	}
	return nil
}

// BlockPageData holds data for rendering the block page.
type BlockPageData struct {
	Domain       string
	Employer     string
	ActionType   string
	Description  string
	Demands      string
	MoreInfoURL  string
	Organization string
	StartDate    string
	Location     string
	DisplayMode  string
	BypassURL    string
	OriginalURL  string
}

// handleBlockPage serves the block page.
func (s *Server) handleBlockPage(w http.ResponseWriter, r *http.Request) {
	// Get the domain from the Host header or query parameter
	domain := r.URL.Query().Get("domain")
	if domain == "" {
		domain = r.Host
	}

	// Remove port from domain if present
	if host, _, err := net.SplitHostPort(domain); err == nil {
		domain = host
	}

	// Normalize domain
	domain = strings.ToLower(strings.TrimSuffix(domain, "."))

	// Get client IP
	clientIP := getClientIP(r)

	s.logger.Debug("Block page request",
		"domain", domain,
		"client", clientIP,
		"host", r.Host,
	)

	// Check if this domain is blocked
	item, blocked := s.apiClient.CheckDomain(domain)
	if !blocked {
		// If not blocked, show a generic info page
		http.Error(w, "This domain is not currently blocked", http.StatusNotFound)
		return
	}

	// Check if client already has a bypass
	if s.sessionManager.HasBypass(clientIP, domain) {
		// Redirect to original site
		originalURL := fmt.Sprintf("https://%s%s", domain, r.URL.Path)
		http.Redirect(w, r, originalURL, http.StatusTemporaryRedirect)
		return
	}

	// Determine display mode
	displayMode := r.URL.Query().Get("mode")
	if displayMode == "" {
		displayMode = s.displayMode
	}

	// Build bypass URL
	bypassURL := fmt.Sprintf("%s/api/bypass?domain=%s", s.externalURL, domain)

	// Build original URL
	originalURL := fmt.Sprintf("https://%s", domain)

	// Prepare template data
	data := BlockPageData{
		Domain:       domain,
		Employer:     item.Employer,
		ActionType:   item.ActionDetails.ActionType,
		Description:  item.ActionDetails.Description,
		Demands:      item.ActionDetails.Demands,
		MoreInfoURL:  item.MoreInfoURL,
		Organization: item.ActionDetails.Organization,
		StartDate:    item.ActionDetails.StartDate,
		Location:     item.Location,
		DisplayMode:  displayMode,
		BypassURL:    bypassURL,
		OriginalURL:  originalURL,
	}

	// Render appropriate template
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

	templateName := "block.html"
	if displayMode == "overlay" {
		templateName = "overlay.html"
	}

	if err := s.templates.ExecuteTemplate(w, templateName, data); err != nil {
		s.logger.Error("Error rendering template", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// BypassRequest is the request body for creating a bypass.
type BypassRequest struct {
	Domain string `json:"domain"`
}

// BypassResponse is the response for a bypass request.
type BypassResponse struct {
	Success     bool   `json:"success"`
	Token       string `json:"token,omitempty"`
	RedirectURL string `json:"redirectUrl,omitempty"`
	Error       string `json:"error,omitempty"`
	ExpiresIn   int    `json:"expiresIn,omitempty"` // seconds
}

// handleBypass creates a bypass token for a client.
func (s *Server) handleBypass(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Get client IP
	clientIP := getClientIP(r)

	// Get domain from query or body
	var domain string
	if r.Method == http.MethodPost {
		var req BypassRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			json.NewEncoder(w).Encode(BypassResponse{
				Success: false,
				Error:   "Invalid request body",
			})
			return
		}
		domain = req.Domain
	} else {
		domain = r.URL.Query().Get("domain")
	}

	if domain == "" {
		json.NewEncoder(w).Encode(BypassResponse{
			Success: false,
			Error:   "Domain is required",
		})
		return
	}

	// Normalize domain
	domain = strings.ToLower(strings.TrimSuffix(domain, "."))

	// Check if domain is actually blocked
	_, blocked := s.apiClient.CheckDomain(domain)
	if !blocked {
		json.NewEncoder(w).Encode(BypassResponse{
			Success: false,
			Error:   "Domain is not blocked",
		})
		return
	}

	// Create bypass token
	token, err := s.sessionManager.CreateBypassToken(clientIP, domain)
	if err != nil {
		s.logger.Error("Error creating bypass token", "error", err)
		json.NewEncoder(w).Encode(BypassResponse{
			Success: false,
			Error:   "Failed to create bypass token",
		})
		return
	}

	s.logger.Info("Bypass token created",
		"domain", domain,
		"client", clientIP,
	)

	if s.statsCollector != nil {
		s.statsCollector.RecordBypass()
	}

	redirectURL := fmt.Sprintf("https://%s", domain)

	// For GET requests, redirect directly
	if r.Method == http.MethodGet {
		http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
		return
	}

	// For POST requests, return JSON
	json.NewEncoder(w).Encode(BypassResponse{
		Success:     true,
		Token:       token,
		RedirectURL: redirectURL,
		ExpiresIn:   86400, // 24 hours in seconds
	})
}

// CheckResponse is the response for a check request.
type CheckResponse struct {
	Blocked      bool   `json:"blocked"`
	HasBypass    bool   `json:"hasBypass"`
	Domain       string `json:"domain"`
	Employer     string `json:"employer,omitempty"`
	ActionType   string `json:"actionType,omitempty"`
	Description  string `json:"description,omitempty"`
	MoreInfoURL  string `json:"moreInfoUrl,omitempty"`
	Organization string `json:"organization,omitempty"`
}

// handleCheck checks if a domain is blocked.
func (s *Server) handleCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	domain := r.URL.Query().Get("domain")
	if domain == "" {
		http.Error(w, `{"error": "domain parameter is required"}`, http.StatusBadRequest)
		return
	}

	// Normalize domain
	domain = strings.ToLower(strings.TrimSuffix(domain, "."))

	// Get client IP
	clientIP := getClientIP(r)

	// Check if blocked
	item, blocked := s.apiClient.CheckDomain(domain)
	if !blocked {
		json.NewEncoder(w).Encode(CheckResponse{
			Blocked: false,
			Domain:  domain,
		})
		return
	}

	// Check if has bypass
	hasBypass := s.sessionManager.HasBypass(clientIP, domain)

	json.NewEncoder(w).Encode(CheckResponse{
		Blocked:      true,
		HasBypass:    hasBypass,
		Domain:       domain,
		Employer:     item.Employer,
		ActionType:   item.ActionDetails.ActionType,
		Description:  item.ActionDetails.Description,
		MoreInfoURL:  item.MoreInfoURL,
		Organization: item.ActionDetails.Organization,
	})
}

// HealthResponse is the response for health check.
type HealthResponse struct {
	Status             string `json:"status"`
	BlocklistLoaded    bool   `json:"blocklistLoaded"`
	ActiveSessions     int    `json:"activeSessions"`
	LastBlocklistFetch string `json:"lastBlocklistFetch,omitempty"`
}

// handleHealth handles health check requests.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	blocklist := s.apiClient.GetCachedBlocklist()
	lastFetch := s.apiClient.LastFetchTime()

	resp := HealthResponse{
		Status:          "ok",
		BlocklistLoaded: blocklist != nil,
		ActiveSessions:  s.sessionManager.GetActiveSessionCount(),
	}

	if !lastFetch.IsZero() {
		resp.LastBlocklistFetch = lastFetch.Format(time.RFC3339)
	}

	json.NewEncoder(w).Encode(resp)
}

// getClientIP extracts the client IP from the request.
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
