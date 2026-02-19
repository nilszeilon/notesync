#!/usr/bin/env bash
set -euo pipefail

# notesync installer
# Usage: curl -fsSL https://raw.githubusercontent.com/nilszeilon/notesync/main/install.sh | bash

REPO="https://github.com/nilszeilon/notesync.git"
INSTALL_DIR="/opt/notesync"

echo "==> notesync installer"
echo ""

OS="$(uname -s)"

# Must be root (on Linux)
if [ "$OS" = "Linux" ] && [ "$(id -u)" -ne 0 ]; then
    echo "Please run as root:"
    echo "  curl -fsSL https://raw.githubusercontent.com/nilszeilon/notesync/main/install.sh | sudo bash"
    exit 1
fi

if [ "$OS" = "Darwin" ]; then
    # --- macOS ---
    if ! command -v docker &>/dev/null; then
        echo "Docker not found. Please install Docker Desktop:"
        echo "  https://docs.docker.com/desktop/install/mac-install/"
        exit 1
    fi
    if ! docker info &>/dev/null 2>&1; then
        echo "Docker is not running. Please start Docker Desktop and re-run."
        exit 1
    fi
    echo "==> Docker found"
else
    # --- Linux ---

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
            # Remove stale Docker repo from a previous failed install
            rm -f /etc/apt/sources.list.d/docker.list
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

    # Install git if missing
    if ! command -v git &>/dev/null; then
        echo "==> Installing git..."
        if [ "$PKG" = "apt" ]; then
            apt-get install -y -qq git
        else
            $PKG install -y -q git
        fi
    fi
fi

# Verify docker compose
if ! docker compose version &>/dev/null; then
    echo "docker compose plugin not found. Please install it manually."
    exit 1
fi

# Choose mode
MODE="${NOTESYNC_MODE:-}"
if [ -z "$MODE" ] && [ -f "$INSTALL_DIR/.env" ]; then
    MODE=$(grep '^MODE=' "$INSTALL_DIR/.env" 2>/dev/null | cut -d= -f2- || true)
fi
if [ -z "$MODE" ]; then
    echo "How do you want to use notesync?"
    echo ""
    echo "  1) Blog    — public website with a domain and TLS"
    echo "  2) Storage — private sync server (e.g. on Tailscale)"
    echo "  3) Client  — sync a local folder to an existing server"
    echo ""
    printf "Choose [1/2/3]: " >/dev/tty
    read -r CHOICE </dev/tty
    case "$CHOICE" in
        1|blog)    MODE="blog" ;;
        2|storage) MODE="storage" ;;
        3|client)  MODE="client" ;;
        *)
            echo "Invalid choice. Please enter 1, 2, or 3."
            exit 1
            ;;
    esac
fi

# --- Blog mode: get domain ---
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

    # Open firewall ports
    if command -v ufw &>/dev/null; then
        echo "==> Allowing ports 80 and 443 through firewall..."
        ufw allow 80/tcp >/dev/null 2>&1 || true
        ufw allow 443/tcp >/dev/null 2>&1 || true
    fi
fi

# --- Client mode: get server URL, optional publish server, and notes dir ---
NOTESYNC_SERVER=""
NOTESYNC_PUBLISH_SERVER=""
NOTESYNC_DIR=""
PUBLISH_TOKEN=""
if [ "$MODE" = "client" ]; then
    if [ -f "$INSTALL_DIR/.env" ]; then
        NOTESYNC_SERVER=$(grep '^NOTESYNC_SERVER=' "$INSTALL_DIR/.env" 2>/dev/null | cut -d= -f2- || true)
        NOTESYNC_PUBLISH_SERVER=$(grep '^NOTESYNC_PUBLISH_SERVER=' "$INSTALL_DIR/.env" 2>/dev/null | cut -d= -f2- || true)
        NOTESYNC_DIR=$(grep '^NOTESYNC_DIR=' "$INSTALL_DIR/.env" 2>/dev/null | cut -d= -f2- || true)
    fi
    if [ -z "$NOTESYNC_SERVER" ]; then
        printf "Enter storage server URL (e.g. http://100.x.x.x:8080): " >/dev/tty
        read -r NOTESYNC_SERVER </dev/tty
    fi
    if [ -z "$NOTESYNC_SERVER" ]; then
        echo "Server URL is required."
        exit 1
    fi
    if [ -z "$NOTESYNC_PUBLISH_SERVER" ]; then
        printf "Enter publish/blog server URL (leave empty to skip): " >/dev/tty
        read -r NOTESYNC_PUBLISH_SERVER </dev/tty
    fi
    if [ -z "$NOTESYNC_DIR" ]; then
        REAL_USER="${SUDO_USER:-$(whoami)}"
        REAL_HOME=$(eval echo "~$REAL_USER")
        DEFAULT_NOTES="$REAL_HOME/notes"
        printf "Enter notes directory [$DEFAULT_NOTES]: " >/dev/tty
        read -r NOTESYNC_DIR </dev/tty
        NOTESYNC_DIR="${NOTESYNC_DIR:-$DEFAULT_NOTES}"
    fi
    mkdir -p "$NOTESYNC_DIR"
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
    PUBLISH_TOKEN=$(grep '^NOTESYNC_PUBLISH_TOKEN=' "$INSTALL_DIR/.env" 2>/dev/null | cut -d= -f2- || true)
    echo "==> Existing config preserved"
