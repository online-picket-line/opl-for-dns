# OPL DNS Plugin for BIND 9

A DNS plugin for BIND 9 that integrates with the Online Picket Line API to detect and notify users about websites involved in labor disputes.

## ⚠️ Important Note

This is a **reference implementation** and **framework** for building a BIND 9 DNS plugin. The DNS response modification functionality is currently incomplete and requires additional implementation to fully redirect DNS queries. The plugin successfully:
- Hooks into BIND 9's query processing
- Queries the Online Picket Line API
- Detects disputed domains
- Logs dispute information

However, the actual DNS response modification needs to be completed using BIND 9's internal APIs for production use. See the implementation notes in `src/opl_plugin.c` for details.

## Overview

The OPL DNS Plugin intercepts DNS queries and checks them against the Online Picket Line API. When a domain is found to be involved in a labor dispute, the plugin is designed to modify the DNS response to point to a block page that:

- Informs users about the labor dispute
- Provides details about workers' concerns
- Gives users the option to learn more, go back, or continue to the site

This enables digital solidarity with workers by making labor disputes visible at the DNS level.

## Features

- **Real-time API Integration**: Queries the Online Picket Line API for each DNS request
- **Transparent Redirection**: Framework for redirecting disputed domains to an informational block page
- **User Choice**: Users can choose to learn more, go back, or continue to the original site
- **Configurable**: Flexible configuration for API endpoints, timeouts, and block page IP
- **Caching**: Built-in caching to minimize API calls and improve performance
- **Logging**: Comprehensive logging through BIND 9's logging system
- **Security**: URL encoding, proper error handling, and fail-open behavior

## Requirements

- BIND 9 (version 9.11 or later with plugin support)
- libcurl (for API requests)
- json-c (for JSON parsing)
- C compiler (gcc or clang)

### Installation of Dependencies

#### Debian/Ubuntu
```bash
sudo apt-get update
sudo apt-get install bind9 bind9-dev libcurl4-openssl-dev libjson-c-dev build-essential
```

#### RHEL/CentOS/Fedora
```bash
sudo yum install bind bind-devel libcurl-devel json-c-devel gcc make
```

#### macOS
```bash
brew install bind libcurl json-c
```

## Building the Plugin

1. Clone the repository:
```bash
git clone https://github.com/oplfun/opl-for-dns.git
git clone https://github.com/online-picket-line/opl-for-dns.git
cd opl-for-dns
```

2. Build the plugin:
```bash
make
```

3. Install the plugin (requires root):
```bash
sudo make install
```

The plugin will be installed to `/usr/lib/bind9/modules/opl-dns-plugin.so`.

## Configuration

### 1. Plugin Configuration File

Create a configuration file at `/etc/bind/opl-plugin.conf`:

```ini
# OPL DNS Plugin Configuration
api_endpoint = https://api.onlinepicketline.org/v1/check
block_page_ip = 192.168.1.100
api_timeout = 5
cache_ttl = 300
enabled = 1
```

**Configuration Options:**

