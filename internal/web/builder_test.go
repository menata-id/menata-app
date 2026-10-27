package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/aiassist"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/metadata"
	"menata.app/internal/rendering"
)

// The builder's plan route answers what a list of operations would do, refuses what the policy refuses with the
// assistant's words, and writes nothing.
// builderFixture installs a small Application in a throwaway Workspace on its own metadata directory and returns
// what a request to the builder routes needs.
type builderFixture struct {
	root, manifest string
	post           func(path, body string) *httptest.ResponseRecorder
	// ctx carries the Workspace (scope and current Workspace), store and cfg are what a runtime action's env needs.
	ctx   context.Context
	store *data.Store
	cfg   config.Config
}

func newBuilderFixture(t *testing.T, reload func(string) error) builderFixture {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const email = "builder_plan_test@example.com"
	ws, err := store.CreateWorkspace(ctx, "Builder Plan Test", "builder-plan-test-workspace")
	if err != nil {
		t.Fatal(err)
	}
	cleanupAuthTest(t, pool, ws.ID, email)
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)

	root := t.TempDir()
	dir := filepath.Join(root, "workspaces") // the production layout: shared Machines sit beside it, named as ../user.yaml
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "user.yaml"), []byte("id: mch_user\nname: User\nfields:\n  - id: fld_name\n    name: Name\n    type: text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(dir, ws.Slug+".yaml")
	if err := os.WriteFile(manifest, []byte("workspace: "+ws.Slug+"\nmachines:\n  - ../user.yaml\napplications: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{SessionSecret: "builder-plan-test-secret", MetadataPath: dir}
	if _, err := aiassist.Write(manifest, aiassist.GeneratedChange{Kind: aiassist.KindNewApplication, Application: &aiassist.GeneratedApplication{
		ID: "app_bp", Name: "Laporan", Roles: []string{"staf"}, PublisherRole: "staf",
		Navigation: []aiassist.GeneratedMenuItem{{Label: "Laporan", MachineID: "mch_bp_laporan"}},
		Machines: []aiassist.GeneratedMachine{{ID: "mch_bp_laporan", Name: "Laporan", Fields: []aiassist.GeneratedField{
			{ID: "fld_judul", Name: "Judul", Type: "text", Required: true},
			{ID: "fld_status", Name: "Status", Type: "status", Options: []string{"draft", "sent"}},
		}}},
	}}, nil); err != nil {
		t.Fatal(err)
	}
	loaded, err := metadata.LoadApplication(manifest)
	if err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	r.Post("/builder/{app}/plan", planBuilderOperations(store, cfg))
	r.Post("/builder/{app}/apply", applyBuilderOperations(store, cfg, reload))
	return builderFixture{root: root, manifest: manifest, store: store, cfg: cfg, ctx: rendering.WithCurrentWorkspace(wsCtx, loaded.Workspace, "Builder Plan Test", false), post: func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req = req.WithContext(rendering.WithCurrentWorkspace(wsCtx, loaded.Workspace, "Builder Plan Test", false))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}}
}

func TestPlanBuilderOperations_reviewsAndWritesNothing(t *testing.T) {
	fx := newBuilderFixture(t, nil)
	root := fx.root
	before := dirContents(t, root)

	post := func(app, body string) (*httptest.ResponseRecorder, builderPlanView) {
		rec := fx.post("/builder/"+app+"/plan", body)
		var view builderPlanView
		_ = json.Unmarshal(rec.Body.Bytes(), &view)
		return rec, view
	}

	rec, view := post("app_bp", `{"ops":[{"kind":"rename_field","machine":"mch_bp_laporan","field":"fld_judul","value":"Judul laporan"},{"kind":"add_option","machine":"mch_bp_laporan","field":"fld_status","value":"done"}]}`)
	if rec.Code != http.StatusOK || len(view.Items) != 2 || len(view.Files) == 0 || !strings.Contains(view.Files[0].Diff, "Judul laporan") {
		t.Fatalf("plan = %d %+v\n%s", rec.Code, view, rec.Body.String())
	}

	rec, view = post("app_bp", `{"ops":[{"kind":"remove_option","machine":"mch_bp_laporan","field":"fld_status","value":"sent"}]}`)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(view.Error, "cannot be removed: records may hold it") {
		t.Errorf("a policy refusal = %d %q, want 422 with the assistant's words", rec.Code, view.Error)
	}
	if rec, _ = post("app_nope", `{"ops":[{"kind":"rename_machine","machine":"mch_bp_laporan","value":"x"}]}`); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("an application not installed here = %d, want 422", rec.Code)
	}
	for _, body := range []string{``, `{}`, `{"ops":[]}`, `{"ops":[{"kind":"rename_field","bogus":1}]}`} {
		if rec, _ = post("app_bp", body); rec.Code != http.StatusBadRequest {
			t.Errorf("body %q = %d, want 400", body, rec.Code)
		}
	}
	if after := dirContents(t, root); after != before {
		t.Error("the plan route changed the Workspace's files")
	}
}

