package web

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"menata.app/internal/rendering"
)

// fingerprintedAssets are the static files a rendered page names by URL, and therefore the only
// ones a stale copy can break a page with. Everything else under static/ is reached *from* these
// (app.css names the Ubuntu woff2 faces; the icons are named by the manifest), and those are
// content-addressed by the sheet that names them rather than by a page.
//
// Keyed by the URL path a page emits, which is also the key rendering.assetURL looks up -- one
// list, so a file added here and forgotten in the renderer is impossible rather than silent.
var fingerprintedAssets = map[string]string{
	"/css/app.css":               "static/css/app.css",
	"/vendor/htmx.min.js":        "static/vendor/htmx.min.js",
	"/vendor/hyperscript.min.js": "static/vendor/hyperscript.min.js",
}

// fingerprintPattern matches the ".<8 hex>" a fingerprinted URL carries before its extension, so
// the file server can be handed the real filename. Eight hex characters of SHA-256 is 32 bits:
// the collision that matters here is not adversarial, it is "two different builds of app.css
// happen to hash alike", and 1 in 4 billion against a handful of files per deploy is a risk with
// no practical shape.
var fingerprintPattern = regexp.MustCompile(`\.[0-9a-f]{8}(\.[^./]+)$`)

// installAssetFingerprints hashes each file in fingerprintedAssets and hands the result to
// internal/rendering, so the URL a page emits and the file this router serves come from one
// computation.
//
// A missing or unreadable file is skipped rather than fatal: it degrades to the un-fingerprinted
// URL, which is exactly what the app did before this existed. A server that refuses to start
// because a stylesheet moved would trade a caching problem for an outage.
func installAssetFingerprints() {
	prints := make(map[string]string, len(fingerprintedAssets))
	for urlPath, diskPath := range fingerprintedAssets {
		if fp, ok := fingerprintOf(diskPath); ok {
			prints[urlPath] = fp
		}
	}
	rendering.SetAssetFingerprints(prints)
}

// fingerprintOf is the hash itself, split out from the loop above so it can be tested against a
// file a test writes rather than against the repo's own -- a test that reads static/css/app.css
// only passes when it happens to run from the repo root, and one that skips otherwise is a test
// passing for the wrong reason.
func fingerprintOf(diskPath string) (string, bool) {
	data, err := os.ReadFile(filepath.Clean(diskPath))
	if err != nil {
		return "", false
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:8], true
}

// staticAssets serves dir at /<prefix>/ with the fingerprint stripped back off the request path,
// and sets the cache policy the fingerprint earns. dir is a parameter rather than "static/"+prefix
// so the policy can be tested against a temp directory instead of the repo's own files.
//
// The policy is the whole point of the change and is deliberately two-sided:
//
//   - a request that *carries* a fingerprint is immutable for a year. It can be, because its URL
//     changes the moment the bytes do -- that is what a content hash buys, and without it a long
//     max-age is the dangerous setting rather than the fast one (ROADMAP.md deferred exactly this,
//     naming that reason: "the filenames carry no content hash, so a long max-age would serve
//     stale assets after a deploy").
//   - a request without one is `no-cache`: still cached, but revalidated every time. Those are the
//     direct hits -- a bookmarked /css/app.css, a probe, anything that predates this change -- and
//     they must never go stale, because nothing about their URL would ever tell the browser to
//     look again.
func staticAssets(prefix, dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.StripPrefix("/"+prefix+"/", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		stripped := fingerprintPattern.ReplaceAllString(req.URL.Path, "$1")
		if stripped != req.URL.Path {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			req = req.Clone(req.Context())
			req.URL.Path = stripped
		} else if !strings.HasSuffix(req.URL.Path, "/") {
			w.Header().Set("Cache-Control", "no-cache")
		}
		fs.ServeHTTP(w, req)
	}))
}