- `api_endpoint`: URL of the Online Picket Line API (default: https://api.onlinepicketline.org/v1/check)
- `block_page_ip`: IP address where the block page is hosted (default: 127.0.0.1)
- `api_timeout`: Timeout for API requests in seconds (default: 5)
- `cache_ttl`: How long to cache API responses in seconds (default: 300)
- `enabled`: Enable (1) or disable (0) the plugin (default: 1)

### 2. BIND 9 Configuration

Add the plugin to your BIND 9 configuration file (`/etc/bind/named.conf`):

```
plugin opl-dns-plugin "/usr/lib/bind9/modules/opl-dns-plugin.so" {
    config "/etc/bind/opl-plugin.conf";
};
```

### 3. Block Page Setup

The block page needs to be hosted on a web server at the IP address specified in `block_page_ip`.

1. Install a web server (e.g., nginx or apache)
2. Copy the block page HTML to the web server's document root:
```bash
sudo cp examples/block-page.html /var/www/html/index.html
```

3. Configure the web server to serve the block page for all requests

Example nginx configuration:
```nginx
server {
    listen 80 default_server;
    listen [::]:80 default_server;
    root /var/www/html;
    index index.html;
    
    location / {
        try_files $uri $uri/ /index.html;
    }
}
```

### 4. Restart BIND 9

After configuration, restart BIND 9:
```bash
sudo systemctl restart bind9
```

## API Specification

The plugin expects the Online Picket Line API to respond with JSON in the following format:

### Request
```
GET https://api.onlinepicketline.org/v1/check?domain=example.com
```

### Response
```json
{
    "disputed": true,
    "info": "Workers at Example Corp are on strike for better wages and working conditions.",
    "details": {
        "organization": "Example Corp",
        "dispute_type": "strike",
        "start_date": "2025-01-15",
        "workers_affected": 500
    }
}
```

**Response Fields:**
- `disputed` (boolean): Whether the domain is involved in a labor dispute
- `info` (string): Human-readable information about the dispute
- `details` (object, optional): Additional structured information

## How It Works

1. **DNS Query Interception**: When a DNS query is received, the plugin hooks into BIND 9's query processing pipeline.

2. **API Check**: The plugin extracts the domain name and sends a request to the Online Picket Line API.

3. **Response Modification**: If the API indicates a labor dispute:
   - The DNS response is modified to return the block page IP instead of the original IP
   - The query is logged with dispute information

4. **User Experience**: Users requesting the disputed domain are redirected to the block page where they can:
   - Learn about the labor dispute
   - Choose to go back
   - Choose to continue anyway (if they wish to cross the digital picket line)

## Logging

The plugin logs its activity through BIND 9's logging system. To see plugin logs:

```bash
sudo tail -f /var/log/syslog | grep OPL
```

Or configure BIND 9 logging in `named.conf`:
```
logging {
    channel opl_log {
        file "/var/log/named/opl-plugin.log" versions 3 size 5m;
        severity info;
        print-time yes;
        print-category yes;
    };
    category default { opl_log; };
};
```

## Testing

### Manual Testing

1. Configure the plugin with a test block page IP
2. Restart BIND 9
3. Test with a known disputed domain:
```bash
dig @localhost disputed-domain.com
```

4. The response should show the block page IP

### API Testing

Test the API integration:
```bash
curl "https://api.onlinepicketline.org/v1/check?domain=example.com"
```

## Troubleshooting

### Plugin not loading
- Check BIND 9 logs: `sudo journalctl -u bind9 -f`
- Verify plugin file exists: `ls -l /usr/lib/bind9/modules/opl-dns-plugin.so`
- Check file permissions: `sudo chmod 644 /usr/lib/bind9/modules/opl-dns-plugin.so`

### API requests failing
- Test API connectivity: `curl https://api.onlinepicketline.org/v1/check?domain=test.com`
- Check firewall rules
- Increase `api_timeout` in configuration

### DNS queries not being intercepted
- Verify plugin is loaded: Check BIND 9 startup logs
- Ensure `enabled = 1` in configuration
- Check BIND 9 plugin API version compatibility

## Security Considerations

- The plugin makes external API calls for each DNS query, which could be a privacy concern
- Consider implementing local caching or whitelisting to reduce API calls
- The block page IP should be secured and regularly monitored
- API requests should use HTTPS to prevent tampering

## Performance

- API calls are cached for the duration specified in `cache_ttl`
- Failed API requests fail open (allow the query) to prevent service disruption
- Timeout settings prevent slow API responses from affecting DNS performance

## Contributing

Contributions are welcome! Please feel free to submit pull requests or open issues.

## License

[Add appropriate license information]

## Support

For issues and questions:
For issues and questions:
- GitHub Issues: https://github.com/online-picket-line/opl-for-dns/issues
- Online Picket Line: https://onlinepicketline.org

## Credits

Created to support digital solidarity with workers everywhere.
