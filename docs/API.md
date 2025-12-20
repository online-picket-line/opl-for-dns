# API Integration Documentation

This document describes how the OPL DNS Plugin integrates with the Online Picket Line API.

## API Overview

The Online Picket Line API provides information about domains involved in labor disputes. The plugin queries this API for each DNS request to determine if a domain should be redirected to the block page.

## API Endpoint

```
GET https://api.onlinepicketline.org/v1/check
```

## Request Format

### Query Parameters

| Parameter | Type   | Required | Description                           |
|-----------|--------|----------|---------------------------------------|
| domain    | string | Yes      | The domain name to check              |

### Example Request

```bash
curl "https://api.onlinepicketline.org/v1/check?domain=example.com"
```

### Request Headers

The plugin sends the following headers:

```
User-Agent: OPL-DNS-Plugin/1.0.0
Accept: application/json
```

## Response Format

### Success Response (Domain Disputed)

**Status Code:** 200 OK

**Response Body:**
```json
{
    "disputed": true,
    "info": "Workers at Example Corp are on strike for better wages and working conditions.",
    "details": {
        "organization": "Example Corp",
        "dispute_type": "strike",
        "start_date": "2025-01-15",
        "workers_affected": 500,
        "union": "Example Workers Union",
        "demands": [
            "15% wage increase",
            "Better healthcare benefits",
            "Improved working conditions"
        ]
    }
}
```

### Success Response (Domain Not Disputed)

**Status Code:** 200 OK

**Response Body:**
```json
{
    "disputed": false
}
```

### Error Responses

#### 400 Bad Request
Missing or invalid domain parameter.

```json
{
    "error": "Missing required parameter: domain"
}
```

#### 404 Not Found
Domain not found in the database.

```json
{
    "disputed": false
}
```

#### 500 Internal Server Error
API server error.

```json
{
    "error": "Internal server error"
}
```

#### 503 Service Unavailable
API temporarily unavailable.

```json
{
    "error": "Service temporarily unavailable"
}
```

## Response Fields

### Root Level Fields

| Field     | Type    | Required | Description                                    |
|-----------|---------|----------|------------------------------------------------|
| disputed  | boolean | Yes      | Whether the domain is involved in a dispute    |
| info      | string  | No       | Human-readable information about the dispute   |
| details   | object  | No       | Additional structured information              |
| error     | string  | No       | Error message (only present on errors)         |

### Details Object Fields

| Field            | Type     | Required | Description                                  |
|------------------|----------|----------|----------------------------------------------|
| organization     | string   | No       | Name of the organization involved            |
| dispute_type     | string   | No       | Type of dispute (strike, boycott, etc.)      |
| start_date       | string   | No       | Date the dispute started (ISO 8601 format)   |
| workers_affected | integer  | No       | Number of workers involved                   |
| union            | string   | No       | Name of the union representing workers       |
| demands          | array    | No       | List of worker demands                       |
| more_info_url    | string   | No       | URL with more information about the dispute  |

## Plugin Behavior

### Request Flow

1. **DNS Query Received**: Plugin intercepts DNS query from BIND 9
2. **Cache Check**: Plugin checks if domain status is cached
3. **API Request**: If not cached, plugin sends HTTP GET request to API
4. **Parse Response**: Plugin parses JSON response
5. **Cache Result**: Plugin caches the result for `cache_ttl` seconds
6. **Modify DNS Response**: If disputed, modify DNS response to point to block page
7. **Return**: Return to BIND 9 for final processing

### Timeout Handling

- **Default Timeout**: 5 seconds (configurable via `api_timeout`)
- **Timeout Behavior**: If API request times out, the plugin allows the DNS query to proceed normally (fail-open)
- **Retry Logic**: No automatic retry on timeout to prevent DNS delays

### Error Handling

| Scenario                  | Plugin Behavior                                    |
|---------------------------|----------------------------------------------------|
| API returns 200 OK        | Process response normally                          |
| API returns 4xx/5xx       | Log error, allow DNS query (fail-open)             |
| Network timeout           | Log error, allow DNS query (fail-open)             |
| Invalid JSON response     | Log error, allow DNS query (fail-open)             |
| Missing `disputed` field  | Treat as not disputed, allow DNS query             |

