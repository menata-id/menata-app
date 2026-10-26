package aiassist

import (
	"strings"
	"testing"
)

// The log of 2026-10-22 (session aise_387fe67e): "why can't I create a card?" was answered with a guess ("you may not
// hold one of those roles"), because the model was never told which roles the person holds. It is told now: for each
// installed Application, the roles the person talking holds and, per permission, the roles it asks for. Only that
// person's own roles -- never another member's.
func TestViewerRoles_theModelIsToldWhatThePersonHoldsAndWhatEachActionAsks(t *testing.T) {
	apps := []InstalledApplication{{Current: GeneratedApplication{ID: "app_pm", Name: "Project Management", Roles: []string{"admin", "member"},
		Machines: []GeneratedMachine{{ID: "mch_card", Name: "Card", Permissions: []GeneratedPermission{
			{ID: "prm_card_create", Action: "create", Roles: []string{"admin", "member"}},
			{ID: "prm_card_delete", Action: "delete", Roles: []string{"admin"}},
			{ID: "prm_card_edit", Action: "edit", ActorField: "fld_owner"},
		}}}}}}

	none := ViewerRolesSection(apps, map[string][]string{})
	for _, want := range []string{"Project Management", "you hold no role", "create Card: admin, member", "delete Card: admin", "edit Card: only the person named in fld_owner"} {
		if !strings.Contains(none, want) {
			t.Errorf("section for a person with no role lacks %q:\n%s", want, none)
		}
	}
	member := ViewerRolesSection(apps, map[string][]string{"app_pm": {"member"}, "app_other": {"zzrole"}})
	if !strings.Contains(member, "you hold: member") {
		t.Errorf("section does not say the person holds member:\n%s", member)
	}
	if strings.Contains(member, "zzrole") {
		t.Errorf("a role in an Application that is not installed here leaked into the prompt:\n%s", member)
	}
	if ViewerRolesSection(nil, nil) != "" {
		t.Error("with nothing installed there is nothing to say")
	}
}
