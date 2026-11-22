/**
 * Multigravity Public Tunnel Dispatcher (Cloudflare Serverless Worker)
 *
 * 架构：方案 A+ — Cloudflare 原生隧道与多 Zone 自动溢流 + 僵尸隧道 GC + TOFU 归属绑定 + Relay V1 协议预埋
 * 作用：
 *   1. 为 Multigravity 网关客户端自动按机器密钥创建、检索专属永久 Cloudflare Tunnel；
 *   2. 支持单 Zone（向后兼容 CF_ZONE_ID + BASE_DOMAIN）与多 Zone 域名池（CF_ZONES），
 *      在单域名触发免费版 200 条 DNS 配额上限时自动 GC 并溢流至备用域名，吃满单账号 1,000 Tunnel 配额；
 *   3. 自动回收长期失联的僵尸隧道与孤儿 DNS 记录（支持 Cron 定时触发 + 请求惰性触发 + 配额满即时触发）；
 *   4. 向前兼容的调度协议：读取客户端 capabilities / client_version 与 Cloudflare request.cf 地理区域，
 *      返回 mode / region / relay_endpoint / zone_domain，预埋无状态 HMAC 鉴权的自建中继（relay_v1）灰度/溢流入口。
 *
 * 安全加固 (CRIT-02):
 *   - 计算 256-bit `ownerHash = SHA256("mgy-owner:" + machineId)` 写入 Cloudflare Tunnel `metadata.owner_hash`；
 *   - 命中已有同名 `mgy-<8hex>` 隧道时：若存量隧道尚无 `metadata.owner_hash` 则自动补写绑定（TOFU 平滑迁移）；
 *     若已存在则使用恒定时间比较强校验，彻底阻断 32-bit `shortId` 碰撞接管；
 *   - 内置单 IP 滑动窗口注册限流（FIFO 满容淘汰）与 `INVITE_CODE` SHA-256 恒定时间校验。
 */

const WORKER_VERSION = "1.7.0";
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

/**
 * Parses configured Cloudflare DNS zones.
 * Supports:
 *   1. Legacy single zone via CF_ZONE_ID + BASE_DOMAIN (always preserved as primary zone[0] when set).
 *   2. Multi-zone pool via CF_ZONES:
 *      - JSON array: `[{"zone_id":"...","base_domain":"jiuge.space"}, ...]`
 *      - Compact string: `zoneId1:jiuge.space,zoneId2:mgy1.xyz`
 */
export function parseZonesConfig(env) {
  const zones = [];
  const seenZoneIds = new Set();
  const seenDomains = new Set();

  const pushZone = (zoneId, baseDomain) => {
    const zid = String(zoneId || "").trim();
    const dom = String(baseDomain || "").trim().toLowerCase().replace(/^\.+|\.+$/g, "");
    if (!zid || !dom) return;
    if (seenZoneIds.has(zid) || seenDomains.has(dom)) return;
    seenZoneIds.add(zid);
    seenDomains.add(dom);
    zones.push({ zoneId: zid, baseDomain: dom });
  };

  const legacyZoneId = (env.CF_ZONE_ID || env.CLOUDFLARE_ZONE_ID || "").trim();
  const legacyBaseDomain = (env.BASE_DOMAIN || "jiuge.space").trim().toLowerCase();
  if (legacyZoneId && legacyBaseDomain) {
    pushZone(legacyZoneId, legacyBaseDomain);
  }

  const rawZones = String(env.CF_ZONES || env.CLOUDFLARE_ZONES || "").trim();
  if (rawZones) {
    if (rawZones.startsWith("[")) {
      try {
        const parsed = JSON.parse(rawZones);
        if (Array.isArray(parsed)) {
          for (const item of parsed) {
            if (item && typeof item === "object") {
              pushZone(item.zone_id || item.zoneId || item.id, item.base_domain || item.baseDomain || item.domain);
            }
          }
        }
      } catch (e) {
        console.error("Failed to parse CF_ZONES JSON:", e);
      }
    } else {
      for (const token of rawZones.split(",")) {
        const trimmed = token.trim();
        if (!trimmed) continue;
        const sepIdx = trimmed.indexOf(":");
        if (sepIdx <= 0) continue;
        pushZone(trimmed.slice(0, sepIdx), trimmed.slice(sepIdx + 1));
      }
    }
  }

  return zones;
}

