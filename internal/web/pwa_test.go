package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"menata.app/internal/config"
)

func TestAssetLinks_publishesTheConfiguredAndroidApp(t *testing.T) {
	cfg := config.Config{AndroidPackageName: "app.menata.twa", AndroidCertFingerprints: []string{"AA:BB", "CC:DD"}}
	rec := httptest.NewRecorder()
	serveAssetLinks(cfg)(rec, httptest.NewRequest(http.MethodGet, "/.well-known/assetlinks.json", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got []struct {
		Relation []string `json:"relation"`
		Target   struct {
			Namespace    string   `json:"namespace"`
			PackageName  string   `json:"package_name"`
			Fingerprints []string `json:"sha256_cert_fingerprints"`
		} `json:"target"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body is not the Digital Asset Links shape: %v\n%s", err, rec.Body.String())
	}
	if len(got) != 1 || got[0].Relation[0] != "delegate_permission/common.handle_all_urls" ||
		got[0].Target.Namespace != "android_app" || got[0].Target.PackageName != "app.menata.twa" ||
		strings.Join(got[0].Target.Fingerprints, ",") != "AA:BB,CC:DD" {
		t.Errorf("unexpected statement: %+v", got)
	}
}

func TestAssetLinks_isNotFoundUntilBothHalvesAreConfigured(t *testing.T) {
	for name, cfg := range map[string]config.Config{
		"nothing":      {},
		"package only": {AndroidPackageName: "app.menata.twa"},
		"prints only":  {AndroidCertFingerprints: []string{"AA:BB"}},
	} {
		rec := httptest.NewRecorder()
		serveAssetLinks(cfg)(rec, httptest.NewRequest(http.MethodGet, "/.well-known/assetlinks.json", nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", name, rec.Code)
		}
	}
}

func TestManifest_declaresMaskableIconsAndAStableID(t *testing.T) {
	var m struct {
		ID    string `json:"id"`
		Icons []struct {
			Sizes   string `json:"sizes"`
			Purpose string `json:"purpose"`
		} `json:"icons"`
	}
	if err := json.Unmarshal([]byte(manifestJSON), &m); err != nil {
		t.Fatalf("manifest is not valid JSON: %v", err)
	}
	if m.ID != "/home" {
		t.Errorf("id = %q, want /home (the start_url installs already derived it from)", m.ID)
	}
	seen := map[string]bool{}
	for _, ic := range m.Icons {
		seen[ic.Purpose+" "+ic.Sizes] = true
	}
	for _, want := range []string{"any 192x192", "any 512x512", "maskable 192x192", "maskable 512x512"} {
		if !seen[want] {
			t.Errorf("manifest has no %q icon", want)
		}
	}
}