### Caching

The plugin implements client-side caching to reduce API load:

- **Cache Key**: Domain name (normalized to lowercase)
- **Cache Duration**: Configurable via `cache_ttl` (default: 300 seconds)
- **Cache Storage**: In-memory (per BIND process)
- **Cache Invalidation**: Automatic after TTL expires

## Testing the API

### Test with curl

```bash
# Test a disputed domain
curl "https://api.onlinepicketline.org/v1/check?domain=disputed-example.com"

# Test a non-disputed domain
curl "https://api.onlinepicketline.org/v1/check?domain=safe-example.com"

# Test with verbose output
curl -v "https://api.onlinepicketline.org/v1/check?domain=example.com"
```

### Test with the Plugin

1. Enable debug logging in BIND 9
2. Query a test domain:
```bash
dig @localhost test-domain.com
```
3. Check logs:
```bash
sudo journalctl -u bind9 -f | grep OPL
```

## Rate Limiting

The API may implement rate limiting to prevent abuse:

- **Limit**: TBD (to be determined by API provider)
- **Plugin Behavior**: Caching helps stay within limits
- **Best Practice**: Use appropriate `cache_ttl` value (300-3600 seconds)

## Authentication

Currently, the API does not require authentication. If authentication is added in the future:

1. The plugin will need to be updated to include authentication headers
2. Configuration will need to include API key or token
3. Plugin will handle authentication errors appropriately

## Privacy Considerations

### Data Sent to API

The plugin sends only:
- Domain name being queried
- User-Agent header identifying the plugin version

### Data NOT Sent

The plugin does NOT send:
- Client IP addresses
- Query timestamps
- Any personally identifiable information
- Full DNS packet contents

### HTTPS Requirement

The plugin REQUIRES the API to use HTTPS to ensure:
- Data confidentiality
- API authenticity
- Protection against man-in-the-middle attacks

## API Versioning

Current API Version: v1

The API version is included in the endpoint URL:
```
https://api.onlinepicketline.org/v1/check
```

Future versions will use different version numbers (v2, v3, etc.). The plugin can be configured to use different API versions via the `api_endpoint` configuration parameter.

## Local API Mirror

For organizations that want to run their own API mirror:

1. Set up a local API server that implements the same interface
2. Update plugin configuration:
```ini
api_endpoint = http://localhost:8080/v1/check
```

3. Ensure the local API returns responses in the same format

## Monitoring and Debugging

### Plugin Logging

The plugin logs all API interactions:

```
OPL: Checking domain example.com against API
OPL: API returned disputed=true for domain example.com
OPL: Labor dispute detected for domain example.com
```

### API Health Check

Monitor API health with:

```bash
#!/bin/bash
response=$(curl -s -w "%{http_code}" -o /dev/null "https://api.onlinepicketline.org/v1/check?domain=health-check.com")
if [ "$response" = "200" ]; then
    echo "API is healthy"
    exit 0
else
    echo "API returned status $response"
    exit 1
fi
```

## Example Implementation (Mock API)

For testing purposes, here's a simple mock API implementation:

```python
from flask import Flask, request, jsonify

app = Flask(__name__)

# Mock dispute database
DISPUTED_DOMAINS = {
    "strike-example.com": {
        "disputed": True,
        "info": "Workers on strike for better wages",
        "details": {
            "organization": "Example Corp",
            "dispute_type": "strike",
            "start_date": "2025-01-15"
        }
    }
}

@app.route('/v1/check')
def check_domain():
    domain = request.args.get('domain')
    
    if not domain:
        return jsonify({"error": "Missing required parameter: domain"}), 400
    
    domain = domain.lower()
    
    if domain in DISPUTED_DOMAINS:
        return jsonify(DISPUTED_DOMAINS[domain])
    
    return jsonify({"disputed": False})

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=8080)
```

Run with:
```bash
pip install flask
python mock_api.py
```

## Support

For API-related questions:
## Support

For API-related questions:
- API Documentation: https://github.com/online-picket-line/online-picket-line/blob/main/API_DOCUMENTATION.md
- Plugin Issues: https://github.com/online-picket-line/opl-for-dns/issues
