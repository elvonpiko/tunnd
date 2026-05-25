---
title: Troubleshooting
description: 'Error: cannot reach server: dial tcp: connection refused'
---
## Client can't connect to server

```
Error: cannot reach server: dial tcp: connection refused
```

**Check:**
```bash
# Is the server running?
systemctl status tunnd
# or: docker compose ps

# Is the server reachable?
curl https://tunnd.yourdomain.com/api/stats
# Expected: 200 OK or 401 (server is up)

# Is port 443 open?
curl -v https://tunnd.yourdomain.com

# Current client config
tunnd status
```

**Common fixes:**

| Symptom | Fix |
|---------|-----|
| `connection refused` | Server not running — start it |
| `i/o timeout` | Firewall blocking port 443 — open it |
| `certificate verify failed` | TLS cert invalid or expired — see [TLS docs](/deployment/tls-certificates) |
| `401` from server | Server running — token may be wrong |

---

## Dev server says "Blocked request. This host is not allowed"

```
Blocked request. This host ("myapp.tunnd.yourdomain.com") is not allowed.
```

This means your dev server (Vite, Next.js, webpack-dev-server) is checking the `Host` header and rejecting the public hostname. **You shouldn't see this on a default tunnd install** — tunnd rewrites the Host to `localhost:<port>` before forwarding, so dev servers see what they expect.

If you do see it, you've likely set `--host-header=preserve` somewhere. Drop the flag (or set it to `rewrite`) and the request will go through.

---

## Token rejected

```
✗  invalid or revoked token
   Your token may be invalid or revoked.
   Run: tunnd setup   to reconfigure.
```

```bash
# List tokens on the server
tunnd-server token list
# or via Docker:
docker compose exec tunnd tunnd-server token list

# If revoked, create a new one and re-run setup
tunnd-server token create my-laptop
tunnd setup
```

---

## Not set up yet

```
Error: Tunnd is not set up yet.
  Run: tunnd setup
```

Run `tunnd setup` — it guides you through entering your server address and token.

---

## Subdomain already in use

```
✗  subdomain 'myapp' is already in use
   Try a different subdomain: tunnd http 3000 --subdomain myapp2
```

Another session has that subdomain. Wait for it to close, or use a different name.

---

## Inspector port conflict

```
failed to start inspector: bind: address already in use
```

Use a different port or disable the inspector:
```bash
tunnd http 3000 --inspector-port 4041
tunnd http 3000 --inspector-port 0    # disable
```

---

## Server won't start

```bash
# Check logs
journalctl -u tunnd -n 50 --no-pager
docker compose logs tunnd
```

| Error | Fix |
|-------|-----|
| `domain is required` | Set `domain:` in config or `TUNND_DOMAIN` env var |
| `TLS is required on port 443` | Add `tls_email:` or `tls_cert_file:` to config |
| `address already in use :443` | Another process has port 443 — `ss -tlnp \| grep 443` |
| `permission denied /data` | Fix ownership: `chown -R tunnd:tunnd /var/lib/tunnd` |

---

## Let's Encrypt not issuing

- Port 80 must be publicly reachable — test: `curl http://tunnd.yourdomain.com`
- DNS must resolve to your server IP — test: `dig +short tunnd.yourdomain.com`
- Rate limits: Let's Encrypt allows 5 failures per hour per domain

---

## Wildcard DNS not working

```
dig +short myapp.tunnd.yourdomain.com
# should return your server IP
```

- Verify the wildcard A record `*.tunnd.yourdomain.com` exists in your DNS provider
- Some providers require the wildcard as a separate record from the base domain
- Wait for DNS propagation (up to a few minutes with TTL 300)

---

## Admin dashboard inaccessible

```bash
# Is the admin port listening?
ss -tlnp | grep 9091

# Test locally on the server
curl http://localhost:9091/api/stats
```

If behind Caddy, the dashboard is at `https://tunnd.yourdomain.com` — port 9091 doesn't need to be publicly open.

---

## Enable debug logging

```bash
# Server
TUNND_LOG_LEVEL=debug tunnd-server
# or in config: log_level: "debug"

# Client (set in ~/.config/tunnd/config.json)
# "log_level": "debug"
```

---

## Quick diagnostic checklist

```bash
# 1. Server running?
systemctl status tunnd

# 2. Server healthy?
curl http://localhost:9091/api/stats

# 3. DNS resolving?
dig +short myapp.tunnd.yourdomain.com

# 4. TLS working?
curl -v https://tunnd.yourdomain.com 2>&1 | grep "SSL certificate"

# 5. Client configured?
tunnd status

# 6. Token valid?
tunnd-server token list
```
