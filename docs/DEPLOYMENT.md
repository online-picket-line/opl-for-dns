# Deployment Guide for OPL DNS Plugin

This guide provides step-by-step instructions for deploying the OPL DNS Plugin in various environments.

## Prerequisites

Before deploying, ensure you have:
- Root or sudo access to the DNS server
- BIND 9 (version 9.11+) installed
- Network connectivity to the Online Picket Line API
- A web server for hosting the block page

## Deployment Steps

### Step 1: Install Dependencies

#### On Debian/Ubuntu:
```bash
sudo apt-get update
sudo apt-get install bind9 bind9-dev libcurl4-openssl-dev libjson-c-dev build-essential
```

#### On RHEL/CentOS:
```bash
sudo yum install bind bind-devel libcurl-devel json-c-devel gcc make
```
git clone https://github.com/online-picket-line/opl-for-dns.git
### Step 2: Build and Install the Plugin

```bash
# Clone the repository
git clone https://github.com/oplfun/opl-for-dns.git
cd opl-for-dns

# Build the plugin
make

# Install the plugin (requires root)
sudo make install
```

### Step 3: Set Up the Block Page Server

You need a web server to host the block page. This can be on the same server as BIND or a separate server.

#### Option A: Using Nginx

1. Install nginx:
```bash
sudo apt-get install nginx  # Debian/Ubuntu
# or
sudo yum install nginx      # RHEL/CentOS
```

2. Copy the block page:
```bash
sudo mkdir -p /var/www/opl-block-page
sudo cp examples/block-page.html /var/www/opl-block-page/index.html
```

3. Configure nginx (`/etc/nginx/sites-available/opl-block-page`):
```nginx
server {
    listen 80 default_server;
    listen [::]:80 default_server;
    
    root /var/www/opl-block-page;
    index index.html;
    
    server_name _;
    
    location / {
        try_files $uri $uri/ /index.html;
    }
}
```

4. Enable the site:
```bash
sudo ln -s /etc/nginx/sites-available/opl-block-page /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl restart nginx
```

#### Option B: Using Apache

1. Install Apache:
```bash
sudo apt-get install apache2  # Debian/Ubuntu
# or
sudo yum install httpd        # RHEL/CentOS
```

2. Copy the block page:
```bash
sudo cp examples/block-page.html /var/www/html/index.html
```

3. Restart Apache:
```bash
sudo systemctl restart apache2  # Debian/Ubuntu
# or
### Step 4: Configure the Plugin

sudo nano /etc/bind/opl-plugin.conf
```
api_endpoint = https://api.onlinepicketline.org/v1/check
block_page_ip = 192.168.1.100
api_timeout = 5
cache_ttl = 300
GitHub Issues: https://github.com/online-picket-line/opl-for-dns/issues
Documentation: https://github.com/online-picket-line/opl-for-dns/docs

**Important:** Replace `192.168.1.100` with the actual IP address of your block page server.

### Step 5: Configure BIND 9

1. Edit the BIND configuration file:
```bash
sudo nano /etc/bind/named.conf
```

2. Add the plugin configuration:
```
plugin opl-dns-plugin "/usr/lib/bind9/modules/opl-dns-plugin.so" {
    config "/etc/bind/opl-plugin.conf";
};
```

3. Verify the BIND configuration:
```bash
sudo named-checkconf
```

### Step 6: Start/Restart BIND 9

```bash
sudo systemctl restart bind9    # Debian/Ubuntu
# or
sudo systemctl restart named    # RHEL/CentOS
```

### Step 7: Verify the Installation

1. Check BIND logs for plugin loading:
```bash
sudo journalctl -u bind9 -f | grep OPL
```

You should see a message like:
```
OPL DNS Plugin v1.0.0 loaded successfully
```

2. Test DNS resolution with a test domain:
```bash
dig @localhost test-domain.com
```

3. Check if the API is accessible:
```bash
curl "https://api.onlinepicketline.org/v1/check?domain=test.com"
```

## Production Considerations

### High Availability

For production deployments, consider:

1. **Multiple DNS Servers**: Deploy the plugin on multiple BIND servers for redundancy
2. **Load Balancing**: Use anycast or round-robin DNS for load distribution
3. **API Caching**: Increase `cache_ttl` to reduce API load
4. **Local API Mirror**: Consider running a local mirror of the OPL API for better performance

### Monitoring

Set up monitoring for:

1. **BIND Health**: Monitor BIND process and query response times
2. **Plugin Logs**: Set up log aggregation for plugin activity
3. **API Availability**: Monitor connectivity to the OPL API
4. **Block Page Availability**: Monitor the web server hosting the block page

Example monitoring script:
```bash
#!/bin/bash
# Check if BIND is running
if ! systemctl is-active --quiet bind9; then
    echo "BIND is not running!"
    exit 1
fi

# Check API connectivity
if ! curl -s -f "https://api.onlinepicketline.org/v1/check?domain=test.com" > /dev/null; then
    echo "Cannot reach OPL API!"
    exit 1
fi

echo "All systems operational"
exit 0
```

### Security

1. **Firewall Rules**: Only allow necessary traffic to the block page server
2. **HTTPS**: Consider using HTTPS for the block page (requires SSL certificate)
3. **API Authentication**: If the OPL API requires authentication, configure it in the plugin
4. **Log Rotation**: Set up log rotation to prevent disk space issues

### Performance Tuning

1. **Increase Cache TTL**: For better performance, increase the cache TTL:
```ini
cache_ttl = 3600  # 1 hour
```

2. **Adjust API Timeout**: Balance between responsiveness and reliability:
```ini
api_timeout = 3  # Faster timeout for better DNS performance
```

3. **BIND Worker Threads**: Increase BIND worker threads if handling high query volume

## Updating the Plugin

To update the plugin:

```bash
cd opl-for-dns
git pull
make clean
make
sudo make install
sudo systemctl restart bind9
```

## Rollback Procedure

If you need to disable or remove the plugin:

1. Comment out the plugin line in `/etc/bind/named.conf`:
```
# plugin opl-dns-plugin "/usr/lib/bind9/modules/opl-dns-plugin.so";
```

2. Restart BIND:
```bash
sudo systemctl restart bind9
```

3. To completely remove:
```bash
sudo rm /usr/lib/bind9/modules/opl-dns-plugin.so
sudo rm /etc/bind/opl-plugin.conf
```

## Troubleshooting

### Plugin fails to load
- Check file permissions: `sudo chmod 644 /usr/lib/bind9/modules/opl-dns-plugin.so`
- Verify BIND version supports plugins: `named -V`
- Check for missing dependencies: `ldd /usr/lib/bind9/modules/opl-dns-plugin.so`

### DNS queries not being modified
- Verify plugin is enabled in config: `enabled = 1`
- Check if domain is actually disputed via API
- Review BIND logs for errors

### Performance issues
- Increase `api_timeout` to prevent timeouts
- Increase `cache_ttl` to reduce API calls
- Consider deploying local API mirror

## Support

For deployment assistance:
- GitHub Issues: https://github.com/oplfun/opl-for-dns/issues
- Documentation: https://github.com/oplfun/opl-for-dns/docs
