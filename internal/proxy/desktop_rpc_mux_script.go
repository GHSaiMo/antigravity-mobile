package proxy

// desktopRPCMuxJS is injected into the desktop workbench page. It tunnels long-lived streaming
// RPCs (Subscribe*/Watch*/*Stream) through one WebSocket (/gateway/rpc-mux) so they no longer
// occupy the browser's 6-connections-per-host HTTP/1.1 budget. Unary RPCs keep using fetch, and
// any failure to reach the mux falls back to a plain fetch.
//
// NOTE: this is embedded in a Go raw string elsewhere, so it must not contain backticks.
const desktopRPCMuxJS = `
(() => {
  const STREAM_RE = /\/exa\.language_server_pb\.[\w.]+\/\w*(Subscribe|Watch|Stream)\w*$/;
  const OPEN = 1, CANCEL = 3, HEAD = 4, CHUNK = 5, END = 6, ERR = 7;
  const enc = new TextEncoder(), dec = new TextDecoder();
  const baseFetch = window.fetch;
  let sock = null, sockPromise = null, nextId = 1, brokenUntil = 0;
  const streams = new Map();

  function frame(type, id, payload) {
    const out = new Uint8Array(5 + payload.length);
    out[0] = type;
    new DataView(out.buffer).setUint32(1, id);
    out.set(payload, 5);
    return out;
  }

  function failAll(err) {
    for (const s of streams.values()) s.fail(err);
    streams.clear();
  }

  function connect() {
    if (sockPromise) return sockPromise;
    sockPromise = new Promise((resolve, reject) => {
      let opened = false;
      const ws = new WebSocket((location.protocol === "https:" ? "wss://" : "ws://") + location.host + "/gateway/rpc-mux");
      ws.binaryType = "arraybuffer";
      ws.onopen = () => { opened = true; sock = ws; resolve(ws); };
      ws.onmessage = (ev) => {
        const buf = new Uint8Array(ev.data);
        if (buf.length < 5) return;
        const type = buf[0];
        const id = new DataView(buf.buffer, buf.byteOffset).getUint32(1);
        const s = streams.get(id);
        if (!s) return;
        const payload = buf.subarray(5);
        if (type === HEAD) s.head(JSON.parse(dec.decode(payload)));
        else if (type === CHUNK) s.chunk(payload.slice());
        else if (type === END) { streams.delete(id); s.end(); }
        else if (type === ERR) { streams.delete(id); s.fail(new TypeError(dec.decode(payload))); }
      };
      ws.onerror = () => { if (!opened) reject(new Error("rpc-mux unavailable")); };
      ws.onclose = () => {
        if (sock === ws) sock = null;
        sockPromise = null;
        if (!opened) { reject(new Error("rpc-mux closed")); return; }
        failAll(new TypeError("network error"));
      };
    });
    return sockPromise;
  }

  async function readBody(input, init) {
    const isReq = typeof Request !== "undefined" && input instanceof Request;
    const body = init && init.body !== undefined ? init.body : null;
    if (body === null) return isReq ? new Uint8Array(await input.clone().arrayBuffer()) : new Uint8Array(0);
    if (typeof body === "string") return enc.encode(body);
    if (body instanceof ArrayBuffer) return new Uint8Array(body);
    if (ArrayBuffer.isView(body)) return new Uint8Array(body.buffer, body.byteOffset, body.byteLength).slice();
    return new Uint8Array(await new Response(body).arrayBuffer());
  }

  function muxFetch(ws, url, headers, bodyBytes, signal) {
    return new Promise((resolve, reject) => {
      const id = nextId++;
      let ctrl = null, done = false;
      const body = new ReadableStream({
        start(c) { ctrl = c; },
        cancel() { if (!done) { done = true; streams.delete(id); try { ws.send(frame(CANCEL, id, new Uint8Array(0))); } catch (_) {} } }
      });
      const abortErr = () => new DOMException("The operation was aborted.", "AbortError");
      const entry = {
        head(h) {
          const noBody = h.status === 101 || h.status === 204 || h.status === 205 || h.status === 304;
          resolve(new Response(noBody ? null : body, { status: h.status, headers: h.headers || {} }));
        },
        chunk(b) { try { ctrl.enqueue(b); } catch (_) {} },
        end() { done = true; try { ctrl.close(); } catch (_) {} },
        fail(err) { done = true; reject(err); try { ctrl.error(err); } catch (_) {} }
      };
      streams.set(id, entry);
      if (signal) {
        const onAbort = () => {
          if (done) return;
          done = true; streams.delete(id);
          try { ws.send(frame(CANCEL, id, new Uint8Array(0))); } catch (_) {}
          reject(abortErr()); try { ctrl.error(abortErr()); } catch (_) {}
        };
        if (signal.aborted) { onAbort(); return; }
        signal.addEventListener("abort", onAbort, { once: true });
      }
      const meta = enc.encode(JSON.stringify({ path: url.pathname, headers: headers }));
      const payload = new Uint8Array(4 + meta.length + bodyBytes.length);
      new DataView(payload.buffer).setUint32(0, meta.length);
      payload.set(meta, 4);
      payload.set(bodyBytes, 4 + meta.length);
      try { ws.send(frame(OPEN, id, payload)); } catch (e) { streams.delete(id); reject(e); }
    });
  }

  window.fetch = async function (input, init) {
    try {
      const isReq = typeof Request !== "undefined" && input instanceof Request;
      const rawUrl = typeof input === "string" ? input : (isReq ? input.url : String(input));
      const method = String((init && init.method) || (isReq && input.method) || "GET").toUpperCase();
      const url = new URL(rawUrl, location.href);
      if (method === "POST" && url.origin === location.origin && STREAM_RE.test(url.pathname) && Date.now() >= brokenUntil && typeof WebSocket !== "undefined") {
        const bytes = await readBody(input, init);
        const hdrs = {};
        new Headers((init && init.headers) || (isReq ? input.headers : undefined)).forEach((v, k) => { hdrs[k] = v; });
        let ws;
        try { ws = await connect(); } catch (_) { brokenUntil = Date.now() + 60000; ws = null; }
        if (ws) return await muxFetch(ws, url, hdrs, bytes, (init && init.signal) || (isReq ? input.signal : null));
        return baseFetch.call(this, rawUrl, Object.assign({}, init, { method: "POST", headers: hdrs, body: bytes }));
      }
    } catch (e) {
      if (e && e.name === "AbortError") throw e;
    }
    return baseFetch.apply(this, arguments);
  };
})();
`

