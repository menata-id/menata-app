package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"menata.app/internal/rendering"
)

// TestFingerprintedAssetCachePolicy gates a failure that is invisible from inside the app: a
// browser serving a stylesheet from before the deploy. The page renders, every test passes, and
// the only symptom is a person reporting that the UI "still looks the old way" -- which is what
// happened twice on 2026-09-24, and cost a round of debugging the rendering rather than the cache.
//
// Both halves are asserted because each is dangerous without the other. A fingerprint with no
// long max-age buys nothing; a long max-age with no fingerprint is the setting that *causes* the
// bug (ROADMAP.md deferred `Cache-Control` for exactly that reason, naming it).
func TestFingerprintedAssetCachePolicy(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.css"), []byte("body{}"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	h := staticAssets("css", dir)

	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec
	}

	// A fingerprinted URL resolves to the real file and may be cached forever, because its own
	// URL changes the moment the bytes do.
	hashed := get("/css/app.deadbeef.css")
	if hashed.Code != http.StatusOK {
		t.Fatalf("fingerprinted asset: status = %d, want 200 -- the hash is not being stripped", hashed.Code)
	}
	if got, want := hashed.Header().Get("Cache-Control"), "public, max-age=31536000, immutable"; got != want {
		t.Errorf("fingerprinted asset: Cache-Control = %q, want %q", got, want)
	}

	// The bare URL still works -- a bookmark, a probe, anything predating the fingerprint -- and
	// must revalidate every time, since nothing about that URL would ever tell a browser to look
	// again.
	bare := get("/css/app.css")
	if bare.Code != http.StatusOK {
		t.Fatalf("bare asset: status = %d, want 200", bare.Code)
	}
	if got, want := bare.Header().Get("Cache-Control"), "no-cache"; got != want {
		t.Errorf("bare asset: Cache-Control = %q, want %q -- an un-fingerprinted URL must never go stale", got, want)
	}
}

// TestAssetURLChangesWithContent is the other half of the same guarantee, on the renderer's side:
// the URL a page emits must move when the file does. Both halves are exercised for real -- the
// hash against a file this test writes, the URL against the renderer that emits it -- rather than
// against the repo's own static/, which only resolves when the test happens to run from the root.
func TestAssetURLChangesWithContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.css")
	write := func(body string) string {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		fp, ok := fingerprintOf(path)
		if !ok {
			t.Fatal("fingerprintOf could not read the file it was just handed")
		}
		rendering.SetAssetFingerprints(map[string]string{"/css/app.css": fp})
		return rendering.AssetURLForTest("/css/app.css")
	}

	before := write("body{color:red}")
	after := write("body{color:blue}")
	if before == after {
		t.Errorf("assetURL returned %q for two different files -- a redeploy would bust no cache, which is the whole mechanism", before)
	}
	for _, url := range []string{before, after} {
		if !fingerprintPattern.MatchString(url) {
			t.Errorf("assetURL = %q, want the hash before the extension so staticAssets can strip it back off", url)
		}
	}

	// A path nobody fingerprinted passes through untouched, which is what keeps every test that
	// renders a page without building a router working.
	if got := rendering.AssetURLForTest("/icons/favicon-32.png"); got != "/icons/favicon-32.png" {
		t.Errorf("unfingerprinted path = %q, want it unchanged", got)
	}
}
