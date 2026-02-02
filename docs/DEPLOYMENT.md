# Deployment Guide

This guide provides detailed instructions for deploying the OPL DNS Server on Ubuntu 24.04.

## Prerequisites

- Ubuntu 24.04 LTS (or compatible Linux distribution)
- Root or sudo access
- Public IP address (for external access)
- Go 1.21 or later (for building from source)

## Quick Installation

### 1. Build from Source

```bash
# Install Go (if not already installed)
sudo apt update
sudo apt install -y golang-go git

# Clone and build
git clone https://github.com/online-picket-line/opl-for-dns.git
cd opl-for-dns
go build -o opl-dns ./cmd/opl-dns
```

### 2. Configure

```bash
# Create directories
sudo mkdir -p /etc/opl-dns /var/lib/opl-dns

# Copy and edit configuration
sudo cp config.example.json /etc/opl-dns/config.json
sudo nano /etc/opl-dns/config.json
```

**Required Configuration Changes:**

1. Set `dns.block_page_ip` to your server's public IP
2. Set `web.external_url` to your server's public URL
3. Set `session.secret` to a secure random string (generate with: `openssl rand -hex 32`)
4. Optionally set `api.api_key` if you have an Online Picket Line API key

### 3. Install Binary

```bash
sudo cp opl-dns /usr/local/bin/
sudo chmod +x /usr/local/bin/opl-dns
```

### 4. Create Service User

```bash
sudo useradd -r -s /bin/false -d /var/lib/opl-dns opl-dns
sudo chown -R opl-dns:opl-dns /var/lib/opl-dns
```

### 5. Install Systemd Service

```bash
sudo cp deploy/opl-dns.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable opl-dns
sudo systemctl start opl-dns
```

### 6. Verify Installation

```bash
# Check service status
sudo systemctl status opl-dns

# Check logs
sudo journalctl -u opl-dns -f

# Test DNS resolution
dig @localhost example.com

# Test block page
curl http://localhost:8080/health
```

## Firewall Configuration

Open the required ports:

```bash
# DNS (UDP and TCP)
sudo ufw allow 53/udp
sudo ufw allow 53/tcp

# Block page web server
sudo ufw allow 8080/tcp

# Enable firewall
sudo ufw enable
```

## HTTPS Setup (Recommended for Production)

For production deployments, use a reverse proxy with HTTPS:

### Using Nginx

```bash
sudo apt install -y nginx certbot python3-certbot-nginx
```

Create `/etc/nginx/sites-available/opl-dns`:

```nginx
server {
    listen 80;
    server_name dns.yourdomain.com;
    
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

Enable and get SSL certificate:

```bash
sudo ln -s /etc/nginx/sites-available/opl-dns /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
sudo certbot --nginx -d dns.yourdomain.com
```

Then update your config.json:
```json
{
  "web": {
    "listen_addr": "127.0.0.1:8080",
    "external_url": "https://dns.yourdomain.com"
  }
}
```

## High Availability Setup

For production environments, consider:

### Multiple DNS Servers

Deploy the OPL DNS server on multiple machines for redundancy:

1. Deploy to multiple VPS instances
2. Use DNS load balancing or anycast
3. Share configuration across instances

### Health Monitoring

Set up monitoring using the `/health` endpoint:

```bash
#!/bin/bash
# /etc/cron.d/opl-dns-health

*/5 * * * * root curl -sf http://localhost:8080/health || systemctl restart opl-dns
```

### Log Rotation

Configure logrotate for the journal:

```bash
sudo journalctl --vacuum-time=30d
```

## Performance Tuning

### Increase Cache TTL

For better performance with lower API usage:

```json
{
  "dns": {
    "cache_ttl": "1h"
  },
  "api": {
    "refresh_interval": "30m"
  }
}
```

### Resource Limits

Add resource limits to the systemd service:

```ini
[Service]
MemoryMax=512M
CPUQuota=50%
```

## Security Hardening

### 1. Run as Non-Root

The systemd service already runs as the `opl-dns` user with minimal privileges.

### 2. Restrict Network Access

```bash
# Only allow DNS from internal network
sudo iptables -A INPUT -p udp --dport 53 -s 192.168.0.0/16 -j ACCEPT
sudo iptables -A INPUT -p udp --dport 53 -j DROP
```

### 3. Enable Rate Limiting

Consider adding rate limiting at the firewall level:

```bash
sudo iptables -A INPUT -p udp --dport 53 -m limit --limit 100/sec -j ACCEPT
sudo iptables -A INPUT -p udp --dport 53 -j DROP
```

### 4. Secure the Secret

Ensure the configuration file has proper permissions:

```bash
sudo chmod 600 /etc/opl-dns/config.json
sudo chown root:opl-dns /etc/opl-dns/config.json
```

## Troubleshooting

### DNS Server Not Starting

```bash
# Check if port 53 is already in use
sudo ss -tulpn | grep :53

# Disable systemd-resolved if conflicting
sudo systemctl stop systemd-resolved
sudo systemctl disable systemd-resolved
```

### Block Page Not Loading

```bash
# Check web server is running
curl http://localhost:8080/health

# Check firewall
sudo ufw status

# Check logs
sudo journalctl -u opl-dns --since "1 hour ago"
```

### Blocklist Not Loading

```bash
# Check API connectivity
curl -H "User-Agent: OPL-DNS-Server/1.0.0" \
  "https://onlinepicketline.com/api/blocklist.json"

# Check logs for API errors
sudo journalctl -u opl-dns | grep -i "blocklist"
```

### High Memory Usage

If memory usage is high:

1. Reduce `session.token_ttl` to clean up sessions faster
2. Reduce `session.cleanup_interval` for more frequent cleanup
3. Add memory limits to systemd service

## Updating

To update to a new version:

```bash
cd opl-for-dns
git pull
go build -o opl-dns ./cmd/opl-dns
sudo cp opl-dns /usr/local/bin/
sudo systemctl restart opl-dns
```

## Backup and Recovery

### Configuration Backup

```bash
sudo cp /etc/opl-dns/config.json /backup/opl-dns-config.json
```

### Session Data

Session data is stored in memory and is lost on restart. This is by design - bypass tokens expire after 24 hours anyway.

## Uninstallation

```bash
sudo systemctl stop opl-dns
sudo systemctl disable opl-dns
sudo rm /etc/systemd/system/opl-dns.service
sudo rm /usr/local/bin/opl-dns
sudo rm -rf /etc/opl-dns
sudo rm -rf /var/lib/opl-dns
sudo userdel opl-dns
```
