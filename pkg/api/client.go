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
	Version     string
	GeneratedAt string
	TotalURLs   int
	Employers   []Employer
	BlockList   []BlockListItem

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
	URL           string
	Domain        string
	Employer      string
	EmployerID    string
	Label         string
	Category      string
	Reason        string
	StartDate     string
	MoreInfoURL   string
	Location      string
	ActionDetails ActionDetails
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
	Location     string `json:"location"`
	ContactInfo  string `json:"contactInfo"`
	UnionLogoURL string `json:"unionLogoUrl"`
	LearnMoreURL string `json:"learnMoreUrl"`
}

// OPLBlocklistEntry represents an entry in the OPL blocklist API response.
// The API returns a map keyed by employer name.
type OPLBlocklistEntry struct {
	MoreInfoURL        string        `json:"moreInfoUrl"`
	MatchingURLRegexes []string      `json:"matchingUrlRegexes"`
	StartTime          string        `json:"startTime"`
	ActionDetails      ActionDetails `json:"actionDetails"`
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

	// Parse the OPL blocklist format (map keyed by employer name)
	var rawBlocklist map[string]json.RawMessage
	if err := json.Unmarshal(body, &rawBlocklist); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	blocklist := &Blocklist{
		GeneratedAt: time.Now().Format(time.RFC3339),
		domainMap:   make(map[string]*BlockListItem),
	}

	employerSet := make(map[string]bool)

	for employerName, rawEntry := range rawBlocklist {
		// Skip internal fields like _optimizedPatterns
		if strings.HasPrefix(employerName, "_") {
			continue
		}

		var entry OPLBlocklistEntry
		if err := json.Unmarshal(rawEntry, &entry); err != nil {
			// Skip entries that don't match expected format
			continue
		}

		// Add employer to set
		if !employerSet[employerName] {
			employerSet[employerName] = true
			blocklist.Employers = append(blocklist.Employers, Employer{
				ID:       entry.ActionDetails.ID,
				Name:     employerName,
				URLCount: len(entry.MatchingURLRegexes),
			})
		}

		// Add each URL/domain to the blocklist
		for _, urlPattern := range entry.MatchingURLRegexes {
			domain := extractDomain(urlPattern)
			if domain == "" {
				continue
			}

			item := BlockListItem{
				URL:         urlPattern,
				Domain:      domain,
				Employer:    employerName,
				EmployerID:  entry.ActionDetails.ID,
				Reason:      entry.ActionDetails.ActionType,
				StartDate:   entry.ActionDetails.StartDate,
				MoreInfoURL: entry.MoreInfoURL,
				Location:    entry.ActionDetails.Location,
				ActionDetails: ActionDetails{
					ID:           entry.ActionDetails.ID,
					Organization: entry.ActionDetails.Organization,
					ActionType:   entry.ActionDetails.ActionType,
					Status:       entry.ActionDetails.Status,
					StartDate:    entry.ActionDetails.StartDate,
					Description:  entry.ActionDetails.Description,
					Demands:      entry.ActionDetails.Demands,
					Location:     entry.ActionDetails.Location,
					UnionLogoURL: entry.ActionDetails.UnionLogoURL,
					LearnMoreURL: entry.ActionDetails.LearnMoreURL,
				},
			}

			blocklist.BlockList = append(blocklist.BlockList, item)
			blocklist.TotalURLs++

			// Add to domain map for fast lookup
			normalizedDomain := strings.ToLower(domain)
			blocklist.domainMap[normalizedDomain] = &blocklist.BlockList[len(blocklist.BlockList)-1]
		}
	}

	// Update cache
	c.mu.Lock()
	c.blocklist = blocklist
	c.lastFetch = time.Now()
	if newHash := resp.Header.Get("X-Content-Hash"); newHash != "" {
		c.contentHash = newHash
	}
	c.mu.Unlock()

	return blocklist, nil
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
