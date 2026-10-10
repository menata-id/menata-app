package conformance

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestSettingsHubNamesNoApplication holds S2.1's reason for existing: the Settings hub is drawn for
// whichever Application declares one, so the files that draw it may not carry a word of any one
// Application. Before it, appsettings.templ held a "Document types" placeholder, a Notifications row
// and an Approval flow row that every hub-bearing Application would have shown, whether or not it
// had documents. A zero-floor gate, installed after the migration (build, migrate, then gate).
//
// Comments are stripped first, so explaining what the page used to carry does not count. It reads
// the three files that decide what a hub shows; a navigation id, a route or an Application word in
// any of them is the regression.
func TestSettingsHubNamesNoApplication(t *testing.T) {
	banned := regexp.MustCompile(`(?i)document|approval|nav_app_settings|/document-approval|task|sprint|board`)
	files := []string{
		filepath.Join("internal", "rendering", "appsettings.templ"),
		filepath.Join("internal", "composition", "appsettings.go"),
		filepath.Join("internal", "web", "appsettings.go"),
	}
	for _, f := range files {
		src := stripLineComments(readFile(t, filepath.Join(repoRoot(), f)))
		if strings.TrimSpace(src) == "" {
			t.Fatalf("%s read as empty -- the gate would measure nothing", f)
		}
		for _, loc := range banned.FindAllStringIndex(src, -1) {
			line := strings.Count(src[:loc[0]], "\n") + 1
			t.Errorf("%s (comments stripped) line %d names %q -- a Settings hub is drawn for any Application, so what it says about one must come from that Application's navigation", f, line, src[loc[0]:loc[1]])
		}
	}
}