/**
 * Parses a JSON map or single scalar value for regional relay config.
 */
export function parseRegionMap(raw) {
  const trimmed = String(raw || "").trim();
  if (!trimmed) return {};
  if (trimmed.startsWith("{")) {
    try {
      const obj = JSON.parse(trimmed);
      if (obj && typeof obj === "object" && !Array.isArray(obj)) {
        const out = {};
        for (const [k, v] of Object.entries(obj)) {
          const key = String(k).trim().toLowerCase();
          const val = String(v || "").trim();
          if (key && val) out[key] = val;
        }
        return out;
      }
    } catch (e) {
      console.error("Failed to parse region map JSON:", e);
      return {};
    }
  }
  return { default: trimmed };
}

/**
 * Maps Cloudflare request.cf continent metadata to coarse routing region ("ap" | "us" | "eu" | "auto").
 */
export function detectClientRegion(request) {
  const continent = String(request?.cf?.continent || "").trim().toUpperCase();
  switch (continent) {
    case "AS":
    case "OC":
      return "ap";
    case "EU":
    case "AF":
      return "eu";
    case "NA":
    case "SA":
      return "us";
    default:
      return "auto";
  }
}

/**
 * Orders candidate zones for new tunnel allocation.
 * - "spillover" (default): always try primary zone[0] first, then overflow to backup zones when quotaExceeded.
 * - "hash": deterministically start at (shortId % zones.length), then spill over to remaining zones.
 */
export function selectOrderedZones(zones, shortId, strategy = "spillover") {
  if (!Array.isArray(zones) || zones.length <= 1) {
    return Array.isArray(zones) ? zones.slice() : [];
  }
  if (String(strategy || "").toLowerCase() !== "hash") {
    return zones.slice();
  }
  const hashNum = parseInt(String(shortId || "0").slice(0, 8), 16);
  const startIdx = Number.isFinite(hashNum) ? Math.abs(hashNum) % zones.length : 0;
  const ordered = [];
  for (let i = 0; i < zones.length; i++) {
    ordered.push(zones[(startIdx + i) % zones.length]);
  }
  return ordered;
}

function resolveZoneFromMetadata(zones, meta) {
  if (!Array.isArray(zones) || zones.length === 0) return null;
  if (meta && typeof meta === "object") {
    const metaZoneId = String(meta.zone_id || "").trim();
    const metaDomain = String(meta.zone_domain || "").trim().toLowerCase();
    if (metaZoneId) {
      const byId = zones.find((z) => z.zoneId === metaZoneId);
      if (byId) return byId;
    }
    if (metaDomain) {
      const byDom = zones.find((z) => z.baseDomain === metaDomain);
      if (byDom) return byDom;
    }
  }
  return zones[0];
}

function parseCapabilities(raw) {
  if (!Array.isArray(raw) || raw.length === 0) {
    return new Set(["cloudflare"]);
  }
  const caps = new Set();
  for (const item of raw) {
    const s = String(item || "").trim().toLowerCase();
    if (s) caps.add(s);
  }
  if (caps.size === 0) caps.add("cloudflare");
  return caps;
}

/**
 * Parses the multi-VPS relay pool configuration.
 * Primary format: `RELAY_POOL` JSON array:
 *   [{"id":"us1","region":"us","priority":10,"endpoint":"wss://relay.jiuge.space/_mgy_tunnel","health_url":"https://relay.jiuge.space/healthz","base_domain":"jiuge.space","subdomain_prefix":"r-"}]
 * Backward-compatible fallback: synthesizes pool nodes from RELAY_ENDPOINTS + RELAY_BASE_DOMAINS + RELAY_SUBDOMAIN_PREFIX.
 */
