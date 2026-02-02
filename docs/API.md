# API Documentation

This document describes the APIs exposed by the OPL DNS Server.

## Block Page Web Server API

The block page web server runs alongside the DNS server and provides endpoints for:
- Serving block pages to users
- Managing bypass tokens
- Checking domain status

### Base URL

```
http://YOUR_SERVER_IP:8080
```

### Authentication

The block page API does not require authentication. Bypass tokens are tied to client IP addresses and signed with HMAC to prevent tampering.

---

## Endpoints

### GET /

Serves the block page for a blocked domain.

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| domain | string | Yes | The blocked domain to display information for |
| mode | string | No | Display mode: "block" (default) or "overlay" |

**Example:**

```bash
curl "http://localhost:8080/?domain=example.com"
```

**Response:**

Returns an HTML page with:
- Information about the labor action
- Three action buttons: Learn More, Go Back, Continue Anyway

---

### GET /api/check

Checks if a domain is blocked.

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| domain | string | Yes | The domain to check |

**Example:**

```bash
curl "http://localhost:8080/api/check?domain=example.com"
```

**Response (Blocked):**

```json
{
  "blocked": true,
  "hasBypass": false,
  "domain": "example.com",
  "employer": "Example Corp",
  "actionType": "strike",
  "description": "Workers on strike for better wages and benefits",
  "moreInfoUrl": "https://example.com/strike-info",
  "organization": "Workers United Local 123"
}
```

**Response (Not Blocked):**

```json
{
  "blocked": false,
  "domain": "example.com"
}
```

---

### GET /api/bypass

Creates a bypass token and redirects to the original site.

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| domain | string | Yes | The domain to bypass |

**Example:**

```bash
curl -L "http://localhost:8080/api/bypass?domain=example.com"
```

**Response:**

HTTP 307 Temporary Redirect to `https://example.com`

---

### POST /api/bypass

Creates a bypass token and returns JSON response.

**Request Body:**

```json
{
  "domain": "example.com"
}
```

**Example:**

```bash
curl -X POST "http://localhost:8080/api/bypass" \
  -H "Content-Type: application/json" \
  -d '{"domain": "example.com"}'
```

**Response (Success):**

```json
{
  "success": true,
  "token": "BASE64_ENCODED_TOKEN",
  "redirectUrl": "https://example.com",
  "expiresIn": 86400
}
```

**Response (Error - Domain Not Blocked):**

```json
{
  "success": false,
  "error": "Domain is not blocked"
}
```

---

### GET /health

Health check endpoint for monitoring.

**Example:**

```bash
curl "http://localhost:8080/health"
```

**Response:**

```json
{
  "status": "ok",
  "blocklistLoaded": true,
  "activeSessions": 42,
  "lastBlocklistFetch": "2024-01-15T10:30:00Z"
}
```

---

## Bypass Token Format

Bypass tokens are base64-encoded strings containing:

- Client IP address
- Domain name
- Timestamp
- Random bytes (for uniqueness)
- HMAC signature

The token format is:
```
base64(clientIP|domain|timestamp|random|hmac)
```

Tokens are:
- Valid for 24 hours (configurable via `session.token_ttl`)
- Tied to the client IP address
- Specific to a single domain
- Cryptographically signed to prevent tampering

---

## Online Picket Line API Integration

The DNS server fetches blocklist data from the Online Picket Line API:

**Endpoint:**
```
GET https://onlinepicketline.com/api/blocklist.json
```

**Headers:**
```
X-API-Key: YOUR_API_KEY (optional but recommended)
User-Agent: OPL-DNS-Server/1.0.0
```

**Response Format:**

```json
{
  "version": "1.0",
  "generatedAt": "2024-01-15T10:30:00Z",
  "totalUrls": 150,
  "employers": [
    {
      "id": "emp-123",
      "name": "Example Corp",
      "urlCount": 5
    }
  ],
  "blocklist": [
    {
      "url": "https://example.com",
      "employer": "Example Corp",
      "employerId": "emp-123",
      "label": "Main Website",
      "category": "corporate",
      "reason": "Active labor action: strike",
      "startDate": "2024-01-01",
      "moreInfoUrl": "https://union.org/strike-info",
      "location": "Detroit, MI",
      "actionDetails": {
        "id": "action-123",
        "organization": "UAW Local 456",
        "actionType": "strike",
        "status": "active",
        "startDate": "2024-01-01",
        "description": "Workers striking for better wages",
        "demands": "15% wage increase, healthcare",
        "contactInfo": "contact@union.org",
        "unionLogoUrl": "/union_logos/uaw.png",
        "learnMoreUrl": "https://union.org/strike-info"
      }
    }
  ]
}
```

**Caching:**

- The server uses hash-based caching to minimize bandwidth
- Sends `X-Content-Hash` header value in subsequent requests
- Server returns 304 Not Modified if data hasn't changed
- Default refresh interval: 15 minutes

For more details, see the [Online Picket Line API Documentation](https://github.com/online-picket-line/online-picketline/blob/main/doc/API_DOCUMENTATION.md).

---

## Error Responses

All API endpoints return consistent error responses:

```json
{
  "error": "Error message description"
}
```

HTTP status codes:
- `200` - Success
- `400` - Bad Request (missing/invalid parameters)
- `404` - Not Found (domain not blocked)
- `429` - Too Many Requests (rate limited)
- `500` - Internal Server Error
