---
title: Security
description: 'Strong bcrypt-hashed admin passwords, loopback-bound admin port, per-IP rate limiting, and token revocation that kills active tunnels.'
---
## Admin password

Set a strong admin password (minimum 12 characters). On first run, you set it via the browser — no config file entry needed. The password is stored as a **bcrypt hash** in the database; the plaintext never touches disk.

Generate a strong password:
```bash
openssl rand -hex 16
```

Change it anytime from the dashboard: **Settings → Change admin password**. Changing it signs out every other admin session automatically.

If you set it in the config file or via environment variable (legacy, still supported):
```yaml
# tunnd-server.yaml
admin_password: "your-strong-password-here"
```
```bash
TUNND_ADMIN_PASSWORD=your-strong-password tunnd-server
```

While `admin_password` is set via config/env, the dashboard change-password flow is disabled (it would be overwritten on the next restart) — change the value in the config instead, or remove it and use the dashboard flow. The server warns on startup if the password is short or matches a known weak default.

---

## Restrict admin dashboard access

The admin port `9091` binds to **`127.0.0.1` by default** — it is not reachable from the network at all unless you opt in. The dashboard is still reachable two ways:

- **HTTPS on the base domain** — `https://tunnd.yourdomain.com` routes to the dashboard on the public port (443), with TLS.
- **SSH tunnel** (no firewall changes needed):
  ```bash
  ssh -L 9091:localhost:9091 user@your-server
  # then open http://localhost:9091 in your browser
  ```

If you need LAN/plain-HTTP access on the admin port, opt in explicitly:
```yaml
admin_bind: "0.0.0.0"   # listen on all interfaces (make sure to firewall it)
```
```bash
TUNND_ADMIN_BIND=0.0.0.0 tunnd-server
```

**Firewall (ufw)**, if you opted in:
```bash
sudo ufw deny 9091
sudo ufw allow from YOUR_IP to any port 9091
```

**Behind Caddy:** The admin dashboard is routed through Caddy on port 443 (`tunnd.yourdomain.com`) — port 9091 never needs to be open publicly.

---

## Auth tokens

- Create a separate token per device: `tunnd-server token create my-laptop`
- Use `--max-tunnels` to limit concurrent tunnels per token
- Revoke tokens immediately when a device is lost: `tunnd-server token revoke <id>` — revocation **disconnects that token's active tunnels right away** (their clients see close code 4429 and exit). Revocations made from the CLI reach running sessions within one minute via periodic revalidation.
- Token values are shown only at creation time — treat them like passwords

---

## Rate limiting

Brute force is throttled at both entry points, keyed on the client's TCP address (never spoofable `X-Forwarded-For` values):

| Surface | Budget | Behavior |
|---|---|---|
| Admin login | 5 failed attempts / IP / 15 min | Over-budget attempts get `429 Too Many Requests`; successful logins reset the budget |
| Control-plane handshake | 10 failed registrations / IP / min | Over-budget connections refused with `429` before the WebSocket upgrade; successful registrations reset the budget |

Behind a reverse proxy, all clients share one bucket per proxy IP — acceptable for a single-operator tool; keep the admin port loopback-bound and use the base-domain HTTPS route instead.

---

## TLS

- Always use `wss://` (not `ws://`) for production server addresses
- Use a wildcard certificate for best coverage
- Keep TLS 1.2 minimum (Tunnd enforces this when handling TLS directly)

---

## Security headers

The admin dashboard sets these headers on every response automatically:

| Header | Value |
|--------|-------|
| `X-Frame-Options` | `DENY` |
| `X-Content-Type-Options` | `nosniff` |
| `Content-Security-Policy` | `default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'` |
| `X-XSS-Protection` | `1; mode=block` |
| `Referrer-Policy` | `strict-origin-when-cross-origin` |

---

## Authentication logging

Every failed login attempt is logged with timestamp and source IP:

```json
{"level":"warn","source_ip":"203.0.113.10","path":"/login","message":"admin login failure"}
```

Note that repeated failures are also cut off by the built-in rate limit — seeing `429`s in the logs means the limiter is doing its job.

Monitor these to detect brute-force attempts:
```bash
docker logs tunnd 2>&1 | grep "login failure"
# or
journalctl -u tunnd | grep "login failure"
```

---

## Checklist

- [ ] Admin password is at least 12 characters (bcrypt-hashed, changed via Settings)
- [ ] Admin port stays loopback-bound (`admin_bind` unset) — dashboard via base-domain HTTPS or SSH tunnel
- [ ] Auth tokens are unique per device with appropriate limits; revocation kills active tunnels
- [ ] Tunnels use `wss://` (TLS) in production
- [ ] Server is behind a reverse proxy (Caddy) or has direct TLS configured
