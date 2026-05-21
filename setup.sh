#!/usr/bin/env bash
# Tunnd VPS Setup Script
# Sets up tunnd-server as a systemd service with TLS via Let's Encrypt.
#
# Interactive (recommended):
#   curl -fsSL https://raw.githubusercontent.com/elvonpiko/tunnd/main/setup.sh | sudo bash
#
# Non-interactive (CI / automation):
#   curl -fsSL https://raw.githubusercontent.com/elvonpiko/tunnd/main/setup.sh | sudo \
#     DOMAIN=tunnel.yourdomain.com EMAIL=you@example.com bash
#
# Or with a manual cert:
#   curl -fsSL https://raw.githubusercontent.com/elvonpiko/tunnd/main/setup.sh | sudo \
#     DOMAIN=tunnel.yourdomain.com \
#     TLS_CERT=/etc/ssl/certs/tunnel.pem \
#     TLS_KEY=/etc/ssl/private/tunnel.key bash
#
# Supported: Ubuntu 20.04+, Debian 11+

set -euo pipefail

# ── Colours ───────────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; RESET='\033[0m'

info()    { echo -e "${CYAN}${BOLD}[tunnd]${RESET} $*"; }
success() { echo -e "${GREEN}${BOLD}  ✔${RESET} $*"; }
warn()    { echo -e "${YELLOW}${BOLD}  ⚠${RESET} $*"; }
die()     { echo -e "${RED}${BOLD}  ✗ ERROR:${RESET} $*" >&2; exit 1; }

# ── Config from env ───────────────────────────────────────────────────────────
DOMAIN="${DOMAIN:-}"
EMAIL="${EMAIL:-}"
TLS_CERT="${TLS_CERT:-}"
TLS_KEY="${TLS_KEY:-}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-$(openssl rand -hex 16)}"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
DATA_DIR="${DATA_DIR:-/var/lib/tunnd}"
CONFIG_DIR="${CONFIG_DIR:-/etc/tunnd}"
REPO="elvonpiko/tunnd"

# ── Root check ────────────────────────────────────────────────────────────────
if [[ $EUID -ne 0 ]]; then
  die "This script must be run as root (or with sudo)."
fi

# ── Interactive prompts (only if running on a real TTY) ──────────────────────
# When piped (curl … | bash), stdin is the curl output, not a terminal — so
# we re-attach to /dev/tty for interactive prompts. If that fails (e.g.
# truly non-interactive CI), fall back to env vars only.
if [[ -z "$DOMAIN" || ( -z "$EMAIL" && -z "$TLS_CERT" ) ]]; then
  if [[ -e /dev/tty ]] && exec 3< /dev/tty 2>/dev/null; then
    echo
    echo -e "${BOLD}⬆  Tunnd Server Setup${RESET}"
    echo -e "   Press ${BOLD}Ctrl+C${RESET} at any prompt to cancel."
    echo

    if [[ -z "$DOMAIN" ]]; then
      while [[ -z "$DOMAIN" ]]; do
        printf "   %sDomain%s (e.g. tunnel.yourdomain.com): " "${BOLD}" "${RESET}"
        IFS= read -r DOMAIN <&3 || true
      done
    fi

    if [[ -z "$EMAIL" && -z "$TLS_CERT" ]]; then
      while [[ -z "$EMAIL" ]]; do
        printf "   %sEmail%s for Let's Encrypt (you@example.com): " "${BOLD}" "${RESET}"
        IFS= read -r EMAIL <&3 || true
      done
    fi
    exec 3<&-
    echo
  fi
fi

# ── Validate ──────────────────────────────────────────────────────────────────
[[ -z "$DOMAIN" ]] && die "DOMAIN is required. Example: DOMAIN=tunnel.yourdomain.com"

if [[ -z "$EMAIL" && -z "$TLS_CERT" ]]; then
  die "Either EMAIL (for Let's Encrypt) or TLS_CERT+TLS_KEY (manual cert) is required."
fi

if [[ -n "$TLS_CERT" && -z "$TLS_KEY" ]]; then
  die "TLS_KEY is required when TLS_CERT is set."
fi