fi
if [ -z "${TOKEN:-}" ]; then
    if [ "$MODE" = "client" ]; then
        printf "Enter storage server token: " >/dev/tty
        read -r TOKEN </dev/tty
        if [ -z "$TOKEN" ]; then
            echo "Token is required."
            exit 1
        fi
    else
        TOKEN=$(openssl rand -base64 48 | tr -d '/+=' | head -c 64)
    fi
fi
if [ "$MODE" = "client" ] && [ -n "${NOTESYNC_PUBLISH_SERVER:-}" ] && [ -z "${PUBLISH_TOKEN:-}" ]; then
    printf "Enter publish server token: " >/dev/tty
    read -r PUBLISH_TOKEN </dev/tty
    if [ -z "$PUBLISH_TOKEN" ]; then
        echo "Publish token is required when a publish server is set."
        exit 1
    fi
fi

# Write .env
case "$MODE" in
    blog)
        cat > "$INSTALL_DIR/.env" <<EOF
MODE=$MODE
DOMAIN=$DOMAIN
NOTESYNC_TOKEN=$TOKEN
EOF
        ;;
    storage)
        cat > "$INSTALL_DIR/.env" <<EOF
MODE=$MODE
NOTESYNC_TOKEN=$TOKEN
EOF
        ;;
    client)
        cat > "$INSTALL_DIR/.env" <<EOF
MODE=$MODE
NOTESYNC_TOKEN=$TOKEN
NOTESYNC_SERVER=$NOTESYNC_SERVER
NOTESYNC_DIR=$NOTESYNC_DIR
NOTESYNC_PUBLISH_TOKEN=${PUBLISH_TOKEN:-}
NOTESYNC_PUBLISH_SERVER=${NOTESYNC_PUBLISH_SERVER:-}
EOF
        ;;
esac

# Start services
echo "==> Starting notesync..."
cd "$INSTALL_DIR"
docker compose down 2>/dev/null || true

case "$MODE" in
    blog)    docker compose -f docker-compose.yml up -d --pull always ;;
    storage) docker compose -f docker-compose.storage.yml up -d --pull always ;;
    client)  docker compose -f docker-compose.client.yml up -d --pull always ;;
esac

echo ""
echo "========================================"
echo "  notesync is running!"
echo "========================================"
echo ""

case "$MODE" in
    blog)
        echo "  Mode:   Blog (public site)"
        echo "  URL:    https://$DOMAIN"
        echo "  Token:  $TOKEN"
        echo ""
        echo "  SAVE THIS TOKEN — you need it to sync."
        echo ""
        echo "  Set up a client to sync to this server:"
        echo "    curl -fsSL https://raw.githubusercontent.com/nilszeilon/notesync/main/install.sh | sudo bash"
        echo "    # choose '3) Client' and enter:"
        echo "    #   server: https://$DOMAIN"
        echo "    #   token:  $TOKEN"
        ;;
    storage)
        # Try to detect Tailscale IP
        TS_IP=""
        if command -v tailscale &>/dev/null; then
            TS_IP=$(tailscale ip -4 2>/dev/null || true)
        fi
        SERVER_ADDR="${TS_IP:-<this-server>}"

        echo "  Mode:   Storage (private sync)"
        echo "  URL:    http://$SERVER_ADDR:8080"
        echo "  Token:  $TOKEN"
        echo ""
        echo "  SAVE THIS TOKEN — you need it to sync."
        echo ""
        echo "  Set up a client on another machine:"
        echo "    curl -fsSL https://raw.githubusercontent.com/nilszeilon/notesync/main/install.sh | sudo bash"
        echo "    # choose '3) Client' and enter:"
        echo "    #   server: http://$SERVER_ADDR:8080"
        echo "    #   token:  $TOKEN"
        ;;
    client)
        echo "  Mode:    Client"
        echo "  Storage: $NOTESYNC_SERVER"
        if [ -n "${NOTESYNC_PUBLISH_SERVER:-}" ]; then
            echo "  Blog:    $NOTESYNC_PUBLISH_SERVER"
        fi
        echo "  Dir:     $NOTESYNC_DIR"
        echo ""
        echo "  Your notes will sync automatically."
        ;;
esac

echo ""
echo "  Logs:  cd $INSTALL_DIR && docker compose logs -f"
echo "========================================"