// desktopThemePresetFixJS normalises the theme preset names the workbench stores in localStorage.
// The gateway localises preset names inside main.js ("Default Dark" -> "深邃炭黑", ...), so a value
// saved before localisation (or any unknown value) makes the Appearance settings look up an
// undefined preset and the whole React tree unmounts (blank page after opening system settings).
//
// NOTE: embedded in a Go raw string elsewhere; no backticks allowed.
const desktopThemePresetFixJS = `
(() => {
  const LEGACY = {
    "Default Light": "经典浅白", "Default Dark": "深邃炭黑",
    "One Light": "One Light 亮色", "One Dark Pro": "One Dark Pro 深色",
    "Tokyo Night": "Tokyo Night 东京之夜",
    "Solarized Light": "Solarized 浅色", "Solarized Dark": "Solarized 深色"
  };
  const VALID = {
    "theme-preset-light": ["经典浅白", "Catppuccin", "One Light 亮色", "Solarized 浅色"],
    "theme-preset-dark": ["深邃炭黑", "Catppuccin", "Dracula", "Monokai", "One Dark Pro 深色", "Tokyo Night 东京之夜", "Solarized 深色", "Vesper"]
  };
  try {
    const proto = Storage.prototype, orig = proto.getItem;
    proto.getItem = function (key) {
      const v = orig.call(this, key);
      if (this === window.localStorage && v !== null && VALID[key]) {
        const fixed = LEGACY[v] || v;
        if (VALID[key].indexOf(fixed) === -1) return null;
        return fixed;
      }
      return v;
    };
  } catch (_) {}
})();
`
