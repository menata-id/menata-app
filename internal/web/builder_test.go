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
func TestPlanBuilderOperations_reviewsAndWritesNothing(t *testing.T) {
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
	before := dirContents(t, root)

	r := chi.NewRouter()
	r.Post("/builder/{app}/plan", planBuilderOperations(store, cfg))
	post := func(app, body string) (*httptest.ResponseRecorder, builderPlanView) {
		req := httptest.NewRequest(http.MethodPost, "/builder/"+app+"/plan", strings.NewReader(body))
		req = req.WithContext(rendering.WithCurrentWorkspace(wsCtx, loaded.Workspace, "Builder Plan Test", false))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
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
