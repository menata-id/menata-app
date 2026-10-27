package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
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
	for _, want := range []string{"document.yaml", ">applications</summary>", ">tracking.yaml<", "path=applications%2Ftracking.yaml"} {
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

// review pastes content for one file through the review action and reads the page that answers, the way the admin does.
func (s *installTestSetup) review(t *testing.T, path, body string) (code int, page string) {
	t.Helper()
	rec := s.postTo(t, "/runtime-actions/review_file?record="+url.QueryEscape(path), url.Values{"yaml": {body}})
	if rec.Code != http.StatusSeeOther {
		return rec.Code, rec.Body.String()
	}
	return rec.Code, s.getAs(t, rec.Header().Get("Location")).Body.String()
}

// A review shows the loader's verdict and the diff and writes nothing; only a file that loads and differs offers
// Apply, and Apply replaces the file with exactly the draft that was reviewed.
func TestWorkspaceFilesReviewOffersApplyOnlyForAFileThatLoadsAndChanges(t *testing.T) {
	s := newInstallTestSetup(t, "wfreview")
	live := filepath.Join(filepath.Dir(s.manifestPath), s.slug, "document.yaml")
	before, err := os.ReadFile(live)
	if err != nil {
		t.Fatal(err)
	}
	const apply = "apply_file"

	_, good := s.review(t, "document.yaml", string(before)+"  - id: fld_extra\n    name: Extra\n    type: text\n")
	for _, want := range []string{"Loads", "+  - id: fld_extra", apply} {
		if !strings.Contains(good, want) {
			t.Errorf("a loadable change is missing %q", want)
		}
	}

	_, bad := s.review(t, "document.yaml", strings.Replace(string(before), "type: text", "type: bogus", 1))
	if !strings.Contains(bad, "Rejected") || strings.Contains(bad, apply) {
		t.Errorf("a file the loader rejects must say Rejected and offer no Apply (present: %v)", strings.Contains(bad, apply))
	}

	_, same := s.review(t, "document.yaml", string(before))
	if !strings.Contains(same, "Unchanged") || strings.Contains(same, apply) {
		t.Error("an unchanged file must say so and offer no Apply")
	}

	if code, _ := s.review(t, "nothere.yaml", "id: x\n"); code != http.StatusNotFound {
		t.Errorf("a file the Workspace does not have = %d, want 404", code)
	}

	// A file the Workspace owns but nothing loads parses and differs, yet was not checked: no Apply.
	writeTestFile(t, filepath.Join(filepath.Dir(s.manifestPath), s.slug, "orphan.yaml"), "id: mch_orphan\nname: Orphan\n")
	if _, unchecked := s.review(t, "orphan.yaml", "id: mch_orphan\nname: Orphan Two\n"); !strings.Contains(unchecked, "Not checked") || strings.Contains(unchecked, apply) {
		t.Error("a changed file nothing loads must say Not checked and offer no Apply")
	}

	if after, _ := os.ReadFile(live); string(after) != string(before) {
		t.Error("a review changed the live file")
	}
}

// Apply acts on the reviewed draft and nothing else: with no draft it refuses, with one it writes that content.
func TestWorkspaceFilesApplyWritesTheReviewedDraftOnly(t *testing.T) {
	s := newInstallTestSetup(t, "wfapply")
	live := filepath.Join(filepath.Dir(s.manifestPath), s.slug, "document.yaml")
	before, _ := os.ReadFile(live)
	if rec := s.postTo(t, "/runtime-actions/apply_file?record=document.yaml", url.Values{"yaml": {"id: smuggled\n"}}); rec.Code != http.StatusConflict {
		t.Errorf("apply with no reviewed draft = %d, want 409", rec.Code)
	}
	if after, _ := os.ReadFile(live); string(after) != string(before) {
		t.Fatal("an apply with no draft changed the file")
	}
	draft := string(before) + "  - id: fld_applied\n    name: Applied\n    type: text\n"
	s.review(t, "document.yaml", draft)
	if rec := s.postTo(t, "/runtime-actions/apply_file?record=document.yaml", url.Values{"yaml": {"id: smuggled\n"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("apply = %d: %s", rec.Code, rec.Body.String())
	}
	if after, _ := os.ReadFile(live); string(after) != draft {
		t.Error("apply did not write the reviewed draft (or wrote the posted body)")
	}
	if rec := s.postTo(t, "/runtime-actions/apply_file?record=document.yaml", url.Values{}); rec.Code != http.StatusConflict {
		t.Errorf("a second apply = %d, want 409: the draft is spent once applied", rec.Code)
	}
	if rec := s.postTo(t, "/runtime-actions/apply_file?record=nothere.yaml", url.Values{}); rec.Code != http.StatusNotFound {
		t.Errorf("apply of a file the Workspace does not have = %d, want 404", rec.Code)
	}
}

// A process started without a metadata directory has no files to show; that is an environment fact (503), not
// a server error, on both the picker and the review.
func TestWorkspaceFilesScreenAnswers503WithoutAMetadataDirectory(t *testing.T) {
	s := newInstallTestSetup(t, "wfnodir")
	s.deps.Cfg.MetadataPath = ""
	s.handler = Routes(s.deps)
	if rec := s.getAs(t, "/workspace-settings/files"); rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "document.yaml") {
		t.Errorf("GET = %d: a process with no metadata directory lists no files, and the page still draws", rec.Code)
	}
	if code, _ := s.review(t, "document.yaml", "x: 1\n"); code != http.StatusServiceUnavailable {
		t.Errorf("review = %d, want 503", code)
	}
}

// The picker is a Tree: a file under a folder sits beneath that folder's heading and says its last segment, and the file the request opened is the
// one row marked current. A request naming none marks none.
func TestWorkspaceFilesScreenListsFilesUnderTheirFoldersAndMarksTheOpenOne(t *testing.T) {
	s := newInstallTestSetup(t, "wftree")

	none := s.getAs(t, "/workspace-settings/files").Body.String()
	if !strings.Contains(none, "<summary") || !strings.Contains(none, ">applications</summary>") {
		t.Errorf("the picker does not draw the applications folder as a heading:\n%s", none)
	}
	if strings.Contains(none, `aria-current="true"`) {
		t.Error("no file was opened and a row is marked current")
	}
	row := regexp.MustCompile(`(?s)<li[^>]*aria-current="true"[^>]*>(.*?)</li>`).FindAllStringSubmatch(
		s.getAs(t, "/workspace-settings/files?path=applications%2Ftracking.yaml").Body.String(), -1)
	if len(row) != 1 {
		t.Fatalf("want exactly the opened file's row marked current, got %d", len(row))
	}
	if !regexp.MustCompile(`>tracking\.yaml<`).MatchString(row[0][1]) || strings.Contains(row[0][1], "applications/tracking.yaml<") {
		t.Errorf("the marked row names the file by its last segment only: %s", row[0][1])
	}
}