export function parseRelayPool(env) {
  const nodes = [];
  const seenIds = new Set();
  const defaultPrefix = String(env.RELAY_SUBDOMAIN_PREFIX || "").trim().toLowerCase();

  const deriveHealthUrl = (endpoint) => {
    try {
      const u = new URL(endpoint);
      const scheme = u.protocol === "wss:" ? "https:" : (u.protocol === "ws:" ? "http:" : u.protocol);
      return `${scheme}//${u.host}/healthz`;
    } catch {
      return "";
    }
  };

  const pushNode = (raw) => {
    if (!raw || typeof raw !== "object") return;
    if (raw.enabled === false) return;
    const endpoint = String(raw.endpoint || "").trim();
    let baseDomain = String(raw.base_domain || raw.baseDomain || "").trim().toLowerCase().replace(/^\.+|\.+$/g, "");
    let prefix = raw.subdomain_prefix !== undefined
      ? String(raw.subdomain_prefix || "").trim().toLowerCase()
      : (raw.subdomainPrefix !== undefined ? String(raw.subdomainPrefix || "").trim().toLowerCase() : defaultPrefix);

    // Also accept compact `host_template` (e.g. "r-{sid}.jiuge.space" or "r-{sid}.jp1.jiuge.space")
    const hostTemplate = String(raw.host_template || raw.hostTemplate || "").trim().toLowerCase();
    if (!baseDomain && hostTemplate.includes("{sid}.")) {
      const idx = hostTemplate.indexOf("{sid}.");
      prefix = hostTemplate.slice(0, idx);
      baseDomain = hostTemplate.slice(idx + "{sid}.".length).replace(/^\.+|\.+$/g, "");
    }
    if (!endpoint || !baseDomain) return;

    const region = String(raw.region || "auto").trim().toLowerCase() || "auto";
    const id = String(raw.id || `${region}-${nodes.length + 1}`).trim().toLowerCase();
    if (seenIds.has(id)) return;
    seenIds.add(id);
    const priority = Number.isFinite(Number(raw.priority)) ? Number(raw.priority) : (nodes.length + 1) * 10;
    const healthUrl = String(raw.health_url || raw.healthUrl || "").trim() || deriveHealthUrl(endpoint);
    const secretEnv = String(raw.secret_env || raw.secretEnv || "").trim();
    const signingSecret = (secretEnv && String(env[secretEnv] || "").trim()) || String(env.RELAY_SIGNING_SECRET || "").trim();
    nodes.push({
      id,
      region,
      priority,
      endpoint,
      healthUrl,
      baseDomain,
      subdomainPrefix: prefix,
      signingSecret,
    });
  };

  const rawPool = String(env.RELAY_POOL || "").trim();
  if (rawPool.startsWith("[")) {
    try {
      const parsed = JSON.parse(rawPool);
      if (Array.isArray(parsed)) {
        for (const item of parsed) pushNode(item);
      }
    } catch (e) {
      console.error("Failed to parse RELAY_POOL JSON:", e);
    }
  }

  if (nodes.length === 0) {
    const endpoints = parseRegionMap(env.RELAY_ENDPOINTS || "");
    const baseDomains = parseRegionMap(env.RELAY_BASE_DOMAINS || "");
    const keys = new Set([...Object.keys(endpoints), ...Object.keys(baseDomains)]);
    for (const key of keys) {
      const ep = endpoints[key] || endpoints.default || "";
      const bd = baseDomains[key] || baseDomains.default || "";
      if (!ep || !bd) continue;
      pushNode({
        id: key === "default" ? "primary" : key,
        region: key === "default" ? "auto" : key,
        priority: key === "default" ? 100 : 10,
        endpoint: ep,
        base_domain: bd,
        subdomain_prefix: defaultPrefix,
      });
    }
  }

  nodes.sort((a, b) => a.priority - b.priority || a.id.localeCompare(b.id));
  return nodes;
}

