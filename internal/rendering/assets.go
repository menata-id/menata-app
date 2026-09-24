package rendering

import (
	"strings"
	"sync/atomic"
)

// assetFingerprints maps a static asset's own path ("/css/app.css") to a short content hash,
// installed once at startup by internal/web.Routes. Empty until then, which is what makes every
// test that renders a page without building a router keep working: assetURL falls back to the
// plain path.
//
// A package-level value rather than a context one, and the distinction this repo draws elsewhere
// is exactly why. Which Workspace a request is in is a *per-request* fact and lives on ctx
// (CLAUDE.md, "One manifest per Workspace"); a file's content hash is a property of the files this
// binary was built and deployed with -- identical for every request, every viewer and every
// Workspace, and it changes only when the process restarts. Putting it on ctx would claim a
// variability it does not have, and threading it as a parameter would reach ~20 page components
// to carry one constant.
//
// atomic.Pointer rather than a plain map because `go test -race` renders pages concurrently while
// a router may be built in another test.
var assetFingerprints atomic.Pointer[map[string]string]

// SetAssetFingerprints installs the fingerprints the URLs in rendered pages will carry. Called
// once, from internal/web.Routes, with the hashes it computed off the files it serves -- one
// computation, so the URL a page emits and the file the router serves can never disagree.
func SetAssetFingerprints(m map[string]string) {
	copied := make(map[string]string, len(m))
	for k, v := range m {
		copied[k] = v
	}
	assetFingerprints.Store(&copied)
}

// assetURL turns "/css/app.css" into "/css/app.<hash>.css" -- the URL a page's <link>/<script>
// actually carries.
//
// The fingerprint goes in the *filename*, not a ?v= query. Both change the URL when the bytes
// change, which is all a browser needs, but a query string is the weaker form: some proxies and
// CDNs drop it from the cache key or refuse to cache a URL that has one at all, and this app is
// already served through one (caddy) with more to come. A filename has no such special cases.
//
// This is what closes the failure the owner hit twice on 2026-09-24: `app.css` had no fingerprint
// and no Cache-Control at all, so browsers fell back to heuristic freshness and kept serving a
// stylesheet from before a deploy. The symptom was a UI that "still looks the old way" after a
// restart -- a cache problem wearing a rendering problem's clothes, which is the expensive kind.
func assetURL(path string) string {
	m := assetFingerprints.Load()
	if m == nil {
		return path
	}
	fp := (*m)[path]
	if fp == "" {
		return path
	}
	dot := strings.LastIndex(path, ".")
	if dot < 0 {
		return path
	}
	return path[:dot] + "." + fp + path[dot:]
}

// AssetURLForTest exposes assetURL to internal/web's own test, which owns the other half of this
// mechanism (the fingerprints and the cache headers) and is the only place the two can be checked
// as one thing. Exported for that alone -- a page calls the unexported assetURL.
func AssetURLForTest(path string) string { return assetURL(path) }
