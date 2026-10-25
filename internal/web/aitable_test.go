package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"menata.app/internal/aiassist"
	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/metadata"
	"menata.app/internal/storage"
)

// TestGeneratedTablesSearchFilterAndScopeEndToEnd is plan 2026-10-25, D2 family 1's proof that what the assistant
// writes for a table is what a person gets: an Application with an "all" table (a search box, status chips) and a
// "mine" table is published, and the real router answers /pages/<id>. The chips count what the data holds, a search
// narrows the rows and keeps the chip, a chip narrows them, an unset control lists everything, and the "mine" table
// shows each of two people only their own records. Passing the loader is not the same as showing the right rows.
func TestGeneratedTablesSearchFilterAndScopeEndToEnd(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const slug, emailA, emailB = "ai-table-test", "ai_table_a@example.com", "ai_table_b@example.com"
	ws, err := store.CreateWorkspace(ctx, "AI Table Test", slug)
	if err != nil {
		t.Fatal(err)
	}
	cleanupAuthTest(t, pool, ws.ID, emailA)
	cleanupAuthTest(t, pool, ws.ID, emailB)
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)

	root := t.TempDir()
	for _, f := range []string{"user.yaml", "activity.yaml", "notification.yaml"} {
		b, err := os.ReadFile(filepath.Join("..", "..", "metadata", f))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, f), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "workspaces"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(root, "workspaces", slug+".yaml")
	if err := os.WriteFile(manifest, []byte("workspace: "+slug+"\nmachines:\n  - ../activity.yaml\n  - ../notification.yaml\n  - ../user.yaml\napplications: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	change := aiassist.GeneratedChange{Kind: aiassist.KindNewApplication, Application: &aiassist.GeneratedApplication{
		ID: "app_ai_tbl", Name: "Permintaan", Roles: []string{"staf"}, PublisherRole: "staf",
		Navigation: []aiassist.GeneratedMenuItem{
			{Label: "Semua permintaan", MachineID: "mch_ai_tbl_minta", List: &aiassist.GeneratedList{
				Scope: aiassist.ListScopeAll, Limit: 50, Columns: []string{"fld_judul", "fld_status", "fld_pemohon"},
				SearchFields: []string{"fld_judul", "fld_catatan"}, FilterField: "fld_status"}},
			{Label: "Milik saya", MachineID: "mch_ai_tbl_minta", List: &aiassist.GeneratedList{
				Scope: aiassist.ListScopeMine, PersonField: "fld_pemohon", Limit: 50, Columns: []string{"fld_judul", "fld_status"}}},
		},
		Machines: []aiassist.GeneratedMachine{{
			ID: "mch_ai_tbl_minta", Name: "Permintaan",
			Fields: []aiassist.GeneratedField{
				{ID: "fld_judul", Name: "Judul", Type: "text", Required: true},
				{ID: "fld_status", Name: "Status", Type: "status", Required: true, Options: []string{"baru", "selesai"}, Default: "baru"},
				{ID: "fld_pemohon", Name: "Pemohon", Type: "person"},
				{ID: "fld_catatan", Name: "Catatan", Type: "long_text"},
			},
			CardFields:  []aiassist.GeneratedCardField{{Field: "fld_judul", Role: "title"}, {Field: "fld_status", Role: "status"}, {Field: "fld_pemohon", Role: "person"}},
			Permissions: []aiassist.GeneratedPermission{{ID: "prm_minta_create", Action: "create", Roles: []string{"staf"}}},
		}},
	}}
	loaded, err := metadata.LoadApplication(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := aiassist.Validate(change, aiassist.ExistingStateFrom(loaded.Workspace)); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if _, err := aiassist.Write(manifest, change, nil); err != nil {
		t.Fatalf("Write: %v", err)
	}
	loaded, err = metadata.LoadApplication(manifest)
	if err != nil {
		t.Fatalf("the generated Workspace does not load: %v", err)
	}
	installed := loaded.Workspace
	installed.Slug = ws.Slug
	routes := map[string]string{}
	for _, n := range installed.Applications[0].AllNavigation {
		routes[n.Label] = n.Route
	}
	allRoute, mineRoute := routes["Semua permintaan"], routes["Milik saya"]
	if allRoute == "" || mineRoute == "" {
		t.Fatalf("the generated menu lacks the two tables: %v", routes)
	}

	member := func(email string) *data.Record {
		u, err := store.CreateRecord(wsCtx, "mch_user", map[string]any{"fld_email": email})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.AddMember(ctx, ws.ID, u.ID, email, "admin", ""); err != nil {
			t.Fatal(err)
		}
		if err := store.SetMemberAppRole(ctx, ws.ID, u.ID, "app_ai_tbl", "staf"); err != nil {
			t.Fatal(err)
		}
		return u
	}
	a, b := member(emailA), member(emailB)
	for _, r := range []map[string]any{
		{"fld_judul": "Laptop baru", "fld_status": "baru", "fld_pemohon": a.ID, "fld_catatan": "untuk desain"},
		{"fld_judul": "Kursi kantor", "fld_status": "selesai", "fld_pemohon": b.ID, "fld_catatan": "ergonomis"},
		{"fld_judul": "Monitor", "fld_status": "baru", "fld_pemohon": b.ID, "fld_catatan": "layar lebar"},
	} {
		if _, err := store.CreateRecord(wsCtx, "mch_ai_tbl_minta", r); err != nil {
			t.Fatal(err)
		}
	}

	cfg := config.Config{SessionSecret: slug + "-secret", TemplatePath: realLibrary(t)}
	files, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h := Routes(Deps{UserMachine: userMachineOf(t, loaded), Store: store, Files: files, Cfg: cfg,
		Workspaces: map[string]domain.Workspace{slug: installed}, DefaultWorkspaceID: ws.ID})
	get := func(userID, route string, q url.Values) string {
		target := route
		if len(q) > 0 {
			target += "?" + q.Encode()
		}
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, cfg, userID, 0)})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d: %s", target, rec.Code, firstChars(rec.Body.String()))
		}
		return rec.Body.String()
	}
	expect := func(what, out string, shown, hidden []string) {
		t.Helper()
		for _, s := range shown {
			if !strings.Contains(out, s) {
				t.Errorf("%s: the page does not show %q", what, s)
			}
		}
		for _, s := range hidden {
			if strings.Contains(out, s) {
				t.Errorf("%s: the page shows %q", what, s)
			}
		}
	}

	all := get(a.ID, allRoute, nil)
	expect("all, nothing chosen", all, []string{"Laptop baru", "Kursi kantor", "Monitor", "Judul", "Status", "Pemohon", `name="q"`}, nil)
	// The chips count what the data holds: baru 2, selesai 1, and the record count for "All".
	expect("chips", all, []string{"baru", "selesai"}, nil)
	expect("a search keeps what matches and drops the rest", get(a.ID, allRoute, url.Values{"q": {"laptop"}}),
		[]string{"Laptop baru"}, []string{"Kursi kantor", "Monitor"})
	expect("a search reads the second field too", get(a.ID, allRoute, url.Values{"q": {"ergonomis"}}),
		[]string{"Kursi kantor"}, []string{"Laptop baru", "Monitor"})
	// A chip is a link the page renders; following it, not typing the parameter, is what a person does. Chips that
	// pointed at a parameter the Dataset does not read would do nothing, and the typed requests below would not show it.
	chip := regexp.MustCompile(`href="([^"]*filter=selesai[^"]*)"`).FindStringSubmatch(all)
	if chip == nil {
		t.Fatalf("the page renders no chip link for the status \"selesai\":\n%s", all)
	}
	link := strings.ReplaceAll(chip[1], "&amp;", "&")
	if strings.HasPrefix(link, "?") { // a chip's link is relative to the page that draws it
		link = allRoute + link
	}
	followed := get(a.ID, link, nil)
	expect("following the rendered chip", followed, []string{"Kursi kantor"}, []string{"Laptop baru", "Monitor"})
	expect("a chip narrows to its status", get(a.ID, allRoute, url.Values{"filter": {"selesai"}}),
		[]string{"Kursi kantor"}, []string{"Laptop baru", "Monitor"})
	expect("a chip and a search together", get(a.ID, allRoute, url.Values{"filter": {"baru"}, "q": {"monitor"}}),
		[]string{"Monitor"}, []string{"Laptop baru", "Kursi kantor"})
	expect("an empty control lists everything", get(a.ID, allRoute, url.Values{"q": {""}, "filter": {""}}),
		[]string{"Laptop baru", "Kursi kantor", "Monitor"}, nil)

	for _, c := range []struct {
		viewer       *data.Record
		sees, hidden []string
	}{
		{a, []string{"Laptop baru"}, []string{"Kursi kantor", "Monitor"}},
		{b, []string{"Kursi kantor", "Monitor"}, []string{"Laptop baru"}},
	} {
		expect("mine, as "+c.viewer.ID, get(c.viewer.ID, mineRoute, nil), c.sees, c.hidden)
	}
}