export function readWorkerConfig(env) {
  const CF_ACCOUNT_ID = (env.CF_ACCOUNT_ID || env.CLOUDFLARE_ACCOUNT_ID || "").trim();
  const CF_API_TOKEN = (env.CF_API_TOKEN || env.CLOUDFLARE_API_TOKEN || "").trim();
  const zones = parseZonesConfig(env);
  if (!CF_ACCOUNT_ID || !CF_API_TOKEN || zones.length === 0) {
    return null;
  }
  const staleDays = Math.max(3, Number(env.STALE_DAYS) || 14);
  const neverUsedDays = Math.max(1, Number(env.NEVER_USED_DAYS) || 3);
  const zoneStrategy = String(env.ZONE_STRATEGY || "spillover").trim().toLowerCase() === "hash" ? "hash" : "spillover";
  const rawRelayPolicy = String(env.RELAY_POLICY || "off").trim().toLowerCase();
  const relayPolicy = (rawRelayPolicy === "prefer" || rawRelayPolicy === "spillover") ? rawRelayPolicy : "off";
  const relayEndpoints = parseRegionMap(env.RELAY_ENDPOINTS || "");
  const relayBaseDomains = parseRegionMap(env.RELAY_BASE_DOMAINS || "");
  const relaySigningSecret = String(env.RELAY_SIGNING_SECRET || "").trim();
  const relaySubdomainPrefix = String(env.RELAY_SUBDOMAIN_PREFIX || "").trim().toLowerCase();
  const relayPool = parseRelayPool(env);

  return {
    accountId: CF_ACCOUNT_ID,
    apiToken: CF_API_TOKEN,
    zones,
    zoneId: zones[0].zoneId,
    baseDomain: zones[0].baseDomain,
    zoneStrategy,
    relayPolicy,
    relayEndpoints,
    relayBaseDomains,
    relaySigningSecret,
    relaySubdomainPrefix,
    relayPool,
    staleMs: staleDays * 86400 * 1000,
    neverUsedMs: neverUsedDays * 86400 * 1000,
    headers: {
      "Authorization": `Bearer ${CF_API_TOKEN}`,
      "Content-Type": "application/json",
    },
  };
}

