# OPL DNS Server - Online Picket Line

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-ISC-blue.svg)](LICENSE)

A DNS server that integrates with the [Online Picket Line](https://onlinepicketline.com) to help users stay informed about labor actions and boycotts. When a user attempts to access a website involved in a labor dispute, the DNS server redirects them to an informational block page where they can:

- **Learn more** about the labor action
- **Go back** to the previous page
- **Continue anyway** with a bypass token (valid for 24 hours)

This enables digital solidarity with workers by making labor disputes visible at the DNS level.

## Features

- 🚧 **Real-time Labor Action Detection**: Integrates with the Online Picket Line API
- 🔄 **Session-Based Bypass**: Users can choose to continue, receiving a 24-hour bypass token
- 📱 **Two Display Modes**: Block page or overlay style (matching the browser plugin)
- 🔒 **Secure Token System**: HMAC-signed bypass tokens prevent tampering
- ⚡ **High Performance**: Efficient caching, upstream DNS forwarding
- 🐳 **Easy Deployment**: Single binary, systemd service, or Docker

## Quick Start

### Prerequisites

- Go 1.21 or later
- Linux (Ubuntu 24.04 recommended) or macOS

### Installation

```bash
# Clone the repository
git clone https://github.com/online-picket-line/opl-for-dns.git
cd opl-for-dns

# Build the server
go build -o opl-dns ./cmd/opl-dns

# Generate example configuration
./opl-dns -generate-config
# Edit config.example.json with your settings, then rename to config.json

# Run the server (requires root for port 53)
sudo ./opl-dns -config config.json
```

### Configuration

Create a `config.json` file (see `config.example.json`):

```json
{
  "dns": {
    "listen_addr": "0.0.0.0:53",
    "upstream_dns": ["8.8.8.8:53", "8.8.4.4:53"],
    "block_page_ip": "YOUR_SERVER_IP",
    "cache_ttl": "5m",
    "query_timeout": "5s"
  },
  "api": {
    "base_url": "https://onlinepicketline.com/api",
    "api_key": "",
    "refresh_interval": "15m",
    "timeout": "10s"
  },
  "web": {
    "listen_addr": "0.0.0.0:8080",
    "external_url": "http://YOUR_SERVER_IP:8080",
    "display_mode": "block"
  },
  "session": {
    "token_ttl": "24h",
    "secret": "CHANGE_THIS_TO_A_SECURE_RANDOM_STRING",
    "cleanup_interval": "1h"
  },
  "logging": {
    "level": "info",
    "format": "text"
  }
}
```

**Important:** Replace `YOUR_SERVER_IP` with your server's public IP address, and set a secure random string for `session.secret`.

## How It Works

1. **DNS Query Interception**: When a user's device queries a domain, the DNS server checks if it's on the blocklist.

2. **Blocklist Check**: The server maintains a cached copy of the Online Picket Line blocklist, refreshed every 15 minutes.

3. **Block or Forward**:
   - If the domain is **blocked** and the user doesn't have a bypass token → Return the block page IP
   - If the domain is **allowed** or user has a bypass → Forward to upstream DNS

4. **Block Page**: Users see information about the labor action with three options:
   - **Learn More**: Opens the action's info URL
   - **Go Back**: Returns to previous page
   - **Continue Anyway**: Creates a bypass token and redirects to the original site

5. **Bypass Token**: Valid for 24 hours, stored in the server's session manager. Subsequent DNS queries from the same IP for the same domain are forwarded normally.

## Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   User Device   │────▶│   OPL DNS       │────▶│  Upstream DNS   │
│                 │     │   Server        │     │  (8.8.8.8)      │
└─────────────────┘     └────────┬────────┘     └─────────────────┘
                                 │
                                 ▼
                        ┌─────────────────┐
                        │  Block Page     │
                        │  Web Server     │
                        └────────┬────────┘
                                 │
                                 ▼
                        ┌─────────────────┐
                        │  Online Picket  │
                        │  Line API       │
                        └─────────────────┘
```

## API Endpoints

The block page server exposes the following endpoints:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/` | GET | Serves the block page for blocked domains |
| `/api/bypass` | GET/POST | Creates a bypass token and redirects |
| `/api/check` | GET | Checks if a domain is blocked |
| `/health` | GET | Health check endpoint |

### Check Domain

```bash
curl "http://localhost:8080/api/check?domain=example.com"
```

Response:
```json
{
  "blocked": true,
  "hasBypass": false,
  "domain": "example.com",
  "employer": "Example Corp",
  "actionType": "strike",
  "description": "Workers on strike for better wages"
}
```

### Create Bypass

```bash
curl -X POST "http://localhost:8080/api/bypass" \
  -H "Content-Type: application/json" \
  -d '{"domain": "example.com"}'
```

Response:
```json
{
  "success": true,
  "token": "BASE64_ENCODED_TOKEN",
  "redirectUrl": "https://example.com",
  "expiresIn": 86400
}
```

## Deployment

### Ubuntu 24.04 Deployment

```bash
# Install dependencies
sudo apt update
sudo apt install -y golang-go

# Build the binary
cd opl-for-dns
go build -o opl-dns ./cmd/opl-dns

# Install
sudo mkdir -p /etc/opl-dns /var/lib/opl-dns
sudo cp opl-dns /usr/local/bin/
sudo cp config.example.json /etc/opl-dns/config.json
# Edit /etc/opl-dns/config.json with your settings

# Create service user
sudo useradd -r -s /bin/false opl-dns

# Install systemd service
sudo cp deploy/opl-dns.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable opl-dns
sudo systemctl start opl-dns

# Check status
sudo systemctl status opl-dns
sudo journalctl -u opl-dns -f
```

### Using as DNS Server

Configure your device or network to use your OPL DNS server:

**On Linux:**
```bash
# Edit /etc/resolv.conf
nameserver YOUR_SERVER_IP
```

**On macOS:**
```bash
# System Preferences → Network → Advanced → DNS
# Add YOUR_SERVER_IP as a DNS server
```

**On Windows:**
```
# Network Settings → Change adapter options → Properties
# Internet Protocol Version 4 → Properties
# Use the following DNS server addresses: YOUR_SERVER_IP
```

**On Router/Firewall:**
Configure your router to use YOUR_SERVER_IP as the DNS server to protect all devices on your network.

## Display Modes

### Block Mode (Default)

A full-page block screen with detailed information about the labor action.

### Overlay Mode

A semi-transparent overlay that appears over the page (similar to the browser plugin).

To use overlay mode, add `?mode=overlay` to the block page URL or set `"display_mode": "overlay"` in the configuration.

## Development

### Running Tests

```bash
go test ./... -v
```

### Running Locally

```bash
# Run with debug logging
go run ./cmd/opl-dns -config config.json
```

### Project Structure

```
opl-for-dns/
├── cmd/opl-dns/           # Main application entry point
├── pkg/
│   ├── api/               # Online Picket Line API client
│   ├── blockpage/         # Block page web server
│   ├── config/            # Configuration management
│   ├── dns/               # DNS server implementation
│   └── session/           # Bypass session management
├── deploy/                # Deployment files
├── docs/                  # Documentation
└── config.example.json    # Example configuration
```

## Comparison with Browser Plugin

| Feature | Browser Plugin | DNS Server |
|---------|---------------|------------|
| Device Support | Browser only | All devices |
| Network-wide | No | Yes (via router) |
| Installation | Per browser | Once on network |
| Bypass Tokens | Session storage | Server-side |
| Display Modes | Banner, Block | Block, Overlay |

The DNS server provides network-wide protection, making it ideal for:
- Protecting all devices on a home/office network
- Mobile devices without browser extension support
- IoT devices and smart TVs

## Security Considerations

- **HTTPS**: The block page server should be placed behind a reverse proxy (nginx, Caddy) with HTTPS in production
- **Token Security**: The session secret should be a strong, randomly generated string
- **Rate Limiting**: Consider adding rate limiting for the bypass endpoint
- **Logging**: Logs include client IPs; ensure compliance with privacy regulations

## API Integration

The server integrates with the [Online Picket Line API](https://github.com/online-picket-line/online-picketline/blob/main/doc/API_DOCUMENTATION.md). It uses the `/api/blocklist.json` endpoint to fetch the list of domains involved in labor actions.

## Legacy BIND 9 Plugin

The legacy BIND 9 plugin code is available in the `src/` directory for reference. This was the original approach but has been superseded by the Go-based DNS server for the following reasons:

- Easier session management for bypass tokens
- Simpler deployment (single binary vs. BIND 9 plugin)
- Better integration with the web service
- More flexible architecture

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Make your changes with tests
4. Submit a pull request

## License

ISC License - See [LICENSE](LICENSE) for details.

## Credits

- [Online Picket Line](https://onlinepicketline.com) - The API and labor action database
- [opl-browser-plugin](https://github.com/online-picket-line/opl-browser-plugin) - Design inspiration
- [miekg/dns](https://github.com/miekg/dns) - DNS library for Go

---

✊ Digital solidarity with workers everywhere ✊
