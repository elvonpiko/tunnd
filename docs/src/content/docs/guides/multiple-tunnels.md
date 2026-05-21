---
title: Multiple Tunnels
description: 'Run several tunnels simultaneously — each gets its own subdomain and public URL.'
---
Run several tunnels simultaneously — each gets its own subdomain and public URL.

## How to run multiple tunnels

Open one terminal per tunnel:

```bash
# Terminal 1 — frontend on port 3000
tunnd http 3000 --subdomain frontend

# Terminal 2 — backend API on port 8080
tunnd http 8080 --subdomain api --inspector-port 4041

# Terminal 3 — database (raw TCP)
tunnd tcp 5432 --subdomain db
```

Each prints its own public URL:
```
  Forwarding    https://frontend.tunnd.yourdomain.com → localhost:3000
  Forwarding    https://api.tunnd.yourdomain.com → localhost:8080
  Forwarding    (TCP) db.tunnd.yourdomain.com → localhost:5432
```

::: tip Inspector ports
Each HTTP tunnel's inspector needs a unique port. Use `--inspector-port 4041`, `4042`, etc., or `--inspector-port 0` to disable for tunnels you don't need to inspect.
:::

---

## Background tunnels

### Using `nohup`

```bash
nohup tunnd http 3000 --subdomain frontend > ~/tunnd-frontend.log 2>&1 &
nohup tunnd http 8080 --subdomain api --inspector-port 0 > ~/tunnd-api.log 2>&1 &
```

### Using systemd

`/etc/systemd/system/tunnd-frontend.service`:

```ini
[Unit]
Description=Tunnd — frontend tunnel
After=network.target

[Service]
Type=simple
User=youruser
ExecStart=/usr/local/bin/tunnd http 3000 --subdomain frontend --inspector-port 0
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable --now tunnd-frontend
journalctl -u tunnd-frontend -f
```

---

## Server-side token limits

Limit concurrent tunnels per token:

```bash
# Admin dashboard: Tokens → + New Token → set Max Tunnels
# Or via CLI:
tunnd-server token create my-laptop --max-tunnels 5
```

`0` = unlimited (default).

---

## Next steps

- [Custom Subdomains](/guides/custom-subdomains)
- [CLI Reference](/configuration/cli-reference)
- [Troubleshooting](/guides/troubleshooting)