export default {
  async fetch(request, env, ctx) {
    const url = new URL(request.url);

    if (request.method === "OPTIONS") {
      return new Response(null, { status: 204 });
    }

    if (request.method === "GET" && (url.pathname === "/" || url.pathname === "/healthz")) {
      const cfg = readWorkerConfig(env);
      const zones = cfg ? cfg.zones : parseZonesConfig(env);
      const pool = cfg ? cfg.relayPool : parseRelayPool(env);
      return jsonResponse({
        status: "ok",
        service: "Multigravity Cloudflare Tunnel Dispatcher",
        version: WORKER_VERSION,
        architecture: "Scheme A+ (Multi-Zone Cloudflare Tunnel + Multi-Node E2EE Relay Pool + Auto GC)",
        base_domain: zones[0]?.baseDomain || env.BASE_DOMAIN || "jiuge.space",
        zones_count: zones.length,
        zone_strategy: cfg ? cfg.zoneStrategy : "spillover",
        relay_policy: cfg ? cfg.relayPolicy : "off",
        relay_nodes_count: pool.length,
      });
    }

    if (request.method === "GET" && url.pathname === "/api/relay/pool") {
      const cfg = readWorkerConfig(env);
      const pool = cfg ? cfg.relayPool : parseRelayPool(env);
      const region = detectClientRegion(request);
      const ordered = orderRelayNodesForRegion(pool, region);
      return new Response(JSON.stringify({
        status: "ok",
        version: WORKER_VERSION,
        region,
        relay_policy: cfg ? cfg.relayPolicy : "off",
        nodes: ordered.map((n) => ({
          id: n.id,
          region: n.region,
          priority: n.priority,
          health_url: n.healthUrl,
          host_template: `${n.subdomainPrefix}{sid}.${n.baseDomain}`,
        })),
      }, null, 2), {
        status: 200,
        headers: {
          "Content-Type": "application/json; charset=utf-8",
          "Cache-Control": "public, max-age=60",
        },
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

async function handleTunnelRegister(request, env, ctx) {
  const cfg = readWorkerConfig(env);
  if (!cfg) {
    return jsonResponse({
      error: "Worker environment misconfigured. Please check CF_ACCOUNT_ID, CF_API_TOKEN, CF_ZONE_ID/CF_ZONES, BASE_DOMAIN.",
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
  const capabilities = parseCapabilities(body.capabilities);
  const region = detectClientRegion(request);

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
  const reqPort = String(body.target_port || "").trim();
  const targetPort = reqPort === "58900" ? reqPort : (env.TARGET_PORT || "58900");

  // Optional Relay V1 preferred dispatch (or explicit relay-only capability request for dual-track migration)
  const relayOnlyReq = capabilities.has("relay_v1") && !capabilities.has("cloudflare");
  if ((cfg.relayPolicy === "prefer" || (relayOnlyReq && cfg.relayPolicy !== "off")) &&
      capabilities.has("relay_v1") && canDispatchRelay(cfg, region)) {
    const relayResp = await buildRelayV1Response(cfg, shortId, ownerHash, region);
    if (relayResp) {
      return jsonResponse(relayResp);
    }
  }

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
      }

      const boundZone = resolveZoneFromMetadata(cfg.zones, existingMeta);
      const candidateZones = [boundZone, ...cfg.zones.filter((z) => z.zoneId !== boundZone.zoneId)];
      const provision = await provisionDnsAcrossZones(env, cfg, candidateZones, tunnelId, shortId);
      if (!provision.ok) {
        if (cfg.relayPolicy === "spillover" && capabilities.has("relay_v1") && canDispatchRelay(cfg, region)) {
          const relayResp = await buildRelayV1Response(cfg, shortId, ownerHash, region);
          if (relayResp) return jsonResponse(relayResp);
        }
        return jsonResponse({
          error: "Failed to bind Cloudflare DNS record for existing tunnel",
          details: provision.errors,
        }, 502);
      }

      const winZone = provision.zone;
      const subdomain = `${shortId}.${winZone.baseDomain}`;
      const needsMetaPatch = !storedOwnerHash ||
        existingMeta.zone_id !== winZone.zoneId ||
        existingMeta.zone_domain !== winZone.baseDomain;

      const tasks = [
        setTunnelConfigurations(cfg.accountId, tunnelId, subdomain, targetPort, cfg.headers),
      ];
      if (needsMetaPatch) {
        tasks.push(bindTunnelMetadata(cfg.accountId, tunnelId, existingMeta, {
          owner_hash: ownerHash,
          zone_id: winZone.zoneId,
          zone_domain: winZone.baseDomain,
        }, cfg.headers));
      }
      await Promise.all(tasks);
      rememberOwnerBinding(tunnelId, ownerHash);

      const token = await fetchTunnelToken(cfg.accountId, tunnelId, cfg.headers);
      if (token) {
        return jsonResponse({
          success: true,
          reused: true,
          mode: "cloudflare",
          region,
          relay_endpoint: "",
          zone_domain: winZone.baseDomain,
          tunnel_id: tunnelId,
          tunnel_name: tunnelName,
          subdomain,
          url: `https://${subdomain}`,
          token,
        });
      }
    }

    // 2. Create new Tunnel with cryptographic owner_hash + initial zone metadata
    const candidateZones = selectOrderedZones(cfg.zones, shortId, cfg.zoneStrategy);
    const primaryCand = candidateZones[0];

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
            zone_id: primaryCand.zoneId,
            zone_domain: primaryCand.baseDomain,
          },
        }),
      }
    );
    const createData = await createRes.json();
    if (!createData.success || !createData.result) {
      if (cfg.relayPolicy === "spillover" && capabilities.has("relay_v1") && canDispatchRelay(cfg, region)) {
        const relayResp = await buildRelayV1Response(cfg, shortId, ownerHash, region);
        if (relayResp) return jsonResponse(relayResp);
      }
      return jsonResponse({
        error: "Failed to create Cloudflare Tunnel",
        details: createData.errors,
      }, 502);
    }
    const tunnelId = createData.result.id;
    rememberOwnerBinding(tunnelId, ownerHash);

    // 3. Provision DNS across configured zones (with GC retry + automatic zone spillover)
    const provision = await provisionDnsAcrossZones(env, cfg, candidateZones, tunnelId, shortId);
    if (!provision.ok) {
      // Roll back newly created orphan tunnel so we do not leak an unroutable tunnel
      ownerBindCache.delete(tunnelId);
      await deleteTunnelSafe(cfg.accountId, tunnelId, cfg.headers);
      if (cfg.relayPolicy === "spillover" && capabilities.has("relay_v1") && canDispatchRelay(cfg, region)) {
        const relayResp = await buildRelayV1Response(cfg, shortId, ownerHash, region);
        if (relayResp) return jsonResponse(relayResp);
      }
      return jsonResponse({
        error: "Failed to provision Cloudflare DNS record (zone record quota may be full)",
        details: provision.errors,
      }, 502);
    }

    const winZone = provision.zone;
    const subdomain = `${shortId}.${winZone.baseDomain}`;
    const postTasks = [
      setTunnelConfigurations(cfg.accountId, tunnelId, subdomain, targetPort, cfg.headers),
    ];
    if (winZone.zoneId !== primaryCand.zoneId) {
      postTasks.push(bindTunnelMetadata(cfg.accountId, tunnelId, { owner_hash: ownerHash }, {
        owner_hash: ownerHash,
        zone_id: winZone.zoneId,
        zone_domain: winZone.baseDomain,
      }, cfg.headers));
    }
    await Promise.all(postTasks);

    const token = await fetchTunnelToken(cfg.accountId, tunnelId, cfg.headers);
    if (!token) {
      return jsonResponse({
        error: "Tunnel created but failed to retrieve run token",
      }, 502);
    }

    return jsonResponse({
      success: true,
      reused: false,
      mode: "cloudflare",
      region,
      relay_endpoint: "",
      zone_domain: winZone.baseDomain,
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

export function orderRelayNodesForRegion(pool, region) {
  if (!Array.isArray(pool) || pool.length === 0) return [];
  const norm = String(region || "auto").toLowerCase();
  return pool.slice().sort((a, b) => {
    const aMatch = (norm !== "auto" && a.region === norm) ? 0 : 1;
    const bMatch = (norm !== "auto" && b.region === norm) ? 0 : 1;
    if (aMatch !== bMatch) return aMatch - bMatch;
    if (a.priority !== b.priority) return a.priority - b.priority;
    return a.id.localeCompare(b.id);
  });
}

function canDispatchRelay(cfg) {
  return Boolean(
    cfg &&
    Array.isArray(cfg.relayPool) &&
    cfg.relayPool.some((n) => Boolean(n.signingSecret || cfg.relaySigningSecret))
  );
}

async function buildRelayV1Response(cfg, shortId, ownerHash, region) {
  if (!canDispatchRelay(cfg)) {
    return null;
  }
  const ordered = orderRelayNodesForRegion(cfg.relayPool, region);
  const iat = Math.floor(Date.now() / 1000);
  const relayNodes = [];

  for (const node of ordered) {
    const secret = node.signingSecret || cfg.relaySigningSecret;
    if (!secret) continue;
    const subdomain = `${node.subdomainPrefix}${shortId}.${node.baseDomain}`;
    const token = await signRelayV1Token(secret, {
      v: 1,
      sid: shortId,
      oh: ownerHash.slice(0, 32),
      dom: subdomain,
      reg: node.region || region,
      iat,
    });
    relayNodes.push({
      id: node.id,
      region: node.region,
      priority: node.priority,
      endpoint: node.endpoint,
      health_url: node.healthUrl,
      subdomain,
      url: `https://${subdomain}`,
      token,
    });
  }
  if (relayNodes.length === 0) {
    return null;
  }

  const primary = relayNodes[0];
  return {
    success: true,
    reused: false,
    mode: "relay_v1",
    region,
    relay_endpoint: primary.endpoint,
    zone_domain: ordered[0].baseDomain,
    tunnel_id: `relay-${shortId}`,
    tunnel_name: `mgy-${shortId}`,
    subdomain: primary.subdomain,
    url: primary.url,
    token: primary.token,
    relay_nodes: relayNodes,
  };
}

export async function signRelayV1Token(secret, payload) {
  const enc = new TextEncoder();
  const payloadB64 = base64UrlEncode(enc.encode(JSON.stringify(payload)));
  const key = await crypto.subtle.importKey(
    "raw",
    enc.encode(secret),
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign"]
  );
  const sigBuf = await crypto.subtle.sign("HMAC", key, enc.encode(payloadB64));
  const sigB64 = base64UrlEncode(new Uint8Array(sigBuf));
  return `mgyr1.${payloadB64}.${sigB64}`;
}

function base64UrlEncode(bytes) {
  let binary = "";
  for (let i = 0; i < bytes.length; i++) {
    binary += String.fromCharCode(bytes[i]);
  }
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
}

async function provisionDnsAcrossZones(env, cfg, candidateZones, tunnelId, shortId) {
  let lastResult = { ok: false, quotaExceeded: false, errors: [{ message: "No DNS zones configured" }] };
  let gcAttempted = false;

  for (const zone of candidateZones) {
    if (!zone) continue;
    const subdomain = `${shortId}.${zone.baseDomain}`;
    let dnsRes = await syncTunnelDns(zone.zoneId, tunnelId, subdomain, cfg.headers);
    if (dnsRes.ok) {
      return { ok: true, zone };
    }
    if (dnsRes.quotaExceeded && !gcAttempted) {
      gcAttempted = true;
      await cleanupStaleTunnels(env);
      dnsRes = await syncTunnelDns(zone.zoneId, tunnelId, subdomain, cfg.headers);
      if (dnsRes.ok) {
        return { ok: true, zone };
      }
    }
    lastResult = dnsRes;
    if (!dnsRes.quotaExceeded) {
      // Non-quota transient API failure: stop rather than scattering records across zones
      break;
    }
  }
  return lastResult;
}

async function bindTunnelMetadata(accountId, tunnelId, existingMeta, patch, headers) {
  try {
    await fetch(
      `https://api.cloudflare.com/client/v4/accounts/${accountId}/cfd_tunnel/${tunnelId}`,
      {
        method: "PATCH",
        headers,
        body: JSON.stringify({
          metadata: {
            ...(existingMeta || {}),
            ...patch,
          },
        }),
      }
    );
  } catch (e) {
    console.error("Failed to patch tunnel metadata:", tunnelId, e);
  }
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
 * Reclaims stale `mgy-<8hex>` tunnels and orphaned `<8hex>.<zone.baseDomain>` CNAME records across all configured zones.
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
    if (deletedTunnels.length >= 10) {
      activeTunnelIds.add(t.id);
      continue;
    }

    const zone = resolveZoneFromMetadata(cfg.zones, t.metadata);
    const subdomain = `${shortId}.${zone.baseDomain}`;
    await deleteDnsByNameAndTunnel(zone.zoneId, subdomain, t.id, cfg.headers, deletedDns);
    const removed = await deleteTunnelSafe(cfg.accountId, t.id, cfg.headers);
    if (removed) {
      ownerBindCache.delete(t.id);
      deletedTunnels.push(t.name);
    } else {
      activeTunnelIds.add(t.id);
    }
  }

  // 3. Sweep orphaned <8hex>.<baseDomain> CNAME records pointing to deleted tunnels across configured zones
  let remainingPageBudget = 4;
  for (const zone of cfg.zones) {
    if (!tunnelListComplete || remainingPageBudget <= 0 || deletedDns.length >= 15) {
      break;
    }
    const escapedBase = zone.baseDomain.replace(/\./g, "\\.");
    const mgySubdomainRe = new RegExp(`^[0-9a-f]{8}\\.${escapedBase}$`);

    let dnsPage = 1;
    while (remainingPageBudget > 0 && dnsPage <= 4 && deletedDns.length < 15) {
      remainingPageBudget--;
      const res = await fetch(
        `https://api.cloudflare.com/client/v4/zones/${zone.zoneId}/dns_records?type=CNAME&per_page=100&page=${dnsPage}`,
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
            `https://api.cloudflare.com/client/v4/zones/${zone.zoneId}/dns_records/${rec.id}`,
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
