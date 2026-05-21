---
title: WebSocket Protocol Reference
description: 'Tunnd uses a binary WebSocket connection as the **control plane** between the server and each connected client. This connection carries tunnel registration, str'
---
Tunnd uses a binary WebSocket connection as the **control plane** between the server and each connected client. This connection carries tunnel registration, stream lifecycle signalling, and proxied HTTP request/response data.

## Connection Endpoint

```
wss://<your-domain>/_tunnd/control
```

The path `/_tunnd/control` is reserved on the public port (default: 443). The client connects here immediately on startup and holds the connection open for the lifetime of the tunnel session.

## Transport Details

| Property | Value |
|---|---|
| Protocol | WebSocket (RFC 6455) |
| Message format | Binary frames containing JSON |
| Max frame size | 4 MiB |
| Read buffer | 64 KiB |
| Write buffer | 64 KiB |
| Keepalive | Server sends WebSocket ping every ~54 seconds |
| Pong deadline | Client must respond within 60 seconds |
| Handshake timeout | 30 seconds to send the first `register` message |

---

## Message Format

Every message is a **JSON envelope** with a `type` discriminator and an optional `payload`:

```json
{
  "type": "<message-type>",
  "payload": { ... }
}
```

When there is no payload (e.g., `ping`, `pong`), the `payload` field is omitted.

Messages are serialized to JSON and sent as binary WebSocket frames.

---

## Message Types

### MsgRegister — Client → Server

Sent by the client immediately after connecting. This is the only message the server accepts before a tunnel is established.

```json
{
  "type": "register",
  "payload": {
    "token": "tnnd_a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6",
    "subdomain": "my-app",
    "protocol": "http",
    "local_port": 3000
  }
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `token` | string | Yes | Auth token issued by the server admin |
| `subdomain` | string | No | Requested subdomain. If empty, the server assigns a random one |
| `protocol` | string | Yes | `"http"` or `"tcp"`. Defaults to `"http"` if invalid |
| `local_port` | integer | Yes | The local port on the client side being forwarded |

---

### MsgRegistered — Server → Client

Sent by the server after a successful `register`. Confirms the tunnel is live.

```json
{
  "type": "registered",
  "payload": {
    "subdomain": "my-app",
    "public_url": "https://my-app.tunnel.example.com",
    "tunnel_id": "uuid-string"
  }
}
```

| Field | Type | Description |
|---|---|---|
| `subdomain` | string | The registered subdomain (may differ from requested if sanitized) |
| `public_url` | string | Full public HTTPS URL for the tunnel |
| `tunnel_id` | string | Persistent tunnel record ID (for admin inspection) |

---

### MsgError — Server → Client

Sent when registration fails or a fatal error occurs. The connection is closed after this message.

```json
{
  "type": "error",
  "payload": {
    "code": "subdomain_in_use",
    "message": "subdomain 'my-app' is already in use"
  }
}
```

| Field | Type | Description |
|---|---|---|
| `code` | string | Machine-readable error code (see [Error Codes](#error-codes)) |
| `message` | string | Human-readable description |

---

### MsgOpen — Server → Client

Sent when an inbound HTTP request arrives for the tunnel. Instructs the client to open a local connection for this stream.

```json
{
  "type": "open",
  "payload": {
    "stream_id": "uuid-string"
  }
}
```

| Field | Type | Description |
|---|---|---|
| `stream_id` | string | Unique ID for this request/response stream |

After receiving `open`, the client should dial `localhost:<local_port>` and associate that connection with the given `stream_id`. The client will then receive `data` frames containing the raw HTTP request bytes.

---

### MsgData — Bidirectional

Carries raw bytes for a stream in both directions.

```json
{
  "type": "data",
  "payload": {
    "stream_id": "uuid-string",
    "data": "<base64-encoded bytes>"
  }
}
```

| Field | Type | Description |
|---|---|---|
| `stream_id` | string | Identifies which stream these bytes belong to |
| `data` | string | Base64-encoded raw bytes |

**Server → Client**: Raw HTTP request bytes (headers + body). The client writes these to the local connection.

**Client → Server**: Raw HTTP response bytes (status line + headers + body). The server reads these and forwards to the browser.

---

### MsgReqDone — Server → Client

Signals that the server has finished sending all HTTP request bytes for a stream. The client should stop reading request data and start reading the local response.

```json
{
  "type": "req_done",
  "payload": {
    "stream_id": "uuid-string"
  }
}
```

| Field | Type | Description |
|---|---|---|
| `stream_id` | string | Identifies the completed request stream |

---

### MsgClose — Client → Server

Sent by the client when it has finished sending the HTTP response for a stream. This signals the server to unblock its `http.ReadResponse` call and finalize the response.

```json
{
  "type": "close",
  "payload": {
    "stream_id": "uuid-string"
  }
}
```

| Field | Type | Description |
|---|---|---|
| `stream_id` | string | Identifies the stream being closed |

---

### MsgPing — Client → Server

Sent by the client to check liveness. The server responds immediately with `pong`.

```json
{
  "type": "ping"
}
```

No payload.

---

### MsgPong — Server → Client

Sent in response to a client `ping`.

```json
{
  "type": "pong"
}
```

No payload.

> **Note**: The server also sends WebSocket-level ping frames (not `MsgPing` messages) on a ~54 second timer. These are handled at the WebSocket protocol layer, not the application layer.

---

## Registration Protocol

The handshake sequence on connection:

```
Client                                  Server
  |                                       |
  |--- WebSocket Upgrade ---------------→ |
  |←-- 101 Switching Protocols ---------- |
  |                                       |
  |--- register (token, subdomain) -----→ |  (within 30s)
  |                                       |  validates token
  |                                       |  registers subdomain
  |←-- registered (subdomain, url) ------ |
  |                                       |
  |         [tunnel is live]              |
