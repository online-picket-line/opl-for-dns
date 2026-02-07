# OPL DNS Server - Online Picket Line

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-ISC-blue.svg)](LICENSE)

A DNS server that integrates with the [Online Picket Line](https://onlinepicketline.com) to help users stay informed about labor actions and boycotts. When a user attempts to access a website involved in a labor dispute, the DNS server blocks the domain by returning `0.0.0.0`, preventing access to the site.

This enables digital solidarity with workers by making labor disputes visible and actionable at the DNS level across all devices on a network.

## Features

- 🚧 **Real-time Labor Action Detection**: Integrates with the Online Picket Line API for up-to-date blocklist
- 🛡️ **Network-Wide Blocking**: Blocks domains across all devices on a network at the DNS level
- ⚡ **High Performance**: Efficient caching with configurable refresh intervals, upstream DNS forwarding
- 🔄 **Automatic Blocklist Updates**: Syncs with Online Picket Line blocklist every 15 minutes
- 🐳 **Easy Deployment**: Single binary, systemd service, or Docker container
- 📊 **Simple and Transparent**: No complex UIs or intermediate pages—just DNS-level blocking

## Quick Start

### Prerequisites

- Go 1.21 or later
- Linux (Ubuntu 24.04 recommended) or macOS

### Installation

#### RHEL/CentOS/Fedora
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
    "cache_ttl": "5m",
    "query_timeout": "5s"
  },
  "api": {
    "base_url": "https://onlinepicketline.com/api",
    "api_key": "",
    "refresh_interval": "15m",
    "timeout": "10s"
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

**Important:** Set a secure random string for `session.secret`. You can generate one with:
```bash
openssl rand -hex 32
```

## How It Works

1. **DNS Query Reception**: When a device on the network queries a domain, the DNS server receives the request.

2. **Blocklist Check**: The server checks if the domain is on the current blocklist fetched from the Online Picket Line API.

3. **Response Decision**:
   - If the domain is **blocked** → Return `0.0.0.0` (no host available)
   - If the domain is **not blocked** → Forward to upstream DNS servers (e.g., 8.8.8.8)

4. **User Experience**: When a user tries to access a blocked domain, their system gets `0.0.0.0` and the connection fails. This provides a clear signal that the domain is part of a labor action.

5. **Blocklist Updates**: The DNS server automatically refreshes the Online Picket Line blocklist every 15 minutes, ensuring users see the latest information about active labor actions.

## Architecture

```
┌─────────────────────────┐         ┌──────────────────────┐
│   User Devices          │         │  OPL DNS Server      │
│  (Phone, Desktop, etc)  │────────▶│                      │
└─────────────────────────┘         │  - Check blocklist   │
                                    │  - Cache results     │
                                    └──────────┬───────────┘
                                               │
                                       ┌───────┴────────┐
                                       │                │
                                    Blocked?        Not Blocked?
                                       │                │
                                       ▼                ▼
                                  Return             Forward to
                                  0.0.0.0         Upstream DNS
                                               (8.8.8.8, etc)
                                               
                   Every 15 minutes:
                   
                   OPL DNS Server ───────────► Online Picket Line API
                    (refresh)                    (fetch blocklist)
```

## Testing the Server

Once your DNS server is running, test it with:

```bash
# Test a blocked domain (returns 0.0.0.0)
nslookup amazon.com YOUR_SERVER_IP

# Test a non-blocked domain (returns upstream DNS results)
nslookup github.com YOUR_SERVER_IP

# Check server status
journalctl -u opl-dns -f
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

## Use Cases

The OPL DNS server is ideal for:

- **Home Networks**: Protect all devices (computers, phones, tablets, IoT) on a home network
- **Office Networks**: Implement labor action awareness across an entire workplace
- **Community Networks**: Support labor movements with network-wide solidarity
- **Managed Networks**: Easy to deploy via router or firewall configuration
- **Device Agnostic**: Works on any device that can use custom DNS servers

## Limitations & Considerations

- **DNS Caching**: Some devices or networks may cache DNS responses. Clearing DNS cache may be needed for immediate effect
- **VPN/Proxy Bypass**: Users can bypass DNS blocks using VPNs or proxy servers (this is intentional—we inform, not force)
- **Configuration Complexity**: Requires server setup and network configuration knowledge
- **No User Prompts**: Since there's no block page UI, users won't see why access failed (consider adding informational posters or documentation)

## Development

### Running Tests

```bash
go test ./... -v
```

### Running Locally

### API Testing

Test the API integration:
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
| Device Support | Browser only | All devices (comprehensive) |
| Network-wide | No | Yes (via DNS configuration) |
| Installation | Per browser | Once on network router/firewall |
| User Bypass | Click "Continue" button | Use VPN or proxy (manual) |
| Block Mechanism | Overlay/banner on page | DNS returns 0.0.0.0 |
| User Feedback | Show labor action details | Simple DNS failure (suggest docs) |

**Choose Browser Plugin if:** Users want detailed information and easy bypass options

**Choose DNS Server if:** You want network-wide protection for all devices with minimal overhead

## Security Considerations

- **API Key Security**: Protect your API key in `config.json` (don't commit it to public repos)
- **Token Secret**: The session secret should be a strong, randomly generated string (use `openssl rand -hex 32`)
- **Network Access**: Restrict DNS server access to authorized networks using firewall rules
- **DNS over HTTPS (DoH)**: Clients using DoH will bypass your DNS server; this is expected behavior
- **Logging**: Logs may contain client IPs and queried domains; ensure compliance with privacy regulations
- **Upstream DNS**: Choose reputable, privacy-respecting DNS providers as your upstream servers

## API Integration

The server integrates with the [Online Picket Line API](https://github.com/online-picket-line/online-picketline/blob/main/doc/API_DOCUMENTATION.md). It uses the `/api/blocklist.json` endpoint to fetch the list of domains involved in labor actions.

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
