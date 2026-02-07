// Package dns provides a DNS server that blocks domains involved in labor disputes.
package dns

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	"github.com/online-picket-line/opl-for-dns/pkg/api"
	"github.com/online-picket-line/opl-for-dns/pkg/stats"
)

// Server is a DNS server that blocks domains involved in labor disputes.
type Server struct {
	listenAddr   string
	upstreamDNS  []string
	queryTimeout time.Duration

	apiClient      *api.Client
	statsCollector *stats.Collector
	logger         *slog.Logger

	server *dns.Server
	mu     sync.RWMutex
}

// NewServer creates a new DNS server.
func NewServer(listenAddr string, upstreamDNS []string, queryTimeout time.Duration, apiClient *api.Client, statsCollector *stats.Collector, logger *slog.Logger) (*Server, error) {
	if listenAddr == "" {
		return nil, fmt.Errorf("listen address is required")
	}

	return &Server{
		listenAddr:     listenAddr,
		upstreamDNS:    upstreamDNS,
		queryTimeout:   queryTimeout,
		apiClient:      apiClient,
		statsCollector: statsCollector,
		logger:         logger,
	}, nil
}

// Start starts the DNS server.
func (s *Server) Start() error {
	s.mu.Lock()
	s.server = &dns.Server{
		Addr:    s.listenAddr,
		Net:     "udp",
		Handler: s,
	}
	s.mu.Unlock()

	s.logger.Info("Starting DNS server", "addr", s.listenAddr)
	return s.server.ListenAndServe()
}

// StartTCP starts the DNS server on TCP.
func (s *Server) StartTCP() error {
	tcpServer := &dns.Server{
		Addr:    s.listenAddr,
		Net:     "tcp",
		Handler: s,
	}
	s.logger.Info("Starting DNS server (TCP)", "addr", s.listenAddr)
	return tcpServer.ListenAndServe()
}

// Stop stops the DNS server.
func (s *Server) Stop() error {
	s.mu.RLock()
	server := s.server
	s.mu.RUnlock()

	if server != nil {
		return server.Shutdown()
	}
	return nil
}

// ServeDNS handles DNS queries.
func (s *Server) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = false
	m.RecursionAvailable = true

	if len(r.Question) == 0 {
		w.WriteMsg(m)
		return
	}

	q := r.Question[0]
	domain := strings.ToLower(strings.TrimSuffix(q.Name, "."))

	// Get client IP
	clientIP := ""
	if addr, ok := w.RemoteAddr().(*net.UDPAddr); ok {
		clientIP = addr.IP.String()
	} else if addr, ok := w.RemoteAddr().(*net.TCPAddr); ok {
		clientIP = addr.IP.String()
	}

	// Check if domain is blocked
	if q.Qtype == dns.TypeA || q.Qtype == dns.TypeAAAA {
		if item, blocked := s.apiClient.CheckDomain(domain); blocked {
			s.logger.Info("Blocking domain",
				"domain", domain,
				"client", clientIP,
				"employer", item.Employer,
				"action_type", item.ActionDetails.ActionType,
			)

			// Return 0.0.0.0 for A queries, :: for AAAA queries
			// This causes connections to fail immediately
			if q.Qtype == dns.TypeA {
				rr := &dns.A{
					Hdr: dns.RR_Header{
						Name:   q.Name,
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    60,
					},
					A: net.IPv4zero,
				}
				m.Answer = append(m.Answer, rr)
			} else if q.Qtype == dns.TypeAAAA {
				rr := &dns.AAAA{
					Hdr: dns.RR_Header{
						Name:   q.Name,
						Rrtype: dns.TypeAAAA,
						Class:  dns.ClassINET,
						Ttl:    60,
					},
					AAAA: net.IPv6zero,
				}
				m.Answer = append(m.Answer, rr)
			}

			if s.statsCollector != nil {
				s.statsCollector.RecordBlock(domain)
			}

			w.WriteMsg(m)
			return
		}
	}

	// Forward to upstream DNS
	if s.statsCollector != nil {
		s.statsCollector.RecordQuery()
	}
	s.forwardQuery(w, r, m)
}

// forwardQuery forwards a DNS query to upstream DNS servers.
func (s *Server) forwardQuery(w dns.ResponseWriter, r *dns.Msg, m *dns.Msg) {
	c := new(dns.Client)
	c.Timeout = s.queryTimeout

	for _, upstream := range s.upstreamDNS {
		resp, _, err := c.Exchange(r, upstream)
		if err != nil {
			s.logger.Debug("Upstream DNS query failed",
				"upstream", upstream,
				"error", err,
			)
			continue
		}

		// Copy response
		resp.Id = r.Id
		w.WriteMsg(resp)
		return
	}

	// All upstreams failed
	s.logger.Error("All upstream DNS servers failed")
	m.Rcode = dns.RcodeServerFailure
	w.WriteMsg(m)
}

// BlockedDomainInfo holds information about why a domain is blocked.
type BlockedDomainInfo struct {
	Domain       string
	Employer     string
	ActionType   string
	Description  string
	Demands      string
	MoreInfoURL  string
	Organization string
	StartDate    string
	Location     string
}

// GetBlockedDomainInfo returns information about a blocked domain.
func (s *Server) GetBlockedDomainInfo(domain string) (*BlockedDomainInfo, bool) {
	item, blocked := s.apiClient.CheckDomain(domain)
	if !blocked {
		return nil, false
	}

	return &BlockedDomainInfo{
		Domain:       domain,
		Employer:     item.Employer,
		ActionType:   item.ActionDetails.ActionType,
		Description:  item.ActionDetails.Description,
		Demands:      item.ActionDetails.Demands,
		MoreInfoURL:  item.MoreInfoURL,
		Organization: item.ActionDetails.Organization,
		StartDate:    item.ActionDetails.StartDate,
		Location:     item.Location,
	}, true
}

// RefreshBlocklist refreshes the blocklist from the API.
func (s *Server) RefreshBlocklist(ctx context.Context) error {
	_, err := s.apiClient.FetchBlocklist(ctx)
	return err
}