func dirContents(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			c, _ := os.ReadFile(p)
			b.WriteString(p + "\n" + string(c) + "\n")
		}
		return nil
	})
	return b.String()
}

// The apply route writes the reviewed plan once, behind the version it was made against and the owner's tick on a removal,
// and reloads the Workspace.
func TestApplyBuilderOperations_writesBehindVersionAndTick(t *testing.T) {
	var reloaded []string
	fx := newBuilderFixture(t, func(slug string) error { reloaded = append(reloaded, slug); return nil })
	plan := func(ops string) builderPlanView {
		var v builderPlanView
		_ = json.Unmarshal(fx.post("/builder/app_bp/plan", `{"ops":`+ops+`}`).Body.Bytes(), &v)
		return v
	}
	rename := `[{"kind":"rename_field","machine":"mch_bp_laporan","field":"fld_judul","value":"Judul laporan"}]`
	remove := `[{"kind":"remove_field","machine":"mch_bp_laporan","field":"fld_judul"}]`
	apply := func(ops, version, confirm string) *httptest.ResponseRecorder {
		return fx.post("/builder/app_bp/apply", `{"ops":`+ops+`,"version":"`+version+`","confirm":`+confirm+`}`)
	}

	reviewed := plan(rename)
	if reviewed.Version == "" {
		t.Fatal("a plan carries no version")
	}
	if rec := fx.post("/builder/app_bp/apply", `{"ops":`+rename+`}`); rec.Code != http.StatusBadRequest {
		t.Errorf("an apply with no version = %d, want 400", rec.Code)
	}
	if rec := apply(rename, reviewed.Version, `[]`); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"applied":true`) {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body.String())
	}
	if len(reloaded) != 1 || reloaded[0] != "builder-plan-test-workspace" {
		t.Errorf("the Workspace was reloaded %v, want once", reloaded)
	}
	if b, _ := os.ReadFile(filepath.Join(fx.root, "workspaces", "builder-plan-test-workspace", "bp_laporan.yaml")); !strings.Contains(string(b), "Judul laporan") {
		t.Errorf("the Machine file lacks the rename:\n%s", b)
	}
	// The same plan again is stale: the installation moved.
	if rec := apply(rename, reviewed.Version, `[]`); rec.Code != http.StatusConflict {
		t.Errorf("a stale apply = %d, want 409", rec.Code)
	}
	// A removal waits for the tick named in the plan.
	fresh := plan(remove)
	if len(fresh.Items) != 1 || fresh.Items[0].Confirm == "" {
		t.Fatalf("a removal's plan names no confirmation: %+v", fresh.Items)
	}
	before := dirContents(t, fx.root)
	if rec := apply(remove, fresh.Version, `[]`); rec.Code != http.StatusUnprocessableEntity || dirContents(t, fx.root) != before {
		t.Errorf("an unticked removal = %d (files changed: %v)", rec.Code, dirContents(t, fx.root) != before)
	}
	if rec := apply(remove, fresh.Version, `["`+fresh.Items[0].Confirm+`"]`); rec.Code != http.StatusOK {
		t.Errorf("a ticked removal = %d %s", rec.Code, rec.Body.String())
	}
}

// The two runtime actions switch an Application off and on through its file, reload exactly when something changed, and
// answer 422 for an Application that is not installed here.
func TestApplicationSwitchActions(t *testing.T) {
	var reloads int
	fx := newBuilderFixture(t, func(string) error { reloads++; return nil })
	env := actionEnv{store: fx.store, cfg: fx.cfg, reload: func(string) error { reloads++; return nil }}
	run := func(runner actionRunner, app string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		runner(rec, httptest.NewRequest(http.MethodPost, "/runtime-actions/x?record="+app, nil).WithContext(fx.ctx), env, app)
		return rec
	}
	fileOf := func() string {
		b, _ := os.ReadFile(filepath.Join(fx.root, "workspaces", "builder-plan-test-workspace.yaml"))
		a, _ := os.ReadFile(filepath.Join(fx.root, "workspaces", "builder-plan-test-workspace", "applications", "bp.yaml"))
		return string(b) + string(a)
	}
	if rec := run(runDeactivateApplication, "app_bp"); rec.Code != http.StatusOK || reloads != 1 || !strings.Contains(fileOf(), "inactive: true") {
		t.Fatalf("deactivate = %d, reloads %d, file switched: %v", rec.Code, reloads, strings.Contains(fileOf(), "inactive: true"))
	}
	if rec := run(runDeactivateApplication, "app_bp"); rec.Code != http.StatusOK || reloads != 1 {
		t.Errorf("deactivating twice = %d with %d reloads, want a no-op", rec.Code, reloads)
	}
	if rec := run(runReactivateApplication, "app_bp"); rec.Code != http.StatusOK || reloads != 2 || strings.Contains(fileOf(), "inactive") {
		t.Errorf("reactivate = %d, reloads %d, key gone: %v", rec.Code, reloads, !strings.Contains(fileOf(), "inactive"))
	}
	if rec := run(runDeactivateApplication, "app_nope"); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("an Application not installed here = %d, want 422", rec.Code)
	}
}
