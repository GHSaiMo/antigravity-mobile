/**
 * Multigravity Cloudflare Tunnel Dispatcher (Serverless Worker)
 *
 * 架构：方案 A - Cloudflare 原生隧道与 DNS 自动化联动（客户端智能 Anycast 优选加速 + 僵尸隧道自动 GC + TOFU 归属绑定）
 * 作用：为 Multigravity 网关客户端自动按机器密钥创建、检索专属永久 Cloudflare Tunnel，并自动回收长期失联的僵尸隧道与孤儿 DNS 记录，防止触碰免费版 200 条 DNS 配额上限。
 * 安全加固 (CRIT-02):
 *   - 计算 256-bit `ownerHash = SHA256("mgy-owner:" + machineId)` 写入 Cloudflare Tunnel `metadata.owner_hash`；
 *   - 命中已有同名 `mgy-<8hex>` 隧道时：若存量隧道尚无 `metadata.owner_hash` 则自动补写绑定（TOFU 平滑迁移）；若已存在则使用恒定时间比较强校验，彻底阻断 32-bit `shortId` 碰撞接管；
 *   - 内置单 IP 滑动窗口注册限流（FIFO 满容淘汰）与 `INVITE_CODE` SHA-256 恒定时间校验。
 */

const MGY_TUNNEL_NAME_RE = /^mgy-([0-9a-f]{8})$/;
const OPPORTUNISTIC_GC_INTERVAL_MS = 6 * 60 * 60 * 1000; // 6 hours per Worker isolate
const MAX_REGISTER_BODY_BYTES = 4096;

const IP_RATE_WINDOW_MS = 60_000;
const IP_MAX_REGISTERS_PER_WINDOW = 10;
const IP_RATE_MAX_ENTRIES = 4096;
const ipRegisterMap = new Map(); // clientIP -> { windowStartAt, count }

// Isolate-level immediate TOFU cache so concurrent/rapid re-queries before CF API metadata propagation stay protected
const ownerBindCache = new Map(); // tunnelId -> ownerHash (bounded FIFO)
const OWNER_CACHE_MAX_ENTRIES = 4096;

let lastOpportunisticGcAt = 0;

function checkIPRegisterLimit(clientIP) {
  const now = Date.now();
  const key = String(clientIP || "unknown").slice(0, 64);
  if (!ipRegisterMap.has(key) && ipRegisterMap.size >= IP_RATE_MAX_ENTRIES) {
    for (const [k, v] of ipRegisterMap) {
      if (now - v.windowStartAt >= IP_RATE_WINDOW_MS) {
        ipRegisterMap.delete(k);
      }
      if (ipRegisterMap.size < IP_RATE_MAX_ENTRIES) break;
    }
    if (ipRegisterMap.size >= IP_RATE_MAX_ENTRIES) {
      const oldestKey = ipRegisterMap.keys().next().value;
      if (oldestKey !== undefined) {
        ipRegisterMap.delete(oldestKey);
      }
    }
  }

  let entry = ipRegisterMap.get(key);
  if (!entry || now - entry.windowStartAt >= IP_RATE_WINDOW_MS) {
    entry = { windowStartAt: now, count: 0 };
  }
  entry.count += 1;
  // Refresh insertion order for active keys
  ipRegisterMap.delete(key);
  ipRegisterMap.set(key, entry);
  return entry.count <= IP_MAX_REGISTERS_PER_WINDOW;
}

function rememberOwnerBinding(tunnelId, ownerHash) {
  if (!tunnelId || !ownerHash) return;
  if (!ownerBindCache.has(tunnelId) && ownerBindCache.size >= OWNER_CACHE_MAX_ENTRIES) {
    const oldest = ownerBindCache.keys().next().value;
    if (oldest !== undefined) ownerBindCache.delete(oldest);
  }
  ownerBindCache.delete(tunnelId);
  ownerBindCache.set(tunnelId, ownerHash);
}

