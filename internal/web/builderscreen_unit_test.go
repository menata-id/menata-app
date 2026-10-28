package web

import (
	"context"
	"reflect"
	"testing"

	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

func TestWantsCode_onlyTheSelectedPartOnTheCodeTab(t *testing.T) {
	for _, c := range []struct {
		id, selected, tab string
		want              bool
	}{
		{"a/x", "a/x", "Code", true},
		{"a/x", "a/x", "Details", false},
		{"a/y", "a/x", "Code", false},
		{"", "", "Code", false},
	} {
		if got := wantsCode(c.id, c.selected, c.tab); got != c.want {
			t.Errorf("wantsCode(%q, %q, %q) = %v, want %v", c.id, c.selected, c.tab, got, c.want)
		}
	}
}

// The actor a preview is drawn for holds exactly the previewed role and is an ordinary member -- never the admin's own roles or the Workspace admin
// role -- and outside a preview, or outside an Application, the actor is untouched.
func TestAsPreviewRole_narrowsToOneRoleAndNeverWidens(t *testing.T) {
	admin := domain.Actor{ID: "u1", Roles: map[string][]string{"app_a": {"owner", "reviewer"}, "app_b": {"x"}}, WorkspaceRole: "admin"}
	app := domain.Application{ID: "app_a", Roles: []string{"owner", "reviewer", "clerk"}}
	in := func(role string) context.Context {
		ctx := rendering.WithCurrentApplication(context.Background(), app)
		if role != "" {
			ctx = rendering.WithPreviewRole(ctx, role)
		}
		return ctx
	}

	got := asPreviewRole(in("clerk"), admin)
	if !reflect.DeepEqual(got.Roles, map[string][]string{"app_a": {"clerk"}}) || got.WorkspaceRole != "member" || got.ID != "u1" {
		t.Errorf("previewed as clerk: %+v", got)
	}
	if got := asPreviewRole(in(rendering.PreviewNoRole), admin); len(got.Roles) != 0 || got.WorkspaceRole != "member" {
		t.Errorf("previewed as no role: %+v", got)
	}
	if got := asPreviewRole(in(""), admin); !reflect.DeepEqual(got, admin) {
		t.Errorf("no preview changed the actor: %+v", got)
	}
	if got := asPreviewRole(rendering.WithPreviewRole(context.Background(), "clerk"), admin); !reflect.DeepEqual(got, admin) {
		t.Errorf("a preview outside any Application changed the actor: %+v", got)
	}
	if admin.WorkspaceRole != "admin" || len(admin.Roles["app_a"]) != 2 {
		t.Error("the actor that was given was changed in place")
	}
}
