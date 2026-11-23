/**
 * Multigravity APNs Stateless Push Relay (Cloudflare Worker)
 *
 * 架构：零存储、无状态 APNs HTTP/2 签名中继
 * 安全与隐私边界 (MED-06 加固):
 *   - 不建数据库、不落盘、不记录任何通知正文与设备映射；
 *   - apns-topic 严格锁定为 Worker Secret 中配置的 APNS_BUNDLE_ID，绝不接受客户端覆盖；
 *   - Alert 与 Live Activity 双路径均强制执行单 Token 突发限流 + 单 IP 滑动窗口限流（FIFO 淘汰防清表绕过）；
 *   - 移除浏览器跨域通配符 CORS (*)，且限制跳转 `url` 仅允许应用内安全协议 (`antigravity://`, `agy://`) 或站内相对路径 (`/`)。
 */

const APNS_PROD_HOST = "https://api.push.apple.com";
const APNS_SANDBOX_HOST = "https://api.sandbox.push.apple.com";
const JWT_CACHE_TTL_MS = 45 * 60 * 1000; // 45 minutes (< Apple 50-minute refresh ceiling)
const MAX_REQUEST_BYTES = 8 * 1024;      // 8 KB request guard
const MAX_APNS_PAYLOAD_BYTES = 4096;     // Apple APNs hard limit (4 KB)
const HEX_TOKEN_RE = /^[0-9a-fA-F]{64,256}$/;
const SAFE_APP_SCHEME_RE = /^(?:agy|antigravity):\/\/[A-Za-z0-9._~:/?#\[\]@!$&'()*+,;=%-]+$/;
const SAFE_BG_ACTION_RE = /^[a-z0-9][a-z0-9_.-]{0,63}$/i;

// Isolate-level in-memory cache (zero persistence, zero PII)
let cachedSigningKey = null;
let cachedKeyFingerprint = "";
let cachedJWT = "";
let cachedJWTIssuedAt = 0;

const TOKEN_THROTTLE_MAX_ENTRIES = 2000;
const TOKEN_ROUTINE_UPDATE_MIN_MS = 1500;
const TOKEN_WINDOW_MS = 10_000;
const TOKEN_MAX_BURST_PER_WINDOW = 12;
const ALERT_MAX_BURST_PER_WINDOW = 8;
const tokenThrottleMap = new Map(); // tokenKey -> { lastRoutineAt, windowStartAt, count }

const IP_THROTTLE_MAX_ENTRIES = 4096;
const IP_WINDOW_MS = 10_000;
const IP_MAX_REQUESTS_PER_WINDOW = 40;
const ipThrottleMap = new Map(); // ipKey -> { windowStartAt, count }

function evictMapFIFO(map, maxEntries, windowMs, now) {
  if (map.size < maxEntries) return;
  for (const [k, v] of map) {
    if (now - v.windowStartAt > windowMs) {
      map.delete(k);
    }
    if (map.size < maxEntries) return;
  }
  while (map.size >= maxEntries) {
    const oldestKey = map.keys().next().value;
    if (oldestKey === undefined) break;
    map.delete(oldestKey);
  }
}

function checkIPThrottle(clientIP) {
  const now = Date.now();
  const key = String(clientIP || "unknown").slice(0, 64);
  if (!ipThrottleMap.has(key)) {
    evictMapFIFO(ipThrottleMap, IP_THROTTLE_MAX_ENTRIES, 60_000, now);
  }
  let entry = ipThrottleMap.get(key);
  if (!entry || now - entry.windowStartAt >= IP_WINDOW_MS) {
    entry = { windowStartAt: now, count: 0 };
  }
  entry.count += 1;
  ipThrottleMap.delete(key);
  ipThrottleMap.set(key, entry);
  return entry.count <= IP_MAX_REQUESTS_PER_WINDOW;
}

function checkTokenThrottle(rawToken, event, priority) {
  const now = Date.now();
  const kind = event === "alert" ? "a" : event === "background" ? "bg" : "la";
  const key = `${kind}:${rawToken.slice(0, 16)}:${rawToken.slice(-8)}`;
  if (!tokenThrottleMap.has(key)) {
    evictMapFIFO(tokenThrottleMap, TOKEN_THROTTLE_MAX_ENTRIES, 60_000, now);
  }

  let entry = tokenThrottleMap.get(key);
  if (!entry || now - entry.windowStartAt >= TOKEN_WINDOW_MS) {
    entry = {
      lastRoutineAt: entry ? entry.lastRoutineAt : 0,
      windowStartAt: now,
      count: 0,
    };
  }

  // Coalesce duplicate low-priority routine updates within 1.5s without hitting Apple APNs
  if (event === "update" && priority === "5" && now - entry.lastRoutineAt < TOKEN_ROUTINE_UPDATE_MIN_MS) {
    return { action: "coalesce" };
  }

  entry.count += 1;
  tokenThrottleMap.delete(key);
  tokenThrottleMap.set(key, entry);

  const maxBurst = (event === "alert" || event === "background") ? ALERT_MAX_BURST_PER_WINDOW : TOKEN_MAX_BURST_PER_WINDOW;
  if (entry.count > maxBurst) {
    return { action: "ratelimit" };
  }

  if (event === "update" && priority === "5") {
    entry.lastRoutineAt = now;
  }
  return { action: "allow" };
}

function isSafePushTargetURL(raw) {
  const u = String(raw || "").trim();
  if (!u || u.length > 512) return false;
  if (/[\r\n\t\\]/.test(u) || u.includes("..")) return false;
  if (u.startsWith("/")) {
    return !u.startsWith("//");
  }
  return SAFE_APP_SCHEME_RE.test(u);
}

async function verifyOptionalRelayAuth(request, env) {
  const requiredSecret = (env.APNS_RELAY_AUTH_TOKEN || "").trim();
  if (!requiredSecret) {
    return true;
  }
  const authHeader = String(request.headers.get("authorization") || "").trim();
  const match = /^Bearer\s+(.+)$/i.exec(authHeader);
  if (!match || !match[1]) {
    return false;
  }
  const enc = new TextEncoder();
  const [da, db] = await Promise.all([
    crypto.subtle.digest("SHA-256", enc.encode(match[1].trim())),
    crypto.subtle.digest("SHA-256", enc.encode(requiredSecret)),
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

export default {
  async fetch(request, env) {
    const url = new URL(request.url);

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
        version: "1.2.0",
        configured,
      });
    }

    if (request.method !== "POST") {
      return jsonResponse({ ok: false, error: "method not allowed" }, 405);
    }

    if (!(await verifyOptionalRelayAuth(request, env))) {
      return jsonResponse({ ok: false, error: "unauthorized" }, 401);
    }

    const clientIP = request.headers.get("CF-Connecting-IP") || "unknown";
    if (!checkIPThrottle(clientIP)) {
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

    if (url.pathname === "/v1/push/background") {
      return handleBackgroundPush(body, env);
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

  if (body.url && String(body.url).trim()) {
    const candidateUrl = String(body.url).trim();
    if (!isSafePushTargetURL(candidateUrl)) {
      return jsonResponse({ ok: false, error: "disallowed url scheme (only relative '/' or app deep-links permitted)" }, 400);
    }
  }

  const throttle = checkTokenThrottle(deviceToken, "alert", "10");
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

async function handleBackgroundPush(body, env) {
  const cfg = readAPNsConfig(env);
  if (!cfg) {
    return jsonResponse({ ok: false, error: "relay not configured" }, 500);
  }

  const deviceToken = String(body.device_token || "").trim();
  if (!HEX_TOKEN_RE.test(deviceToken)) {
    return jsonResponse({ ok: false, error: "invalid device_token" }, 400);
  }

  const action = String(body.action || "").trim();
  if (!SAFE_BG_ACTION_RE.test(action)) {
    return jsonResponse({ ok: false, error: "invalid or missing action" }, 400);
  }

  const throttle = checkTokenThrottle(deviceToken, "background", "5");
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

  const apnsPayload = {
    aps: {
      "content-available": 1,
    },
    action,
    category: action,
  };
  if (body.cascade_id && String(body.cascade_id).trim()) {
    apnsPayload.cascadeId = String(body.cascade_id).trim().slice(0, 128);
  }
  if (body.collapse_id && String(body.collapse_id).trim()) {
    apnsPayload.collapseId = String(body.collapse_id).trim().slice(0, 64);
  }

  const payloadStr = JSON.stringify(apnsPayload);
  if (new TextEncoder().encode(payloadStr).byteLength > MAX_APNS_PAYLOAD_BYTES) {
    return jsonResponse({ ok: false, error: "apns payload exceeds 4KB limit" }, 400);
  }

  const extraHeaders = {
    "apns-push-type": "background",
    "apns-topic": cfg.bundleId,
    "apns-priority": "5",
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

function jsonResponse(data, status = 200) {
  return new Response(JSON.stringify(data), {
    status,
    headers: {
      "Content-Type": "application/json; charset=utf-8",
      "Cache-Control": "no-store",
    },
  });
}
