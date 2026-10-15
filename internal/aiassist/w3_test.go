package aiassist

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// w3Workspace is w2Workspace plus the runtime's notification log, which a notifying Application needs.
func w3Workspace(t *testing.T, withNotification bool) string {
	t.Helper()
	manifest := w2Workspace(t)
	if !withNotification {
		return manifest
	}
	src, err := os.ReadFile(filepath.Join("..", "..", "metadata", "notification.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(manifest), "..", "notification.yaml"), src, 0o644); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(manifest)
	if err := os.WriteFile(manifest, []byte(strings.Replace(string(b), "  - ../user.yaml\n", "  - ../user.yaml\n  - ../notification.yaml\n", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	return manifest
}

// notifyingApp is the Desain Request pattern the owner wrote by hand: tell the designer when a request is
// assigned, the requester when it is delivered, and the coordinator when one is created.
func notifyingApp() *GeneratedApplication {
	return &GeneratedApplication{
		ID: "app_w3", Name: "Permintaan", Roles: []string{"staf"}, PublisherRole: "staf",
		Machines: []GeneratedMachine{{ID: "mch_w3_request", Name: "Permintaan",
			Fields: []GeneratedField{
				{ID: "fld_title", Name: "Judul", Type: "text", Required: true},
				{ID: "fld_status", Name: "Status", Type: "status", Required: true, Options: []string{"baru", "ditugaskan", "selesai"}},
				{ID: "fld_requester", Name: "Pemohon", Type: "person"},
				{ID: "fld_designer", Name: "Desainer", Type: "person"},
			},
			Events: []GeneratedEvent{
				{ID: "evt_log", OnCreate: true, Summary: "Dibuat: {fld_title}"},
				{ID: "evt_assigned", Service: "send_notification", On: "fld_status", WhenEquals: "ditugaskan", RecipientField: "fld_designer", PreferenceKey: "assigned", Summary: "Kamu ditugaskan: {fld_title}"},
				{ID: "evt_done", Service: "send_notification", On: "fld_status", WhenEquals: "selesai", RecipientField: "fld_requester", PreferenceKey: "decided", Summary: "Selesai: {fld_title}"},
			}}},
		Navigation: []GeneratedMenuItem{{Label: "Permintaan", MachineID: "mch_w3_request"}},
	}
}

// TestW3_notificationEventsAreValidatedWrittenLoadedAndReadBack: roadmap stage 1, W3. The writer no longer
// forces log_activity: a send_notification event is written as such, loads through the real loader, and
// is read back with its recipient and preference key.
func TestW3_notificationEventsAreValidatedWrittenLoadedAndReadBack(t *testing.T) {
	manifest := w3Workspace(t, true)
	change := GeneratedChange{Kind: KindNewApplication, Application: notifyingApp()}
	state, _ := stateOf(t, manifest)
	if err := Validate(change, state); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if _, err := Write(manifest, change, nil); err != nil {
		t.Fatalf("Write: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(filepath.Dir(manifest), "w2", "w3_request.yaml"))
	for _, want := range []string{"service: send_notification", "recipient_field: fld_designer", "preference_key: assigned", "service: log_activity"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("the written file lacks %q:\n%s", want, b)
		}
	}
	state, _ = stateOf(t, manifest)
	got := DescribeApplication("app_w3", state.Applications["app_w3"]).Machines[0].Events
	want := notifyingApp().Machines[0].Events
	if len(got) != len(want) {
		t.Fatalf("read back %d events, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		w := want[i]
		w.Service = w.EventService() // a log_activity event reads back with the explicit service name
		g := got[i]
		if g.EventService() != w.EventService() || g.RecipientField != w.RecipientField || g.PreferenceKey != w.PreferenceKey || g.Summary != w.Summary {
			t.Errorf("event %s read back %+v, want %+v", w.ID, g, w)
		}
	}
}

func TestW3_eachMisuseIsRefusedForItsOwnReason(t *testing.T) {
	cases := []struct {
		name         string
		edit         func(a *GeneratedApplication)
		withNotifier bool
		want         string
	}{
		{"a recipient that is not a field", func(a *GeneratedApplication) { a.Machines[0].Events[1].RecipientField = "fld_nope" }, true, "recipient_field"},
		{"a recipient that is not a person", func(a *GeneratedApplication) { a.Machines[0].Events[1].RecipientField = "fld_title" }, true, "must be a person field"},
		{"an unknown preference key", func(a *GeneratedApplication) { a.Machines[0].Events[1].PreferenceKey = "whenever" }, true, "preference_key"},
		{"a recipient on log_activity", func(a *GeneratedApplication) { a.Machines[0].Events[0].RecipientField = "fld_designer" }, true, "belong to send_notification"},
		{"a service the assistant cannot emit", func(a *GeneratedApplication) { a.Machines[0].Events[0].Service = "rollup_parent_status" }, true, "not one a generated event may name"},
		{"a workspace with no notification machine", func(a *GeneratedApplication) {}, false, "mch_notification"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manifest := w3Workspace(t, tc.withNotifier)
			app := notifyingApp()
			tc.edit(app)
			state, _ := stateOf(t, manifest)
			err := Validate(GeneratedChange{Kind: KindNewApplication, Application: app}, state)
			if err == nil {
				t.Fatalf("Validate accepted %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("refusal for %s does not say %q: %v", tc.name, tc.want, err)
			}
		})
	}
}

// An update adds a notification, keeps one the model never changed, and refuses to change an existing one.
func TestW3_updatesAddAndPreserveNotifications(t *testing.T) {
	manifest := w3Workspace(t, true)
	app := notifyingApp()
	app.Machines[0].Events = app.Machines[0].Events[:2] // the update adds the third
	if _, err := Write(manifest, GeneratedChange{Kind: KindNewApplication, Application: app}, nil); err != nil {
		t.Fatalf("Write: %v", err)
	}
	desire := func(edit func(m *GeneratedMachine)) GeneratedChange {
		state, _ := stateOf(t, manifest)
		d := DescribeApplication("app_w3", state.Applications["app_w3"])
		edit(&d.Machines[0])
		return GeneratedChange{Kind: KindUpdateApplication, TargetAppID: "app_w3", Application: &d}
	}
	state, _ := stateOf(t, manifest)
	if err := Validate(desire(func(m *GeneratedMachine) { m.Events[1].RecipientField = "fld_requester" }), state); err == nil || !strings.Contains(err.Error(), "cannot be changed") {
		t.Errorf("changing an existing notification's recipient was not refused as a change: %v", err)
	}
	publish(t, manifest, desire(func(m *GeneratedMachine) {
		m.Events = append(m.Events, GeneratedEvent{ID: "evt_done", Service: "send_notification", On: "fld_status", WhenEquals: "selesai", RecipientField: "fld_requester", PreferenceKey: "decided", Summary: "Selesai: {fld_title}"})
	}))
	state, _ = stateOf(t, manifest)
	got := DescribeApplication("app_w3", state.Applications["app_w3"]).Machines[0].Events
	if len(got) != 3 || got[1].RecipientField != "fld_designer" || got[2].RecipientField != "fld_requester" || got[2].PreferenceKey != "decided" {
		t.Errorf("after the update the notifications are %+v", got)
	}
}

// The response schema offers the model exactly the services it may emit and exactly the preference keys the
// runtime knows, so it cannot return one outside them.
func TestW3_schemaOffersOnlyWhatTheRuntimeKnows(t *testing.T) {
	props := generatedEventSchema.Properties
	if got := strings.Join(props["service"].Enum, ","); got != "log_activity,send_notification" {
		t.Errorf("service enum = %q", got)
	}
	keys := props["preference_key"].Enum
	if len(keys) == 0 || len(keys) != len(domainNotificationKeys()) {
		t.Fatalf("preference_key enum = %v, want every key the runtime knows %v", keys, domainNotificationKeys())
	}
	for _, k := range keys {
		if !domainNotificationKeysSet()[k] {
			t.Errorf("preference_key offers %q, which the runtime does not know", k)
		}
	}
}

func domainNotificationKeys() []string { return notificationPreferenceEnum() }

func domainNotificationKeysSet() map[string]bool {
	out := map[string]bool{}
	for k := range domain.KnownNotificationPreferenceKeys {
		out[k] = true
	}
	return out
}
