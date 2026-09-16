const CACHE_NAME = "antigravity-mobile-v10";
const ASSETS = [
  "/style.css",
  "/app.js",
  "/manifest.json",
  "/mermaid.min.js",
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

  // Network-only for HTML, APIs, websocket — never persist documents that can steal tokens.
  if (
    url.pathname.startsWith("/api/") ||
    url.pathname.startsWith("/gateway/") ||
    url.pathname.startsWith("/static/") ||
    url.pathname === "/connect-websocket" ||
    url.pathname === "/" ||
    url.pathname === "/index.html" ||
    event.request.method !== "GET" ||
    (event.request.mode === "navigate")
  ) {
    return;
  }

  // Stale-While-Revalidate strategy for app shell assets
  event.respondWith(
    caches.open(CACHE_NAME).then(async (cache) => {
      const cachedResponse = await cache.match(event.request);
      const networkFetch = fetch(event.request)
        .then((networkResponse) => {
          if (networkResponse && networkResponse.ok) {
            cache.put(event.request, networkResponse.clone());
          }
          return networkResponse;
        })
        .catch(() => cachedResponse);

      // Return cached response immediately if available, while updating cache in background
      return cachedResponse || networkFetch;
    })
  );
});