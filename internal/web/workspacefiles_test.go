package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"menata.app/internal/authorization"
)

func (s *installTestSetup) getAs(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: s.cookie})
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)
	return rec
}

// The picker lists exactly the .yaml files the Workspace owns, ?path= opens one with its current content, and a
// path that is not on the list -- whatever it points at -- shows the picker and reads nothing.
func TestWorkspaceFilesScreenPicksOnlyOwnedFilesAndReadsOnlyThose(t *testing.T) {
	s := newInstallTestSetup(t, "wfpicker")
	picker := s.getAs(t, "/workspace-settings/files")
	if picker.Code != http.StatusOK {
		t.Fatalf("GET = %d: %s", picker.Code, picker.Body.String())
	}
	for _, want := range []string{"document.yaml", "applications/tracking.yaml", "path=applications%2Ftracking.yaml"} {
		if !strings.Contains(picker.Body.String(), want) {
			t.Errorf("the picker does not offer %q", want)
		}
	}
	if strings.Contains(picker.Body.String(), "<textarea") {
		t.Error("the picker draws an editor before a file was chosen")
	}

	open := s.getAs(t, "/workspace-settings/files?path=document.yaml")
	if !strings.Contains(open.Body.String(), "<textarea") || !strings.Contains(open.Body.String(), "Tracked Document") {
		t.Errorf("an owned file did not open with its content: %d", open.Code)
	}

	for _, escape := range []string{"../user.yaml", "../../.env", "/etc/passwd", "nothere.yaml"} {
		rec := s.getAs(t, "/workspace-settings/files?path="+url.QueryEscape(escape))
		if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "<textarea") {
			t.Errorf("path %q opened an editor (%d); only a listed file may", escape, rec.Code)
		}
	}
}

// A review shows the loader's verdict and the diff and writes nothing; only a file that loads and differs offers
// Apply, and the Apply form carries the exact path and content that were reviewed.
func TestWorkspaceFilesReviewOffersApplyOnlyForAFileThatLoadsAndChanges(t *testing.T) {
	s := newInstallTestSetup(t, "wfreview")
	live := filepath.Join(filepath.Dir(s.manifestPath), s.slug, "document.yaml")
	before, err := os.ReadFile(live)
	if err != nil {
		t.Fatal(err)
	}
	review := func(path, body string) string {
		t.Helper()
		rec := s.postTo(t, "/workspace-settings/files", url.Values{"path": {path}, "yaml": {body}})
		if rec.Code != http.StatusOK {
			t.Fatalf("review of %q answered %d: %s", path, rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}
	const applyForm = `action="/workspace-settings/apply"`

	good := review("document.yaml", string(before)+"  - id: fld_extra\n    name: Extra\n    type: text\n")
	for _, want := range []string{"Loads", "+  - id: fld_extra", applyForm, `name="path" value="document.yaml"`, `name="yaml" value="`} {
		if !strings.Contains(good, want) {
			t.Errorf("a loadable change is missing %q", want)
		}
	}

	bad := review("document.yaml", strings.Replace(string(before), "type: text", "type: bogus", 1))
	if !strings.Contains(bad, "Rejected") || strings.Contains(bad, applyForm) {
		t.Errorf("a file the loader rejects must say Rejected and offer no Apply (apply form present: %v)", strings.Contains(bad, applyForm))
	}

	same := review("document.yaml", string(before))
	if !strings.Contains(same, "Nothing would change") || strings.Contains(same, applyForm) {
		t.Error("an unchanged file must say so and offer no Apply")
	}

	unowned := review("nothere.yaml", "id: x\n")
	if !strings.Contains(unowned, "Rejected") || strings.Contains(unowned, applyForm) {
		t.Error("a file the Workspace does not have must be Rejected with no Apply")
	}

	// A file the Workspace owns but nothing loads parses and differs, yet was not checked: no Apply.
	writeTestFile(t, filepath.Join(filepath.Dir(s.manifestPath), s.slug, "orphan.yaml"), "id: mch_orphan\nname: Orphan\n")
	if unchecked := review("orphan.yaml", "id: mch_orphan\nname: Orphan Two\n"); !strings.Contains(unchecked, "Not checked") || strings.Contains(unchecked, applyForm) {
		t.Error("a changed file nothing loads must say Not checked and offer no Apply")
	}

	if after, _ := os.ReadFile(live); string(after) != string(before) {
		t.Error("a review changed the live file")
	}
}

// A process started without a metadata directory has no files to show; that is an environment fact (503), not
// a server error, on both the picker and the review.
func TestWorkspaceFilesScreenAnswers503WithoutAMetadataDirectory(t *testing.T) {
	s := newInstallTestSetup(t, "wfnodir")
	s.deps.Cfg.MetadataPath = ""
	s.handler = Routes(s.deps)
	if rec := s.getAs(t, "/workspace-settings/files"); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("GET = %d, want 503", rec.Code)
	}
	if rec := s.postTo(t, "/workspace-settings/files", url.Values{"path": {"document.yaml"}, "yaml": {"x: 1\n"}}); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("POST = %d, want 503", rec.Code)
	}
}
