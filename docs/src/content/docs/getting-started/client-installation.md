---
title: Client Installation
description: 'Install the `tunnd` client on your machine, then run `tunnd setup` to connect to your server.'
---
Install the `tunnd` client on your machine, then run `tunnd setup` to connect to your server.

## Install

### macOS / Linux

```bash
curl -fsSL https://raw.githubusercontent.com/elvonpiko/tunnd/main/install.sh | bash
```

Installs to `/usr/local/bin/tunnd`. To install elsewhere:

```bash
TUNND_INSTALL_DIR=~/.local/bin curl -fsSL https://raw.githubusercontent.com/elvonpiko/tunnd/main/install.sh | bash
```

### Windows (PowerShell)

```powershell
iwr -useb https://raw.githubusercontent.com/elvonpiko/tunnd/main/install.ps1 | iex
```

Installs to `%LOCALAPPDATA%\Programs\tunnd\` and adds it to your user `PATH`. Open a new shell after installing.

### Manual download

Download the binary for your platform from [Releases](https://github.com/elvonpiko/tunnd/releases/latest):

| Platform | File |
|----------|------|
| macOS Apple Silicon | `tunnd_*_darwin_arm64.tar.gz` |
| macOS Intel | `tunnd_*_darwin_amd64.tar.gz` |
| Linux x86-64 | `tunnd_*_linux_amd64.tar.gz` |
| Linux ARM64 | `tunnd_*_linux_arm64.tar.gz` |
| Windows x86-64 | `tunnd_*_windows_amd64.zip` |

---

## Verify

```bash
tunnd version
# tunnd dev (unknown) built unknown
```

---

## First-time setup

Run the interactive setup wizard once after installing:

```bash
tunnd setup
```

It asks for:
1. **Server address** — the `wss://` URL of your Tunnd server. The wizard verifies the server is reachable before continuing.
2. **Auth token** — create one in your admin dashboard (Tokens tab → + New Token).

Your settings are saved to `~/.config/tunnd/config.json`.

```
  ▲  Tunnd Setup

  Server address (e.g. wss://tunnd.example.com): wss://tunnd.yourdomain.com
  Checking server… ✓

  Create a token in your admin dashboard (Tokens tab → + New Token),
  then paste it here.

  Auth token (tnnd_...): tnnd_xxxxxxxxxxxxxxxxxxxx

  ✓ All set!
```

---

## Commands

```bash
tunnd setup               # configure server + token (run once)
tunnd status              # show current config
tunnd http <port>         # tunnel an HTTP service
tunnd tcp  <port>         # tunnel a raw TCP port
tunnd version             # print version
```

### `tunnd http` flags

| Flag | Default | Description |
|------|---------|-------------|
| `--subdomain`, `-s` | random | Pin a specific subdomain |
| `--inspector-port` | 4040 | Local inspector UI port (0 to disable) |

---

## Reconfigure

To change your server or token, just run `tunnd setup` again:

```bash
tunnd setup
# Current server: wss://tunnd.yourdomain.com
# Reconfigure? [y/N] y
```

---

## Next steps

- [Quick Start](/getting-started/quick-start)
- [Custom Subdomains](/guides/custom-subdomains)
- [CLI Reference](/configuration/cli-reference)
