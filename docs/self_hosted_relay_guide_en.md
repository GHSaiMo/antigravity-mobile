# Multigravity (`mgy`) Self-Hosted Relay (BYO-Relay) Guide

> 🌐 [简体中文](self_hosted_relay_guide.md) | **English** | [日本語](self_hosted_relay_guide_ja.md)

If you want all remote Multigravity traffic to route **exclusively through your own server (VPS / cloud host)**, use the built-in **BYO-Relay (Bring Your Own Relay)** mode described in this guide.

- **No code edits and no mobile app rebuilds**: Works directly with the official App Store, TestFlight, and GitHub Release builds;
- **No Cloudflare Worker dispatcher required**: The `mgy-relay` binary ships with an offline token-signing subcommand;
- **Completely detached from official relays**: Once configured, your desktop `mgy` gateway stops contacting the official dispatcher and skips downloading `cloudflared`, and your phone will never phone home to the official relay pool even when offline;
- **Mandatory zero-knowledge E2EE (`mgy-e2ee-v1`)**: Your VPS only forwards AES-256-GCM ciphertext; any plaintext request arriving over the relay is rejected by your desktop gateway with `HTTP 426`.

> **Version requirements**: Desktop gateway `mgy >= v1.2.8` and iOS / Android app `>= v1.2.8`.

---

## Table of Contents

