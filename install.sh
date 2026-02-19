#!/usr/bin/env bash
set -euo pipefail

# notesync installer
# Usage: curl -fsSL https://raw.githubusercontent.com/nilszeilon/notesync/main/install.sh | bash

REPO="https://github.com/nilszeilon/notesync.git"
INSTALL_DIR="/opt/notesync"

echo "==> notesync installer"
echo ""

# Must be root
if [ "$(id -u)" -ne 0 ]; then
    echo "Please run as root:"
    echo "  curl -fsSL https://raw.githubusercontent.com/nilszeilon/notesync/main/install.sh | sudo bash"
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

        # Docker repos use "debian" for both debian and raspbian,
        # and may not have the latest codename (e.g. trixie) yet
        DOCKER_DISTRO=$(. /etc/os-release && echo "$ID")
        DOCKER_CODENAME=$(. /etc/os-release && echo "$VERSION_CODENAME")
        case "$DOCKER_DISTRO" in raspbian) DOCKER_DISTRO="debian" ;; esac
        case "$DOCKER_CODENAME" in trixie) DOCKER_CODENAME="bookworm" ;; esac

        curl -fsSL "https://download.docker.com/linux/$DOCKER_DISTRO/gpg" \
            | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
        chmod a+r /etc/apt/keyrings/docker.gpg
        echo \
            "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] \
            https://download.docker.com/linux/$DOCKER_DISTRO \
            $DOCKER_CODENAME stable" \
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

# Ask if user wants a blog (public site) or storage only
MODE="${NOTESYNC_MODE:-}"
if [ -z "$MODE" ] && [ -f "$INSTALL_DIR/.env" ]; then
    MODE=$(grep '^MODE=' "$INSTALL_DIR/.env" 2>/dev/null | cut -d= -f2- || true)
fi
if [ -z "$MODE" ]; then
    echo "How do you want to use notesync?"
    echo ""
    echo "  1) Blog    — public site with a domain and TLS (publish server)"
    echo "  2) Storage — private sync server, no public site (storage only)"
    echo ""
    printf "Choose [1/2]: " >/dev/tty
    read -r CHOICE </dev/tty
    case "$CHOICE" in
        1|blog)  MODE="blog" ;;
        2|storage) MODE="storage" ;;
        *)
            echo "Invalid choice. Please enter 1 or 2."
            exit 1
            ;;
    esac
fi

# Get domain for blog mode
DOMAIN=""
if [ "$MODE" = "blog" ]; then
    DOMAIN="${1:-}"
    if [ -z "$DOMAIN" ] && [ -f "$INSTALL_DIR/.env" ]; then
        DOMAIN=$(grep '^DOMAIN=' "$INSTALL_DIR/.env" 2>/dev/null | cut -d= -f2- || true)
    fi
    if [ -z "$DOMAIN" ]; then
        printf "Enter your domain (e.g. notes.example.com): " >/dev/tty
        read -r DOMAIN </dev/tty
    fi
    if [ -z "$DOMAIN" ]; then
        echo "Domain is required for blog mode."
        exit 1
    fi
fi

# Open firewall ports if ufw is present (blog mode only)
if [ "$MODE" = "blog" ] && command -v ufw &>/dev/null; then
    echo "==> Allowing ports 80 and 443 through firewall..."
    ufw allow 80/tcp >/dev/null 2>&1 || true
    ufw allow 443/tcp >/dev/null 2>&1 || true
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
    TOKEN=$(grep '^NOTESYNC_TOKEN=' "$INSTALL_DIR/.env" | cut -d= -f2- || true)
    echo "==> Existing token preserved"
fi
if [ -z "${TOKEN:-}" ]; then
    TOKEN=$(openssl rand -base64 48 | tr -d '/+=' | head -c 64)
fi

# Write .env
if [ "$MODE" = "blog" ]; then
    cat > "$INSTALL_DIR/.env" <<EOF
MODE=$MODE
DOMAIN=$DOMAIN
NOTESYNC_TOKEN=$TOKEN
EOF
else
    cat > "$INSTALL_DIR/.env" <<EOF
MODE=$MODE
NOTESYNC_TOKEN=$TOKEN
EOF
fi

# Start services
echo "==> Starting notesync..."
cd "$INSTALL_DIR"
docker compose down 2>/dev/null || true

if [ "$MODE" = "blog" ]; then
    docker compose -f docker-compose.yml up -d --pull always
else
    docker compose -f docker-compose.storage.yml up -d --pull always
fi

echo ""
echo "========================================"
echo "  notesync is running!"
echo "========================================"
echo ""

if [ "$MODE" = "blog" ]; then
    echo "  Mode:  Blog (public site)"
    echo "  URL:   https://$DOMAIN"
    echo "  Token: $TOKEN"
    echo ""
    echo "  SAVE THIS TOKEN — you need it to sync."
    echo "  It will not be shown again."
    echo ""
    echo "  Sync published notes to this server:"
    echo "    NOTESYNC_PUBLISH_TOKEN=$TOKEN \\"
    echo "      notesync-client -dir ~/notes -publish-server https://$DOMAIN"
else
    echo "  Mode:  Storage (private sync)"
    echo "  URL:   http://<this-server>:8080"
    echo "  Token: $TOKEN"
    echo ""
    echo "  SAVE THIS TOKEN — you need it to sync."
    echo "  It will not be shown again."
    echo ""
    echo "  Sync all notes to this server:"
    echo "    NOTESYNC_TOKEN=$TOKEN \\"
    echo "      notesync-client -dir ~/notes -server http://<this-server>:8080"
fi

echo ""
echo "  Both targets at once:"
echo "    NOTESYNC_TOKEN=<storage-token> NOTESYNC_PUBLISH_TOKEN=<blog-token> \\"
echo "      notesync-client -dir ~/notes \\"
echo "        -server http://<storage-server>:8080 \\"
echo "        -publish-server https://<blog-domain>"
echo ""
echo "  Logs:  cd $INSTALL_DIR && docker compose logs -f"
echo "========================================"
