/**
 * Multigravity APNs Stateless Push Relay (Cloudflare Worker)
 *
 * 架构：零存储、无状态 APNs HTTP/2 签名中继
 * 作用：接收自托管 mgy 网关发来的 iOS 原生通知与灵动岛 (Live Activity) 更新请求，
 *      使用存储在 Cloudflare Worker Secrets 中的 Apple APNs .p8 私钥签发 ES256 JWT，
 *      通过原生 HTTP/2 转发至 Apple APNs (api.push.apple.com / api.sandbox.push.apple.com)，
 *      并将送达结果与失效状态 (410 Unregistered) 透传回网关。
 *
 * 安全与隐私边界：
 *   - 不建数据库、不落盘、不记录任何通知正文与设备映射；
 *   - apns-topic 严格锁定为 Worker Secret 中配置的 APNS_BUNDLE_ID，绝不接受客户端覆盖；
 *   - ES256 JWT 在 Worker Isolate 内存中缓存 45 分钟（Apple 允许最长 50 分钟），单次转发 CPU < 1ms。
 *
 * 必需环境变量 / Secrets (Worker -> Settings -> Variables and Secrets):
 *   - APNS_PRIVATE_KEY : [Secret] Apple AuthKey_XXXXXXXXXX.p8 完整 PEM 内容
 *   - APNS_KEY_ID      : [Text]   10 位 Key ID
 *   - APNS_TEAM_ID     : [Text]   Apple Developer Team ID (如 7ZW5X4CFNP)
 *   - APNS_BUNDLE_ID   : [Text]   主 App Bundle ID (如 com.ghsaimo.antigravity)
 */

const APNS_PROD_HOST = "https://api.push.apple.com";
const APNS_SANDBOX_HOST = "https://api.sandbox.push.apple.com";
const JWT_CACHE_TTL_MS = 45 * 60 * 1000; // 45 minutes (< Apple 50-minute refresh ceiling)
const MAX_REQUEST_BYTES = 8 * 1024;      // 8 KB request guard
const MAX_APNS_PAYLOAD_BYTES = 4096;     // Apple APNs hard limit (4 KB)
const HEX_TOKEN_RE = /^[0-9a-fA-F]{64,256}$/;

// Isolate-level in-memory cache (zero persistence, zero PII)
let cachedSigningKey = null;
let cachedKeyFingerprint = "";
let cachedJWT = "";
let cachedJWTIssuedAt = 0;

const TOKEN_THROTTLE_MAX_ENTRIES = 2000;
const TOKEN_ROUTINE_UPDATE_MIN_MS = 1500;
const TOKEN_WINDOW_MS = 10_000;
const TOKEN_MAX_BURST_PER_WINDOW = 12;
const tokenThrottleMap = new Map(); // tokenKey -> { lastRoutineAt, windowStartAt, count }

function checkTokenThrottle(rawToken, event, priority) {
  const now = Date.now();
  // Key only on non-reversible prefix + tail length; evict stale entries when full
  const key = `${rawToken.slice(0, 16)}:${rawToken.slice(-8)}`;
  if (tokenThrottleMap.size >= TOKEN_THROTTLE_MAX_ENTRIES && !tokenThrottleMap.has(key)) {
    for (const [k, v] of tokenThrottleMap) {
      if (now - v.windowStartAt > 60_000) {
        tokenThrottleMap.delete(k);
      }
      if (tokenThrottleMap.size < TOKEN_THROTTLE_MAX_ENTRIES / 2) break;
    }
    if (tokenThrottleMap.size >= TOKEN_THROTTLE_MAX_ENTRIES) {
      tokenThrottleMap.clear();
    }
  }

  let entry = tokenThrottleMap.get(key);
  if (!entry || now - entry.windowStartAt >= TOKEN_WINDOW_MS) {
    entry = {
      lastRoutineAt: entry ? entry.lastRoutineAt : 0,
      windowStartAt: now,
      count: 0,
    };
    tokenThrottleMap.set(key, entry);
  }

  // Coalesce duplicate low-priority routine updates within 1.5s without hitting Apple APNs
  if (event === "update" && priority === "5" && now - entry.lastRoutineAt < TOKEN_ROUTINE_UPDATE_MIN_MS) {
    return { action: "coalesce" };
  }

  entry.count += 1;
  if (entry.count > TOKEN_MAX_BURST_PER_WINDOW) {
    return { action: "ratelimit" };
  }

  if (event === "update" && priority === "5") {
    entry.lastRoutineAt = now;
  }
  return { action: "allow" };
}

