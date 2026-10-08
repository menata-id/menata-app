package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// bespokeScreens is every `templ …Page(` function in internal/rendering, each classed and given a reason.
// It is the inventory for 007 §12.4's live breach ("a View MUST NOT be the universal composition
// primitive" while screens are bespoke Go functions), and it is **shrink-only in both directions**:
//
//   - a **new** `…Page(` function not listed here fails, and the message says what to do first: can the
//     Application declare the screen with a `page:` block instead? (Once that capability exists. Until
//     then the answer is to list it with a category and a reason.)
//   - an entry whose function **no longer exists** fails too, so a migrated screen is locked in rather
//     than left as a stale excuse.
//
// This gate exists to direct work, like `handWrittenLayoutSites`: the `candidate` rows are the screens the
// `page:` block (plan `menata-app-document/audits/2026-10-07-rencana-page-yaml-dan-gate-yang-akan-terkena.md`)
// is meant to absorb. It is built **after** the plan and **before** the primitive on purpose -- it tests no
// failure class that does not exist yet, only that the inventory of what the primitive replaces stays true.
//
// **Method, stated beside the claim (2026-10-07):** `grep -E '^templ [A-Z][A-Za-z]*Page\('` over
// `internal/rendering/*.templ` -- 34 functions. Counting what is *present* by syntax is sound; the
// *categories* are a reading of each screen and the owning route, not a measurement, and `candidate` is a
// hypothesis the plan's Tahap 0.2 tests by reading the screen (an Application screen whose figures are not
// Dataset measures cannot migrate without an aggregate Dataset -- plan §2.2). Do not read an `outside`
// category as "cannot ever be declarative"; read it as "no Application's `navigation:` owns it, or it is
// already generic over metadata, or it needs Page/Slot (UI IR audit Tahap 3), which is not built".
var bespokeScreens = map[string]struct{ category, reason string }{
	// identity: reached before any Workspace exists, or about the person rather than an Application
	// (runtimeLevelRoutes' own criterion; plan §6; 001 #9).
	"LoginPage":                {"identity", "pre-auth: no Workspace on ctx, so no manifest can declare it"},
	"RegistrationPage":         {"identity", "pre-auth: no Workspace on ctx, so no manifest can declare it"},
	"ForgotPasswordPage":       {"identity", "pre-auth: no Workspace on ctx, so no manifest can declare it"},
	"ResetPasswordPage":        {"identity", "pre-auth: no Workspace on ctx, so no manifest can declare it"},
	"ResendVerificationPage":   {"identity", "pre-auth: no Workspace on ctx, so no manifest can declare it"},
	"CheckYourEmailPage":       {"identity", "pre-auth: no Workspace on ctx, so no manifest can declare it"},
	"AcceptInvitePage":         {"identity", "runs before the invitee is a member of any Workspace"},
	"ChooseWorkspacePage":      {"identity", "chooses the Workspace, so none is on ctx yet"},
	"CreateWorkspacePage":      {"identity", "creates the Workspace in question (see Deps.UserMachine's reason)"},
	"ProfilePage":              {"identity", "the person's own name/email/credential -- owner-only identity data, no Application's"},
	"AccountNotificationsPage": {"identity", "the person's own notification preferences, no Application's"},
	"SecurityPage":             {"identity", "the person's own credential, no Application's"},
	"DeleteAccountPage":        {"identity", "the person's own account deletion (anonymization), no Application's"},
	"DeleteAccountInfoPage":    {"identity", "public, pre-auth: the URL an app-store listing names for deletion requests"},

	// workspace: runtime chrome registered as fixed literals in router.go, owned by no Application's
	// `navigation:` (see runtimeLevelRoutes). Needs a Workspace-level `page:` to be declarable, which
	// the plan does not build.
	"WorkspaceHomePage":        {"workspace", "Workspace Home: aggregates every installed Application's home cards"},
	"WorkspaceSettingsPage":    {"workspace", "Workspace-level settings, not an Application's"},
	"WorkspaceMembersPage":     {"workspace", "Workspace membership; identity data (project_identity-model)"},
	"EditMemberPage":           {"workspace", "Workspace membership; identity data (project_identity-model)"},
	"GroupsPage":               {"workspace", "Workspace groups, registered as a fixed literal"},
	"GroupDetailPage":          {"workspace", "Workspace groups, registered as a fixed literal"},
	"NotificationsPage":        {"workspace", "the person's inbox across Applications"},
	"InferencePage":            {"workspace", "nav_inference is in domain.RuntimeScreens (the runtime owns it, not an Application) and its subject is a runtime artifact, not a Dataset; read 2026-10-07"},
	"InstallApplicationPage":   {"workspace", "installs into the Workspace; form binding is 007 §11.3 (item 3), unbuilt"},
	"NewApplicationPage":       {"workspace", "the AI assistant's prompt form; form binding is 007 §11.3 (item 3), unbuilt"},
	"NewApplicationReviewPage": {"workspace", "the AI assistant's review pane; form binding is 007 §11.3 (item 3), unbuilt"},

	// generic: already one function over every Machine; the target pattern, not debt (001 #9).
	"MachinePage":      {"generic", "one renderer over any Machine's declared fields"},
	"RecordDetailPage": {"generic", "one renderer over any record's declared fields"},

	// orchestration: Document Approval's multi-step engine screens (case-portfolio.md Case 3). They need
	// Page/Slot (UI IR audit Tahap 3) and the write side of Binding, neither built.
	"ApprovalInboxPage":      {"orchestration", "Case 3: three declared nav items over one handler; needs Collection + write-side Binding"},
	"DocumentSubmitPage":     {"orchestration", "Case 3: submit wizard; form binding is 007 §11.3 (item 3), unbuilt"},
	"ReviewDocumentPage":     {"orchestration", "Case 3: decision form and signed-PDF view; form binding unbuilt"},
	"SignaturePlacementPage": {"orchestration", "Case 3: placement editor with client script"},

	// candidate: an Application's own screen, declared by that Application's `navigation:`. These are what
	// `page:` is for. Tahap 0.2 reads each to decide Jalur A (figures are Dataset measures) or B.
	"DashboardPage":           {"candidate", "nav_dashboard; read 2026-10-07: every figure is Go over one ds_all_tasks selection (grouping per project and person, an overdue test against `now`) -- no aggregate Dataset exists for it, so not Jalur A"},
	"MyTasksPage":             {"candidate", "nav_my_tasks; read 2026-10-07: ds_my_tasks selects records, bucketing by date is Go, each row carries a PATCH form (write side, 007 §11.3 unbuilt) -- not Jalur A"},
	"CalendarPage":            {"candidate", "nav_calendar; read 2026-10-07: week grid bucketed in Go from ds_all_tasks, ?week= parameter, PATCH form per card -- not Jalur A"},
	"BoardSettingsPage":       {"candidate", "nav_board_settings; read 2026-10-07: the nearest to Jalur A -- read-only, three Datasets, the one aggregate in the library (ds_label_usage) -- but needs a collection Binding over select:records, a per-row join of two Datasets by record id (007 §7.5), and tagChip is not a registered Component"},
	"ApplicationSettingsPage": {"candidate", "nav_app_settings; read 2026-10-07: reads no Dataset and no record -- links plus a role matrix derived from Machine permissions at request time; a candidate for `page:` as static/link nodes, never for Jalur A"},
}