export default {
  async fetch(request, env, ctx) {
    const url = new URL(request.url);

    if (request.method === "OPTIONS") {
      return new Response(null, { status: 204 });
    }

    if (request.method === "GET" && (url.pathname === "/" || url.pathname === "/healthz")) {
      return jsonResponse({
        status: "ok",
        service: "Multigravity Cloudflare Tunnel Dispatcher",
        version: "1.5.0",
        architecture: "Scheme A (Native Tunnel + Anycast Edge Acceleration + Auto GC + TOFU Ownership)",
        base_domain: env.BASE_DOMAIN || "jiuge.space",
      });
    }

    if (url.pathname === "/api/tunnel/register" && request.method === "POST") {
      return handleTunnelRegister(request, env, ctx);
    }

    return jsonResponse({ error: "Not Found" }, 404);
  },

  async scheduled(event, env, ctx) {
    ctx.waitUntil(cleanupStaleTunnels(env));
  },
};

function readWorkerConfig(env) {
  const CF_ACCOUNT_ID = (env.CF_ACCOUNT_ID || env.CLOUDFLARE_ACCOUNT_ID || "").trim();
  const CF_API_TOKEN = (env.CF_API_TOKEN || env.CLOUDFLARE_API_TOKEN || "").trim();
  const CF_ZONE_ID = (env.CF_ZONE_ID || env.CLOUDFLARE_ZONE_ID || "").trim();
  const BASE_DOMAIN = (env.BASE_DOMAIN || "jiuge.space").trim().toLowerCase();
  if (!CF_ACCOUNT_ID || !CF_API_TOKEN || !CF_ZONE_ID || !BASE_DOMAIN) {
    return null;
  }
  const staleDays = Math.max(3, Number(env.STALE_DAYS) || 14);
  const neverUsedDays = Math.max(1, Number(env.NEVER_USED_DAYS) || 3);
  return {
    accountId: CF_ACCOUNT_ID,
    apiToken: CF_API_TOKEN,
    zoneId: CF_ZONE_ID,
    baseDomain: BASE_DOMAIN,
    staleMs: staleDays * 86400 * 1000,
    neverUsedMs: neverUsedDays * 86400 * 1000,
    headers: {
      "Authorization": `Bearer ${CF_API_TOKEN}`,
      "Content-Type": "application/json",
    },
  };
}

