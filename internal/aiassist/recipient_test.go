package aiassist

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func roleNotifyingApp(role, group, field string) *GeneratedApplication {
	return &GeneratedApplication{
		ID: "app_rcpt", Name: "Permintaan", Roles: []string{"koordinator", "staf"}, PublisherRole: "staf",
		Machines: []GeneratedMachine{{ID: "mch_rcpt_request", Name: "Permintaan",
			Fields: []GeneratedField{
				{ID: "fld_title", Name: "Judul", Type: "text", Required: true},
				{ID: "fld_pelapor", Name: "Pelapor", Type: "person"},
				{ID: "fld_catatan", Name: "Catatan", Type: "text"},
				{ID: "fld_tim", Name: "Tim", Type: "group"},
			},
			Events: []GeneratedEvent{{ID: "evt_baru", OnCreate: true, Service: EventServiceSendNotification,
				RecipientRole: role, RecipientGroupField: group, RecipientField: field, PreferenceKey: "assigned", Summary: "Permintaan baru: {fld_title}"}}}},
		Navigation: []GeneratedMenuItem{{Label: "Permintaan", MachineID: "mch_rcpt_request"}},
	}
}

func TestRoleAndGroupNotificationsAreWrittenLoadedAndDescribedBack(t *testing.T) {
	for name, tc := range map[string]struct{ role, group, want string }{
		"a role":  {"koordinator", "", "recipient_role: koordinator"},
		"a group": {"", "fld_tim", "recipient_group_field: fld_tim"},
	} {
		manifest := w3Workspace(t, true)
		change := GeneratedChange{Kind: KindNewApplication, Application: roleNotifyingApp(tc.role, tc.group, "")}
		state, _ := stateOf(t, manifest)
		if err := Validate(change, state); err != nil {
			t.Fatalf("%s: Validate: %v", name, err)
		}
		if _, err := Write(manifest, change, nil); err != nil {
			t.Fatalf("%s: Write: %v", name, err)
		}
		b, _ := os.ReadFile(filepath.Join(filepath.Dir(manifest), "w2", "rcpt_request.yaml"))
		if !strings.Contains(string(b), tc.want) {
			t.Errorf("%s: the written file lacks %q:\n%s", name, tc.want, b)
		}
		state, _ = stateOf(t, manifest)
		got := DescribeApplication("app_rcpt", state.Applications["app_rcpt"]).Machines[0].Events[0]
		if got.RecipientRole != tc.role || got.RecipientGroupField != tc.group || got.RecipientField != "" {
			t.Errorf("%s: described back %+v", name, got)
		}
	}
}

func TestNotificationRecipientsAreHeldToTheLoadersRules(t *testing.T) {
	for name, tc := range map[string]struct {
		role, group, field string
		want               string
	}{
		"a role the application lacks": {role: "bos", want: "bos"},
		"two recipients at once":       {role: "koordinator", field: "fld_pelapor", want: "exactly one"},
		"no recipient":                 {want: "exactly one"},
		"a group field that is text":   {group: "fld_catatan", want: "group field"},
	} {
		manifest := w3Workspace(t, true)
		state, _ := stateOf(t, manifest)
		err := Validate(GeneratedChange{Kind: KindNewApplication, Application: roleNotifyingApp(tc.role, tc.group, tc.field)}, state)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: Validate = %v, want an error containing %q", name, err, tc.want)
		}
	}
}

// An update may add a notification to a role, and the role is held to the application's vocabulary on that path too.
func TestAnUpdateMayTellARoleButOnlyOneTheApplicationDeclares(t *testing.T) {
	manifest := w3Workspace(t, true)
	base := roleNotifyingApp("koordinator", "", "")
	base.Machines[0].Events = nil
	state, _ := stateOf(t, manifest)
	if err := Validate(GeneratedChange{Kind: KindNewApplication, Application: base}, state); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(manifest, GeneratedChange{Kind: KindNewApplication, Application: base}, nil); err != nil {
		t.Fatal(err)
	}
	for role, wantErr := range map[string]bool{"koordinator": false, "bos": true} {
		st, _ := stateOf(t, manifest)
		desired := DescribeApplication("app_rcpt", st.Applications["app_rcpt"])
		desired.Machines[0].Events = append(desired.Machines[0].Events, GeneratedEvent{ID: "evt_baru", OnCreate: true, Service: EventServiceSendNotification,
			RecipientRole: role, PreferenceKey: "assigned", Summary: "Baru"})
		err := Validate(GeneratedChange{Kind: KindUpdateApplication, TargetAppID: "app_rcpt", Application: &desired}, st)
		if (err != nil) != wantErr {
			t.Errorf("role %q: Validate = %v, wantErr %v", role, err, wantErr)
		}
	}
}