export default {
  async fetch(request, env) {
    const url = new URL(request.url);

    if (request.method === "OPTIONS") {
      return new Response(null, {
        status: 204,
        headers: corsHeaders(),
      });
    }

    if (request.method === "GET" && (url.pathname === "/" || url.pathname === "/healthz")) {
      const configured = Boolean(
        (env.APNS_PRIVATE_KEY || "").trim() &&
        (env.APNS_KEY_ID || "").trim() &&
        (env.APNS_TEAM_ID || "").trim() &&
        (env.APNS_BUNDLE_ID || "").trim()
      );
      return jsonResponse({
        status: configured ? "ok" : "unconfigured",
        service: "Multigravity APNs Stateless Push Relay",
        version: "1.0.0",
        configured,
      });
    }

    if (request.method !== "POST") {
      return jsonResponse({ ok: false, error: "method not allowed" }, 405);
    }

    // Enforce strict request size cap before parsing
    const contentLength = Number(request.headers.get("content-length") || "0");
    if (contentLength > MAX_REQUEST_BYTES) {
      return jsonResponse({ ok: false, error: "payload too large" }, 413);
    }

    let rawText;
    try {
      rawText = await request.text();
    } catch {
      return jsonResponse({ ok: false, error: "failed to read request body" }, 400);
    }
    if (new TextEncoder().encode(rawText).byteLength > MAX_REQUEST_BYTES) {
      return jsonResponse({ ok: false, error: "payload too large" }, 413);
    }

    let body;
    try {
      body = JSON.parse(rawText);
    } catch {
      return jsonResponse({ ok: false, error: "invalid json" }, 400);
    }

    if (url.pathname === "/v1/push/alert") {
      return handleAlertPush(body, env);
    }

    if (url.pathname === "/v1/push/live-activity") {
      return handleLiveActivityPush(body, env);
    }

    return jsonResponse({ ok: false, error: "not found" }, 404);
  },
};

async function handleAlertPush(body, env) {
  const cfg = readAPNsConfig(env);
  if (!cfg) {
    return jsonResponse({ ok: false, error: "relay not configured" }, 500);
  }

  const deviceToken = String(body.device_token || "").trim();
  if (!HEX_TOKEN_RE.test(deviceToken)) {
    return jsonResponse({ ok: false, error: "invalid device_token" }, 400);
  }

  const title = String(body.title || "").trim();
  const bodyText = String(body.body || "").trim();
  if (!title && !bodyText) {
    return jsonResponse({ ok: false, error: "title or body is required" }, 400);
  }

  const alert = {};
  if (title) alert.title = title;
  if (body.subtitle && String(body.subtitle).trim()) {
    alert.subtitle = String(body.subtitle).trim();
  }
  if (bodyText) alert.body = bodyText;

  const aps = {
    alert,
    sound: String(body.sound || "default").trim() || "default",
  };

  const level = normalizeInterruptionLevel(body.interruption_level);
  if (level) {
    aps["interruption-level"] = level;
  }
  if (typeof body.badge === "number" && body.badge >= 0) {
    aps.badge = Math.floor(body.badge);
  }
  if (body.thread_id && String(body.thread_id).trim()) {
    aps["thread-id"] = String(body.thread_id).trim();
  }
  if (body.category && String(body.category).trim()) {
    aps.category = String(body.category).trim();
  }

  const apnsPayload = { aps };
  if (body.cascade_id && String(body.cascade_id).trim()) {
    apnsPayload.cascadeId = String(body.cascade_id).trim();
  }
  if (body.url && String(body.url).trim()) {
    apnsPayload.url = String(body.url).trim();
  }
  if (body.category && String(body.category).trim()) {
    apnsPayload.category = String(body.category).trim();
  }

  const payloadStr = JSON.stringify(apnsPayload);
  if (new TextEncoder().encode(payloadStr).byteLength > MAX_APNS_PAYLOAD_BYTES) {
    return jsonResponse({ ok: false, error: "apns payload exceeds 4KB limit" }, 400);
  }

  const extraHeaders = {
    "apns-push-type": "alert",
    "apns-topic": cfg.bundleId,
    "apns-priority": "10",
  };
  if (body.collapse_id && String(body.collapse_id).trim()) {
    extraHeaders["apns-collapse-id"] = String(body.collapse_id).trim().slice(0, 64);
  }

  return dispatchToAPNs(cfg, body.env, deviceToken, extraHeaders, payloadStr);
}