1. [Architecture & Prerequisites](#1-architecture--prerequisites)
2. [Part 1: Set Up Your Relay Server (VPS)](#2-part-1-set-up-your-relay-server-vps)
3. [Part 2: Configure the Desktop Gateway (`mgy`)](#3-part-2-configure-the-desktop-gateway-mgy)
4. [Part 3: Configure Your Phone & Web Client](#4-part-3-configure-your-phone--web-client)
5. [Part 4: Multiple Computers, APNs Push & Privacy Hardening](#5-part-4-multiple-computers-apns-push--privacy-hardening)
6. [Flags & Environment Variable Reference](#6-flags--environment-variable-reference)
7. [Troubleshooting (FAQ)](#7-troubleshooting-faq)

---

## 1. Architecture & Prerequisites

### 1.1 How It Works

```text
Your Phone (iOS / Android / Web)
   │  HTTPS / WSS (application-layer AES-256-GCM E2EE ciphertext)
   ▼  Tunnel hostname: https://dev.example.com
Your Relay Server (mgy-relay)
   ▲  Control reverse tunnel: wss://ctl.example.com/_mgy_tunnel (HMAC token auth + yamux multiplexing)
   │
Your Computer (mgy gateway) ──▶ Local Antigravity IDE / CLI
```

### 1.2 What You Need

1. **A Linux VPS with a public IP**: 1 vCPU / 512 MB RAM is plenty (`mgy-relay` uses ~10 MB RAM at idle).
2. **A domain name with two DNS records** pointing to your VPS IP (`example.com` is used below—replace it with your own domain):

| Role | Example Hostname | DNS Record | Purpose |
| :--- | :--- | :--- | :--- |
| **Control host** | `ctl.example.com` | `A` → Your VPS IP | Desktop `mgy` reverse-tunnel ingress (`/_mgy_tunnel`) and `/healthz` |
| **Tunnel host** | `dev.example.com` | `A` → Your VPS IP | Public hostname used by your phone & pairing QR code (or wildcard `*.mgy.example.com` for multiple computers) |

> ⚠️ **Two distinct hostnames are required**: `mgy-relay` automatically adds every hostname in `MGY_RELAY_CONTROL_HOSTS` to its reserved list, so **the control host and the tunnel host cannot be the same domain**.

3. **Firewall rules**: Allow inbound TCP **`443`** (HTTPS/WSS traffic). If you use Caddy for automatic TLS certificates, also allow TCP **`80`**.

---

## 2. Part 1: Set Up Your Relay Server (VPS)

### 2.1 Build the `mgy-relay` Static Binary

GitHub Releases currently prebuild only the desktop gateway (`mgy`), so build `mgy-relay` from source as a standalone static Go binary (~7 MB, zero runtime dependencies). You can cross-compile on your local computer and upload it to your VPS, or build directly on the VPS:

```bash
git clone https://github.com/GHSaiMo/antigravity-mobile.git
cd antigravity-mobile

# Linux x86_64 server:
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o mgy-relay ./cmd/relay

# For ARM64 servers (Oracle ARM / Raspberry Pi / AWS Graviton), use:
# CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o mgy-relay ./cmd/relay
```

Install the binary on your VPS at `/opt/mgy-relay/mgy-relay`:

```bash
sudo mkdir -p /opt/mgy-relay /etc/mgy-relay
sudo mv mgy-relay /opt/mgy-relay/mgy-relay
sudo chmod +x /opt/mgy-relay/mgy-relay
```

---

### 2.2 Generate a Signing Secret & Config File

Generate a 256-bit random secret on your VPS (used to authenticate offline tokens; keep it private):

```bash
openssl rand -hex 32
```

Create `/etc/mgy-relay/relay.env` (readable only by root):

```bash
sudo tee /etc/mgy-relay/relay.env > /dev/null <<'EOF'
# Replace with the 64-char hex secret generated above
MGY_RELAY_SIGNING_SECRET=replace_with_your_64_hex_secret

# Control hostname (must differ from your tunnel hostname)
MGY_RELAY_CONTROL_HOSTS=ctl.example.com

# Allowed tunnel domain suffix (MUST override the default .jiuge.space; leave empty to allow any non-reserved domain)
MGY_RELAY_ALLOWED_SUFFIXES=.example.com

# Listener settings (adjust based on Option A or Option B in section 2.3)
MGY_RELAY_LISTEN=127.0.0.1:8443
MGY_RELAY_TLS_CERT=
MGY_RELAY_TLS_KEY=
EOF

sudo chmod 600 /etc/mgy-relay/relay.env
```

> ⚠️ **Common pitfall**: By default, `mgy-relay` restricts allowed suffixes to `.jiuge.space` and control hosts to `relay.jiuge.space`. You **must override both variables** for your own domain, or the gateway will receive `403 domain_not_allowed`.

---

### 2.3 Configure HTTPS (Choose One)

#### Option A (Recommended, Easiest): Behind Caddy or Nginx Reverse Proxy

Keep the plain-HTTP loopback listener in `/etc/mgy-relay/relay.env`:

```env
MGY_RELAY_LISTEN=127.0.0.1:8443
MGY_RELAY_TLS_CERT=
MGY_RELAY_TLS_KEY=
```

- **With Caddy (recommended—automatic HTTPS issuance & renewal)**:

  Add to `/etc/caddy/Caddyfile`:
  ```caddyfile
  ctl.example.com, dev.example.com {
      reverse_proxy 127.0.0.1:8443
  }
  ```
  Reload: `sudo systemctl reload caddy`

- **With Nginx**:

  > Make sure Nginx preserves the original `Host` header (`mgy-relay` routes tunnels by `Host`), forwards WebSocket `Upgrade`/`Connection` headers, disables response buffering, and uses a long read timeout.

  ```nginx
  map $http_upgrade $connection_upgrade {
      default upgrade;
      ''      close;
  }

  server {
      listen 443 ssl http2;
      server_name ctl.example.com dev.example.com;

      ssl_certificate     /path/to/fullchain.pem;
      ssl_certificate_key /path/to/privkey.pem;
      ssl_protocols       TLSv1.2 TLSv1.3;

      client_max_body_size 64m;

      location / {
          proxy_pass http://127.0.0.1:8443;
          proxy_http_version 1.1;

          proxy_set_header Host $host;
          proxy_set_header X-Real-IP $remote_addr;
          proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
          proxy_set_header X-Forwarded-Proto $scheme;

          # Required for reverse WSS tunnel & streaming responses
          proxy_set_header Upgrade $http_upgrade;
          proxy_set_header Connection $connection_upgrade;
          proxy_buffering off;
          proxy_read_timeout 3600s;
          proxy_send_timeout 3600s;
      }
  }
  ```
  Test and reload: `sudo nginx -t && sudo systemctl reload nginx`

---

#### Option B (Minimal, No Reverse Proxy): Listen Directly on Port 443

If port 443 is free on your VPS and you already have a TLS certificate covering both `ctl.example.com` and `dev.example.com` (or wildcard `*.example.com`, e.g. from `acme.sh` or `certbot`), update `/etc/mgy-relay/relay.env`:

```env
MGY_RELAY_LISTEN=:443
MGY_RELAY_TLS_CERT=/etc/mgy-relay/fullchain.pem
MGY_RELAY_TLS_KEY=/etc/mgy-relay/privkey.pem
```

> Tip: When renewing certificates with `acme.sh`, set `--reloadcmd "systemctl restart mgy-relay"` to apply updated certificates automatically.

---

### 2.4 Create a `systemd` Service & Start It

Create `/etc/systemd/system/mgy-relay.service`:

```bash
sudo tee /etc/systemd/system/mgy-relay.service > /dev/null <<'EOF'
[Unit]
Description=Multigravity Zero-Knowledge E2EE Reverse Relay
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=/etc/mgy-relay/relay.env
ExecStart=/opt/mgy-relay/mgy-relay
Restart=always
RestartSec=2
LimitNOFILE=65536
Environment=GOMEMLIMIT=450MiB

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now mgy-relay
sudo systemctl status mgy-relay --no-pager
```

### 2.5 Verify the Relay Server

Query `/healthz` on your control hostname from any machine:

```bash
curl -s https://ctl.example.com/healthz
```

Expected output:

```json
{"active_conns":0,"active_streams":0,"active_tunnels":0,"service":"mgy-relay","status":"ok","total_streams":0,"uptime_sec":12,"version":"1.1.0"}
```

---

### 2.6 Mint an Offline Gateway Token (No Worker Needed)

Run the built-in `token` subcommand on your VPS using the same `MGY_RELAY_SIGNING_SECRET`:

```bash
sudo bash -c '
  set -a
  source /etc/mgy-relay/relay.env
  set +a
  /opt/mgy-relay/mgy-relay token \
    --domain dev.example.com \
    --endpoint wss://ctl.example.com/_mgy_tunnel
'
```

It prints the signed `mgyr1.*` token along with a ready-to-paste 3-line `.env` snippet:

```env
# Add to your gateway ~/.multigravity/.env (BYO-Relay self-hosted mode):
MGY_CUSTOM_RELAY_ENDPOINT=wss://ctl.example.com/_mgy_tunnel
MGY_CUSTOM_RELAY_TOKEN=mgyr1.eyJ2IjoxLCJzaWQiOi....xxxxxxxx
MGY_CUSTOM_RELAY_PUBLIC_URL=https://dev.example.com
```

Copy these three lines for the next step.

---

## 3. Part 2: Configure the Desktop Gateway (`mgy`)

### 3.1 Verify Gateway Version (`>= v1.2.8`)

Run on the computer hosting Antigravity:

```bash
mgy version
```

If it is older than `v1.2.8`, update first:

```bash
mgy update
```

### 3.2 Add BYO-Relay Settings to `.env`

Open (or create) the gateway config file:
- **macOS / Linux**: `~/.multigravity/.env`
- **Windows**: `%USERPROFILE%\.multigravity\.env`

Paste the three lines generated in Section 2.6:

```env
# Self-hosted zero-knowledge E2EE relay (BYO-Relay)
MGY_CUSTOM_RELAY_ENDPOINT=wss://ctl.example.com/_mgy_tunnel
MGY_CUSTOM_RELAY_TOKEN=mgyr1.eyJ2IjoxLCJzaWQiOi....xxxxxxxx
MGY_CUSTOM_RELAY_PUBLIC_URL=https://dev.example.com
```

> Notes:
> - Whenever both `MGY_CUSTOM_RELAY_ENDPOINT` and `MGY_CUSTOM_RELAY_TOKEN` are set, `mgy` **automatically enters BYO-Relay mode**, bypassing the official `dispatcher.multigravity.app` and skipping `cloudflared` (you do not need to set `CF_TUNNEL_ENABLED=0`).
> - `MGY_CUSTOM_RELAY_PUBLIC_URL` is optional—if omitted, `mgy` automatically derives `https://<domain>` from the token payload.

### 3.3 Restart the Gateway & Confirm Logs

- **Foreground terminal**: Press `Ctrl + C` in the existing terminal, then run:
  ```bash
  mgy
  ```
- **Background daemon (`mgy service`)**:
  ```bash
  mgy service restart
  mgy service logs -f
  ```

Look for the following banner and log line confirming the self-hosted tunnel is active:

```text
  ➜  Cloud Tunnel: Self-hosted E2EE relay active (https://dev.example.com | BYO-Relay)
```
```text
INFO ✅ [BYO-Relay] 自建 E2EE 中继隧道已连接 endpoint=wss://ctl.example.com/_mgy_tunnel url=https://dev.example.com
```

### 3.4 End-to-End Verification (Recommended)

Run these two checks from any machine:

```bash
# 1. Confirm active_tunnels is 1 and active_conns >= 1
curl -s https://ctl.example.com/healthz

# 2. Send a plaintext request to your tunnel domain; MUST return HTTP 426 e2ee_required
curl -i https://dev.example.com/api/v1/version
```

Receiving **`HTTP/2 426`** with `{"error":"e2ee_required", ...}` proves that the entire path (`Internet → your VPS → your desktop gateway`) is live and plaintext downgrade protection is active.

---

## 4. Part 3: Configure Your Phone & Web Client

No custom build or code change is needed on your phone. Use the official **iOS (App Store / TestFlight)** or **Android (GitHub Release APK)** app (`>= v1.2.8`).

### 4.1 Scenario A: First-Time Pairing

1. Run `mgy pair` on your computer (or use the QR code printed when `mgy` starts). The QR code already embeds `https://dev.example.com`, your LAN IP, and your gateway's E2EE public key.
2. Open Multigravity on your phone and scan the QR code.
3. Pairing completes in 1–2 seconds—no manual relay URL entry is required.

### 4.2 Scenario B: Migrating an Already-Paired Phone

Re-bind the phone to your self-hosted hostname using either method:

- **Method 1 (Recommended, keeps other paired hosts intact)**:
  1. Run `mgy pair` on your computer;
  2. In the mobile app, tap the host name at the top of the home screen to open **Switch Host** → tap **`+ Add Host`**;
  3. Scan the new QR code. Because it matches the same gateway ID, your existing host entry is updated in place with your self-hosted relay URL and E2EE host record.
- **Method 2 (Clean reset)**:
  1. Go to **Settings → Device Pairing → Unpair Device**;
  2. Run `mgy pair` on your computer;
  3. Scan the QR code to pair fresh.

### 4.3 Verify Mobile Traffic Uses Your Server

1. **Disable Wi-Fi on your phone and switch to 5G / 4G cellular data** (to bypass LAN direct access);
2. Open a conversation or send a message in the app;
3. Check your VPS relay counter:
   ```bash
   curl -s https://ctl.example.com/healthz
   ```
   Confirm `total_streams` increments with your mobile activity.

### 4.4 Automatic Mobile Route Priority

```text
① Local Wi-Fi LAN (sub-millisecond direct connection on the same network; zero VPS bandwidth)
   ↓ Leave home / switch to 5G
② Your Self-Hosted E2EE Relay (https://dev.example.com, end-to-end AES-256-GCM encrypted)
```

> Mobile clients isolate custom hostnames automatically: when using your own domain, the app **never queries the official `dispatcher.multigravity.app` pool manifest**, even if your VPS is temporarily unreachable.

### 4.5 Web / PWA Access

Unless you disable Web access on the relay, open `https://dev.example.com` in any browser and enter the 6-character pairing code from `mgy pair` (all browser traffic is also end-to-end encrypted via the Web Crypto API).

---

## 5. Part 4: Multiple Computers, APNs Push & Privacy Hardening

### 5.1 Sharing One VPS Across Multiple Computers

One `mgy-relay` instance can serve multiple computers with no Worker required:
1. Point a wildcard DNS record `*.mgy.example.com` to your VPS IP and configure a wildcard TLS certificate;
2. Set `MGY_RELAY_ALLOWED_SUFFIXES=.mgy.example.com` in `/etc/mgy-relay/relay.env`;
3. Mint a separate token per computer using distinct `--domain` values:
   ```bash
   # Work Mac
   /opt/mgy-relay/mgy-relay token --domain mac.mgy.example.com --endpoint wss://ctl.example.com/_mgy_tunnel
   # Home Windows PC
   /opt/mgy-relay/mgy-relay token --domain win.mgy.example.com --endpoint wss://ctl.example.com/_mgy_tunnel
   ```
4. Add each computer's 3 lines to its `~/.multigravity/.env`, restart `mgy`, and scan each QR code via **`+ Add Host`** on your phone.

### 5.2 iOS Remote Push (APNs) & Zero Official Contact

With BYO-Relay enabled, **100% of chat messages, diffs, files, and terminal traffic stays between your phone, your VPS, and your computer**. The only optional feature that still defaults to an official endpoint is iOS App Store remote push / Live Activity updates (`https://push.multigravity.app`), because Apple ties the APNs `.p8` push key to the official App Store Bundle ID.

Choose your preferred policy in your desktop `~/.multigravity/.env`:

| Policy | Setting | Behavior |
| :--- | :--- | :--- |
| **Keep push (default)** | *(none)* | Lock-screen approvals & Live Activity updates via stateless push relay (no DB, no logs) |
| **Keep push + redact content (recommended)** | `MGY_PUSH_REDACT=1` | Strips titles, commands, paths, and hostnames; sends only alert category and numeric progress (e.g., `3/5`) |
| **Zero official contact** | `APNS_ENABLED=0` | Completely disables APNs relay requests (**0 bytes** to any official domain). To self-host APNs, re-sign the iOS app with your own Apple Developer account and deploy [scripts/cloudflare-apns-worker/](../scripts/cloudflare-apns-worker/README.md) |

### 5.3 Optional Security Hardening (Desktop `.env`)

```env
# Native-App-only mode: disables Web/PWA shell over the relay and hides static e2ee_pub/host info on /healthz
MGY_RELAY_DISABLE_WEB=1

# Strict replay protection: reject legacy E2EE requests without timestamps/req_id (enable after all apps are >= v1.2.8)
MGY_E2EE_STRICT_REPLAY=1
```

### 5.4 Advanced: Multi-VPS Self-Hosted Failover Pool

If you run **multiple VPS nodes across regions** and want automatic latency probing and failover, deploy [scripts/cloudflare-worker/worker.js](../scripts/cloudflare-worker/worker.js) to your own free Cloudflare account (with `RELAY_POLICY=prefer` and `RELAY_POOL`), remove `MGY_CUSTOM_RELAY_*` from your desktop `.env`, and set:

```env
CF_WORKER_URL=https://your-own-dispatcher.example.com
```

---

## 6. Flags & Environment Variable Reference

### 6.1 Relay Server `mgy-relay` (VPS)

| Environment Variable | CLI Flag | Default | Description |
| :--- | :--- | :--- | :--- |
| `MGY_RELAY_SIGNING_SECRET` | — | *(required)* | Shared HMAC-SHA256 secret used with `mgy-relay token` |
| `MGY_RELAY_LISTEN` | `-listen` | `:443` | Listen address; use `127.0.0.1:8443` behind Caddy/Nginx |
| `MGY_RELAY_TLS_CERT` | `-tls-cert` | `/etc/mgy-relay/origin.crt` | Full TLS certificate chain path; **leave empty for plain HTTP behind a reverse proxy** |
| `MGY_RELAY_TLS_KEY` | `-tls-key` | `/etc/mgy-relay/origin.key` | TLS private key path |
| `MGY_RELAY_CONTROL_HOSTS` | `-control-hosts` | `relay.jiuge.space` | Comma-separated control hostnames (`/_mgy_tunnel` & `/healthz`); **must override** |
| `MGY_RELAY_ALLOWED_SUFFIXES` | `-allowed-suffixes` | `.jiuge.space` | Comma-separated allowed tunnel domain suffixes; **must override** (empty allows any non-reserved domain) |
| `MGY_RELAY_RESERVED_HOSTS` | `-reserved-hosts` | `""` | Additional comma-separated hostnames forbidden as tunnel domains |
| `MGY_RELAY_MAX_TUNNELS` | `-max-tunnels` | `10000` | Maximum concurrent gateway tunnels |
| `MGY_RELAY_MAX_STREAMS_PER_TUNNEL` | `-max-streams` | `64` | Maximum concurrent HTTP/WS streams per tunnel |

### 6.2 Desktop Gateway `mgy` (`~/.multigravity/.env`)

| Environment Variable | Required | Example | Description |
| :--- | :---: | :--- | :--- |
| `MGY_CUSTOM_RELAY_ENDPOINT` | Yes | `wss://ctl.example.com/_mgy_tunnel` | Control WebSocket endpoint (`https://ctl.example.com` is auto-normalized to `wss://.../_mgy_tunnel`) |
| `MGY_CUSTOM_RELAY_TOKEN` | Yes | `mgyr1.xxxx.yyyy` | Offline-signed token from `mgy-relay token` |
| `MGY_CUSTOM_RELAY_PUBLIC_URL` | No | `https://dev.example.com` | Public URL for mobile/web clients (auto-derived from token `dom` claim if omitted) |
| `MGY_PUSH_REDACT` | No | `1` | Redact sensitive text from push payloads |
| `APNS_ENABLED` | No | `0` | Disable official APNs push relay completely |
| `MGY_RELAY_DISABLE_WEB` | No | `1` | Disable Web/PWA shell over the relay (native apps only) |

---

## 7. Troubleshooting (FAQ)

1. **Desktop logs show `403 domain_not_allowed`?**
   - Check `/etc/mgy-relay/relay.env`: make sure `MGY_RELAY_ALLOWED_SUFFIXES` is no longer `.jiuge.space` and matches your tunnel domain (e.g., `.example.com` for `dev.example.com`).
   - Verify you did **not** reuse the control hostname (`ctl.example.com`) as the tunnel `--domain`.

2. **Desktop logs show `401 invalid_token`?**
   - The `MGY_RELAY_SIGNING_SECRET` used when running `mgy-relay token` does not match the running `mgy-relay` service, or extra whitespace/newlines were pasted into `.env`. Re-mint the token with the exact same secret and restart.

3. **Accessing `https://dev.example.com` returns `503 {"error":"gateway_offline"}`?**
   - DNS, TLS, and your reverse proxy are working, but your desktop `mgy` is not connected yet (or the computer is asleep/offline).
   - Check desktop logs (`mgy service logs -f`), VPS logs (`sudo journalctl -u mgy-relay -n 50 --no-pager`), and confirm `active_tunnels` is `1` at `https://ctl.example.com/healthz`.

4. **Behind Nginx, the tunnel drops every 60 seconds or streaming replies stall?**
   - Ensure `location /` includes `proxy_http_version 1.1;`, `Upgrade`/`Connection` headers, `proxy_read_timeout 3600s;`, and `proxy_buffering off;`.
   - Also make sure `proxy_set_header Host $host;` is present; otherwise `mgy-relay` sees `127.0.0.1` instead of your domain and returns `unknown_host`.

5. **`curl https://dev.example.com/api/v1/version` returns `HTTP 426 e2ee_required`—is that an error?**
   - **No—that is expected!** It confirms your relay tunnel reaches your desktop gateway and mandatory E2EE protection is blocking plaintext requests. Once paired via QR code, the mobile app communicates over the encrypted channel automatically.

6. **How do I switch back to the free official relay pool later?**
   - Remove or comment out the three `MGY_CUSTOM_RELAY_*` lines in `~/.multigravity/.env`, restart `mgy`, and scan `mgy pair` once on your phone.
