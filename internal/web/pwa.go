package web

import "net/http"

// manifestJSON is the Web App Manifest that makes this app installable (ROADMAP.md's "Installable
// as a PWA"). The name is the platform's own brand ("Menata App", the same suffix pageShell's
// <title> already hardcodes), not Deps.AppName -- that resolves to the current metadata
// Application's own name (e.g. "Task Tracker"), a different, narrower thing. start_url is /home
// (Phase 21's Workspace Home), where an authenticated identity actually lands today.
const manifestJSON = `{
  "name": "Menata App",
  "short_name": "Menata",
  "start_url": "/home",
  "display": "standalone",
  "background_color": "#2563EB",
  "theme_color": "#2563EB",
  "icons": [
    { "src": "/icons/icon-192.png", "sizes": "192x192", "type": "image/png", "purpose": "any" },
    { "src": "/icons/icon-512.png", "sizes": "512x512", "type": "image/png", "purpose": "any" }
  ]
}`

func serveManifest(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/manifest+json")
	_, _ = w.Write([]byte(manifestJSON))
}

// serviceWorkerJS is deliberately minimal: a fetch handler is the one thing installability
// requires (no offline data caching -- this app's data is live and session-scoped, so caching it
// would serve stale or wrong records, not a feature). The one real behavior beyond a plain
// passthrough is a small offline fallback for a failed page navigation.
const serviceWorkerJS = `self.addEventListener('install', () => self.skipWaiting());
self.addEventListener('activate', (event) => event.waitUntil(self.clients.claim()));
self.addEventListener('fetch', (event) => {
  if (event.request.mode === 'navigate') {
    event.respondWith(
      fetch(event.request).catch(() => new Response(
        '<!doctype html><meta charset="utf-8"><title>Offline</title>' +
        '<p style="font-family:system-ui;text-align:center;margin-top:3rem">' +
        "You're offline. Try again once you're back online.</p>",
        { headers: { 'Content-Type': 'text/html' } }
      ))
    );
    return;
  }
  event.respondWith(fetch(event.request));
});
`

func serveServiceWorker(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	_, _ = w.Write([]byte(serviceWorkerJS))
}
