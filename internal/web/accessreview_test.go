package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/aiassist"
	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/metadata"
	"menata.app/internal/rendering"
)

// Publishing a new Application gives the publisher their role and, as the review chose, nobody else, the people ticked, or every active
// member -- the role checked against the Application's own and each person against the Workspace's active members before anything is written.
func TestPublishGivesTheChosenPeopleTheirRole(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	ws, err := store.CreateWorkspace(ctx, "Access Test", "access-review-test-workspace")
	if err != nil {
		t.Fatal(err)
	}
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)
	member := func(email, role string) string {
		cleanupAuthTest(t, pool, ws.ID, email)
		u, err := store.CreateRecord(wsCtx, domain.UserMachineID, map[string]any{"fld_email": email})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.AddMember(ctx, ws.ID, u.ID, email, role, ""); err != nil {
			t.Fatal(err)
		}
		return u.ID
	}
	publisher := member("access_pub@example.com", "admin")
	bea, cai := member("access_bea@example.com", "member"), member("access_cai@example.com", "member")
	gone := member("access_gone@example.com", "member")
	if err := store.DeactivateMember(ctx, ws.ID, gone); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	dir := filepath.Join(root, "workspaces")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(root, "user.yaml"), []byte("id: mch_user\nname: User\nfields:\n  - id: fld_name\n    name: Name\n    type: text\n"), 0o644)
	manifest := filepath.Join(dir, ws.Slug+".yaml")
	os.WriteFile(manifest, []byte("workspace: "+ws.Slug+"\nmachines:\n  - ../user.yaml\napplications: []\n"), 0o644)
	cfg := config.Config{SessionSecret: "access-review-secret", MetadataPath: dir}

	n := 0
	publish := func(form url.Values) (*httptest.ResponseRecorder, string) {
		n++
		appID := "app_access_" + string(rune('a'+n))
		change := aiassist.GeneratedChange{Kind: aiassist.KindNewApplication, Application: &aiassist.GeneratedApplication{
			ID: appID, Name: "Laporan " + appID, Roles: []string{"staf", "lihat"}, PublisherRole: "staf",
			Navigation: []aiassist.GeneratedMenuItem{{Label: "Laporan", MachineID: "mch_" + appID}},
			Machines: []aiassist.GeneratedMachine{{ID: "mch_" + appID, Name: "Laporan",
				Fields: []aiassist.GeneratedField{{ID: "fld_judul", Name: "Judul", Type: "text", Required: true}}}},
		}}
		loaded, err := metadata.LoadApplication(manifest)
		if err != nil {
			t.Fatal(err)
		}
		content, _ := json.Marshal(aiassist.Reply{Message: "ok", Change: &change})
		session, err := store.CreateAISession(wsCtx, ws.ID, publisher, aiassist.KindNewApplication)
		if err != nil {
			t.Fatal(err)
		}
		store.AppendAISessionTurn(wsCtx, session.ID, "model", string(content))
		store.UpdateAISessionStatus(wsCtx, ws.ID, session.ID, data.AISessionStatusGenerated)
		r := chi.NewRouter()
		r.Post("/new-application/{session}/publish", publishNewApplication(store, nil, cfg, func(string) error { return nil }))
		req := httptest.NewRequest(http.MethodPost, "/new-application/"+session.ID+"/publish", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, cfg, publisher, 0)})
		req = req.WithContext(rendering.WithCurrentWorkspace(wsCtx, loaded.Workspace, "Access Test", false))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec, appID
	}
	rolesOf := func(appID string) map[string]string {
		members, err := store.ListMembers(ctx, ws.ID)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]string{}
		for _, m := range members {
			if r := m.AppRoles[appID]; r != "" {
				out[m.UserRecordID] = r
			}
		}
		return out
	}
	same := func(got, want map[string]string) bool {
		if len(got) != len(want) {
			return false
		}
		for k, v := range want {
			if got[k] != v {
				return false
			}
		}
		return true
	}

	rec, app := publish(url.Values{})
	if rec.Code != http.StatusSeeOther || !same(rolesOf(app), map[string]string{publisher: "staf"}) {
		t.Errorf("no choice: %d, roles %v -- want only the publisher", rec.Code, rolesOf(app))
	}
	rec, app = publish(url.Values{"access": {"only_me"}, "people": {bea}, "people_role": {"lihat"}})
	if rec.Code != http.StatusSeeOther || !same(rolesOf(app), map[string]string{publisher: "staf"}) {
		t.Errorf("only me with people ticked: %d, roles %v -- the ticks must be ignored", rec.Code, rolesOf(app))
	}
	rec, app = publish(url.Values{"access": {"some"}, "people": {bea}, "people_role": {"lihat"}})
	if rec.Code != http.StatusSeeOther || !same(rolesOf(app), map[string]string{publisher: "staf", bea: "lihat"}) {
		t.Errorf("some people: %d, roles %v", rec.Code, rolesOf(app))
	}
	rec, app = publish(url.Values{"access": {"everyone"}, "people_role": {"lihat"}})
	if rec.Code != http.StatusSeeOther || !same(rolesOf(app), map[string]string{publisher: "staf", bea: "lihat", cai: "lihat"}) {
		t.Errorf("everyone: %d, roles %v -- a deactivated member must not be granted", rec.Code, rolesOf(app))
	}

	before, _ := os.ReadFile(manifest)
	for name, form := range map[string]url.Values{
		"a role the Application does not declare": {"access": {"everyone"}, "people_role": {"admin"}},
		"no role":                      {"access": {"some"}, "people": {bea}},
		"a person who is not a member": {"access": {"some"}, "people": {"usr_nobody"}, "people_role": {"lihat"}},
		"a deactivated member":         {"access": {"some"}, "people": {gone}, "people_role": {"lihat"}},
		"an unknown choice":            {"access": {"public"}, "people_role": {"lihat"}},
	} {
		if rec, _ := publish(form); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s = %d, want 422", name, rec.Code)
		}
	}
	if after, _ := os.ReadFile(manifest); string(after) != string(before) {
		t.Error("a refused choice still wrote the Workspace")
	}
}
