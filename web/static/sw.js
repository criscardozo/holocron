// Holocron's service worker. It caches nothing the screens show: every number
// on them is live, and a stale one would be worse than none. Its only job is
// the page that says Ginebra did not answer — off the home network with
// Tailscale off, or the machine asleep — instead of Safari's own error, which
// in an installed app has no way back.
//
// VERSION and OFFLINE_ASSETS are prepended by the server, so a new release
// is a new worker and its stylesheet URL is the one the release ships.

const CACHE = "holocron-offline-" + VERSION;
const OFFLINE = "/offline";

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches.open(CACHE)
      .then((c) => c.addAll([OFFLINE, ...OFFLINE_ASSETS]))
      .then(() => self.skipWaiting()),
  );
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches.keys()
      .then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k))))
      .then(() => self.clients.claim()),
  );
});

self.addEventListener("fetch", (event) => {
  const req = event.request;
  if (req.method !== "GET") {
    return;
  }
  if (req.mode === "navigate") {
    event.respondWith(fetch(req).catch(() => caches.match(OFFLINE)));
    return;
  }
  // The offline page's own assets, only when the network fails: online they
  // come from the server like everything else.
  const path = new URL(req.url).pathname + new URL(req.url).search;
  if (OFFLINE_ASSETS.includes(path)) {
    event.respondWith(fetch(req).catch(() => caches.match(req)));
  }
});