async function handleLiveActivityPush(body, env) {
  const cfg = readAPNsConfig(env);
  if (!cfg) {
    return jsonResponse({ ok: false, error: "relay not configured" }, 500);
  }

  const pushToken = String(body.push_token || "").trim();
  if (!HEX_TOKEN_RE.test(pushToken)) {
    return jsonResponse({ ok: false, error: "invalid push_token" }, 400);
  }

  const event = String(body.event || "").trim().toLowerCase();
  if (event !== "start" && event !== "update" && event !== "end") {
    return jsonResponse({ ok: false, error: "event must be start, update, or end" }, 400);
  }

  if (!body.content_state || typeof body.content_state !== "object") {
    return jsonResponse({ ok: false, error: "content_state object is required" }, 400);
  }

  const nowSec = Math.floor(Date.now() / 1000);
  const timestamp = typeof body.timestamp === "number" && body.timestamp > 0
    ? Math.floor(body.timestamp)
    : nowSec;

  const aps = {
    timestamp,
    event,
    "content-state": body.content_state,
  };

  if (event === "start") {
    const attributesType = String(body.attributes_type || "").trim();
    if (!attributesType || !body.attributes || typeof body.attributes !== "object") {
      return jsonResponse({ ok: false, error: "start event requires attributes_type and attributes" }, 400);
    }
    aps["attributes-type"] = attributesType;
    aps.attributes = body.attributes;
    if (body.input_push_token) {
      aps["input-push-token"] = 1;
    }
  }

  if (typeof body.stale_date === "number" && body.stale_date > 0) {
    aps["stale-date"] = Math.floor(body.stale_date);
  }
  if (event === "end" && typeof body.dismissal_date === "number" && body.dismissal_date >= 0) {
    aps["dismissal-date"] = Math.floor(body.dismissal_date);
  }

  // Optional alert for Live Activity (only included if caller explicitly supplies title or body).
  // Omit aps.alert.sound unless explicitly provided so Push-to-Start launches silently.
  if (body.alert && typeof body.alert === "object") {
    const laAlert = {};
    if (body.alert.title) laAlert.title = String(body.alert.title);
    if (body.alert.body) laAlert.body = String(body.alert.body);
    const laSound = body.alert.sound ? String(body.alert.sound).trim() : "";
    if (laSound) {
      laAlert.sound = laSound;
    }
    if (laAlert.title || laAlert.body) {
      aps.alert = laAlert;
      // No explicit sound: send an empty sound so iOS does not fall back to the default chime
      // (e.g. Push-to-Start fired when a message is sent).
      if (!laSound) {
        laAlert.sound = "";
      }
    }
  }

  const payloadStr = JSON.stringify({ aps });
  if (new TextEncoder().encode(payloadStr).byteLength > MAX_APNS_PAYLOAD_BYTES) {
    return jsonResponse({ ok: false, error: "live activity payload exceeds 4KB limit" }, 400);
  }

  const priority = body.priority === 5 || String(body.priority) === "5" ? "5" : "10";
  const throttle = checkTokenThrottle(pushToken, event, priority);
  if (throttle.action === "coalesce") {
    return jsonResponse({
      ok: true,
      status: 200,
      throttled: true,
      env: String(body.env || "").trim().toLowerCase() === "sandbox" ? "sandbox" : "production",
    });
  }
  if (throttle.action === "ratelimit") {
    return jsonResponse(
      {
        ok: false,
        status: 429,
        reason: "TooManyRequests",
        unregistered: false,
      },
      429
    );
  }

  const extraHeaders = {
    "apns-push-type": "liveactivity",
    "apns-topic": `${cfg.bundleId}.push-type.liveactivity`,
    "apns-priority": priority,
  };

  return dispatchToAPNs(cfg, body.env, pushToken, extraHeaders, payloadStr);
}

async function dispatchToAPNs(cfg, apnsEnv, token, extraHeaders, payloadStr) {
  let jwt;
  try {
    jwt = await getOrMintAPNsJWT(cfg);
  } catch (err) {
    return jsonResponse({ ok: false, error: `jwt sign failed: ${err.message}` }, 500);
  }

  const isSandbox = String(apnsEnv || "").trim().toLowerCase() === "sandbox";
  const host = isSandbox ? APNS_SANDBOX_HOST : APNS_PROD_HOST;
  const endpoint = `${host}/3/device/${token}`;

  const headers = {
    authorization: `bearer ${jwt}`,
    "content-type": "application/json",
    ...extraHeaders,
  };

  let lastErr = null;
  for (let attempt = 0; attempt < 2; attempt++) {
    try {
      const resp = await fetch(endpoint, {
        method: "POST",
        headers,
        body: payloadStr,
      });

      const apnsId = resp.headers.get("apns-id") || "";
      if (resp.status === 200) {
        return jsonResponse({
          ok: true,
          status: 200,
          apns_id: apnsId,
          env: isSandbox ? "sandbox" : "production",
        });
      }

      const errText = await resp.text();
      let reason = "";
      try {
        const parsed = JSON.parse(errText);
        reason = parsed.reason || "";
      } catch {
        reason = errText.slice(0, 200);
      }

      // Retry once on transient 5xx from Apple or expired provider token (invalidate cache)
      if (attempt === 0 && (resp.status >= 500 || reason === "ExpiredProviderToken")) {
        if (reason === "ExpiredProviderToken") {
          cachedJWT = "";
          cachedJWTIssuedAt = 0;
          headers.authorization = `bearer ${await getOrMintAPNsJWT(cfg)}`;
        }
        continue;
      }

      const unregistered =
        resp.status === 410 ||
        reason === "Unregistered" ||
        reason === "BadDeviceToken" ||
        reason === "DeviceTokenNotForTopic" ||
        reason === "BadEnvironmentKeyInToken" ||
        reason === "TopicDisallowed" ||
        reason === "ExpiredToken";

      return jsonResponse(
        {
          ok: false,
          status: resp.status,
          apns_id: apnsId,
          reason,
          unregistered,
          env: isSandbox ? "sandbox" : "production",
        },
        resp.status >= 400 && resp.status < 600 ? resp.status : 502
      );
    } catch (err) {
      lastErr = err;
      if (attempt === 0) {
        continue;
      }
    }
  }

  return jsonResponse(
    {
      ok: false,
      status: 502,
      error: `apns upstream error: ${lastErr ? lastErr.message : "unknown"}`,
    },
    502
  );
}