```

If registration fails, the server sends `error` and closes the connection:

```
Client                                  Server
  |                                       |
  |--- register (bad token) -----------→  |
  |←-- error (code: handshake_failed) --- |
  |←-- [connection closed] -------------- |
```

---

## Stream Lifecycle

Each inbound HTTP request creates an independent stream:

```
Browser          Server                   Client          Localhost
  |                |                        |                |
  |-- GET /path -> |                        |                |
  |                |-- open(stream_id) --> |                |
  |                |-- data(req bytes) --→ |                |
  |                |-- req_done ----------→ |                |
  |                |                       |-- connect --→  |
  |                |                       |-- write req --> |
  |                |                       |←- read resp --- |
  |                |←- data(resp bytes) -- |                |
  |                |←- close ------------- |                |
  |←- HTTP resp -- |                       |                |
```

Multiple streams can be in-flight simultaneously on the same WebSocket connection. Each stream is independent and identified by its `stream_id`.

---

## Error Codes

| Code | Trigger | Description |
|---|---|---|
| `subdomain_in_use` | `register` | The requested subdomain is already claimed by an active session |
| `invalid_subdomain` | `register` | The subdomain failed validation (see rules below) |
| `handshake_failed` | `register` | Authentication failed or protocol error (bad token, malformed message, wrong message type) |

### Subdomain Validation Rules

A custom subdomain is rejected with `invalid_subdomain` if any of these conditions are true:

| Condition | Error message |
|---|---|
| Empty after sanitization | `subdomain cannot be empty` |
| Contains characters other than `a-z`, `0-9`, `-` | `subdomain contains invalid characters: only a-z, 0-9, and - are allowed` |
| Starts with a hyphen | `subdomain cannot start with a hyphen` |
| Ends with a hyphen | `subdomain cannot end with a hyphen` |
| Contains consecutive hyphens (`--`) | `subdomain cannot contain consecutive hyphens` |
| Length < 3 or > 63 characters | `subdomain must be between 3 and 63 characters` |
| Matches a reserved name (`www`, `api`, `admin`, `mail`, `ftp`) | `subdomain '{name}' is reserved` |

**Sanitization**: Before validation, the server trims leading/trailing whitespace and converts to lowercase.

---

## Random Subdomains

When no subdomain is requested (or `subdomain` is omitted from the `register` payload), the server generates a random subdomain using the pattern `{adjective}-{noun}`, for example:

- `happy-river`
- `brave-mountain`
- `calm-ocean`

The server guarantees uniqueness by retrying generation if there is a collision.

---

## Complete Example

### 1. Connecting and registering

```python
import websocket
import json
import base64

ws = websocket.WebSocket()
ws.connect("wss://tunnel.example.com/_tunnd/control")

# Send register message
register_msg = {
    "type": "register",
    "payload": {
        "token": "tnnd_a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6",
        "subdomain": "my-app",
        "protocol": "http",
        "local_port": 3000
    }
}
ws.send_binary(json.dumps(register_msg).encode())

# Wait for registered confirmation
raw = ws.recv()
env = json.loads(raw)
print(env["type"])     # "registered"
print(env["payload"]["public_url"])  # "https://my-app.tunnel.example.com"
```

### 2. Handling an incoming request

```python
while True:
    raw = ws.recv()
    env = json.loads(raw)

    if env["type"] == "open":
        stream_id = env["payload"]["stream_id"]
        # open a local connection for this stream_id

    elif env["type"] == "data":
        stream_id = env["payload"]["stream_id"]
        data = base64.b64decode(env["payload"]["data"])
        # write data to the local connection for stream_id

    elif env["type"] == "req_done":
        stream_id = env["payload"]["stream_id"]
        # stop reading request data, start reading response from localhost

    # ... send MsgData frames back with response bytes, then MsgClose
```

---

## Next Steps

- [Admin API](/api/admin-api)
- [Architecture Overview](/architecture/overview)
- [Data Flow](/architecture/data-flow)