echo
echo -e "${BOLD}⬆  Tunnd Server Setup${RESET}"
echo -e "   Domain:  ${CYAN}${DOMAIN}${RESET}"
if [[ -n "$EMAIL" ]]; then
  echo -e "   TLS:     Let's Encrypt (${EMAIL})"
else
  echo -e "   TLS:     Manual cert (${TLS_CERT})"
fi
echo

# ── Detect platform ───────────────────────────────────────────────────────────
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)        ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) die "Unsupported architecture: $ARCH" ;;
esac
[[ "$OS" != "linux" ]] && die "This script supports Linux only. For macOS, install manually."

# ── Install binary ────────────────────────────────────────────────────────────
info "Fetching latest release…"
LATEST=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
  | grep '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')
[[ -z "$LATEST" ]] && die "Could not determine latest version"

TARBALL="tunnd-server_${LATEST#v}_linux_${ARCH}.tar.gz"
BASE_URL="https://github.com/${REPO}/releases/download/${LATEST}"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

info "Downloading tunnd-server ${LATEST}…"
curl -fsSL "${BASE_URL}/${TARBALL}" -o "${TMP}/${TARBALL}"
curl -fsSL "${BASE_URL}/checksums.txt" -o "${TMP}/checksums.txt"

cd "$TMP"
if command -v sha256sum &>/dev/null; then
  grep "$TARBALL" checksums.txt | sha256sum --check --status && success "Checksum verified"
fi

tar -xzf "$TARBALL"
mv tunnd-server "${INSTALL_DIR}/tunnd-server"
chmod +x "${INSTALL_DIR}/tunnd-server"
success "Installed to ${INSTALL_DIR}/tunnd-server"

# ── Create directories and user ───────────────────────────────────────────────
info "Creating tunnd system user and directories…"
if ! id -u tunnd &>/dev/null; then
  useradd --system --no-create-home --shell /usr/sbin/nologin tunnd
fi
mkdir -p "$DATA_DIR" "$CONFIG_DIR" "${DATA_DIR}/.autocert-cache"
chown -R tunnd:tunnd "$DATA_DIR" "$CONFIG_DIR"
chmod 750 "$DATA_DIR" "$CONFIG_DIR"
success "Directories created"

# ── Write config ──────────────────────────────────────────────────────────────
info "Writing server config to ${CONFIG_DIR}/tunnd-server.yaml…"
cat > "${CONFIG_DIR}/tunnd-server.yaml" << YAML
# Tunnd Server Configuration
# Generated by setup.sh on $(date -u +"%Y-%m-%dT%H:%M:%SZ")

domain: "${DOMAIN}"
http_port: 443
admin_port: 9091

# ── TLS ──────────────────────────────────────────────────────────────────────
YAML

if [[ -n "$TLS_CERT" ]]; then
  # Manual cert mode
  cat >> "${CONFIG_DIR}/tunnd-server.yaml" << YAML
# Manual certificate (priority over Let's Encrypt)
tls_cert_file: "${TLS_CERT}"
tls_key_file:  "${TLS_KEY}"
YAML
else
  # Let's Encrypt mode
  cat >> "${CONFIG_DIR}/tunnd-server.yaml" << YAML
# Let's Encrypt — auto-issues and renews certs via HTTP-01 ACME challenge.
# Port 80 must be publicly reachable for this to work.
tls_email: "${EMAIL}"
acme_cache_dir: "${DATA_DIR}/.autocert-cache"
YAML
fi

cat >> "${CONFIG_DIR}/tunnd-server.yaml" << YAML

# ── Storage ───────────────────────────────────────────────────────────────────
db_path: "${DATA_DIR}/tunnd.db"

# ── Admin dashboard ───────────────────────────────────────────────────────────
# Access at http://<server-ip>:9091
# Username: admin   Password: see below
admin_password: "${ADMIN_PASSWORD}"

# ── Limits ────────────────────────────────────────────────────────────────────
max_tunnels_per_token: 0

# ── TCP tunneling ─────────────────────────────────────────────────────────────
# Range of public TCP ports allocated to \`tunnd tcp <port>\` clients.
# Adjust to your firewall config — these ports must be reachable from clients.
tcp_min_port: 20000
tcp_max_port: 20100

