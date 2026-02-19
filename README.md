# Notesync

A lightweight, self-hosted sync engine for your markdown notes. Syncs files across devices and optionally publishes them as a static blog.

## Quick start

All you need is Docker on each machine. The install script handles everything else.

```bash
curl -fsSL https://raw.githubusercontent.com/nilszeilon/notesync/main/install.sh | sudo bash
```

The installer will ask you to pick a mode:

1. **Blog** — sync server + static site with automatic HTTPS
2. **Storage** — private sync server (no public site)
3. **Client** — watches a local folder and syncs to a server

A typical setup is one server and one client per device.

## Setup guides

### Private sync across your devices

Sync notes between your machines over Tailscale without exposing anything to the internet.

**1. Install Tailscale on all machines**

```bash
# Linux / Raspberry Pi
curl -fsSL https://tailscale.com/install.sh | sh
sudo tailscale up

# macOS
# Install from https://tailscale.com/download/mac
```

Each machine gets a stable IP on your private Tailscale network (e.g. `100.x.x.x`). You can find it with:

```bash
tailscale ip -4
```

**2. Set up the server**

Pick any machine to be the server — a Raspberry Pi, an old laptop, your NAS, whatever is always on.

```bash
curl -fsSL https://raw.githubusercontent.com/nilszeilon/notesync/main/install.sh | sudo bash
# Choose: 2) Storage
```

When it finishes it will print a token. Save it — you need it for every client.

**3. Set up clients**

On each machine you want to sync:

```bash
curl -fsSL https://raw.githubusercontent.com/nilszeilon/notesync/main/install.sh | sudo bash
# Choose: 3) Client
# Server URL: http://<tailscale-ip>:8080
# Token: <the token from step 2>
# Notes dir: ~/notes (or wherever your notes live)
```

That's it. Changes in your notes folder sync automatically.

---

### Public blog (cheap VPS)

Publish selected notes as a blog with automatic HTTPS. Works great on a ~3€/month VPS from Hetzner, DigitalOcean, etc.

**1. Point a domain at your server**

Create an A record for your domain (e.g. `notes.example.com`) pointing to your VPS IP.

**2. Install on the VPS**

```bash
curl -fsSL https://raw.githubusercontent.com/nilszeilon/notesync/main/install.sh | sudo bash
# Choose: 1) Blog
# Domain: notes.example.com
```

Caddy handles TLS automatically. Your site is live at `https://notes.example.com`.

**3. Set up a client**

On your laptop:

```bash
curl -fsSL https://raw.githubusercontent.com/nilszeilon/notesync/main/install.sh | sudo bash
# Choose: 3) Client
# Server URL: https://notes.example.com
# Token: <the token from step 2>
```

**4. Publish a note**

Add `publish: true` to the frontmatter of any note:

```yaml
---
publish: true
title: My first post
---

Hello world!
```

Save, and it's live. Remove `publish: true` to unpublish.

---

### Private sync + public blog (both)

If you don't want all your notes on a public-facing server, use two servers: a private one for storage and a public one for publishing.

**1. Set up a private storage server** (e.g. Raspberry Pi on Tailscale)

```bash
curl -fsSL https://raw.githubusercontent.com/nilszeilon/notesync/main/install.sh | sudo bash
# Choose: 2) Storage
```

**2. Set up a public blog server** (e.g. Hetzner VPS)

```bash
curl -fsSL https://raw.githubusercontent.com/nilszeilon/notesync/main/install.sh | sudo bash
# Choose: 1) Blog
# Domain: notes.example.com
```

**3. Set up clients that sync to both**

```bash
curl -fsSL https://raw.githubusercontent.com/nilszeilon/notesync/main/install.sh | sudo bash
# Choose: 3) Client
# Storage server: http://<tailscale-ip>:8080
# Storage token: <private server token>
# Publish server: https://notes.example.com
# Publish token: <blog server token>
```

All notes sync privately. Only notes with `publish: true` get pushed to the blog.

## Push-only mode (work computers)

If you want to sync from a machine (like a work laptop) without pulling down all your personal notes, set the `NOTESYNC_PUSH_ONLY` environment variable:

```bash
# After installing as a client, edit the .env:
sudo nano /opt/notesync/.env

# Add:
NOTESYNC_PUSH_ONLY=true
```

Then restart:

```bash
cd /opt/notesync && sudo docker compose -f docker-compose.client.yml up -d
```

In push-only mode:
- Notes you create on this machine sync to the server and to your other devices
- Notes you already have locally still receive updates from other devices
- New notes from other devices are **not** downloaded

## Publishing

Any markdown file with `publish: true` in the frontmatter becomes a blog post:

```yaml
---
publish: true
title: Optional custom title
date: 2025-01-15
---
```

- **title** — defaults to the filename if not set
- **date** — defaults to file modification time if not set
- **Wikilinks** — `[[Note Title]]` links between published notes
- **Images** — `![[photo.png]]` embeds are supported
- **GFM** — tables, strikethrough, task lists, and autolinks all work

Create an `index.md` with `publish: true` to use a custom homepage instead of the auto-generated note listing.

## Commands

```bash
# View logs
cd /opt/notesync && sudo docker compose logs -f

# Restart
cd /opt/notesync && sudo docker compose -f docker-compose.<mode>.yml up -d

# Update to latest version
cd /opt/notesync && sudo git pull && sudo docker compose -f docker-compose.<mode>.yml up -d --pull always

# Uninstall
cd /opt/notesync && sudo docker compose down
sudo rm -rf /opt/notesync
```

## Prerequisites

- Docker with the compose plugin
- Git (the installer will try to install it if missing)
- Tailscale (only for private sync setups)
