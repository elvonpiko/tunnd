---
title: Architecture Overview
description: 'Tunnd is a self-hosted tunnel server that exposes local services on the internet. It is structurally similar to ngrok — a small Go binary runs on a public VPS'
---
Tunnd is a self-hosted tunnel server that exposes local services on the internet. It is structurally similar to ngrok — a small Go binary runs on a public VPS, and a lightweight client binary runs on the developer's machine.

## System Components

Tunnd consists of five main components:

```
┌──────────────────────────────────────────────────────────────────────┐
│                            VPS / Server                              │
│                                                                      │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │                       tunnd-server                            │  │
│  │                                                               │  │
│  │  ┌─────────────┐   ┌──────────────┐   ┌──────────────────┐  │  │
│  │  │  Public Mux  │   │ Control Plane│   │  Admin Server    │  │  │
│  │  │  :443 / :80  │   │  WebSocket   │   │  :9091           │  │  │
│  │  │              │   │  Handler     │   │                  │  │  │
│  │  └──────┬───────┘   └──────┬───────┘   └────────┬─────────┘  │  │
│  │         │                  │                     │            │  │
│  │  ┌──────▼───────────────────▼──────┐   ┌────────▼─────────┐  │  │
│  │  │         Tunnel Registry         │   │   Auth Service   │  │  │
│  │  │     (in-memory sessions)        │   │  (token mgmt)    │  │  │
│  │  └──────────────────┬──────────────┘   └────────┬─────────┘  │  │
│  │                     │                           │            │  │
│  │             ┌────────▼────────────────────────── ▼──────┐    │  │
│  │             │              SQLite Store                  │    │  │
│  │             │  (tokens, tunnel history, request logs)    │    │  │
│  │             └────────────────────────────────────────────┘    │  │
│  └───────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────────┘

Developer machine:
┌─────────────────────────────────────┐
│  tunnd (client)                     │
│  ├── WebSocket connection → server  │
│  └── Dials localhost:<port>         │
└─────────────────────────────────────┘
```

### Public Mux (`:443`)

Handles all inbound public traffic. Routes requests based on the `Host` header subdomain:

- `/_tunnd/control` → WebSocket control plane handler
- All other paths → Tunnel Registry (HTTP reverse proxy)

### Control Plane (WebSocket Handler)

Manages the persistent WebSocket connection with each connected client. Responsibilities:

- Authenticates the client token on connection
- Registers the tunnel subdomain with the Registry
- Relays `MsgOpen`, `MsgData`, `MsgReqDone` frames to the client
- Receives `MsgData`, `MsgClose`, `MsgPing` frames from the client
- Deregisters the tunnel when the WebSocket closes

See [WebSocket Protocol](/api/websocket-protocol) for the full message specification.

### Tunnel Registry

In-memory map of active sessions keyed by subdomain. Responsibilities:

- Validates and sanitizes custom subdomain requests
- Generates random subdomains for clients that don't request one
- Serves as an `http.Handler` for the public mux, routing inbound requests to the correct client via pipe-based streams
- Persists tunnel open/close events to SQLite

### Admin Server (`:9091`)

A separate HTTP server providing:

- REST API for managing tokens and inspecting tunnels (`/api/*`)
- Embedded single-page admin dashboard (`/`)
- HTTP Basic Auth protection (username: `admin`)

See [Admin API Reference](/api/admin-api) for endpoint documentation.

### Auth Service

Manages cryptographically random auth tokens stored in SQLite. Each token:

- Has a `tnnd_` prefixed, 48-character hex value
- Can be assigned a label and a `max_tunnels` limit
- Can be revoked without restarting the server

### SQLite Store

Persistent storage for three tables:

| Table | Purpose |
|---|---|
| `tokens` | Auth token records |
| `tunnels` | Tunnel history (open/close timestamps, subdomain, protocol) |
| `request_logs` | Per-request inspector logs (method, path, status, duration) |

The database runs in WAL mode for concurrent reads alongside the single writer.

---

## Component Diagram

