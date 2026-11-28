# Multigravity (`mgy`) IPv6 Direct Connection Guide

> 🌐 [简体中文](ipv6_direct_guide.md) | **English** | [日本語](ipv6_direct_guide_ja.md)

If your home broadband, campus network, office intranet, or cloud host has **IPv6**, you can enable **IPv4/IPv6 dual-stack listening** (and an optional **reverse-proxy-free native HTTPS port**) on the Multigravity gateway. Your phone can then connect directly to your computer over 5G/4G or any IPv6-capable network with lower latency and higher throughput—while automatically falling back to the official zero-knowledge E2EE relay whenever you are on an IPv4-only Wi-Fi network.

> **Version requirements**: Desktop gateway `mgy >= v1.2.9-hotfix.1` (and upcoming `v1.2.9+` stable releases). **Disabled by default (IPv4-only `tcp4`)**, opt-in with zero impact on existing users.  
> **Mobile apps**: No iOS / Android update is required for **DDNS + HTTPS** direct connections.

---

## Table of Contents

1. [Upgrade the Gateway](#1-upgrade-the-gateway)
2. [Choose Your Direct Connection Mode](#2-choose-your-direct-connection-mode)
3. [Prerequisites: Verify IPv6 & Router Firewall](#3-prerequisites-verify-ipv6--router-firewall)
4. [Option 1 (Recommended): DDNS + Native Gateway HTTPS (No Reverse Proxy)](#4-option-1-recommended-ddns--native-gateway-https-no-reverse-proxy)
5. [Option 2: DDNS + Reverse Proxy (Caddy / Nginx / Lucky)](#5-option-2-ddns--reverse-proxy-caddy--nginx--lucky)
6. [Option 3: Tailscale / Private LAN IPv6 Direct](#6-option-3-tailscale--private-lan-ipv6-direct)
7. [Mobile App Setup & Automatic Routing Priority](#7-mobile-app-setup--automatic-routing-priority)
8. [Advanced Options: Pure Direct / Force E2EE / Auto-Start Service](#8-advanced-options-pure-direct--force-e2ee--auto-start-service)
9. [CLI Flags & Environment Variables Reference](#9-cli-flags--environment-variables-reference)
10. [FAQ & Troubleshooting](#10-faq--troubleshooting)

---

## 1. Upgrade the Gateway

Run the following command on the computer hosting `mgy`:

```bash
# Install the hotfix build with opt-in IPv6 dual-stack & native TLS support
mgy update v1.2.9-hotfix.1

# If you already installed an earlier v1.2.9-hotfix.1 build, add -force to refresh:
mgy update -force v1.2.9-hotfix.1
```

Verify that the new flags are available:

```bash
mgy help | grep -E "ipv6|tls-"
```

You should see `-ipv6`, `-tls-cert`, `-tls-key`, and `-tls-port`.

---

## 2. Choose Your Direct Connection Mode

| Mode | Best For | Domain & TLS Cert Needed? | Recommendation |
| :--- | :--- | :--- | :--- |
| **Option 1: DDNS + Native Gateway HTTPS** | You already have a domain & ACME cert and don't want to run Nginx/Caddy | Yes (served directly by `mgy` with automatic cert hot-reload) | ⭐⭐⭐⭐⭐ Simplest |
| **Option 2: DDNS + Caddy / Nginx / Lucky** | You already run a reverse proxy on your host, NAS, or router | Managed automatically by your reverse proxy | ⭐⭐⭐⭐⭐ Best if you already have a proxy |
| **Option 3: Tailscale / Private IPv6** | You use Tailscale mesh VPN or only need private LAN IPv6 | No (encrypted tunnel or local network) | ⭐⭐⭐⭐ Zero-domain setup |

> **Why is cleartext HTTP on a bare public IPv6 literal (e.g. `http://[2408:...]:58900`) not supported?**
> 1. **Security**: Plaintext HTTP over the public internet exposes device Bearer tokens and conversation data in transit;
> 2. **Platform policies**: Both iOS App Transport Security (ATS) and the Android cleartext security interceptor block cleartext HTTP to public addresses;
> 3. **Address stability**: Residential ISP IPv6 prefixes and OS privacy extensions rotate periodically—using a DDNS hostname (AAAA record) keeps connections stable without re-pairing.

---

## 3. Prerequisites: Verify IPv6 & Router Firewall

### 3.1 Check Whether Your Computer Has a Public IPv6 Address

- **macOS / Linux**:
  ```bash
  curl -6 -s https://api64.ipify.org && echo
  ```
- **Windows (PowerShell)**:
  ```powershell
  curl.exe -6 -s https://api64.ipify.org
  ```

If this prints a global unicast address (such as `2001:...`, `2408:...`, `2409:...`, `240e:...`, or `2606:...`), your computer has public IPv6 connectivity.

### 3.2 Allow Inbound Traffic on Your Router's IPv6 Firewall

Most residential routers and ISP ONTs enable an **IPv6 Firewall** by default, blocking unsolicited inbound TCP connections.

- **Recommended practice**: Add a targeted inbound TCP allow rule in your router's IPv6 Firewall for **only your HTTPS port** (for example `58943` in Option 1, or your reverse proxy's HTTPS port in Option 2). **Do not expose HTTP port `58900` to the internet and do not disable the entire IPv6 firewall.**
- **Avoid ports 80 / 443 on residential ISPs**: Many residential ISPs block inbound TCP `80` and `443`. Use a high port such as the default **`58943`**.

---

## 4. Option 1 (Recommended): DDNS + Native Gateway HTTPS (No Reverse Proxy)

`mgy` can serve **native TLS 1.2+** directly on a dedicated port (default `58943`) while keeping the main HTTP port (`58900`) untouched for local CLI commands (`mgy pair` / `list` / `clear`) and Wi-Fi LAN access.

### Step 1: Prepare a DDNS AAAA Record & PEM Certificate

- Use `ddns-go`, Lucky, router DDNS, or your DNS provider API to point your domain's **AAAA record** (e.g. `home.example.com`) to your computer's public IPv6 address.
- Obtain a free Let's Encrypt / ZeroSSL certificate via `acme.sh`, `certbot`, `ddns-go`, or Lucky, yielding two PEM files:
  - Full certificate chain: `fullchain.pem` (**always use the full chain, not a leaf-only `cert.pem`**)
  - Private key: `privkey.pem`

### Step 2: Edit `~/.multigravity/.env`

Open (or create) `~/.multigravity/.env` (on Windows: `%USERPROFILE%\.multigravity\.env`) and add:

```env
# Enable IPv4 / IPv6 dual-stack listening
MULTIGRAVITY_ENABLE_IPV6=1

# Enable dedicated native HTTPS port (supports automatic ACME cert hot-reload)
MGY_TLS_CERT=/path/to/fullchain.pem
MGY_TLS_KEY=/path/to/privkey.pem
MGY_TLS_PORT=58943

# Optional: display your DDNS domain directly in the startup banner
DDNS_HOST=home.example.com
```

You can also test temporarily via CLI flags:

```bash
mgy -ipv6 -tls-cert /path/to/fullchain.pem -tls-key /path/to/privkey.pem -tls-port 58943 -ddns home.example.com
```

### Step 3: Restart the Gateway & Check the Startup Banner

After restarting `mgy`, the startup banner displays two confirmation lines:

```text
  ➜  Listen Stack: IPv4 / IPv6 dual-stack enabled (:58900 | Tailscale/proxy/DDNS direct ready)
  ➜  Native HTTPS: https://home.example.com:58943 (TLS 1.2+, auto-reload cert, listening on :58943)
```

### Step 4: Verify External IPv6 HTTPS Reachability

After opening inbound TCP `58943` in your router's IPv6 firewall, test from cellular (5G/4G) or another IPv6 host:

```bash
curl -6 -i https://home.example.com:58943/healthz
```

An `HTTP/2 200` (or `HTTP/1.1 200`) response with `{"status":"ok", ...}` confirms your IPv6 HTTPS direct path is working end-to-end.

> **Automatic certificate renewal**: `mgy` checks cert/key file mtime and size every 30 seconds and hot-reloads renewed ACME certificates automatically **without restarting the gateway**. If a file is momentarily half-written during renewal, the previous valid certificate is safely retained.

---

## 5. Option 2: DDNS + Reverse Proxy (Caddy / Nginx / Lucky)

If you already run Caddy, Nginx, or Lucky for HTTPS termination and automatic certificate renewal, simply reverse proxy to `mgy` on port `58900`.

### Step 1: Decide Whether Dual-Stack Listening Is Needed

- **Reverse proxy runs on the same computer as `mgy`**: Proxy straight to `http://127.0.0.1:58900`—you don't even need `MULTIGRAVITY_ENABLE_IPV6=1`.
- **Reverse proxy runs on another device (router / NAS) connecting to `mgy` over IPv6**: Add the following to `~/.multigravity/.env` on the computer running `mgy`:
  ```env
  MULTIGRAVITY_ENABLE_IPV6=1
  ```
  Then point the proxy upstream to `http://[<host-ipv6>]:58900` (or the host's LAN IPv4 `http://<lan-ipv4>:58900`).

### Step 2: Reverse Proxy Examples

> Multigravity streams live updates and events over **WebSocket**. Ensure your proxy forwards `Upgrade` / `Connection` headers and uses a generous read timeout.

#### Caddy (Auto HTTPS + Native WebSocket Support)

```caddyfile
home.example.com:58943 {
    reverse_proxy 127.0.0.1:58900
}
```

#### Nginx

```nginx
server {
    listen [::]:58943 ssl http2;
    listen 58943 ssl http2;
    server_name home.example.com;

    ssl_certificate     /path/to/fullchain.pem;
    ssl_certificate_key /path/to/privkey.pem;
    ssl_protocols       TLSv1.2 TLSv1.3;

    client_max_body_size 64m;

    location / {
        proxy_pass http://127.0.0.1:58900;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Required for WebSocket and live streaming
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
        proxy_buffering off;
    }
}
```

#### Lucky (Popular on Soft-Routers / NAS)

1. Configure your IPv6 DDNS (`AAAA` record) under **Dynamic DNS**;
2. Issue an ACME certificate for your domain under **SSL/TLS Certificates**;
3. Create a **Web Service** rule:
   - Listening type: `tcp6` (or `tcp4` + `tcp6`), port: high port such as `58943`, enable **TLS**;
   - Sub-rule frontend domain: `home.example.com`, backend address: `http://127.0.0.1:58900` (or your host's LAN address);
   - Lucky supports WebSocket passthrough out of the box—save and apply.

---

## 6. Option 3: Tailscale / Private LAN IPv6 Direct

If you use Tailscale and want to connect via a Tailscale IPv6 address (`fd7a:115c:a1e0::/48`) or a private LAN ULA IPv6 address (`fc00::/7`):

1. Enable dual-stack listening (or bind exclusively to your Tailscale IPv6 address) in `~/.multigravity/.env`:
   ```env
   MULTIGRAVITY_ENABLE_IPV6=1
   # Optional: bind exclusively to your Tailscale IPv6 interface:
   # MULTIGRAVITY_HOST=fd7a:115c:a1e0::xxxx
   ```
2. Restart `mgy`;
3. In the mobile app's **Custom Server Address**, enter `http://[fd7a:115c:a1e0::xxxx]:58900` (or continue using Tailscale IPv4 `http://100.x.x.x:58900` / MagicDNS `http://your-host.tailxxxx.ts.net:58900`).

---

## 7. Mobile App Setup & Automatic Routing Priority

1. If you haven't paired yet, pair your phone as usual by scanning the QR code printed by `mgy` (or `mgy pair`);
2. Open **Settings → Network → Custom Server Address** in the mobile app;
3. Enter your HTTPS direct URL and save, for example:
   ```text
   https://home.example.com:58943
   ```
4. No extra switches are needed. The app automatically selects routes in this order:

```text
1. Wi-Fi LAN Direct (lowest latency on the same Wi-Fi)
   ↓ Leaving home / switching to 5G cellular
2. Custom Server Address (your IPv6 HTTPS direct route)
   ↓ On an IPv4-only office Wi-Fi or blocked network
3. Official Zero-Knowledge E2EE Relay (seamless automatic fallback)
```

---

## 8. Advanced Options: Pure Direct / Force E2EE / Auto-Start Service

### 8.1 Disable the Official Cloud Relay (Pure Self-Hosted Direct Mode)

If you want only LAN + your own IPv6 direct path with zero cloud relay fallback, add to `~/.multigravity/.env`:

```env
CF_TUNNEL_ENABLED=0
```

> Tip: Keep `CF_TUNNEL_ENABLED=1` unless every network you roam onto reliably supports IPv6. Keeping it enabled never lowers the priority of your direct IPv6 route and ensures automatic fallback on IPv4-only Wi-Fi.

### 8.2 Enforce Zero-Knowledge E2EE Behind a Reverse Proxy or CDN

If traffic passes through a third-party reverse proxy or CDN and you want the proxy to remain completely blind to plaintext payloads, add:

```env
MGY_FORCE_E2EE=1
```

All external / proxied requests will require zero-knowledge E2EE (iOS and Android automatically detect `HTTP 426` and upgrade transparently, while local CLI and LAN direct connections remain unaffected).

### 8.3 Using with `mgy service` Auto-Start Daemon

- If you put the settings in `~/.multigravity/.env`, simply restart the background service:
  ```bash
  mgy service restart
  ```
- You can also bake `-ipv6` directly into the service manifest on install:
  ```bash
  mgy service install -ipv6
  ```

---

## 9. CLI Flags & Environment Variables Reference

| CLI Flag | Environment Variable (First Wins) | Default | Description |
| :--- | :--- | :--- | :--- |
| `-ipv6` | `MULTIGRAVITY_ENABLE_IPV6` / `MGY_ENABLE_IPV6` | `false` (`0`) | Enable IPv4/IPv6 dual-stack listening (default is IPv4-only `tcp4`) |
| `-host <addr>` | `MULTIGRAVITY_HOST` | `""` | Bind address; passing an IPv6 literal (`::`, `::1`, `[fd7a:...]`) automatically enables IPv6 listening |
| `-port <port>` | `MULTIGRAVITY_PORT` | `58900` | Main HTTP / WebSocket port (for local CLI, LAN, and reverse proxies) |
| `-tls-cert <path>` | `MGY_TLS_CERT` / `MULTIGRAVITY_TLS_CERT` | `""` | Path to native HTTPS full certificate chain (PEM); must be paired with `-tls-key` |
| `-tls-key <path>` | `MGY_TLS_KEY` / `MULTIGRAVITY_TLS_KEY` | `""` | Path to native HTTPS private key (PEM); auto-reloads on ACME renewal |
| `-tls-port <port>` | `MGY_TLS_PORT` / `MULTIGRAVITY_TLS_PORT` | `58943` | Dedicated native HTTPS listening port (must differ from `-port`) |
| `-ddns <domain>` | `DDNS_HOST` | `""` | Public DDNS domain for banner display and pairing endpoints |
| — | `CF_TUNNEL_ENABLED` | `1` | Set to `0` to disable the default cloud relay tunnel |
| — | `MGY_FORCE_E2EE` | `0` | Set to `1` to enforce zero-knowledge E2EE for all external / proxied requests |

---

## 10. FAQ & Troubleshooting

1. **Local checks succeed, but connecting over 5G times out?**
   - In 90% of cases, an **ONT or router IPv6 firewall** is dropping inbound packets. Verify:
     - If your ISP modem/ONT is in router mode rather than bridge mode, check its IPv6 firewall as well;
     - Confirm your router allows inbound TCP on your target port (e.g. `58943`);
     - Verify your DDNS `AAAA` record resolves to your computer's **current active** global IPv6 address (not an expired temporary privacy address or the router's own IPv6).
2. **Mobile app reports "SSL handshake failed" or untrusted certificate?**
   - Make sure `MGY_TLS_CERT` points to the **full certificate chain (`fullchain.pem`)** rather than a leaf-only `cert.pem`;
   - Verify the hostname matches the certificate's SAN/CN, uses `https://`, and targets your TLS port (`58943` or proxy port) rather than the HTTP port `58900`.
3. **Mobile app says public addresses do not allow cleartext HTTP?**
   - By design (iOS ATS and Android `LanCleartextSecurityInterceptor`), cleartext HTTP is forbidden on public addresses. Configure HTTPS via Option 1 or Option 2 and use `https://`.
4. **Why can't `-tls-port` be the same as `-port` (e.g. both `58900`)?**
   - Keeping a dedicated HTTPS port ensures local CLI tools (`mgy pair`, `mgy list`, `mgy clear`, `mgy service status`) and LAN Wi-Fi HTTP continue working seamlessly, while letting you expose only the HTTPS port in your router's IPv6 firewall.
5. **After enabling `-ipv6`, do IPv4 LAN and `127.0.0.1` still work?**
   - Yes. With `-host ""` or `-host ::`, Go binds a standard **IPv4/IPv6 dual-stack socket**, so `127.0.0.1`, `192.168.x.x`, and IPv6 clients all connect simultaneously.
6. **Will my connection drop on an IPv4-only office Wi-Fi?**
   - As long as `CF_TUNNEL_ENABLED` is left enabled (`1`), the mobile app automatically falls back to the official E2EE relay when the custom IPv6 endpoint is unreachable, and switches back to IPv6 direct once IPv6 returns.
