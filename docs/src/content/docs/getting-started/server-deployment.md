---
title: Server Deployment
description: 'Deploy the Tunnd server on your VPS. Three options — pick the one that fits your setup.'
---
Deploy the Tunnd server on your VPS. Three options — pick the one that fits your setup.

## Prerequisites

- A VPS with a public IP (1 vCPU, 512 MB RAM is enough)
- A domain with DNS access
- Ports 80, 443 open in your firewall

## DNS setup

Add a wildcard A record at your DNS provider:

```
Type:  A
Name:  *.tunnd          (i.e. *.tunnd.yourdomain.com)
Value: <your-server-ip>
TTL:   300
```

Also add a non-wildcard record for the base domain itself (needed for the admin dashboard):

```
Type:  A
Name:  tunnd            (i.e. tunnd.yourdomain.com)
Value: <your-server-ip>
TTL:   300
```

Verify propagation before continuing:
```bash
dig +short tunnd.yourdomain.com
dig +short myapp.tunnd.yourdomain.com
# both should return your server IP
```

---

## Option A — Bare metal / systemd (recommended for simplicity)

The setup script installs the binary, creates a system user, writes a config file, registers a systemd service, and starts the server.

### Let's Encrypt (automatic TLS)

Port 80 must be publicly reachable for the ACME HTTP-01 challenge.

```bash
curl -sSL https://raw.githubusercontent.com/elvonpiko/tunnd/main/setup.sh | \
  DOMAIN=tunnd.yourdomain.com EMAIL=you@example.com bash
```

### Manual TLS certificate (wildcard cert)

Use this if you already have a wildcard certificate (e.g. from Certbot DNS-01):

```bash
curl -sSL https://raw.githubusercontent.com/elvonpiko/tunnd/main/setup.sh | \
  DOMAIN=tunnd.yourdomain.com \
  TLS_CERT=/etc/letsencrypt/live/tunnd.yourdomain.com/fullchain.pem \
  TLS_KEY=/etc/letsencrypt/live/tunnd.yourdomain.com/privkey.pem bash
```

### After setup

```bash
systemctl status tunnd          # check it's running
journalctl -u tunnd -f          # follow logs
```

The script prints your admin password. Visit `https://tunnd.yourdomain.com` to log in.

---

## Option B — Docker Compose

### Let's Encrypt (standalone, simplest)

```bash
export TUNND_DOMAIN=tunnd.yourdomain.com
export TUNND_TLS_EMAIL=you@example.com
docker compose up -d
```

Port 80 and 443 are exposed. Tunnd handles TLS itself.

### Manual TLS certificate

```bash
export TUNND_DOMAIN=tunnd.yourdomain.com
export TLS_CERTS_DIR=/etc/letsencrypt/live/tunnd.yourdomain.com
docker compose -f docker-compose.manual-tls.yml up -d
```

### Behind Caddy (existing reverse proxy)

If you already run Caddy with manual certs (most flexible setup):

```bash
export TUNND_DOMAIN=tunnd.yourdomain.com
docker compose -f docker-compose.caddy.yml up -d
```

See the [Caddy guide](/deployment/reverse-proxy/caddy) for the Caddyfile configuration.

---

## Option C — Manual config

For fine-grained control, write your own config file and run the binary directly.

**1. Download the binary:**
```bash
VERSION=$(curl -fsSL https://api.github.com/repos/elvonpiko/tunnd/releases/latest \
  | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
curl -fsSL "https://github.com/elvonpiko/tunnd/releases/download/${VERSION}/tunnd-server_${VERSION#v}_linux_amd64.tar.gz" \
  | tar -xz
sudo mv tunnd-server /usr/local/bin/
```

**2. Create `/etc/tunnd/tunnd-server.yaml`:**

```yaml
domain: "tunnd.yourdomain.com"
http_port: 443
admin_port: 9091
db_path: "/var/lib/tunnd/tunnd.db"
log_level: "info"

# Choose ONE TLS option:

# Option A — Let's Encrypt (port 80 must be open)
tls_email: "you@example.com"
acme_cache_dir: "/var/lib/tunnd/.autocert-cache"

# Option B — Manual certificate
# tls_cert_file: "/etc/tunnd/certs/fullchain.pem"
# tls_key_file:  "/etc/tunnd/certs/privkey.pem"

# Option C — No TLS (local dev only, use http_port != 443)
# http_port: 8080
```

**3. Run:**
```bash
sudo -u tunnd tunnd-server --config /etc/tunnd/tunnd-server.yaml
```

---

## First login (admin setup)

Visit your admin dashboard — `https://tunnd.yourdomain.com` or `http://<server-ip>:9091`.

**First time:** you'll see a setup page to create your admin password. Set it once — it's stored in the database, no config file entry needed.

After that, you'll see the standard login page on every visit.

---

## Create your first client token

From the dashboard (Tokens tab → **+ New Token**), or via CLI:

```bash
# systemd
tunnd-server token create my-laptop

# Docker
docker compose exec tunnd tunnd-server token create my-laptop
```

Copy the token value — it's shown only once.

---

## Common operations

```bash
# Systemd
systemctl restart tunnd
journalctl -u tunnd -f

# Docker
docker compose ps
docker compose logs -f tunnd
docker compose restart tunnd

# Create a token
tunnd-server token create my-laptop
tunnd-server token list
tunnd-server token revoke <id>
```

---

## Troubleshooting startup

```bash
# Check logs for the error
journalctl -u tunnd -n 50 --no-pager

# Common causes:
# "domain is required"       → set domain: in config or TUNND_DOMAIN env var
# "port already in use"      → another process has port 443/80
# "TLS is required on 443"   → add tls_email or tls_cert_file to config
# "permission denied /data"  → fix ownership: chown -R tunnd:tunnd /var/lib/tunnd
```

---

## Next steps

- [Client Installation](/getting-started/client-installation) — install and connect the client
- [Caddy Reverse Proxy](/deployment/reverse-proxy/caddy) — run behind Caddy
- [TLS Certificates](/deployment/tls-certificates) — certificate options in depth
- [Server Config Reference](/configuration/server-config) — all config fields
