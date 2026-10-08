package web

import (
	"encoding/json"
	"net/http"

	"menata.app/internal/config"
)

// manifestJSON is the Web App Manifest that makes this app installable (ROADMAP.md's "Installable
// as a PWA"). The name is the platform's own brand ("Menata App", the same suffix pageShell's
// <title> already hardcodes), not Deps.AppName -- that resolves to the current metadata
// Application's own name (e.g. "Task Tracker"), a different, narrower thing. start_url is /home
// (Phase 21's Workspace Home), where an authenticated identity actually lands today.
//
// "id" pins the app's identity to the start_url it already had: with no id the browser derives it
// from start_url, so spelling it out changes nothing for existing installs while keeping a later
// start_url change from orphaning them. The two "maskable" entries reuse the same files on purpose:
// the glyph is a full-bleed blue square with the "M" inside the central 40%, well within the 80%
// safe zone Android crops to, so no second artwork is needed. Google Play's Trusted Web Activity
// wrapper reads this same manifest (see serveAssetLinks).
const manifestJSON = `{
  "id": "/home",
  "name": "Menata App",
  "short_name": "Menata",
  "start_url": "/home",
  "display": "standalone",
  "background_color": "#2563EB",
  "theme_color": "#2563EB",
  "icons": [
    { "src": "/icons/icon-192.png", "sizes": "192x192", "type": "image/png", "purpose": "any" },
    { "src": "/icons/icon-512.png", "sizes": "512x512", "type": "image/png", "purpose": "any" },
    { "src": "/icons/icon-192.png", "sizes": "192x192", "type": "image/png", "purpose": "maskable" },
    { "src": "/icons/icon-512.png", "sizes": "512x512", "type": "image/png", "purpose": "maskable" }
  ]
}`

func serveManifest(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/manifest+json")
	_, _ = w.Write([]byte(manifestJSON))
}

// serviceWorkerJS is deliberately minimal: a fetch handler is the one thing installability
// requires (no offline data caching -- this app's data is live and session-scoped, so caching it
// would serve stale or wrong records, not a feature). The one real behavior beyond that is a small
// offline fallback for a failed page navigation.
//
// TWO THINGS IT USED TO DO THAT COST REAL TIME, both found on 2026-09-24 by reading the edge log
// after the owner asked why the app did not open immediately. Neither was visible in the app's own
// log, because both happen in the browser *before* a request is sent:
//
//  1. It called `event.respondWith(fetch(event.request))` for every non-navigation request --
//     stylesheets, scripts, fonts, every hx-get. That is exactly what the browser does when a
//     handler calls nothing, minus a trip through the worker, so the line bought nothing and taxed
//     everything. Returning without respondWith still leaves a fetch handler registered, which is
//     all installability asks for.
//  2. A navigation waited for the worker to *boot* before its own fetch could start. Navigation
//     preload is the platform's answer: the browser fires the network request in parallel with
//     starting the worker, and the worker awaits `event.preloadResponse` instead of issuing a
//     second one.
//
// The measurement that found it: of the owner's own page loads, `/home` and `/approval-inbox`
// arrived with `Sec-Fetch-Dest: empty` 410 times against `document` 11 times. A real navigation is
// always `document`; `empty` is a `fetch()`, which is the worker re-issuing the request and losing
// its destination on the way.
const serviceWorkerJS = `self.addEventListener('install', () => self.skipWaiting());
self.addEventListener('activate', (event) => event.waitUntil((async () => {
  if (self.registration.navigationPreload) {
    await self.registration.navigationPreload.enable();
  }
  await self.clients.claim();
})()));
self.addEventListener('fetch', (event) => {
  // Everything that is not a page navigation is the browser's own business. Handing it back
  // untouched is faster than proxying it, and this handler exists so the app stays installable.
  if (event.request.mode !== 'navigate') {
    return;
  }
  event.respondWith((async () => {
    try {
      const preloaded = await event.preloadResponse;
      if (preloaded) {
        return preloaded;
      }
      return await fetch(event.request);
    } catch (err) {
      return new Response(
        '<!doctype html><meta charset="utf-8"><title>Offline</title>' +
        '<p style="font-family:system-ui;text-align:center;margin-top:3rem">' +
        "You're offline. Try again once you're back online.</p>",
        { headers: { 'Content-Type': 'text/html' } }
      );
    }
  })());
});
`

// serveServiceWorker sends the worker with no-cache, which is not belt-and-braces: a stale
// service worker is the worst thing in this app to have cached, because it sits in front of every
// navigation and outlives any page. The reverse proxy in front of this app was matching /sw.js as
// "*.js" and stamping it `max-age=31536000, immutable` (found 2026-09-24); browsers cap a worker
// script's own cache at 24h regardless, so the damage was bounded rather than permanent -- but a
// day is still a day of serving the previous worker to everyone who had it.
func serveServiceWorker(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", "application/javascript")
	_, _ = w.Write([]byte(serviceWorkerJS))
}

// serveAssetLinks publishes Digital Asset Links for the Google Play (Trusted Web Activity) build of
// this PWA. Without this file Chrome shows the TWA with its address bar, as an ordinary browser tab.
// The package name and signing fingerprints are deployment facts that exist only after the app is
// registered in Play Console, so they come from config (ANDROID_PACKAGE_NAME,
// ANDROID_CERT_FINGERPRINTS) rather than from a literal here; with either unset the route is a 404,
// because an assetlinks.json naming no package would claim nothing and read as a bug.
func serveAssetLinks(cfg config.Config) http.HandlerFunc {
	type target struct {
		Namespace    string   `json:"namespace"`
		PackageName  string   `json:"package_name"`
		Fingerprints []string `json:"sha256_cert_fingerprints"`
	}
	type statement struct {
		Relation []string `json:"relation"`
		Target   target   `json:"target"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.AndroidPackageName == "" || len(cfg.AndroidCertFingerprints) == 0 {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]statement{{
			Relation: []string{"delegate_permission/common.handle_all_urls"},
			Target: target{
				Namespace:    "android_app",
				PackageName:  cfg.AndroidPackageName,
				Fingerprints: cfg.AndroidCertFingerprints,
			},
		}})
	}
}