```
Internet Browser
      │
      │  HTTPS request to happy-river.tunnel.example.com
      ▼
┌─────────────┐
│ DNS Wildcard │   *.tunnel.example.com → <server-ip>
└──────┬──────┘
       │
       ▼
┌──────────────────────────────────────────────┐
│  tunnd-server :443                            │
│                                               │
│  Host: happy-river.tunnel.example.com         │
│  → extract subdomain "happy-river"            │
│  → Registry.Lookup("happy-river") → Session   │
│  → Session.openStream() → MsgOpen             │
│                      │                        │
│           WebSocket control channel           │
└──────────────────────┼────────────────────────┘
                       │  MsgOpen(stream_id)
                       │  MsgData(request bytes)
                       │  MsgReqDone
                       ▼
           ┌────────────────────┐
           │  tunnd client      │
           │                    │
           │  receives MsgOpen  │
           │  dials localhost:3000
           │  writes request    │
           │  reads response    │
           │  sends MsgData     │
           │  sends MsgClose    │
           └────────┬───────────┘
                    │  TCP
                    ▼
           ┌────────────────────┐
           │  localhost:3000    │
           │  (your app)        │
           └────────────────────┘
```

---

## TLS Architecture

Tunnd supports three TLS modes:

### Mode 1: Let's Encrypt (Automatic)

```
Internet → :443 (HTTPS, autocert)
           :80  (HTTP-01 ACME challenge responder)
```

The server uses `golang.org/x/crypto/acme/autocert` to issue per-subdomain TLS certificates on demand. The first request to a new subdomain triggers a certificate issuance (~2 seconds); subsequent requests use the cached certificate from disk.

**Limitation**: HTTP-01 challenges cannot issue wildcard certificates. Each subdomain gets its own certificate.

### Mode 2: Manual Certificate

```
Internet → :443 (HTTPS, static cert)
```

Provide a wildcard certificate (e.g., `*.tunnel.example.com`) via `tls_cert_file` and `tls_key_file`. This is the recommended approach for wildcard HTTPS.

### Mode 3: Reverse Proxy (No Direct TLS)

```
Internet → Reverse Proxy (handles TLS) → tunnd-server :9091 (plain HTTP)
```

A reverse proxy (Caddy, Nginx, or Traefik) terminates TLS and forwards plain HTTP to Tunnd on the admin port. The proxy handles wildcard certificate issuance, typically via DNS-01 challenge.

---

## Deployment Options

### Option A: Standalone (Direct)

Tunnd handles TLS directly. Simplest deployment, no additional components.

```
Internet ──→ tunnd-server :443 (TLS)
                         :80  (ACME)
                         :9091 (Admin)
```

**Use when**: You want the simplest setup and don't have an existing reverse proxy.

### Option B: Behind Caddy

```
Internet ──→ Caddy :443 (TLS termination) ──→ tunnd-server :9091
                   :80  (ACME HTTP-01)
```

Caddy automatically obtains and renews TLS certificates. Supports wildcard certs via DNS-01 challenge.

### Option C: Behind Nginx

```
Internet ──→ Nginx :443 (TLS termination) ──→ tunnd-server :9091
```

Manual certificate management or integration with Certbot.

### Option D: Behind Traefik

```
Internet ──→ Traefik :443 (TLS termination) ──→ tunnd-server :9091
```

Traefik auto-discovers services and manages TLS certificates via Let's Encrypt.

See the [Reverse Proxy guide](/deployment/reverse-proxy/caddy) and [Docker deployment docs](/deployment/docker) for configuration files for each option.

---

## Port Reference

| Port | Purpose | Configurable |
|---|---|---|
| `443` | Public tunnel traffic (HTTPS) | `TUNND_HTTP_PORT` |
| `80` | ACME HTTP-01 challenge (Let's Encrypt mode only) | Not configurable |
| `9091` | Admin API and dashboard | `TUNND_ADMIN_PORT` |

When running behind a reverse proxy, only port `9091` needs to be reachable by the proxy (not the public internet).

---

## Technology Stack

| Component | Technology |
|---|---|
| Language | Go |
| WebSocket library | `github.com/gorilla/websocket` |
| CLI framework | `github.com/spf13/cobra` |
| Configuration | `github.com/spf13/viper` |
| Database | SQLite via `github.com/mattn/go-sqlite3` |
| TLS automation | `golang.org/x/crypto/acme/autocert` |
| Logging | `github.com/rs/zerolog` |
| UUID generation | `github.com/google/uuid` |
| Container base | Alpine Linux |

---

## Next Steps

- [Data Flow](/architecture/data-flow)
- [WebSocket Protocol](/api/websocket-protocol)
- [Deployment Options](/deployment/docker)
