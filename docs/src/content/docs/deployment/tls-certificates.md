---
title: TLS Certificates
description: 'Tunnd supports three TLS modes. Choose the one that fits your deployment.'
---
Tunnd supports three TLS modes. Choose the one that fits your deployment.

---

## Mode A — Let's Encrypt (automatic)

Tunnd handles certificate issuance and renewal automatically using the ACME HTTP-01 challenge.

**Requirements:**
- Port 80 must be publicly reachable from the internet
- Set `http_port: 443` in config

**Config:**
```yaml
tls_email: "you@example.com"
acme_cache_dir: "/data/.autocert-cache"   # or /var/lib/tunnd/.autocert-cache
```

**How it works:**

On the first request to each tunnel subdomain, Tunnd requests a certificate from Let's Encrypt (~2 seconds delay on first request, cached after that). The cache directory must be on a persistent volume so certificates survive restarts.

::: warning
HTTP-01 cannot issue wildcard certificates. Each subdomain gets its own cert. If you want a single wildcard cert, use Mode B.
:::

---

## Mode B — Manual certificate (wildcard)

You provide your own TLS certificate. This is the best option if you already have a wildcard cert, use a reverse proxy, or can't expose port 80.

**Config:**
```yaml
tls_cert_file: "/etc/tunnd/certs/fullchain.pem"
tls_key_file:  "/etc/tunnd/certs/privkey.pem"
```

**Getting a wildcard cert with Certbot (DNS-01):**

```bash
# Replace --dns-cloudflare with your provider's plugin
certbot certonly \
  --dns-cloudflare \
  --dns-cloudflare-credentials ~/.cloudflare.ini \
  -d "tunnd.yourdomain.com" \
  -d "*.tunnd.yourdomain.com"
```

See [Certbot DNS plugins](https://certbot.eff.org/docs/using.html#dns-plugins) for your registrar.

**Docker — mount your certs:**
```bash
export TLS_CERTS_DIR=/etc/letsencrypt/live/tunnd.yourdomain.com
docker compose -f docker-compose.manual-tls.yml up -d
```

**Certificate renewal:**

Manual certs aren't auto-renewed by Tunnd. Add a Certbot deploy hook to reload after renewal:

```bash
# /etc/letsencrypt/renewal-hooks/deploy/tunnd-reload.sh
#!/bin/bash
systemctl reload tunnd
# Docker: docker restart tunnd
```

```bash
chmod +x /etc/letsencrypt/renewal-hooks/deploy/tunnd-reload.sh
```

---

## Mode C — No TLS (local dev only)

Set `http_port` to anything other than `443` and omit both TLS options. The server runs plain HTTP — use `ws://` instead of `wss://` for the client.

```yaml
domain: "localhost"
http_port: 8080
admin_port: 9091
```

Client:
```bash
# In ~/.config/tunnd/config.json
{ "server_addr": "ws://localhost:8080", "token": "..." }

# Or during setup:
# Server address: ws://localhost:8080
```

::: danger
Never run without TLS on a public server. Tunnel traffic and tokens would be sent in plaintext.
:::

---

## Behind a reverse proxy (Caddy)

When running behind Caddy (or any reverse proxy), TLS is terminated at the proxy. Set Tunnd to listen on internal ports without TLS:

```yaml
http_port: 9095    # Caddy proxies public :443 → internal :9095
admin_port: 9096
# No tls_email, no tls_cert_file — Caddy handles TLS
```

Caddy uses your wildcard certs:
```caddy
tunnd.yourdomain.com {
    tls /etc/caddy/certs/fullchain.pem /etc/caddy/certs/privkey.pem
    handle /_tunnd/control { reverse_proxy tunnd-server:9095 }
    handle { reverse_proxy tunnd-server:9096 }
}

*.tunnd.yourdomain.com {
    tls /etc/caddy/certs/fullchain.pem /etc/caddy/certs/privkey.pem
    reverse_proxy tunnd-server:9095
}
```

See [Caddy Reverse Proxy](/deployment/reverse-proxy/caddy) for the full setup.

---

## Choosing the right mode

| Situation | Recommended mode |
|-----------|-----------------|
| Fresh VPS, simplest setup | **A — Let's Encrypt** (`docker compose up`) |
| Already have a wildcard cert | **B — Manual cert** |
| Running behind Caddy/Nginx | **B — Manual cert** (at the proxy level) |
| Local development | **C — No TLS** |
