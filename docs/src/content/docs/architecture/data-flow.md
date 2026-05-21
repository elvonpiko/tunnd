---
title: Data Flow
description: 'This document explains how data moves through Tunnd, from a browser request entering the server to a response being delivered back.'
---
This document explains how data moves through Tunnd, from a browser request entering the server to a response being delivered back.

## Overview

Tunnd acts as a bidirectional relay. The server receives public HTTP requests and tunnels them through a persistent WebSocket connection to the client, which forwards them to a locally running application. The response travels the reverse path.

```
Browser → Server → WebSocket → Client → localhost:port
Browser ← Server ← WebSocket ← Client ← localhost:port
```

Every request uses an independent **stream** identified by a UUID. Multiple streams can be in-flight simultaneously on a single WebSocket connection.

---

## Connection Establishment

Before any requests can be tunneled, the client establishes a WebSocket connection and registers a tunnel subdomain.

```
┌────────┐                              ┌────────────────┐
│ Client │                              │     Server     │
└───┬────┘                              └───────┬────────┘
    │                                           │
    │── WebSocket upgrade ──────────────────────▶│
    │   GET /_tunnd/control HTTP/1.1             │
    │   Upgrade: websocket                       │
    │                                           │
    │◀── 101 Switching Protocols ───────────────│
    │                                           │
    │── MsgRegister ────────────────────────────▶│
    │   { type: "register",                      │
    │     payload: {                             │
    │       token: "tnnd_...",                   │
    │       subdomain: "my-app",                 │
    │       protocol: "http",                    │
    │       local_port: 3000                     │
    │     }                                      │
    │   }                                        │
    │                                           │
    │             [token validated]              │
    │             [subdomain registered]         │
    │             [tunnel record persisted]      │
    │                                           │
    │◀── MsgRegistered ─────────────────────────│
    │   { type: "registered",                    │
    │     payload: {                             │
    │       subdomain: "my-app",                 │
    │       public_url: "https://my-app.t.e.c",  │
    │       tunnel_id: "uuid"                    │
    │     }                                      │
    │   }                                        │
    │                                           │
    │         [WebSocket stays open]             │
```

If registration fails (bad token, subdomain conflict, invalid subdomain), the server sends `MsgError` and closes the connection.

---

## Request/Response Cycle

Once the tunnel is live, each inbound HTTP request creates a new stream. The full cycle for one request:

```
┌─────────┐     ┌──────────────────────────────┐     ┌────────┐     ┌───────────┐
│ Browser │     │         tunnd-server          │     │ Client │     │ localhost │
└────┬────┘     └──────────┬───────────────────┘     └───┬────┘     └─────┬─────┘
     │                     │                             │                │
     │── GET /api/users ──▶│                             │                │
     │                     │                             │                │
     │               [extract subdomain                  │                │
     │                from Host header]                  │                │
     │               [Registry.Lookup]                   │                │
     │               [Session.openStream()]              │                │
     │               [allocate req/resp pipes]           │                │
     │                     │                             │                │
     │                     │── MsgOpen ─────────────────▶│                │
     │                     │   { stream_id: "uuid" }     │                │
     │                     │                             │                │
     │               [goroutine: write req               │                │
     │                bytes to req pipe]                 │                │
     │                     │                             │                │
     │                     │── MsgData ─────────────────▶│                │
     │                     │   { stream_id, data: "..." } │               │
     │                     │      (may be multiple       │                │
     │                     │       frames for large reqs)│                │
     │                     │                             │                │
     │                     │── MsgReqDone ──────────────▶│                │
     │                     │   { stream_id: "uuid" }     │                │
     │                     │                             │── connect ────▶│
     │                     │                             │── write req ──▶│
     │                     │                             │◀── read resp ──│
     │                     │                             │                │
     │                     │◀── MsgData ─────────────────│                │
     │                     │   { stream_id, data: "..." } │               │
     │                     │      (response bytes;        │               │
     │                     │       may be multiple frames)│               │
     │                     │                             │                │
     │               [write to resp pipe]                │                │
     │               [http.ReadResponse]                 │                │
     │                     │◀── MsgClose ────────────────│                │
     │                     │   { stream_id: "uuid" }     │                │
     │                     │                             │                │
     │               [close resp pipe]                   │                │
     │               [copy headers + body]               │                │
     │               [log request to DB]                 │                │
     │                     │                             │                │
     │◀── 200 OK ──────────│                             │                │
```

---

## Internal Pipe Architecture

Each stream uses two `io.Pipe` pairs to bridge between goroutines without buffering in memory:

```
                   ┌── Request Pipe ──┐
  ServeHTTP        │                  │     pumpRequest goroutine
  req.Write(reqW) ─▶  reqW → reqR    ─▶  reads reqR → MsgData → client
                   └──────────────────┘

                   ┌── Response Pipe ──┐
  WriteRespData ───▶  respW → respR   ─▶  ServeHTTP
  (control reader)  └───────────────────┘  http.ReadResponse(respR)
```

**Request pipe** (server → client):
- `ServeHTTP` writes the serialized HTTP request to `reqW`
- `pumpRequest` goroutine reads from `reqR` and sends `MsgData` frames to the client
- When `reqW` is closed (EOF), `pumpRequest` sends `MsgReqDone` to signal the end of the request

**Response pipe** (client → server):
- The control-plane reader goroutine receives `MsgData` frames and calls `WriteRespData`, writing to `respW`
- `ServeHTTP` blocks on `http.ReadResponse(respR)` until enough response bytes arrive
- When the client sends `MsgClose`, `respW` is closed, unblocking `http.ReadResponse`

This design means the server never buffers a full request or response in memory — data streams from one goroutine to another via pipes.

---

## Concurrent Streams

Multiple HTTP requests can be in-flight simultaneously on a single WebSocket connection. Each stream is tracked independently in the session's `streams` map:

```
WebSocket connection (one per client)
├── Stream aaa-111  (GET /api/users)
│   ├── req pipe: ServeHTTP → pumpRequest
│   └── resp pipe: control reader → ServeHTTP
├── Stream bbb-222  (POST /api/orders)
│   ├── req pipe: ServeHTTP → pumpRequest
│   └── resp pipe: control reader → ServeHTTP
└── Stream ccc-333  (GET /static/app.js)
    ├── req pipe: ServeHTTP → pumpRequest
    └── resp pipe: control reader → ServeHTTP
```

The `stream_id` field in every `MsgData`, `MsgClose`, and `MsgReqDone` message identifies which stream the data belongs to.

---

## Tunnel Disconnection

When the client disconnects (WebSocket closes), the server tears down all in-flight streams so that any blocked `ServeHTTP` goroutines unblock and return a `502 Bad Gateway` to the browser.

```
┌────────┐                         ┌────────────────┐
│ Client │                         │     Server     │
└───┬────┘                         └───────┬────────┘
    │                                      │
    │── WebSocket close ───────────────────▶│
    │                                      │
    │                        [Deregister(subdomain)]
    │                        [close all stream pipes]
    │                        [db.CloseTunnel(id)]
    │                        [remove from Registry]
    │                                      │
    │                        [ServeHTTP goroutines
    │                         unblock with ErrClosedPipe
    │                         → 502 to browser]
```

---

## Keepalive

The server sends WebSocket-level **ping** frames every ~54 seconds (90% of the 60-second pong timeout). If the client does not respond with a pong within 60 seconds, the connection is considered dead and the tunnel is deregistered.

```
Server ──── WebSocket Ping ────▶ Client
Server ◀─── WebSocket Pong ──── Client
```

The client can also send application-level `MsgPing` messages, to which the server responds with `MsgPong`. This is useful for clients that want to verify the tunnel is still active without waiting for the next server-initiated ping.

---

## Admin Request Logging

After each proxied request completes, the server logs a `request_log` record to SQLite asynchronously (non-blocking):

```go
r.db.LogRequest(&store.RequestLog{
    TunnelID:     sess.TunnelID,
    Method:       req.Method,
    Path:         req.URL.RequestURI(),
    StatusCode:   resp.StatusCode,
    DurationMs:   time.Since(start).Milliseconds(),
    ResponseSize: written,
})
```

These logs are queryable via `GET /api/tunnels/{id}/requests` in the Admin API and displayed in the admin dashboard's inspector view.

---

## Subdomain Routing

The server extracts the subdomain from the incoming request's `Host` header:

```
Host: happy-river.tunnel.example.com
                                ↓
extractSubdomain("happy-river.tunnel.example.com", "tunnel.example.com")
                                ↓
                          "happy-river"
                                ↓
                     Registry.Lookup("happy-river")
                                ↓
                    *Session (if connected) or nil
```

If the subdomain is not found in the registry (no active client), the server returns `502 Bad Gateway` with the message:

```
no active tunnel for 'happy-river' — is the client connected?
```

---

## WebSocket Send Buffer

Each session has a 512-message send channel (`chan []byte`). A dedicated writer goroutine drains this channel and writes to the WebSocket. If the channel fills up (client not consuming fast enough), the server logs a warning and drops the frame:

```
send buffer full — dropping frame
```

This prevents a slow client from blocking the server's `ServeHTTP` goroutines.

---

## Next Steps

- [Architecture Overview](/architecture/overview)
- [WebSocket Protocol](/api/websocket-protocol)
- [Admin API](/api/admin-api)
