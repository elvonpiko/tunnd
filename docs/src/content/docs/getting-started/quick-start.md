---
title: Quick Start
description: 'Get Tunnd running in minutes. Deploy the server, create a token, expose any local service to the internet.'
---
Get Tunnd running in minutes. Deploy the server on a VPS you own, create a token in the dashboard, expose any local service to the internet.

Tunnd works out of the box with anything you'd actually tunnel — Vite, Next.js, webpack, Bun, Deno, Express, FastAPI, Rails, plain `python -m http.server`, raw TCP. No framework configuration, no `allowedHosts` edits, no flags for the common case.

## What you need

- A VPS with a public IP
- A domain you control with DNS access (wildcard record below)
- Linux (Ubuntu 20.04+ / Debian 11+) with `sudo`

---

## Step 1 — Deploy the server

Add a wildcard DNS record at your DNS provider:

```
*.tunnd.yourdomain.com  →  <your-vps-ip>
```

Then SSH into your VPS and run:

```bash
curl -fsSL https://raw.githubusercontent.com/elvonpiko/tunnd/main/setup.sh | sudo bash
```

The script asks for your domain and email, requests a Let's Encrypt cert, installs the binary, and starts a systemd service. For Docker, manual TLS, or running behind Caddy / Nginx, see [Server Deployment](/getting-started/server-deployment).

---

## Step 2 — Set your admin password (first run)

Visit `https://tunnd.yourdomain.com` in a browser. The first time you load it, you'll see a one-time **Setup** page — pick a strong password (12+ characters) and submit. After that, normal login takes over.

---

## Step 3 — Create a client token

1. Log in to the dashboard
2. Go to **Tokens** tab
3. Click **+ New Token**, give it a label (e.g. `my-laptop`)
4. Copy the token value — it's shown only once

---

## Step 4 — Install and set up the client

**macOS / Linux:**

```bash
curl -fsSL https://raw.githubusercontent.com/elvonpiko/tunnd/main/install.sh | bash
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/elvonpiko/tunnd/main/install.ps1 | iex
```

Then run the interactive setup:

```bash
tunnd setup
```

```
  ▲  Tunnd Setup

  Server address (e.g. wss://tunnd.example.com): wss://tunnd.yourdomain.com
  Checking server… ✓

  Auth token (tnnd_...): tnnd_xxxxxxxxxxxxxxxxxxxx

  ✓ All set!

  Open a tunnel:
    tunnd http 3000
    tunnd http 8080 --subdomain myapp
```

---

## Step 5 — Open your first tunnel

Start a local service (e.g. `python3 -m http.server 3000`) then:

```bash
tunnd http 3000
```

```
  ▲  Tunnd

  Forwarding    https://brave-river.tunnd.yourdomain.com → localhost:3000
  Inspector     http://localhost:4040

  Ctrl+C to close tunnel
```

Your local service is now reachable at the public URL. Open the inspector at `http://localhost:4040` to see live request logs.

---

## Common options

```bash
# Pin a subdomain
tunnd http 3000 --subdomain myapp

# Tunnel a raw TCP port (e.g. a database)
tunnd tcp 5432

# Show current config
tunnd status

# Re-run setup (e.g. after getting a new token)
tunnd setup
```

---

## What you get out of the box

Tunnd transparently forwards anything that runs over HTTP/1.1 or raw TCP:

| Scenario | Command | Public URL |
|----------|---------|------------|
| HTTP app (Express, Vite, Django…) | `tunnd http 3000` | `https://random-name.tunnd.example.com` |
| WebSocket app (chat, HMR, dashboards) | `tunnd http 3000` | Public URL works for both `https://` and `wss://` |
| Pinned subdomain | `tunnd http 3000 -s myapp` | `https://myapp.tunnd.example.com` |
| Raw TCP service (Postgres, Redis, SSH…) | `tunnd tcp 5432` | `tcp://tunnd.example.com:20000` |

The server picks a random subdomain on each launch unless you pin one with `--subdomain`. TCP tunnels get a public port allocated from the configured range (default `20000–20100`). Streaming HTTP responses (chunked transfer, Server-Sent Events) are forwarded with flushes preserved end-to-end — no extra config.

### Quick recipes

**Share a Vite/Next.js dev server with a teammate:**
```bash
tunnd http 3000
```

**Receive a webhook (Stripe, GitHub, etc.) on your laptop:**
```bash
tunnd http 8080 --subdomain my-webhooks
# Configure the webhook to POST https://my-webhooks.tunnd.example.com/...
```

**Let a friend connect to your local Postgres:**
```bash
tunnd tcp 5432
# Forwarding tcp://tunnd.example.com:20000 → localhost:5432 (TCP)
# psql "postgres://user:pass@tunnd.example.com:20000/dbname"
```

**Expose your local SSH for a quick remote session:**
```bash
tunnd tcp 22
# ssh -p 20000 user@tunnd.example.com
```

---

## Next steps

- [Server Deployment](/getting-started/server-deployment) — full VPS + Caddy setup walkthrough
- [Custom Subdomains](/guides/custom-subdomains) — subdomain rules and tips
- [Multiple Tunnels](/guides/multiple-tunnels) — running several tunnels at once
- [Troubleshooting](/guides/troubleshooting) — common issues and fixes
