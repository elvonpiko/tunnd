---
title: Security
description: 'Set a strong admin password (minimum 12 characters). On first run, you set it via the browser — no config file entry needed.'
---
## Admin password

Set a strong admin password (minimum 12 characters). On first run, you set it via the browser — no config file entry needed.

Generate a strong password:
```bash
openssl rand -hex 16
```

If you set it in the config file or via environment variable:
```yaml
# tunnd-server.yaml
admin_password: "your-strong-password-here"
```
```bash
TUNND_ADMIN_PASSWORD=your-strong-password tunnd-server
```

The server warns on startup if the password is short or matches a known weak default.

---

## Restrict admin dashboard access

Port `9091` should **not** be publicly accessible. Options:

**Firewall (ufw):**
```bash
sudo ufw deny 9091
sudo ufw allow from YOUR_IP to any port 9091
```

**SSH tunnel** (no firewall changes needed):
```bash
ssh -L 9091:localhost:9091 user@your-server
# then open http://localhost:9091 in your browser
```

**Behind Caddy:** The admin dashboard is routed through Caddy on port 443 (`tunnd.yourdomain.com`) — port 9091 never needs to be open publicly.

---

## Auth tokens

- Create a separate token per device: `tunnd-server token create my-laptop`
- Use `--max-tunnels` to limit concurrent tunnels per token
- Revoke tokens immediately when a device is lost: `tunnd-server token revoke <id>`
- Token values are shown only at creation time — treat them like passwords

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

Monitor these to detect brute-force attempts:
```bash
docker logs tunnd 2>&1 | grep "login failure"
# or
journalctl -u tunnd | grep "login failure"
```

---

## Checklist

- [ ] Admin password is at least 12 characters
- [ ] Port 9091 is firewalled or accessed via SSH tunnel / Caddy only
- [ ] Auth tokens are unique per device with appropriate limits
- [ ] Tunnels use `wss://` (TLS) in production
- [ ] Server is behind a reverse proxy (Caddy) or has direct TLS configured
