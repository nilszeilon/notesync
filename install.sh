#!/usr/bin/env bash
set -euo pipefail

# notesync installer
# Usage: curl -fsSL https://raw.githubusercontent.com/nilszeilon/notesync/main/install.sh | sudo bash

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

# --- Check prerequisites ---
if ! command -v docker &>/dev/null; then
    echo "Docker not found. Please install Docker first:"
    echo ""
    echo "  macOS:   https://docs.docker.com/desktop/install/mac-install/"
    echo "  Linux:   curl -fsSL https://get.docker.com | sh"
    echo "  Raspbian: sudo apt-get install -y docker.io"
    echo ""
    exit 1
fi

if ! docker compose version &>/dev/null; then
    echo "docker compose not found. Please install the compose plugin:"
    echo ""
    echo "  macOS:  included with Docker Desktop"
    echo "  Linux:  sudo apt-get install -y docker-compose-plugin"
    echo ""
    exit 1
fi

echo "==> Docker found"

# Install git if missing (best-effort)
if ! command -v git &>/dev/null; then
    echo "==> Installing git..."
    if command -v apt-get &>/dev/null; then
        apt-get install -y -qq git
    elif command -v dnf &>/dev/null; then
        dnf install -y -q git
    else
        echo "git not found. Please install git and re-run."
        exit 1
    fi
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
NOTESYNC_PUSH_ONLY=""
PUBLISH_TOKEN=""
if [ "$MODE" = "client" ]; then
    if [ -f "$INSTALL_DIR/.env" ]; then
        NOTESYNC_SERVER=$(grep '^NOTESYNC_SERVER=' "$INSTALL_DIR/.env" 2>/dev/null | cut -d= -f2- || true)
        NOTESYNC_PUBLISH_SERVER=$(grep '^NOTESYNC_PUBLISH_SERVER=' "$INSTALL_DIR/.env" 2>/dev/null | cut -d= -f2- || true)
        NOTESYNC_DIR=$(grep '^NOTESYNC_DIR=' "$INSTALL_DIR/.env" 2>/dev/null | cut -d= -f2- || true)
        NOTESYNC_PUSH_ONLY=$(grep '^NOTESYNC_PUSH_ONLY=' "$INSTALL_DIR/.env" 2>/dev/null | cut -d= -f2- || true)
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
    # Auto-add https:// if user entered a bare domain
    if [ -n "$NOTESYNC_PUBLISH_SERVER" ] && ! echo "$NOTESYNC_PUBLISH_SERVER" | grep -q '://'; then
        NOTESYNC_PUBLISH_SERVER="https://$NOTESYNC_PUBLISH_SERVER"
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
    if [ -z "$NOTESYNC_PUSH_ONLY" ]; then
        printf "Push-only mode? (only push local changes, don't download new remote files) [y/N]: " >/dev/tty
        read -r PUSH_ONLY_ANSWER </dev/tty
        case "$PUSH_ONLY_ANSWER" in
            y|Y|yes|Yes) NOTESYNC_PUSH_ONLY="true" ;;
            *)           NOTESYNC_PUSH_ONLY="false" ;;
        esac
    fi
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
NOTESYNC_DATA=$INSTALL_DIR/data
EOF
        ;;
    client)
        cat > "$INSTALL_DIR/.env" <<EOF
MODE=$MODE
NOTESYNC_TOKEN=$TOKEN
NOTESYNC_SERVER=$NOTESYNC_SERVER
NOTESYNC_DIR=$NOTESYNC_DIR
NOTESYNC_PUSH_ONLY=${NOTESYNC_PUSH_ONLY:-false}
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