async function handleTunnelRegister(request, env, ctx) {
  const cfg = readWorkerConfig(env);
  if (!cfg) {
    return jsonResponse({
      error: "Worker environment misconfigured. Please check CF_ACCOUNT_ID, CF_API_TOKEN, CF_ZONE_ID, BASE_DOMAIN.",
    }, 500);
  }

  const clientIP = request.headers.get("CF-Connecting-IP") || "unknown";
  if (!checkIPRegisterLimit(clientIP)) {
    return jsonResponse({ error: "Too many registration requests, please retry later" }, 429);
  }

  const contentLen = Number(request.headers.get("content-length") || "0");
  if (contentLen > MAX_REGISTER_BODY_BYTES) {
    return jsonResponse({ error: "Payload too large" }, 413);
  }

  let rawText;
  try {
    rawText = await request.text();
  } catch {
    return jsonResponse({ error: "Failed to read request body" }, 400);
  }
  if (new TextEncoder().encode(rawText).byteLength > MAX_REGISTER_BODY_BYTES) {
    return jsonResponse({ error: "Payload too large" }, 413);
  }

  let body;
  try {
    body = JSON.parse(rawText);
  } catch {
    return jsonResponse({ error: "Invalid JSON body" }, 400);
  }

  const machineId = String(body.machine_id || "").trim();
  const inviteCode = String(body.invite_code || request.headers.get("X-Invite-Code") || "").trim();

  if (machineId.length < 16 || machineId.length > 256) {
    return jsonResponse({ error: "machine_id must be between 16 and 256 characters" }, 400);
  }

  const requiredInvite = (env.INVITE_CODE || "").trim();
  if (requiredInvite !== "") {
    const inviteOK = await constantTimeStringEqual(inviteCode, requiredInvite);
    if (!inviteOK) {
      return jsonResponse({ error: "Invalid invite code" }, 403);
    }
  }

  // Opportunistic non-blocking background GC (at most once every 6h per isolate)
  const now = Date.now();
  if (ctx && typeof ctx.waitUntil === "function" && now - lastOpportunisticGcAt > OPPORTUNISTIC_GC_INTERVAL_MS) {
    lastOpportunisticGcAt = now;
    ctx.waitUntil(cleanupStaleTunnels(env).catch((e) => console.error("Background GC error:", e)));
  }

  const shortId = await generateStableShortId(machineId);
  const ownerHash = await computeOwnerHash(machineId);
  const tunnelName = `mgy-${shortId}`;
  const subdomain = `${shortId}.${cfg.baseDomain}`;
  const reqPort = String(body.target_port || "").trim();
  const targetPort = reqPort === "58900" ? reqPort : (env.TARGET_PORT || "58900");

  try {
    // 1. Check existing Tunnel by deterministic name
    const searchRes = await fetch(
      `https://api.cloudflare.com/client/v4/accounts/${cfg.accountId}/cfd_tunnel?name=${encodeURIComponent(tunnelName)}&is_deleted=false`,
      { headers: cfg.headers }
    );
    const searchData = await searchRes.json();

    if (searchData.success && Array.isArray(searchData.result) && searchData.result.length > 0) {
      const existingTunnel = searchData.result[0];
      const tunnelId = existingTunnel.id;
      const existingMeta = (existingTunnel && typeof existingTunnel.metadata === "object" && existingTunnel.metadata)
        ? existingTunnel.metadata
        : {};
      const storedOwnerHash = String(existingMeta.owner_hash || ownerBindCache.get(tunnelId) || "").trim();

      if (storedOwnerHash) {
        // CRIT-02: Enforce cryptographic ownership match before issuing tunnel token
        const ownerOK = await constantTimeStringEqual(ownerHash, storedOwnerHash);
        if (!ownerOK) {
          return jsonResponse({
            error: "Forbidden: tunnel ownership verification failed",
          }, 403);
        }
      } else {
        // TOFU (Trust On First Use): seamlessly bind legacy tunnel without metadata.owner_hash
        await bindTunnelOwnerMetadata(cfg.accountId, tunnelId, existingMeta, ownerHash, cfg.headers);
        rememberOwnerBinding(tunnelId, ownerHash);
      }

      const [, dnsSync] = await Promise.all([
        setTunnelConfigurations(cfg.accountId, tunnelId, subdomain, targetPort, cfg.headers),
        ensureTunnelDnsWithGcRetry(env, cfg, tunnelId, subdomain),
      ]);
      if (!dnsSync.ok) {
        return jsonResponse({
          error: "Failed to bind Cloudflare DNS record for existing tunnel",
          details: dnsSync.errors,
        }, 502);
      }

      const token = await fetchTunnelToken(cfg.accountId, tunnelId, cfg.headers);
      if (token) {
        rememberOwnerBinding(tunnelId, ownerHash);
        return jsonResponse({
          success: true,
          reused: true,
          tunnel_id: tunnelId,
          tunnel_name: tunnelName,
          subdomain,
          url: `https://${subdomain}`,
          token,
        });
      }
    }

    // 2. Create new Tunnel with cryptographic owner_hash metadata
    const rawSecret = new Uint8Array(32);
    crypto.getRandomValues(rawSecret);
    const tunnelSecretBase64 = btoa(String.fromCharCode(...rawSecret));

    const createRes = await fetch(
      `https://api.cloudflare.com/client/v4/accounts/${cfg.accountId}/cfd_tunnel`,
      {
        method: "POST",
        headers: cfg.headers,
        body: JSON.stringify({
          name: tunnelName,
          tunnel_secret: tunnelSecretBase64,
          metadata: {
            owner_hash: ownerHash,
          },
        }),
      }
    );
    const createData = await createRes.json();
    if (!createData.success || !createData.result) {
      return jsonResponse({
        error: "Failed to create Cloudflare Tunnel",
        details: createData.errors,
      }, 502);
    }
    const tunnelId = createData.result.id;
    rememberOwnerBinding(tunnelId, ownerHash);

    // 3. Configure Ingress & DNS (with automatic GC retry if DNS record quota is full)
    const [, dnsSync] = await Promise.all([
      setTunnelConfigurations(cfg.accountId, tunnelId, subdomain, targetPort, cfg.headers),
      ensureTunnelDnsWithGcRetry(env, cfg, tunnelId, subdomain),
    ]);
    if (!dnsSync.ok) {
      // Roll back newly created orphan tunnel so we do not leak an unroutable tunnel
      ownerBindCache.delete(tunnelId);
      await deleteTunnelSafe(cfg.accountId, tunnelId, cfg.headers);
      return jsonResponse({
        error: "Failed to provision Cloudflare DNS record (zone record quota may be full)",
        details: dnsSync.errors,
      }, 502);
    }

    const token = await fetchTunnelToken(cfg.accountId, tunnelId, cfg.headers);
    if (!token) {
      return jsonResponse({
        error: "Tunnel created but failed to retrieve run token",
      }, 502);
    }

    return jsonResponse({
      success: true,
      reused: false,
      tunnel_id: tunnelId,
      tunnel_name: tunnelName,
      subdomain,
      url: `https://${subdomain}`,
      token,
    });
  } catch (err) {
    return jsonResponse({
      error: "Internal error processing tunnel dispatch",
      message: err.message,
    }, 500);
  }
}

