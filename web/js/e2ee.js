// Multigravity Zero-Knowledge E2EE Transport Layer (mgy-e2ee-v1)
// Loaded before core.js so both paired API calls and initial pairing transparently encrypt
// over relay_v1 hosts (`r-*`) using native Web Crypto API (ECDH P-256 + HKDF-SHA256 + AES-256-GCM).
(function () {
  "use strict";

  if (typeof window === "undefined" || !window.crypto || !window.crypto.subtle) {
    return;
  }

  const subtle = window.crypto.subtle;
  const nativeFetch = window.fetch.bind(window);
  const NativeWebSocket = window.WebSocket;

  const LS_DEVICE_ID = "agy_e2ee_device_id";
  const LS_KEY_B64 = "agy_e2ee_key_b64";
  const LS_BEARER = "agy_e2ee_bearer";
  const HEADER_DEVICE_ID = "X-Mgy-E2EE-Device";
  const HEADER_ACCEPT_ENC = "X-Mgy-E2EE-Enc";
  const HEADER_CAPS = "X-Mgy-E2EE-Caps";
  const CAP_WS_SEQ1 = "ws-seq1";
  const PAIR_SALT = new TextEncoder().encode("mgy-e2ee-pair-v1");

  const SUPPORTS_DEFLATE_RAW = (() => {
    try {
      if (typeof DecompressionStream === "undefined") return false;
      new DecompressionStream("deflate-raw");
      return true;
    } catch (_) {
      return false;
    }
  })();

  async function inflateRawBytes(u8) {
    if (!u8 || u8.length === 0) return new Uint8Array(0);
    const ds = new DecompressionStream("deflate-raw");
    const stream = new Blob([u8]).stream().pipeThrough(ds);
    return new Uint8Array(await new Response(stream).arrayBuffer());
  }

  const enc = new TextEncoder();
  const dec = new TextDecoder();

  let forceE2EEOrigin = false;
  let cachedGatewayPub = null;
  let cachedAesKey = null;
  let cachedAesKeyB64 = null;
  const serverCaps = new Set();
  let clockOffsetMs = 0;

  function observeCapsFromHeader(raw) {
    if (!raw || typeof raw !== "string") return;
    raw.split(",").forEach((t) => {
      const clean = t.trim().toLowerCase();
      if (clean) serverCaps.add(clean);
    });
  }

  function observeCaps(resp) {
    try {
      if (resp && resp.headers && typeof resp.headers.get === "function") {
        observeCapsFromHeader(resp.headers.get(HEADER_CAPS));
      }
    } catch (_) {}
  }

  function generateRequestId() {
    const bytes = window.crypto.getRandomValues(new Uint8Array(16));
    let hex = "";
    for (let i = 0; i < bytes.length; i++) {
      hex += bytes[i].toString(16).padStart(2, "0");
    }
    return hex;
  }

  function adjustedTimestampMs() {
    return Math.round(Date.now() + clockOffsetMs);
  }

  function isLocalHost(host) {
    const h = (host || "").toLowerCase();
    if (!h) return true;
    if (h === "localhost" || h === "127.0.0.1" || h === "::1" || h.endsWith(".local")) return true;
    if (h.startsWith("192.168.") || h.startsWith("10.") || h.startsWith("127.")) return true;
    if (h.startsWith("172.")) {
      const parts = h.split(".");
      const second = parseInt(parts[1] || "0", 10);
      if (second >= 16 && second <= 31) return true;
    }
    if (h.endsWith(".ts.net")) return true;
    return false;
  }

  function isRelayOrigin() {
    if (window.location.protocol !== "https:") return false;
    const h = (window.location.hostname || "").toLowerCase();
    if (isLocalHost(h)) return false;
    return forceE2EEOrigin || h.startsWith("r-");
  }

  function b64UrlEncode(bytes) {
    let bin = "";
    const u8 = bytes instanceof Uint8Array ? bytes : new Uint8Array(bytes);
    for (let i = 0; i < u8.length; i++) bin += String.fromCharCode(u8[i]);
    return btoa(bin).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
  }

  function b64StdEncode(bytes) {
    let bin = "";
    const u8 = bytes instanceof Uint8Array ? bytes : new Uint8Array(bytes);
    for (let i = 0; i < u8.length; i++) bin += String.fromCharCode(u8[i]);
    return btoa(bin);
  }

  function b64DecodeFlexible(str) {
    let s = (str || "").trim().replace(/-/g, "+").replace(/_/g, "/");
    const rem = s.length % 4;
    if (rem > 0) s += "=".repeat(4 - rem);
    const bin = atob(s);
    const out = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
    return out;
  }

  async function hmacSha256(keyBytes, dataBytes) {
    const k = await subtle.importKey(
      "raw",
      keyBytes,
      { name: "HMAC", hash: "SHA-256" },
      false,
      ["sign"]
    );
    return new Uint8Array(await subtle.sign("HMAC", k, dataBytes));
  }

  async function hkdfSha256(ikm, salt, info) {
    const effSalt = salt && salt.length ? salt : new Uint8Array(32);
    const prk = await hmacSha256(effSalt, ikm);
    const expandInput = new Uint8Array(info.length + 1);
    expandInput.set(info, 0);
    expandInput[info.length] = 0x01;
    return await hmacSha256(prk, expandInput);
  }

  async function generateEphemeralKeyPair() {
    const kp = await subtle.generateKey(
      { name: "ECDH", namedCurve: "P-256" },
      true,
      ["deriveBits"]
    );
    const rawPub = new Uint8Array(await subtle.exportKey("raw", kp.publicKey));
    return { privateKey: kp.privateKey, publicKeyB64: b64UrlEncode(rawPub) };
  }

  async function computeSharedSecret(privateKey, peerPubB64) {
    const rawPeer = b64DecodeFlexible(peerPubB64);
    const peerKey = await subtle.importKey(
      "raw",
      rawPeer,
      { name: "ECDH", namedCurve: "P-256" },
      false,
      []
    );
    const bits = await subtle.deriveBits({ name: "ECDH", public: peerKey }, privateKey, 256);
    return new Uint8Array(bits);
  }

  async function importAesKey(raw32) {
    return await subtle.importKey("raw", raw32, { name: "AES-GCM" }, false, ["encrypt", "decrypt"]);
  }

  async function aesEncrypt(cryptoKey, plaintextBytes, aadBytes) {
    const nonce = window.crypto.getRandomValues(new Uint8Array(12));
    const ct = new Uint8Array(
      await subtle.encrypt(
        { name: "AES-GCM", iv: nonce, additionalData: aadBytes },
        cryptoKey,
        plaintextBytes
      )
    );
    const frame = new Uint8Array(12 + ct.length);
    frame.set(nonce, 0);
    frame.set(ct, 12);
    return frame;
  }

  async function aesDecrypt(cryptoKey, frameBytes, aadBytes) {
    const u8 = frameBytes instanceof Uint8Array ? frameBytes : new Uint8Array(frameBytes);
    if (u8.length < 28) throw new Error("e2ee frame too short");
    const nonce = u8.slice(0, 12);
    const ct = u8.slice(12);
    return new Uint8Array(
      await subtle.decrypt(
        { name: "AES-GCM", iv: nonce, additionalData: aadBytes },
        cryptoKey,
        ct
      )
    );
  }

  function frameAad(deviceId, dir) {
    return enc.encode("mgy-e2ee-v1:" + deviceId.trim() + ":" + dir);
  }

  function frameSeqAad(deviceId, dir, seq) {
    return enc.encode("mgy-e2ee-v1:" + deviceId.trim() + ":" + dir + ":" + seq);
  }

  function pairFrameAad(clientPubB64, dir) {
    return enc.encode("mgy-e2ee-pair-v1:" + clientPubB64.trim() + ":" + dir);
  }

  function getStoredCreds() {
    try {
      const deviceId = (localStorage.getItem(LS_DEVICE_ID) || "").trim();
      const keyB64 = (localStorage.getItem(LS_KEY_B64) || "").trim();
      const bearer = (localStorage.getItem(LS_BEARER) || "").trim();
      if (deviceId && keyB64) return { deviceId, keyB64, bearer };
    } catch (_) {}
    return null;
  }

  function saveStoredCreds(deviceId, keyB64, bearer) {
    try {
      localStorage.setItem(LS_DEVICE_ID, deviceId);
      localStorage.setItem(LS_KEY_B64, keyB64);
      if (bearer) localStorage.setItem(LS_BEARER, bearer);
      cachedAesKey = null;
      cachedAesKeyB64 = null;
    } catch (_) {}
  }

  function clearStoredCreds() {
    try {
      localStorage.removeItem(LS_DEVICE_ID);
      localStorage.removeItem(LS_KEY_B64);
      localStorage.removeItem(LS_BEARER);
      cachedAesKey = null;
      cachedAesKeyB64 = null;
    } catch (_) {}
  }

  async function getActiveAesKey(keyB64) {
    if (cachedAesKey && cachedAesKeyB64 === keyB64) return cachedAesKey;
    const raw = b64DecodeFlexible(keyB64);
    if (raw.length !== 32) throw new Error("invalid e2ee key length");
    cachedAesKey = await importAesKey(raw);
    cachedAesKeyB64 = keyB64;
    return cachedAesKey;
  }

  async function fetchGatewayPubKey() {
    if (cachedGatewayPub) return cachedGatewayPub;
    try {
      const resp = await nativeFetch("/healthz", { credentials: "omit" });
      observeCaps(resp);
      if (resp.ok) {
        const j = await resp.json();
        if (j && Array.isArray(j.e2ee_caps)) {
          j.e2ee_caps.forEach((c) => observeCapsFromHeader(String(c)));
        }
        if (j && j.e2ee_pub) {
          cachedGatewayPub = String(j.e2ee_pub).trim();
          return cachedGatewayPub;
        }
      }
    } catch (_) {}
    return null;
  }

  async function handleEncryptedPairFetch(options) {
    const gwPub = await fetchGatewayPubKey();
    if (!gwPub) {
      return await nativeFetch("/api/v1/auth/pair", options);
    }

    const kp = await generateEphemeralKeyPair();
    const shared = await computeSharedSecret(kp.privateKey, gwPub);
    const pairKeyRaw = await hkdfSha256(
      shared,
      PAIR_SALT,
      enc.encode("pair:" + kp.publicKeyB64 + ":" + gwPub)
    );
    const pairAes = await importAesKey(pairKeyRaw);

    let plainBodyStr = typeof options.body === "string" ? options.body : "{}";
    try {
      const parsed = JSON.parse(plainBodyStr);
      if (!parsed.e2ee_pub) parsed.e2ee_pub = kp.publicKeyB64;
      plainBodyStr = JSON.stringify(parsed);
    } catch (_) {}

    const cipherFrame = await aesEncrypt(
      pairAes,
      enc.encode(plainBodyStr),
      pairFrameAad(kp.publicKeyB64, "c2s")
    );

    const outerResp = await nativeFetch("/api/v1/e2ee/pair", {
      method: "POST",
      credentials: "omit",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        client_pub: kp.publicKeyB64,
        ciphertext: b64StdEncode(cipherFrame),
      }),
    });

    if (!outerResp.ok) {
      return outerResp;
    }

    const outerJson = await outerResp.json();
    const respCipher = b64DecodeFlexible(outerJson.ciphertext || "");
    const wrappedPlain = await aesDecrypt(
      pairAes,
      respCipher,
      pairFrameAad(kp.publicKeyB64, "s2c")
    );
    const wrapped = JSON.parse(dec.decode(wrappedPlain));
    const status = wrapped.status || 200;
    const bodyObj = wrapped.body || {};

    if (status === 200 && bodyObj.device_id && bodyObj.e2ee_gateway_pub && bodyObj.e2ee_salt) {
      const devShared = await computeSharedSecret(kp.privateKey, bodyObj.e2ee_gateway_pub);
      const saltBytes = b64DecodeFlexible(bodyObj.e2ee_salt);
      const devKeyRaw = await hkdfSha256(
        devShared,
        saltBytes,
        enc.encode("mgy-e2ee-device-v1:" + bodyObj.device_id)
      );
      saveStoredCreds(bodyObj.device_id, b64UrlEncode(devKeyRaw), bodyObj.device_token || "");
    }

    return new Response(JSON.stringify(bodyObj), {
      status,
      headers: { "Content-Type": "application/json; charset=utf-8" },
    });
  }

  async function serializeFetchBody(body) {
    if (body == null) return "";
    if (typeof body === "string") return b64StdEncode(enc.encode(body));
    if (body instanceof ArrayBuffer) return b64StdEncode(new Uint8Array(body));
    if (ArrayBuffer.isView(body)) {
      return b64StdEncode(new Uint8Array(body.buffer, body.byteOffset, body.byteLength));
    }
    if (body instanceof Blob) {
      return b64StdEncode(new Uint8Array(await body.arrayBuffer()));
    }
    if (body instanceof URLSearchParams) {
      return b64StdEncode(enc.encode(body.toString()));
    }
    return b64StdEncode(enc.encode(String(body)));
  }

  async function executeEncryptedRPC(parsedUrl, options, creds, retriedClockSkew) {
    const method = ((options && options.method) || "GET").toUpperCase();
    const pathWithQuery = (parsedUrl.pathname || "/") + (parsedUrl.search || "");
    const headersObj = {};
    let hasAuth = false;
    if (options && options.headers) {
      const hdrs = new Headers(options.headers);
      hdrs.forEach((v, k) => {
        if (k.toLowerCase() === "accept-encoding") return;
        if (k.toLowerCase() === "authorization") hasAuth = true;
        headersObj[k] = v;
      });
    }
    if (!hasAuth && creds.bearer) {
      headersObj["Authorization"] = "Bearer " + creds.bearer;
    }

    const bodyB64 = options && options.body != null ? await serializeFetchBody(options.body) : "";
    const inner = {
      m: method,
      p: pathWithQuery,
      h: headersObj,
      ts: adjustedTimestampMs(),
      id: generateRequestId(),
    };
    if (bodyB64) inner.b = bodyB64;

    const aesKey = await getActiveAesKey(creds.keyB64);
    const encPayload = await aesEncrypt(
      aesKey,
      enc.encode(JSON.stringify(inner)),
      frameAad(creds.deviceId, "c2s")
    );

    const outerResp = await nativeFetch("/api/v1/e2ee/rpc", {
      method: "POST",
      credentials: "omit",
      headers: {
        "Content-Type": "application/octet-stream",
        [HEADER_DEVICE_ID]: creds.deviceId,
        [HEADER_ACCEPT_ENC]: SUPPORTS_DEFLATE_RAW ? "bin2,flate" : "bin2",
      },
      body: encPayload,
      signal: options && options.signal,
    });

    observeCaps(outerResp);
    if (outerResp.status !== 200) {
      if (outerResp.status === 401) {
        if (!retriedClockSkew) {
          try {
            const errJson = await outerResp.clone().json();
            if (
              errJson &&
              errJson.error === "e2ee_replay_clock_skew" &&
              typeof errJson.server_ts_ms === "number" &&
              errJson.server_ts_ms > 0
            ) {
              clockOffsetMs = errJson.server_ts_ms - Date.now();
              return await executeEncryptedRPC(parsedUrl, options, creds, true);
            }
          } catch (_) {}
        }
        clearStoredCreds();
      }
      return outerResp;
    }

    const encRespBytes = new Uint8Array(await outerResp.arrayBuffer());
    const plainBytes = await aesDecrypt(aesKey, encRespBytes, frameAad(creds.deviceId, "s2c"));
    let status = 200;
    let decodedHeaders = null;
    let respBody = null;

    if (plainBytes.length > 0 && (plainBytes[0] === 0x01 || plainBytes[0] === 0x02)) {
      const unpacked =
        plainBytes[0] === 0x02 ? await inflateRawBytes(plainBytes.subarray(1)) : plainBytes.subarray(1);
      const dv = new DataView(unpacked.buffer, unpacked.byteOffset, unpacked.byteLength);
      const metaLen = dv.getUint32(0, false);
      const meta = JSON.parse(dec.decode(unpacked.subarray(4, 4 + metaLen)));
      status = meta.s || 200;
      decodedHeaders = meta.h || null;
      const bodySlice = unpacked.subarray(4 + metaLen);
      respBody = bodySlice.length > 0 ? bodySlice : null;
    } else {
      const decoded = JSON.parse(dec.decode(plainBytes));
      status = decoded.s || 200;
      decodedHeaders = decoded.h || null;
      respBody = decoded.b ? b64DecodeFlexible(decoded.b) : null;
    }

    const respHeaders = new Headers();
    if (decodedHeaders && typeof decodedHeaders === "object") {
      for (const [k, v] of Object.entries(decodedHeaders)) {
        const lower = k.toLowerCase();
        if (lower === "set-cookie" || lower === "content-length" || lower === "transfer-encoding") continue;
        try {
          respHeaders.set(k, String(v));
        } catch (_) {}
      }
    }
    if (status === 401) clearStoredCreds();
    return new Response(status === 204 || status === 304 ? null : respBody, {
      status,
      headers: respHeaders,
    });
  }

  function shouldBypassPath(pathname) {
    if (!pathname) return true;
    if (pathname === "/healthz" || pathname.startsWith("/api/v1/e2ee/")) return true;
    if (
      pathname === "/" ||
      pathname === "/index.html" ||
      pathname === "/style.css" ||
      pathname === "/sw.js" ||
      pathname === "/favicon.ico" ||
      pathname === "/mermaid.min.js" ||
      pathname.startsWith("/js/") ||
      pathname.startsWith("/icons/") ||
      pathname.startsWith("/manifest")
    ) {
      return true;
    }
    return false;
  }

  // Wrap window.fetch transparently
  window.fetch = async function (input, init) {
    const rawUrl = typeof input === "string" ? input : input && input.url ? input.url : "";
    let parsedUrl = null;
    try {
      parsedUrl = new URL(rawUrl, window.location.origin);
    } catch (_) {
      return nativeFetch(input, init);
    }

    if (parsedUrl.origin !== window.location.origin || shouldBypassPath(parsedUrl.pathname)) {
      const bypassResp = await nativeFetch(input, init);
      if (parsedUrl.origin === window.location.origin && parsedUrl.pathname === "/healthz") {
        observeCaps(bypassResp);
      }
      return bypassResp;
    }

    // Clear E2EE creds on logout/unpair
    if (parsedUrl.pathname === "/api/v1/auth/logout" || parsedUrl.pathname === "/api/v1/auth/unpair") {
      const creds = getStoredCreds();
      if (isRelayOrigin() && creds) {
        try {
          return await executeEncryptedRPC(parsedUrl, init || {}, creds);
        } finally {
          clearStoredCreds();
        }
      }
      clearStoredCreds();
      return nativeFetch(input, init);
    }

    // Encrypted pairing on relay origins
    if (parsedUrl.pathname === "/api/v1/auth/pair" && isRelayOrigin()) {
      return await handleEncryptedPairFetch(init || {});
    }

    const creds = getStoredCreds();
    if (isRelayOrigin() && creds) {
      return await executeEncryptedRPC(parsedUrl, init || {}, creds);
    }

    const resp = await nativeFetch(input, init);
    if (resp.status === 426 && creds) {
      forceE2EEOrigin = true;
      return await executeEncryptedRPC(parsedUrl, init || {}, creds);
    }
    return resp;
  };

  // Transparent E2EE WebSocket wrapper for relay origins
  class E2EEWebSocket extends EventTarget {
    constructor(url, protocols) {
      super();
      this.url = url;
      this.protocol = "";
      this.extensions = "";
      this.bufferedAmount = 0;
      this.binaryType = "blob";
      this.readyState = NativeWebSocket.CONNECTING;
      this.onopen = null;
      this.onmessage = null;
      this.onerror = null;
      this.onclose = null;

      const parsed = new URL(url, window.location.href);
      const creds = getStoredCreds();
      const innerPath = (parsed.pathname || "/") + (parsed.search || "");
      const wsProto = parsed.protocol === "https:" || parsed.protocol === "wss:" ? "wss:" : "ws:";
      const outerUrl = `${wsProto}//${parsed.host}/api/v1/e2ee/ws?d=${encodeURIComponent(creds.deviceId)}`;

      this._creds = creds;
      this._aesPromise = getActiveAesKey(creds.keyB64);
      this._pendingSends = [];
      this._useSeq = serverCaps.has(CAP_WS_SEQ1);
      this._c2sSeq = 0;
      this._s2cSeq = 0;
      this._sendChain = Promise.resolve();
      this._recvChain = Promise.resolve();
      this._raw = new NativeWebSocket(outerUrl);
      this._raw.binaryType = "arraybuffer";

      this._raw.addEventListener("open", () => {
        this._sendChain = this._sendChain.then(async () => {
          try {
            const aesKey = await this._aesPromise;
            this._useSeq = serverCaps.has(CAP_WS_SEQ1);
            const openHeaders = {};
            if (creds.bearer) openHeaders["Authorization"] = "Bearer " + creds.bearer;
            if (protocols) {
              openHeaders["Sec-WebSocket-Protocol"] = Array.isArray(protocols)
                ? protocols.join(", ")
                : String(protocols);
            }
            const openFrame = {
              path: innerPath,
              headers: openHeaders,
              ts: adjustedTimestampMs(),
              id: generateRequestId(),
            };
            if (SUPPORTS_DEFLATE_RAW) openFrame.enc = "flate";
            if (this._useSeq) openFrame.seq = "v1";
            const frame0 = await aesEncrypt(
              aesKey,
              enc.encode(JSON.stringify(openFrame)),
              frameAad(creds.deviceId, "ws-open")
            );
            this._raw.send(frame0);
            this.readyState = NativeWebSocket.OPEN;
            while (this._pendingSends.length > 0) {
              const item = this._pendingSends.shift();
              this._sendEncrypted(item);
            }
            const ev = new Event("open");
            if (typeof this.onopen === "function") this.onopen(ev);
            this.dispatchEvent(ev);
          } catch (err) {
            this.close(1008, "e2ee_open_failed");
          }
        });
      });

      this._raw.addEventListener("message", (ev) => {
        const rawData = ev.data;
        this._recvChain = this._recvChain.then(async () => {
          let seq = 0;
          try {
            const aesKey = await this._aesPromise;
            let buf;
            if (rawData instanceof ArrayBuffer) {
              buf = new Uint8Array(rawData);
            } else if (rawData instanceof Blob) {
              buf = new Uint8Array(await rawData.arrayBuffer());
            } else {
              return;
            }
            seq = this._s2cSeq++;
            const aad = this._useSeq
              ? frameSeqAad(creds.deviceId, "ws-s2c", seq)
              : frameAad(creds.deviceId, "ws-s2c");
            const plain = await aesDecrypt(aesKey, buf, aad);
            if (!plain || plain.length < 1) return;
            const headerByte = plain[0];
            const isCompressed = (headerByte & 0x80) !== 0;
            const msgType = headerByte & 0x7f;
            const rawPayload = plain.slice(1);
            const payload = isCompressed ? await inflateRawBytes(rawPayload) : rawPayload;
            const outData = msgType === 1 ? dec.decode(payload) : payload.buffer;
            const msgEv = new MessageEvent("message", { data: outData });
            if (typeof this.onmessage === "function") this.onmessage(msgEv);
            this.dispatchEvent(msgEv);
          } catch (_) {
            if (this._useSeq && seq === 0) {
              serverCaps.delete(CAP_WS_SEQ1);
            }
            this.close(1008, "e2ee_decrypt_failed");
          }
        });
      });

      this._raw.addEventListener("error", (ev) => {
        if (typeof this.onerror === "function") this.onerror(ev);
        this.dispatchEvent(new Event("error"));
      });

      this._raw.addEventListener("close", (ev) => {
        if (ev && typeof ev.reason === "string" && ev.reason.startsWith("e2ee_replay_clock_skew:")) {
          const parsedTs = parseInt(ev.reason.slice("e2ee_replay_clock_skew:".length), 10);
          if (Number.isFinite(parsedTs) && parsedTs > 0) {
            clockOffsetMs = parsedTs - Date.now();
          }
        }
        this.readyState = NativeWebSocket.CLOSED;
        const closeEv = new CloseEvent("close", {
          code: ev.code,
          reason: ev.reason,
          wasClean: ev.wasClean,
        });
        if (typeof this.onclose === "function") this.onclose(closeEv);
        this.dispatchEvent(closeEv);
      });
    }

    send(data) {
      if (this.readyState !== NativeWebSocket.OPEN) {
        this._pendingSends.push(data);
        return;
      }
      this._sendEncrypted(data);
    }

    _sendEncrypted(data) {
      this._sendChain = this._sendChain.then(async () => {
        try {
          const aesKey = await this._aesPromise;
          let msgType = 1;
          let payload;
          if (typeof data === "string") {
            msgType = 1;
            payload = enc.encode(data);
          } else if (data instanceof ArrayBuffer) {
            msgType = 2;
            payload = new Uint8Array(data);
          } else if (ArrayBuffer.isView(data)) {
            msgType = 2;
            payload = new Uint8Array(data.buffer, data.byteOffset, data.byteLength);
          } else if (data instanceof Blob) {
            msgType = 2;
            payload = new Uint8Array(await data.arrayBuffer());
          } else {
            msgType = 1;
            payload = enc.encode(String(data));
          }
          const buf = new Uint8Array(1 + payload.length);
          buf[0] = msgType;
          buf.set(payload, 1);
          const seq = this._c2sSeq++;
          const aad = this._useSeq
            ? frameSeqAad(this._creds.deviceId, "ws-c2s", seq)
            : frameAad(this._creds.deviceId, "ws-c2s");
          const frame = await aesEncrypt(aesKey, buf, aad);
          this._raw.send(frame);
        } catch (_) {}
      });
    }

    close(code, reason) {
      this.readyState = NativeWebSocket.CLOSING;
      try {
        this._raw.close(code, reason);
      } catch (_) {}
    }
  }

  E2EEWebSocket.CONNECTING = NativeWebSocket.CONNECTING;
  E2EEWebSocket.OPEN = NativeWebSocket.OPEN;
  E2EEWebSocket.CLOSING = NativeWebSocket.CLOSING;
  E2EEWebSocket.CLOSED = NativeWebSocket.CLOSED;

  window.WebSocket = function (url, protocols) {
    try {
      const parsed = new URL(url, window.location.href);
      const creds = getStoredCreds();
      if (
        creds &&
        parsed.host === window.location.host &&
        !parsed.pathname.startsWith("/api/v1/e2ee/") &&
        isRelayOrigin()
      ) {
        return new E2EEWebSocket(url, protocols);
      }
    } catch (_) {}
    return protocols !== undefined ? new NativeWebSocket(url, protocols) : new NativeWebSocket(url);
  };
  window.WebSocket.CONNECTING = NativeWebSocket.CONNECTING;
  window.WebSocket.OPEN = NativeWebSocket.OPEN;
  window.WebSocket.CLOSING = NativeWebSocket.CLOSING;
  window.WebSocket.CLOSED = NativeWebSocket.CLOSED;
  window.WebSocket.prototype = NativeWebSocket.prototype;
})();