# ── Logging ───────────────────────────────────────────────────────────────────
log_level: "info"
log_format: "json"
YAML

chown tunnd:tunnd "${CONFIG_DIR}/tunnd-server.yaml"
chmod 640 "${CONFIG_DIR}/tunnd-server.yaml"
success "Config written"

# ── Firewall (ufw if present) ─────────────────────────────────────────────────
if command -v ufw &>/dev/null; then
  info "Configuring firewall rules (ufw)…"
  ufw allow 80/tcp   comment "Tunnd ACME challenge" 2>/dev/null || true
  ufw allow 443/tcp  comment "Tunnd tunnel traffic"  2>/dev/null || true
  ufw allow 9091/tcp comment "Tunnd admin (restrict to trusted IPs in production)" 2>/dev/null || true
  ufw allow 20000:20100/tcp comment "Tunnd TCP tunnels" 2>/dev/null || true
  success "Firewall rules added"
fi

# ── systemd service ───────────────────────────────────────────────────────────
info "Installing systemd service…"
cat > /etc/systemd/system/tunnd.service << SERVICE
[Unit]
Description=Tunnd Tunnel Server
Documentation=https://github.com/${REPO}
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=tunnd
Group=tunnd
ExecStart=${INSTALL_DIR}/tunnd-server --config ${CONFIG_DIR}/tunnd-server.yaml
Restart=on-failure
RestartSec=5
TimeoutStopSec=30

# Allow binding to ports 80 and 443 without root
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE

# Security hardening
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=${DATA_DIR}
PrivateTmp=yes
PrivateDevices=yes

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=tunnd

[Install]
WantedBy=multi-user.target
SERVICE

systemctl daemon-reload
systemctl enable tunnd
success "systemd service installed and enabled"

# ── Create first token ────────────────────────────────────────────────────────
info "Starting server to create first auth token…"
systemctl start tunnd
sleep 2  # give it a moment to initialise the DB

FIRST_TOKEN=$(TUNND_DB_PATH="${DATA_DIR}/tunnd.db" \
  "${INSTALL_DIR}/tunnd-server" \
  --config "${CONFIG_DIR}/tunnd-server.yaml" \
  token create "first-token" 2>/dev/null | grep "tnnd_" | awk '{print $NF}')

# ── Done ──────────────────────────────────────────────────────────────────────
echo
echo -e "${GREEN}${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
echo -e "${GREEN}${BOLD}  ✔  Tunnd is running!${RESET}"
echo -e "${GREEN}${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
echo
echo -e "  ${BOLD}Server URL:${RESET}     https://${DOMAIN}"
echo -e "  ${BOLD}Admin panel:${RESET}    http://$(curl -fsSL ifconfig.me 2>/dev/null || echo '<server-ip>'):9091"
echo -e "  ${BOLD}Admin password:${RESET} ${ADMIN_PASSWORD}"
echo
if [[ -n "$FIRST_TOKEN" ]]; then
  echo -e "  ${BOLD}First token:${RESET}    ${CYAN}${FIRST_TOKEN}${RESET}"
  echo
fi
echo -e "  ${BOLD}Install client:${RESET}"
echo -e "    curl -sSL https://raw.githubusercontent.com/${REPO}/main/install.sh | bash"
echo
echo -e "  ${BOLD}Open a tunnel:${RESET}"
echo -e "    ${CYAN}export TUNND_SERVER_ADDR=wss://${DOMAIN}${RESET}"
if [[ -n "$FIRST_TOKEN" ]]; then
  echo -e "    ${CYAN}export TUNND_TOKEN=${FIRST_TOKEN}${RESET}"
fi
echo -e "    ${CYAN}tunnd http 3000${RESET}"
echo
echo -e "  ${BOLD}Manage service:${RESET}"
echo -e "    systemctl status tunnd"
echo -e "    journalctl -u tunnd -f"
echo -e "    tunnd-server token create <label>"
echo
if [[ -n "$EMAIL" ]]; then
  warn "DNS check: make sure *.${DOMAIN} → $(curl -fsSL ifconfig.me 2>/dev/null || echo '<this-server-ip>') is set."
  warn "First tunnel connection may take ~2s while Let's Encrypt issues the cert."
fi
echo
