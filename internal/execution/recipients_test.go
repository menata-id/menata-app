package execution

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// An Event that tells a role reaches everyone who holds it in this Workspace, directly or through a Group, once each;
// not someone holding it in another Application, not a deactivated member, not a member of another Workspace.
func TestSendNotification_toARole(t *testing.T) {
	pool := testPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	ws, err := store.CreateWorkspace(ctx, "Notify Role Test", "notify-role-test-workspace")
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateWorkspace(ctx, "Notify Role Other", "notify-role-other-workspace")
	if err != nil {
		t.Fatal(err)
	}
	cleanupTest(t, pool, ws.ID, "notify_role_direct@example.com")
	cleanupTest(t, pool, other.ID, "notify_role_other@example.com")
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)
	otherCtx := data.WithWorkspaceScope(ctx, other.ID)

	member := func(c context.Context, workspaceID, name string) string {
		u, err := store.CreateRecord(c, domain.UserMachineID, map[string]any{"fld_name": name, "fld_email": "notify_role_" + name + "@example.com"})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.AddMember(ctx, workspaceID, u.ID, "notify_role_"+name+"@example.com", "member", ""); err != nil {
			t.Fatal(err)
		}
		return u.ID
	}
	direct := member(wsCtx, ws.ID, "direct")
	viaGroup := member(wsCtx, ws.ID, "viagroup")
	both := member(wsCtx, ws.ID, "both")
	wrongApp := member(wsCtx, ws.ID, "wrongapp")
	wrongRole := member(wsCtx, ws.ID, "wrongrole")
	gone := member(wsCtx, ws.ID, "gone")
	elsewhere := member(otherCtx, other.ID, "elsewhere")

	grant := func(workspaceID, user, app, role string) {
		if err := store.SetMemberAppRole(ctx, workspaceID, user, app, role); err != nil {
			t.Fatal(err)
		}
	}
	grant(ws.ID, direct, "app_x", "koordinator")
	grant(ws.ID, both, "app_x", "koordinator")
	grant(ws.ID, wrongApp, "app_y", "koordinator")
	grant(ws.ID, wrongRole, "app_x", "staf")
	grant(ws.ID, gone, "app_x", "koordinator")
	grant(other.ID, elsewhere, "app_x", "koordinator")
	// The same person also belongs to this Workspace, with no role here: the role they hold in the other Workspace
	// must not reach them (a user record is one person's identity per Workspace, a grant belongs to one Workspace).
	if err := store.AddMember(ctx, ws.ID, elsewhere, "notify_role_elsewhere@example.com", "member", ""); err != nil {
		t.Fatal(err)
	}
	if err := store.DeactivateMember(ctx, ws.ID, gone); err != nil {
		t.Fatal(err)
	}
	g, err := store.CreateGroup(ctx, ws.ID, "Koordinator")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetGroupAppRole(ctx, g.ID, "app_x", "koordinator"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetGroupMembers(ctx, g.ID, []string{viaGroup, both}); err != nil {
		t.Fatal(err)
	}

	machine := &domain.Machine{ID: "mch_role_fixture", ApplicationID: "app_x"}
	record := &data.Record{ID: "rec_fixture", Values: map[string]any{}}
	machines := map[string]*domain.Machine{machine.ID: machine}
	sendNotification(wsCtx, store, &spyMailer{}, machines, machine, record, domain.Notify{RecipientRole: "koordinator", PreferenceKey: "assigned"}, "Request baru")

	notes, err := store.ListRecords(wsCtx, "mch_notification")
	if err != nil {
		t.Fatal(err)
	}
	var told []string
	for _, n := range notes {
		told = append(told, fmt.Sprint(n.Values["fld_recipient"]))
	}
	slices.Sort(told)
	want := []string{direct, viaGroup, both}
	slices.Sort(want)
	if !slices.Equal(told, want) {
		t.Errorf("told %v, want exactly %v (direct, via a Group, and both once)", told, want)
	}
	if otherNotes, _ := store.ListRecords(otherCtx, "mch_notification"); len(otherNotes) != 0 {
		t.Errorf("another Workspace was told: %d notifications", len(otherNotes))
	}

	// The same machinery tells a Group's members, and only that Workspace's Group.
	before := len(notes)
	gm := &domain.Machine{ID: "mch_group_fixture", ApplicationID: "app_x"}
	grec := &data.Record{ID: "rec_g", Values: map[string]any{"fld_tim": g.ID}}
	sendNotification(wsCtx, store, &spyMailer{}, map[string]*domain.Machine{gm.ID: gm}, gm, grec, domain.Notify{RecipientGroupField: "fld_tim", PreferenceKey: "assigned"}, "Untuk tim")
	notes, _ = store.ListRecords(wsCtx, "mch_notification")
	if got := len(notes) - before; got != 2 {
		t.Errorf("a group notification told %d people, want its 2 members", got)
	}
	foreign := &data.Record{ID: "rec_f", Values: map[string]any{"fld_tim": g.ID}}
	sendNotification(otherCtx, store, &spyMailer{}, map[string]*domain.Machine{gm.ID: gm}, gm, foreign, domain.Notify{RecipientGroupField: "fld_tim", PreferenceKey: "assigned"}, "Lintas")
	if foreignNotes, _ := store.ListRecords(otherCtx, "mch_notification"); len(foreignNotes) != 0 {
		t.Errorf("a Group of another Workspace told %d people", len(foreignNotes))
	}
	if n := len(notificationRecipients(wsCtx, store, gm, &data.Record{Values: map[string]any{}}, domain.Notify{RecipientGroupField: "fld_tim"})); n != 0 {
		t.Errorf("an empty group Field told %d people", n)
	}
}
