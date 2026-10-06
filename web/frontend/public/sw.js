// Secretli's service worker does one thing: it keeps shared text off the wire.
//
// Sharing to the installed app opens /share?text=… (the share target in the
// manifest). Left alone, that query would travel to the server and into the
// logs of everything in front of it, which is exactly what this app promises
// never happens. So the worker answers that navigation itself: it fetches
// /share without the query and hands the page that response. The page still
// sees its own URL, reads the shared text out of it, and removes it from the
// address bar. Nothing is cached, nothing else is touched.
self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (event) => event.waitUntil(self.clients.claim()));

self.addEventListener("fetch", (event) => {
  const url = new URL(event.request.url);
  if (event.request.mode !== "navigate" || url.pathname !== "/share" || !url.search) return;
  event.respondWith(fetch(url.origin + url.pathname, { credentials: "same-origin" }));
});