async function bindTunnelOwnerMetadata(accountId, tunnelId, existingMeta, ownerHash, headers) {
  try {
    await fetch(
      `https://api.cloudflare.com/client/v4/accounts/${accountId}/cfd_tunnel/${tunnelId}`,
      {
        method: "PATCH",
        headers,
        body: JSON.stringify({
          metadata: {
            ...existingMeta,
            owner_hash: ownerHash,
          },
        }),
      }
    );
  } catch (e) {
    console.error("Failed to backfill tunnel owner_hash metadata:", tunnelId, e);
  }
}

async function ensureTunnelDnsWithGcRetry(env, cfg, tunnelId, subdomain) {
  let dnsRes = await syncTunnelDns(cfg.zoneId, tunnelId, subdomain, cfg.headers);
  if (dnsRes.ok) {
    return dnsRes;
  }
  if (dnsRes.quotaExceeded) {
    await cleanupStaleTunnels(env);
    dnsRes = await syncTunnelDns(cfg.zoneId, tunnelId, subdomain, cfg.headers);
  }
  return dnsRes;
}

async function setTunnelConfigurations(accountId, tunnelId, subdomain, targetPort, headers) {
  const res = await fetch(
    `https://api.cloudflare.com/client/v4/accounts/${accountId}/cfd_tunnel/${tunnelId}/configurations`,
    {
      method: "PUT",
      headers,
      body: JSON.stringify({
        config: {
          ingress: [
            {
              hostname: subdomain,
              service: `http://localhost:${targetPort}`,
            },
            {
              service: `http://localhost:${targetPort}`,
            },
          ],
        },
      }),
    }
  );
  return res.json();
}

async function syncTunnelDns(zoneId, tunnelId, subdomain, headers) {
  const dnsTarget = `${tunnelId}.cfargotunnel.com`;

  try {
    const dnsSearchRes = await fetch(
      `https://api.cloudflare.com/client/v4/zones/${zoneId}/dns_records?name=${encodeURIComponent(subdomain)}`,
      { headers }
    );
    const dnsSearchData = await dnsSearchRes.json();

    if (dnsSearchData.success && Array.isArray(dnsSearchData.result) && dnsSearchData.result.length > 0) {
      const existing = dnsSearchData.result[0];
      if (existing.type === "CNAME" && existing.content === dnsTarget && existing.proxied) {
        return { ok: true };
      }
      const updateRes = await fetch(
        `https://api.cloudflare.com/client/v4/zones/${zoneId}/dns_records/${existing.id}`,
        {
          method: "PUT",
          headers,
          body: JSON.stringify({
            type: "CNAME",
            name: subdomain,
            content: dnsTarget,
            proxied: true,
            ttl: 1,
          }),
        }
      );
      const updateData = await updateRes.json();
      return parseDnsMutationResult(updateData);
    }

    const createRes = await fetch(
      `https://api.cloudflare.com/client/v4/zones/${zoneId}/dns_records`,
      {
        method: "POST",
        headers,
        body: JSON.stringify({
          type: "CNAME",
          name: subdomain,
          content: dnsTarget,
          proxied: true,
          ttl: 1,
        }),
      }
    );
    const createData = await createRes.json();
    return parseDnsMutationResult(createData);
  } catch (e) {
    return { ok: false, quotaExceeded: false, errors: [{ message: e.message }] };
  }
}

function parseDnsMutationResult(data) {
  if (data && data.success) {
    return { ok: true };
  }
  const errors = (data && Array.isArray(data.errors)) ? data.errors : [];
  const quotaExceeded = errors.some((err) => {
    const msg = String(err?.message || "").toLowerCase();
    return err?.code === 81045 || msg.includes("quota") || msg.includes("limit");
  });
  return { ok: false, quotaExceeded, errors };
}

