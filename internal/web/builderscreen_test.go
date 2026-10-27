package web

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// The builder's screen lists an Application's parts as a tree -- Machines with their Fields, its screens, its menu -- marks the part the request
// selects and reads it in the inspector, writes nothing, and answers 404 for an Application the Workspace does not have.
func TestBuilderScreenListsTheApplicationsPartsAndReadsTheSelectedOne(t *testing.T) {
	s := newInstallTestSetup(t, "bscreen")
	page := s.getAs(t, "/builder/app_document_tracking")
	if page.Code != http.StatusOK {
		t.Fatalf("GET = %d: %s", page.Code, page.Body.String())
	}
	body := page.Body.String()
	for _, want := range []string{"Document Tracking", ">Data &amp; form</summary>", ">Tracked Document</summary>", ">Title<", ">Views</summary>", ">Documents<", ">Menu</summary>", "Choose a part of the application to read it."} {
		if !strings.Contains(body, want) {
			t.Errorf("the builder does not show %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, `aria-current="true"`) {
		t.Error("nothing was selected and a part is marked")
	}

	node := "app_document_tracking/machines/mch_document/fields/fld_title"
	sel := s.getAs(t, "/builder/app_document_tracking?node="+strings.ReplaceAll(node, "/", "%2F")).Body.String()
	row := regexp.MustCompile(`(?s)<li[^>]*aria-current="true"[^>]*>(.*?)</li>`).FindAllStringSubmatch(sel, -1)
	if len(row) != 1 || !strings.Contains(row[0][1], ">Title<") {
		t.Fatalf("want exactly the Title field marked, got %v", row)
	}
	if !strings.Contains(sel, "machines/mch_document/fields/fld_title") || strings.Contains(sel, "Choose a part of the application to read it.") {
		t.Errorf("the inspector does not read the selected part:\n%s", sel)
	}

	if rec := s.getAs(t, "/builder/app_nobody"); rec.Code != http.StatusNotFound {
		t.Errorf("an Application the Workspace does not have: %d, want 404", rec.Code)
	}
}
