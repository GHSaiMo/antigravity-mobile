const CACHE_NAME = "antigravity-mobile-v43";
const ASSETS = [
  "/",
  "/index.html",
  "/style.css",
  "/js/core.js",
  "/js/nav.js",
  "/js/chat.js",
  "/js/actions.js",
  "/js/media.js",
  "/js/init.js",
  "/js/cockpit.js",
  "/js/scan.js",
  "/js/msgmenu.js",
  "/js/slash.js",
  "/js/changes.js",
  "/js/git.js",
  "/js/models.js",
  "/js/subagents.js",
  "/js/egg.js",
  "/manifest.json",
  "/icons/icon.svg",
  "/icons/icon-192.png",
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

  // Network-only for APIs, dynamic gateway endpoints, websocket
  if (
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

  // Stale-While-Revalidate strategy for app shell assets
  event.respondWith(
    caches.open(CACHE_NAME).then(async (cache) => {
      // ignoreSearch: true ensures /js/core.js?v=26 hits cached /js/core.js
      const cachedResponse = await cache.match(event.request, { ignoreSearch: true });
      const networkFetch = fetch(event.request)
        .then(async (networkResponse) => {
          if (networkResponse && networkResponse.ok) {
            // Await ensures the SW stays alive until cache write completes
            await cache.put(event.request, networkResponse.clone());
          }
          return networkResponse;
        })
        .catch(() => cachedResponse || new Response("Asset not found", { status: 504 }));

      // Keep the Service Worker alive until cache update finishes
      event.waitUntil(networkFetch.catch(() => {}));

      // Return cached response immediately if available, while updating cache in background
      return cachedResponse || networkFetch;
    })
  );
});