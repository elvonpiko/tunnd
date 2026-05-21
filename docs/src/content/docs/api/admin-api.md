---
title: Admin API
description: 'The admin REST API is served on the admin port (default `9091`). All endpoints require an authenticated session — log in via the dashboard at `http://<server>'
---
The admin REST API is served on the admin port (default `9091`). All endpoints require an authenticated session — log in via the dashboard at `http://<server>:9091` first, or use a session cookie from `POST /login`.

## Authentication

The admin dashboard uses session cookie authentication. Log in via:

```
POST /login
Content-Type: application/x-www-form-urlencoded

password=your-admin-password
```

This sets an `HttpOnly` session cookie (`tunnd_session`) valid for 12 hours.

**First run:** Visit `http://<server>:9091/setup` to set your admin password before the first login.

---

## Endpoints

### GET /api/stats

Server statistics.

```bash
curl -b 'tunnd_session=<token>' http://localhost:9091/api/stats
```

**Response:**
```json
{
  "active_tunnels": 2,
  "total_tokens": 3,
  "server_time": "2026-05-15T10:30:00Z"
}
```

---

### GET /api/tunnels/active

Currently connected tunnel sessions.

```bash
curl -b 'tunnd_session=<token>' http://localhost:9091/api/tunnels/active
```

**Response:**
```json
{
  "tunnels": [
    {
      "id": "session-uuid",
      "tunnel_id": "tunnel-uuid",
      "subdomain": "myapp",
      "protocol": "http",
      "public_url": "https://myapp.tunnd.example.com"
    }
  ],
  "count": 1
}
```

---

### GET /api/tunnels

Tunnel history (all tunnels, including closed). Paginated.

| Param | Default | Max | Description |
|-------|---------|-----|-------------|
| `limit` | 50 | 200 | Records to return |
| `offset` | 0 | — | Records to skip |

```bash
curl -b 'tunnd_session=<token>' 'http://localhost:9091/api/tunnels?limit=25'
```

**Response:**
```json
{
  "tunnels": [
    {
      "id": "tunnel-uuid",
      "subdomain": "myapp",
      "protocol": "http",
      "public_url": "https://myapp.tunnd.example.com",
      "local_port": 3000,
      "opened_at": "2026-05-15T10:00:00Z",
      "closed_at": "2026-05-15T10:30:00Z"
    }
  ],
  "limit": 25,
  "offset": 0
}
```

---

### GET /api/tunnels/{id}/requests

Request log for a specific tunnel (HTTP tunnels only).

| Param | Default | Max |
|-------|---------|-----|
| `limit` | 100 | 500 |

```bash
curl -b 'tunnd_session=<token>' http://localhost:9091/api/tunnels/tunnel-uuid/requests
```

**Response:**
```json
{
  "requests": [
    {
      "method": "GET",
      "path": "/api/users",
      "status_code": 200,
      "duration_ms": 42,
      "response_size": 1024,
      "created_at": "2026-05-15T10:05:30Z"
    }
  ]
}
```

---

### GET /api/tokens

List all auth tokens (values truncated to 16 chars).

```bash
curl -b 'tunnd_session=<token>' http://localhost:9091/api/tokens
```

**Response:**
```json
{
  "tokens": [
    {
      "id": "token-uuid",
      "label": "my-laptop",
      "value_hint": "tnnd_a1b2c3d4e5f6g7h8…",
      "max_tunnels": 0,
      "enabled": true
    }
  ]
}
```

---

### POST /api/tokens

Create a new auth token. The full value is returned **only once**.

```bash
curl -b 'tunnd_session=<token>' \
  -X POST -H 'Content-Type: application/json' \
  -d '{"label":"ci-server","max_tunnels":5}' \
  http://localhost:9091/api/tokens
```

**Request body:**
```json
{
  "label": "my-laptop",     // optional, defaults to "unnamed"
  "max_tunnels": 0          // 0 = unlimited
}
```

**Response `201 Created`:**
```json
{
  "id": "token-uuid",
  "label": "my-laptop",
  "value": "tnnd_a1b2c3d4e5f6...",   // save this — shown once only
  "max_tunnels": 0,
  "enabled": true
}
```

---

### DELETE /api/tokens/{id}

Revoke a token. Active tunnels using it are immediately disconnected.

```bash
curl -b 'tunnd_session=<token>' \
  -X DELETE http://localhost:9091/api/tokens/token-uuid
```

**Response:**
```json
{ "revoked": "token-uuid" }
```

---

## Error responses

```json
{ "error": "error message" }
```

| Status | Meaning |
|--------|---------|
| `400` | Bad request (invalid body or missing params) |
| `401` | Not authenticated — log in first |
| `404` | Endpoint or resource not found |
| `503` | Server not configured — visit `/setup` |
| `500` | Server error — check logs |

---

## Next steps

- [WebSocket Protocol](/api/websocket-protocol)
- [Server Config Reference](/configuration/server-config)