var bespokeScreenCategories = map[string]bool{
	"identity": true, "workspace": true, "generic": true, "orchestration": true, "candidate": true,
}

var templPageDecl = regexp.MustCompile(`(?m)^templ ([A-Z][A-Za-z]*Page)\(`)

func TestBespokeScreensOnlyShrink(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	found := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		for _, m := range templPageDecl.FindAllStringSubmatch(string(src), -1) {
			found[m[1]] = e.Name()
		}
	}
	if len(found) == 0 {
		t.Fatal("found no `templ …Page(` function in internal/rendering: the pattern or the directory is wrong, and a gate counting zero passes for the wrong reason")
	}

	names := make([]string, 0, len(found))
	for n := range found {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if _, ok := bespokeScreens[n]; !ok {
			t.Errorf("%s (%s) is a new bespoke screen function not listed in bespokeScreens.\n"+
				"          007 §12.4: a View MUST NOT be the universal composition primitive. Ask first whether the Application can declare\n"+
				"          this screen with a `page:` block (plan menata-app-document/audits/2026-10-07-rencana-page-yaml-dan-gate-yang-akan-terkena.md);\n"+
				"          if it cannot yet, list it with a category and a reason -- and if it is an Application's own screen, class it `candidate`.",
				n, found[n])
		}
	}

	var stale []string
	candidates := 0
	for n, entry := range bespokeScreens {
		if !bespokeScreenCategories[entry.category] {
			t.Errorf("bespokeScreens[%q] has category %q, which is not one of the closed set", n, entry.category)
		}
		if strings.TrimSpace(entry.reason) == "" {
			t.Errorf("bespokeScreens[%q] has no reason; an entry without one is an excuse nobody can check", n)
		}
		if entry.category == "candidate" {
			candidates++
		}
		if _, ok := found[n]; !ok {
			stale = append(stale, n)
		}
	}
	sort.Strings(stale)
	for _, n := range stale {
		t.Errorf("bespokeScreens lists %q but no `templ %s(` exists any more -- the screen was migrated or removed. "+
			"Delete the entry so the improvement is locked in; leaving it is room to regress.", n, n)
	}
	t.Logf("%d bespoke screen functions, %d of them `candidate` (the work `page:` is for)", len(found), candidates)
}