function readAPNsConfig(env) {
  const privateKey = (env.APNS_PRIVATE_KEY || "").trim();
  const keyId = (env.APNS_KEY_ID || "").trim();
  const teamId = (env.APNS_TEAM_ID || "").trim();
  const bundleId = (env.APNS_BUNDLE_ID || "").trim();
  if (!privateKey || !keyId || !teamId || !bundleId) {
    return null;
  }
  return { privateKey, keyId, teamId, bundleId };
}

function normalizeInterruptionLevel(raw) {
  const v = String(raw || "").trim().toLowerCase();
  if (v === "time-sensitive" || v === "timesensitive") return "time-sensitive";
  if (v === "passive") return "passive";
  if (v === "active") return "active";
  return "";
}

async function getOrMintAPNsJWT(cfg) {
  const now = Date.now();
  const fingerprint = `${cfg.teamId}:${cfg.keyId}:${cfg.privateKey.length}`;
  if (
    cachedJWT &&
    cachedKeyFingerprint === fingerprint &&
    now - cachedJWTIssuedAt < JWT_CACHE_TTL_MS
  ) {
    return cachedJWT;
  }

  if (!cachedSigningKey || cachedKeyFingerprint !== fingerprint) {
    cachedSigningKey = await importP8PrivateKey(cfg.privateKey);
    cachedKeyFingerprint = fingerprint;
  }

  const header = base64UrlEncodeString(JSON.stringify({ alg: "ES256", kid: cfg.keyId }));
  const payload = base64UrlEncodeString(
    JSON.stringify({ iss: cfg.teamId, iat: Math.floor(now / 1000) })
  );
  const signingInput = `${header}.${payload}`;

  // Web Crypto ECDSA P-256 outputs raw IEEE P1363 (R || S, 64 bytes), matching APNs ES256 JWT spec directly.
  const signature = await crypto.subtle.sign(
    { name: "ECDSA", hash: { name: "SHA-256" } },
    cachedSigningKey,
    new TextEncoder().encode(signingInput)
  );

  const jwt = `${signingInput}.${base64UrlEncodeBytes(new Uint8Array(signature))}`;
  cachedJWT = jwt;
  cachedJWTIssuedAt = now;
  return jwt;
}

async function importP8PrivateKey(pem) {
  // Support both literal newlines and escaped "\n" in secrets
  const normalized = pem.replace(/\\n/g, "\n");
  const base64 = normalized
    .replace(/-----BEGIN PRIVATE KEY-----/g, "")
    .replace(/-----END PRIVATE KEY-----/g, "")
    .replace(/\s+/g, "");
  const binary = atob(base64);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i);
  }
  return crypto.subtle.importKey(
    "pkcs8",
    bytes.buffer,
    { name: "ECDSA", namedCurve: "P-256" },
    false,
    ["sign"]
  );
}

function base64UrlEncodeString(str) {
  return base64UrlEncodeBytes(new TextEncoder().encode(str));
}

function base64UrlEncodeBytes(bytes) {
  let binary = "";
  for (let i = 0; i < bytes.length; i++) {
    binary += String.fromCharCode(bytes[i]);
  }
  return btoa(binary).replace(/=/g, "").replace(/\+/g, "-").replace(/\//g, "_");
}

function corsHeaders() {
  return {
    "Access-Control-Allow-Origin": "*",
    "Access-Control-Allow-Methods": "GET, POST, OPTIONS",
    "Access-Control-Allow-Headers": "Content-Type",
  };
}

function jsonResponse(data, status = 200) {
  return new Response(JSON.stringify(data), {
    status,
    headers: {
      "Content-Type": "application/json; charset=utf-8",
      "Cache-Control": "no-store",
      ...corsHeaders(),
    },
  });
}