/**
 * Reclaims stale `mgy-<8hex>` tunnels and orphaned `<8hex>.<baseDomain>` CNAME records.
 * Strictly ignores `hal9000` and all non-`mgy-<8hex>` custom subdomains.
 */
async function cleanupStaleTunnels(env) {
  const cfg = readWorkerConfig(env);
  if (!cfg) {
    return { ok: false, reason: "misconfigured" };
  }

  const now = Date.now();
  const activeTunnelIds = new Set();
  const deletedTunnels = [];
  const deletedDns = [];

  // 1. Paginate all non-deleted Cloudflare Tunnels in the account
  let page = 1;
  let tunnelListComplete = false;
  const allTunnels = [];
  while (page <= 10) {
    const res = await fetch(
      `https://api.cloudflare.com/client/v4/accounts/${cfg.accountId}/cfd_tunnel?is_deleted=false&per_page=100&page=${page}`,
      { headers: cfg.headers }
    );
    const data = await res.json();
    if (!data.success || !Array.isArray(data.result)) {
      break;
    }
    allTunnels.push(...data.result);
    if (data.result.length < 100) {
      tunnelListComplete = true;
      break;
    }
    page++;
  }

  // 2. Identify stale mgy-<8hex> tunnels
  for (const t of allTunnels) {
    const match = MGY_TUNNEL_NAME_RE.exec(String(t.name || ""));
    if (!match) {
      // Never touch non-mgy tunnels (e.g. hal9000)
      activeTunnelIds.add(t.id);
      continue;
    }

    const shortId = match[1];
    const hasLiveConns = Array.isArray(t.connections) && t.connections.length > 0;
    if (t.status === "healthy" || t.status === "degraded" || hasLiveConns) {
      activeTunnelIds.add(t.id);
      continue;
    }

    const lastActiveMs = t.conns_active_at ? Date.parse(t.conns_active_at) : NaN;
    const createdMs = t.created_at ? Date.parse(t.created_at) : NaN;

    const isStaleUsed = Number.isFinite(lastActiveMs) && (now - lastActiveMs > cfg.staleMs);
    const isStaleNeverUsed = !t.conns_active_at && Number.isFinite(createdMs) && (now - createdMs > cfg.neverUsedMs);

    if (!isStaleUsed && !isStaleNeverUsed) {
      activeTunnelIds.add(t.id);
      continue;
    }

    // Delete matching CNAME record + Tunnel (bounded per invocation to stay well within Worker subrequest limits)
    if (deletedTunnels.length >= 12) {
      activeTunnelIds.add(t.id);
      continue;
    }

    const subdomain = `${shortId}.${cfg.baseDomain}`;
    await deleteDnsByNameAndTunnel(cfg.zoneId, subdomain, t.id, cfg.headers, deletedDns);
    const removed = await deleteTunnelSafe(cfg.accountId, t.id, cfg.headers);
    if (removed) {
      ownerBindCache.delete(t.id);
      deletedTunnels.push(t.name);
    } else {
      activeTunnelIds.add(t.id);
    }
  }

  // 3. Sweep orphaned <8hex>.<baseDomain> CNAME records pointing to deleted tunnels
  const escapedBase = cfg.baseDomain.replace(/\./g, "\\.");
  const mgySubdomainRe = new RegExp(`^[0-9a-f]{8}\\.${escapedBase}$`);

  let dnsPage = 1;
  while (tunnelListComplete && dnsPage <= 4 && deletedDns.length < 15) {
    const res = await fetch(
      `https://api.cloudflare.com/client/v4/zones/${cfg.zoneId}/dns_records?type=CNAME&per_page=100&page=${dnsPage}`,
      { headers: cfg.headers }
    );
    const data = await res.json();
    if (!data.success || !Array.isArray(data.result) || data.result.length === 0) {
      break;
    }
    for (const rec of data.result) {
      if (deletedDns.length >= 20) break;
      const recName = String(rec.name || "").toLowerCase();
      const recContent = String(rec.content || "").toLowerCase();
      if (!mgySubdomainRe.test(recName) || !recContent.endsWith(".cfargotunnel.com")) {
        continue;
      }
      const refTunnelId = recContent.slice(0, -".cfargotunnel.com".length);
      if (refTunnelId && allTunnels.length > 0 && !activeTunnelIds.has(refTunnelId)) {
        const delRes = await fetch(
          `https://api.cloudflare.com/client/v4/zones/${cfg.zoneId}/dns_records/${rec.id}`,
          { method: "DELETE", headers: cfg.headers }
        );
        if (delRes.ok) {
          deletedDns.push(recName);
        }
      }
    }
    if (data.result.length < 100) {
      break;
    }
    dnsPage++;
  }

  return {
    ok: true,
    deleted_tunnels: deletedTunnels,
    deleted_dns: deletedDns,
  };
}