// pageBlockDeferral declares that a navigation item cannot yet carry a `page:` block, with the reason and
// the forward pointer that lets a later reader check whether that has changed (`declaredPlaceholders`'
// shape). It must be non-nil exactly while `internal/metadata.navItemDoc` declares no `page` key.
//
// **It fails in both directions, and the second direction is the part that matters.** The day the key
// lands, this entry has to go -- and the moment it goes, `writing-guide.md`, `capabilities.md` and 007
// §40 are required to describe the key. CLAUDE.md records a metadata key shipped without those documents
// once already (Theme), found only because the owner asked where it was written. Note what the gate does
// *not* hold: `internal/installer`'s check-doc mirror carries navigation as `[]any`, so a new nav key
// cannot break it -- measured by probe on 2026-10-07 (a tolerated `page` key failed nothing but the
// route-registration gate), not assumed.
//
// **Cleared 2026-10-07**, when `navItemDoc` gained `page` and `/pages/{navID}` gained its consumers. The
// entry it held said a Metric in YAML "could only hold a hand-typed figure"; `binding:` (a Dataset, a Measure
// and `rows: dimension`) is what ended that. The map stays declared, like any emptied ratchet, so the
// next capability that is declared-but-deferred has a place to say so.
var pageBlockDeferral *struct{ reason, pointer string }

var navItemDocBody = regexp.MustCompile(`(?s)type navItemDoc struct \{(.*?)\n\}`)
var yamlPageTag = regexp.MustCompile("`yaml:\"page(,[^\"]*)?\"`")

func TestPageBlockDeferralStaysTrueAndLandsDocumented(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(repoRoot(), "internal", "metadata", "application.go"))
	if err != nil {
		t.Fatalf("read application.go: %v", err)
	}
	m := navItemDocBody.FindStringSubmatch(string(src))
	if m == nil {
		t.Fatal("could not find `type navItemDoc struct` in internal/metadata/application.go: the pattern is stale, and a gate that cannot find its subject passes for the wrong reason")
	}
	declared := yamlPageTag.MatchString(stripLineComments(m[1]))

	if !declared {
		if pageBlockDeferral == nil {
			t.Error("navItemDoc declares no `page` key, yet pageBlockDeferral is nil: the deferral was cleared before the capability landed")
		}
		return
	}

	if pageBlockDeferral != nil {
		t.Errorf("navItemDoc now declares a `page` key, so the deferral (%q, see %s) has expired on its own terms: set pageBlockDeferral to nil.",
			pageBlockDeferral.reason, pageBlockDeferral.pointer)
	}
	// The capability exists: its documents must too (CLAUDE.md "a new metadata key is not done until...").
	for _, doc := range []string{"writing-guide.md", "capabilities.md", "007-composable-runtime-architecture.md"} {
		b, err := os.ReadFile(filepath.Join(repoRoot(), doc))
		if err != nil {
			t.Fatalf("read %s: %v", doc, err)
		}
		if !strings.Contains(string(b), "`page:`") {
			t.Errorf("`page:` is a declared navigation key but %s never mentions it -- a metadata key is not done until the document an author reads shows how to write it (writing-guide.md), the runtime inventory lists it (capabilities.md) and 007 §40 states its status", doc)
		}
	}
}
