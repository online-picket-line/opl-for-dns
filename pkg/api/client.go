// Package api provides a client for the Online Picketline API.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Client is a client for the Online Picketline API.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client

	// Cached blocklist data
	mu          sync.RWMutex
	blocklist   *Blocklist
	lastFetch   time.Time
	contentHash string
}

// Blocklist represents the blocklist data from the API.
type Blocklist struct {
	Version     string          `json:"version"`
	GeneratedAt string          `json:"generatedAt"`
	TotalURLs   int             `json:"totalUrls"`
	Employers   []Employer      `json:"employers"`
	BlockList   []BlockListItem `json:"blocklist"`

	// Pre-computed domain map for fast lookups
	domainMap map[string]*BlockListItem
}

// Employer represents an employer in the blocklist.
type Employer struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	URLCount int    `json:"urlCount"`
}

// BlockListItem represents a blocked URL/domain.
type BlockListItem struct {
	URL           string        `json:"url"`
	Employer      string        `json:"employer"`
	EmployerID    string        `json:"employerId"`
	Label         string        `json:"label"`
	Category      string        `json:"category"`
	Reason        string        `json:"reason"`
	StartDate     string        `json:"startDate"`
	MoreInfoURL   string        `json:"moreInfoUrl"`
	Location      string        `json:"location"`
	ActionDetails ActionDetails `json:"actionDetails"`
}

// ActionDetails provides detailed information about the labor action.
type ActionDetails struct {
	ID           string `json:"id"`
	Organization string `json:"organization"`
	ActionType   string `json:"actionType"`
	Status       string `json:"status"`
	StartDate    string `json:"startDate"`
	Description  string `json:"description"`
	Demands      string `json:"demands"`
	ContactInfo  string `json:"contactInfo"`
	UnionLogoURL string `json:"unionLogoUrl"`
	LearnMoreURL string `json:"learnMoreUrl"`
}

// NewClient creates a new API client.
func NewClient(baseURL, apiKey string, timeout time.Duration) *Client {
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// FetchBlocklist fetches the blocklist from the API.
func (c *Client) FetchBlocklist(ctx context.Context) (*Blocklist, error) {
	reqURL := fmt.Sprintf("%s/blocklist.json", c.baseURL)

	// Add hash for conditional fetch if we have cached data
	c.mu.RLock()
	hash := c.contentHash
	c.mu.RUnlock()

	if hash != "" {
		reqURL = fmt.Sprintf("%s?hash=%s", reqURL, url.QueryEscape(hash))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "OPL-DNS-Server/1.0.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	// Handle 304 Not Modified
	if resp.StatusCode == http.StatusNotModified {
		c.mu.RLock()
		defer c.mu.RUnlock()
		return c.blocklist, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var blocklist Blocklist
	if err := json.Unmarshal(body, &blocklist); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	// Build domain map for fast lookups
	blocklist.domainMap = make(map[string]*BlockListItem)
	for i := range blocklist.BlockList {
		item := &blocklist.BlockList[i]
		domain := extractDomain(item.URL)
		if domain != "" {
			blocklist.domainMap[strings.ToLower(domain)] = item
		}
	}

	// Update cache
	c.mu.Lock()
	c.blocklist = &blocklist
	c.lastFetch = time.Now()
	if newHash := resp.Header.Get("X-Content-Hash"); newHash != "" {
		c.contentHash = newHash
	}
	c.mu.Unlock()

	return &blocklist, nil
}

// GetCachedBlocklist returns the cached blocklist without making an API request.
func (c *Client) GetCachedBlocklist() *Blocklist {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.blocklist
}

// CheckDomain checks if a domain is in the blocklist.
func (c *Client) CheckDomain(domain string) (*BlockListItem, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.blocklist == nil || c.blocklist.domainMap == nil {
		return nil, false
	}

	// Normalize domain
	domain = strings.ToLower(strings.TrimSuffix(domain, "."))

	// Direct lookup
	if item, ok := c.blocklist.domainMap[domain]; ok {
		return item, true
	}

	// Check parent domains (e.g., if "www.example.com" is not found, check "example.com")
	parts := strings.Split(domain, ".")
	for i := 1; i < len(parts)-1; i++ {
		parentDomain := strings.Join(parts[i:], ".")
		if item, ok := c.blocklist.domainMap[parentDomain]; ok {
			return item, true
		}
	}

	return nil, false
}

// LastFetchTime returns the time of the last successful blocklist fetch.
func (c *Client) LastFetchTime() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastFetch
}

// SetBlocklistForTesting sets the blocklist directly (for testing purposes).
func (c *Client) SetBlocklistForTesting(blocklist *Blocklist) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Build domain map
	blocklist.domainMap = make(map[string]*BlockListItem)
	for i := range blocklist.BlockList {
		item := &blocklist.BlockList[i]
		domain := extractDomain(item.URL)
		if domain != "" {
			blocklist.domainMap[strings.ToLower(domain)] = item
		}
	}

	c.blocklist = blocklist
	c.lastFetch = time.Now()
}

// extractDomain extracts the domain from a URL.
func extractDomain(rawURL string) string {
	// Handle URLs that might not have a scheme
	if !strings.Contains(rawURL, "://") {
		rawURL = "https://" + rawURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		// Try treating it as a plain domain
		return strings.Split(rawURL, "/")[0]
	}

	return parsed.Hostname()
}