async function deleteDnsByNameAndTunnel(zoneId, subdomain, tunnelId, headers, deletedDns) {
  try {
    const res = await fetch(
      `https://api.cloudflare.com/client/v4/zones/${zoneId}/dns_records?type=CNAME&name=${encodeURIComponent(subdomain)}`,
      { headers }
    );
    const data = await res.json();
    if (!data.success || !Array.isArray(data.result)) return;
    const expectedTarget = `${tunnelId}.cfargotunnel.com`.toLowerCase();
    for (const rec of data.result) {
      if (String(rec.content || "").toLowerCase() === expectedTarget) {
        const delRes = await fetch(
          `https://api.cloudflare.com/client/v4/zones/${zoneId}/dns_records/${rec.id}`,
          { method: "DELETE", headers }
        );
        if (delRes.ok) {
          deletedDns.push(subdomain);
        }
      }
    }
  } catch (e) {
    console.error("Failed to delete tunnel DNS record:", subdomain, e);
  }
}

async function deleteTunnelSafe(accountId, tunnelId, headers) {
  try {
    await fetch(
      `https://api.cloudflare.com/client/v4/accounts/${accountId}/cfd_tunnel/${tunnelId}/connections`,
      { method: "DELETE", headers }
    );
    const delRes = await fetch(
      `https://api.cloudflare.com/client/v4/accounts/${accountId}/cfd_tunnel/${tunnelId}`,
      { method: "DELETE", headers }
    );
    const delData = await delRes.json();
    return Boolean(delData && delData.success);
  } catch (e) {
    console.error("Failed to delete tunnel:", tunnelId, e);
    return false;
  }
}

async function fetchTunnelToken(accountId, tunnelId, headers) {
  const tokenRes = await fetch(
    `https://api.cloudflare.com/client/v4/accounts/${accountId}/cfd_tunnel/${tunnelId}/token`,
    { headers }
  );
  const tokenData = await tokenRes.json();
  if (tokenData.success && tokenData.result) {
    return tokenData.result;
  }
  return null;
}

async function sha256Hex(input) {
  const msgUint8 = new TextEncoder().encode(input);
  const hashBuffer = await crypto.subtle.digest("SHA-256", msgUint8);
  const hashArray = Array.from(new Uint8Array(hashBuffer));
  return hashArray.map((b) => b.toString(16).padStart(2, "0")).join("");
}

async function generateStableShortId(input) {
  const hex = await sha256Hex(input.toLowerCase().trim());
  return hex.substring(0, 8);
}

async function computeOwnerHash(machineId) {
  return sha256Hex("mgy-owner:" + machineId.toLowerCase().trim());
}

async function constantTimeStringEqual(a, b) {
  if (!a || !b) {
    return false;
  }
  const enc = new TextEncoder();
  const [da, db] = await Promise.all([
    crypto.subtle.digest("SHA-256", enc.encode(String(a))),
    crypto.subtle.digest("SHA-256", enc.encode(String(b))),
  ]);
  if (typeof crypto.subtle.timingSafeEqual === "function") {
    return crypto.subtle.timingSafeEqual(da, db);
  }
  const ua = new Uint8Array(da);
  const ub = new Uint8Array(db);
  let diff = 0;
  for (let i = 0; i < ua.length; i++) {
    diff |= ua[i] ^ ub[i];
  }
  return diff === 0;
}

function jsonResponse(data, status = 200) {
  return new Response(JSON.stringify(data, null, 2), {
    status,
    headers: {
      "Content-Type": "application/json; charset=utf-8",
      "Cache-Control": "no-store",
    },
  });
}
