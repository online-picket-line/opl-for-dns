# OPL DNS Server - Copilot Instructions

## Architecture Overview

This is a Go DNS server that blocks domains involved in labor disputes by integrating with the [Online Picket Line](https://onlinepicketline.com) API. Four main components in `pkg/`:

- **`dns/`** - DNS server using `miekg/dns` library; intercepts queries, checks blocklist, returns block page IP or forwards to upstream
- **`api/`** - HTTP client fetching/caching blocklist from `/api/blocklist.json`; uses hash-based conditional fetching
- **`blockpage/`** - HTTP server serving block pages with bypass token management; uses embedded templates (`//go:embed`)
- **`session/`** - In-memory HMAC-signed bypass token management (IP+domain bound, 24h TTL)
- **`config/`** - JSON config with custom `Duration` type for time parsing (e.g., `"5m"`, `"24h"`)

Entry point: [cmd/opl-dns/main.go](cmd/opl-dns/main.go) wires all components together.

## Build & Test Commands

```bash
make build          # Build to build/opl-dns with version/buildTime ldflags
make test           # Run all tests: go test -v ./...
make test-race      # Race detection: go test -v -race ./...
make coverage       # Generate coverage.html report
make build-linux    # Cross-compile for linux/amd64
```

Run server: `sudo ./build/opl-dns -config config.json` (port 53 requires root)

## Code Patterns & Conventions

### Component Initialization
All server components follow the same pattern - constructor returns `(*Server, error)`, with `Start()` and `Stop()` methods:
```go
server, err := dns.NewServer(addr, blockIP, upstreams, timeout, apiClient, sessionMgr, logger)
server.Start()  // Blocking
server.Stop()   // Graceful shutdown
```

### Logging
Use `log/slog` with component-tagged loggers:
```go
logger.With("component", "dns")  // Tag logs by component
logger.Info("message", "key", value)  // Structured logging
```

### Configuration
- Custom `config.Duration` wraps `time.Duration` for JSON marshaling
- Config validation in `cfg.Validate()` after loading
- Generate example: `./opl-dns -generate-config`

### Testing Pattern
Tests use `*_test.go` in same package. For blocklist testing, use the test helper:
```go
apiClient.SetBlocklistForTesting(&api.Blocklist{...})
```

### Domain Normalization
Domains are normalized throughout: lowercase, trailing dot removed. Parent domain matching supported (e.g., `www.example.com` matches `example.com` blocklist entry).

## Key Files

- [pkg/api/client.go](pkg/api/client.go) - `CheckDomain()` for blocklist lookup, `FetchBlocklist()` for API calls
- [pkg/session/manager.go](pkg/session/manager.go) - `CreateBypassToken()`, `ValidateBypassToken()`, `HasBypass()`
- [pkg/blockpage/templates/](pkg/blockpage/templates/) - HTML templates embedded via `//go:embed`
- [config.example.json](config.example.json) - Reference for all config options

## External Dependencies

- `github.com/miekg/dns` - DNS protocol library (only external runtime dependency)
- Online Picket Line API at `https://onlinepicketline.com/api/blocklist.json`
