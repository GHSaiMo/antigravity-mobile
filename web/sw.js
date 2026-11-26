const CACHE_NAME = "antigravity-mobile-v81";
const ASSETS = [
  "/",
  "/index.html",
  "/style.css?v=68",
  "/js/i18n.js?v=12",
  "/js/e2ee.js?v=1",
  "/js/core.js?v=43",
  "/js/nav.js?v=55",
  "/js/wechat_archive.js?v=1",
  "/js/chat.js?v=48",
  "/js/actions.js?v=42",
  "/js/media.js?v=39",
  "/js/cockpit.js?v=37",
  "/js/scan.js?v=2",
  "/js/msgmenu.js?v=7",
  "/js/slash.js?v=4",
  "/js/workflows.js?v=2",
  "/js/changes.js?v=3",
  "/js/git.js?v=7",
  "/js/models.js?v=4",
  "/js/subagents.js?v=6",
  "/js/swarm.js?v=6",
  "/js/egg.js?v=1",
  "/js/phase3.js?v=9",
  "/js/v233.js?v=8",
  "/js/phase4.js?v=1",
  "/js/init.js?v=48",
  "/manifest.json",
  "/manifest-en.json",
  "/manifest-ja.json",
  "/favicon.ico?v=4",
  "/icons/favicon-32.png?v=4",
  "/icons/apple-touch-icon.png?v=4",
  "/icons/icon.svg",
  "/icons/icon-192.png?v=4",
  "/icons/icon-512.png"
];

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches.open(CACHE_NAME).then((cache) => cache.addAll(ASSETS))
  );
  self.skipWaiting();
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches.keys().then((keys) =>
      Promise.all(
        keys.filter((k) => k !== CACHE_NAME).map((k) => caches.delete(k))
      )
    )
  );
  self.clients.claim();
});

self.addEventListener("fetch", (event) => {
  const url = new URL(event.request.url);

  // Network-only for cross-origin requests, APIs, dynamic gateway endpoints, websocket
  if (
    url.origin !== self.location.origin ||
    url.pathname.startsWith("/api/") ||
    url.pathname.startsWith("/gateway/") ||
    url.pathname.startsWith("/static/") ||
    url.pathname === "/connect-websocket" ||
    event.request.method !== "GET"
  ) {
    return;
  }

  // Network-first with offline fallback for navigation requests
  if (event.request.mode === "navigate" || url.pathname === "/" || url.pathname === "/index.html") {
    event.respondWith(
      fetch(event.request)
        .then(async (networkResponse) => {
          if (networkResponse && networkResponse.ok) {
            const cache = await caches.open(CACHE_NAME);
            await cache.put("/index.html", networkResponse.clone());
          }
          return networkResponse;
        })
        .catch(async () => {
          const cache = await caches.open(CACHE_NAME);
          const cached = await cache.match("/index.html", { ignoreSearch: true });
          if (cached) return cached;
          return new Response("离线模式：无法连接到网关", {
            status: 503,
            headers: { "Content-Type": "text/html; charset=utf-8" }
          });
        })
    );
    return;
  }

  // Immutable versioned assets (?v=...) use pure Cache-First: zero redundant background SWR fetches
  // on startup, while on-demand heavy assets (e.g. mermaid.min.js?v=1, jsQR.js) lazy-cache on first use.
  const hasVersionParam = url.searchParams.has("v");
  if (hasVersionParam) {
    event.respondWith(
      caches.open(CACHE_NAME).then(async (cache) => {
        const cachedResponse = await cache.match(event.request);
        if (cachedResponse) return cachedResponse;
        try {
          const networkResponse = await fetch(event.request);
          if (networkResponse && networkResponse.ok) {
            await cache.put(event.request, networkResponse.clone());
          }
          return networkResponse;
        } catch (_) {
          const fallback = await cache.match(event.request, { ignoreSearch: true });
          return fallback || new Response("Asset not found", { status: 504 });
        }
      })
    );
    return;
  }

  // Unversioned same-origin shell assets: stale-while-revalidate with offline fallback
  event.respondWith(
    caches.open(CACHE_NAME).then(async (cache) => {
      const cachedResponse = await cache.match(event.request, { ignoreSearch: true });
      const networkFetch = fetch(event.request)
        .then(async (networkResponse) => {
          if (networkResponse && networkResponse.ok) {
            await cache.put(event.request, networkResponse.clone());
          }
          return networkResponse;
        })
        .catch(async () => {
          if (cachedResponse) return cachedResponse;
          return new Response("Asset not found", { status: 504 });
        });

      event.waitUntil(networkFetch.catch(() => {}));
      return cachedResponse || networkFetch;
    })
  );
});