#!/usr/bin/env bash
set -euo pipefail

# notesync installer
# Usage: curl -fsSL https://raw.githubusercontent.com/nilszeilon/notesync/main/install.sh | bash -s -- yourdomain.com

REPO="https://github.com/nilszeilon/notesync.git"
INSTALL_DIR="/opt/notesync"

echo "==> notesync installer"
echo ""

# Must be root
if [ "$(id -u)" -ne 0 ]; then
    echo "Please run as root:"
    echo "  curl -fsSL https://raw.githubusercontent.com/nilszeilon/notesync/main/install.sh | sudo bash -s -- yourdomain.com"
    exit 1
fi

# Detect package manager
if command -v apt-get &>/dev/null; then
    PKG="apt"
elif command -v dnf &>/dev/null; then
    PKG="dnf"
elif command -v yum &>/dev/null; then
    PKG="yum"
else
    echo "Unsupported package manager. Install Docker manually and re-run."
    exit 1
fi

# Install Docker if missing
if ! command -v docker &>/dev/null; then
    echo "==> Installing Docker..."
    if [ "$PKG" = "apt" ]; then
        apt-get update -qq
        apt-get install -y -qq ca-certificates curl gnupg
        install -m 0755 -d /etc/apt/keyrings
        curl -fsSL https://download.docker.com/linux/$(. /etc/os-release && echo "$ID")/gpg \
            | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
        chmod a+r /etc/apt/keyrings/docker.gpg
        echo \
            "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] \
            https://download.docker.com/linux/$(. /etc/os-release && echo "$ID") \
            $(. /etc/os-release && echo "$VERSION_CODENAME") stable" \
            > /etc/apt/sources.list.d/docker.list
        apt-get update -qq
        apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-compose-plugin
    else
        # RHEL/CentOS/Fedora
        $PKG install -y -q yum-utils || true
        yum-config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo 2>/dev/null || \
            $PKG config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo 2>/dev/null || true
        $PKG install -y -q docker-ce docker-ce-cli containerd.io docker-compose-plugin
    fi
    systemctl enable --now docker
    echo "==> Docker installed"
else
    echo "==> Docker found"
fi

# Verify docker compose
if ! docker compose version &>/dev/null; then
    echo "docker compose plugin not found. Please install it manually."
    exit 1
fi

# Install git if missing
if ! command -v git &>/dev/null; then
    echo "==> Installing git..."
    if [ "$PKG" = "apt" ]; then
        apt-get install -y -qq git
    else
        $PKG install -y -q git
    fi
fi

# Get domain from argument or prompt
DOMAIN="${1:-}"
if [ -z "$DOMAIN" ]; then
    printf "Enter your domain (e.g. notes.example.com): " >/dev/tty
    read -r DOMAIN </dev/tty
fi
if [ -z "$DOMAIN" ]; then
    echo "Domain is required. Usage:"
    echo "  curl -fsSL https://raw.githubusercontent.com/nilszeilon/notesync/main/install.sh | sudo bash -s -- yourdomain.com"
    exit 1
fi

# Clone or update repo
if [ -d "$INSTALL_DIR" ]; then
    echo "==> Updating notesync..."
    git -C "$INSTALL_DIR" pull --ff-only
else
    echo "==> Cloning notesync..."
    git clone "$REPO" "$INSTALL_DIR"
fi

# Preserve existing token on re-run, generate new one on first install
if [ -f "$INSTALL_DIR/.env" ]; then
    TOKEN=$(grep '^NOTESYNC_TOKEN=' "$INSTALL_DIR/.env" | cut -d= -f2-)
    echo "==> Existing token preserved"
fi
if [ -z "${TOKEN:-}" ]; then
    TOKEN=$(openssl rand -base64 48 | tr -d '/+=' | head -c 64)
fi

# Write .env
cat > "$INSTALL_DIR/.env" <<EOF
DOMAIN=$DOMAIN
NOTESYNC_TOKEN=$TOKEN
EOF

# Open firewall ports if ufw is present
if command -v ufw &>/dev/null; then
    echo "==> Allowing ports 80 and 443 through firewall..."
    ufw allow 80/tcp >/dev/null 2>&1 || true
    ufw allow 443/tcp >/dev/null 2>&1 || true
fi

# Start services
echo "==> Starting notesync..."
cd "$INSTALL_DIR"
docker compose down 2>/dev/null || true
docker compose up --build -d

echo ""
echo "========================================"
echo "  notesync is running!"
echo "========================================"
echo ""
echo "  URL:   https://$DOMAIN"
echo "  Token: $TOKEN"
echo ""
echo "  SAVE THIS TOKEN — you need it to sync."
echo "  It will not be shown again."
echo ""
echo "  Sync from your machine:"
echo "    NOTESYNC_TOKEN=$TOKEN \\"
echo "      go run ./cmd/client -dir ~/notes -server https://$DOMAIN"
echo ""
echo "  Logs:  cd $INSTALL_DIR && docker compose logs -f"
echo "========================================"
