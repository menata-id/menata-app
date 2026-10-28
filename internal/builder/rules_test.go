package builder

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"menata.app/internal/aiassist"
)

// installedForRules publishes an Application with the Fields the four examples need (a person, a date, a status) in a Workspace that has the notification and activity Machines a
// rule writes to.
func installedForRules(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "workspaces"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"user.yaml", "notification.yaml", "activity.yaml"} {
		src, err := os.ReadFile(filepath.Join("..", "..", "metadata", f))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, f), src, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	manifest := filepath.Join(dir, "workspaces", "nana.yaml")
	if err := os.WriteFile(manifest, []byte("workspace: nana\nmachines:\n  - ../user.yaml\n  - ../notification.yaml\n  - ../activity.yaml\napplications: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	change := aiassist.GeneratedChange{Kind: aiassist.KindNewApplication, Application: &aiassist.GeneratedApplication{
		ID: appID, Name: "Sampah", Roles: []string{"approver", "submitter"}, PublisherRole: "approver",
		Machines: []aiassist.GeneratedMachine{{ID: "mch_catat", Name: "Catat", Fields: []aiassist.GeneratedField{
			{ID: "fld_judul", Name: "Judul", Type: "text", Required: true},
			{ID: "fld_pemilik", Name: "Pemilik", Type: "person"},
			{ID: "fld_tenggat", Name: "Tenggat", Type: "date"},
			{ID: "fld_status", Name: "Status", Type: "status", Required: true, Options: []string{"draft", "sent", "closed"}},
		}, CardFields: []aiassist.GeneratedCardField{{Field: "fld_judul", Role: "title"}},
			Permissions: []aiassist.GeneratedPermission{{ID: "prm_catat_create", Action: "create", Roles: []string{"approver"}}}}},
		Navigation: []aiassist.GeneratedMenuItem{{Label: "Catat", MachineID: "mch_catat"}},
	}}
	if _, err := aiassist.Write(manifest, change, nil); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	return manifest
}

// Each of the four examples makes an ordinary rule that loads: the person a record is assigned to is told, a record that reaches a status is forwarded to a role, a person is reminded
// when a date has passed (the reminder and the entry the runtime reads to tell it already ran), and a record is closed by itself when a date has passed.
func TestTheFourRuleExamplesMakeRulesThatLoad(t *testing.T) {
	for name, tc := range map[string]struct {
		op   Op
		want []string
	}{
		"assigned": {Op{Kind: OpAddRule, Machine: "mch_catat", Item: RuleExampleAssigned, Field: "fld_pemilik"}, []string{"recipient_field: fld_pemilik", "was assigned to you"}},
		"forward":  {Op{Kind: OpAddRule, Machine: "mch_catat", Item: RuleExampleForward, Field: "fld_status", Value: "sent", Roles: []string{"approver"}}, []string{"when_equals: sent", "recipient_role: approver"}},
		"remind":   {Op{Kind: OpAddScheduledRule, Machine: "mch_catat", Item: RuleExampleRemind, Field: "fld_tenggat", Watch: "fld_status", Value: "closed"}, []string{"date_field: fld_tenggat", "guard_not_equals: closed", "service: log_activity", "preference_key: sla_breach"}},
		"close":    {Op{Kind: OpAddScheduledRule, Machine: "mch_catat", Item: RuleExampleClose, Field: "fld_tenggat", Watch: "fld_status", Value: "closed"}, []string{"service: set_field", "target_field: fld_status", "value: closed", "guard_not_equals: closed"}},
	} {
		manifest := installedForRules(t)
		if err := Commit(manifest, appID, []Op{tc.op}, mustVersion(t, manifest), nil); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		var machineFile string
		for path, text := range tree(t, manifest) {
			if strings.HasSuffix(path, "catat.yaml") {
				machineFile = text
			}
		}
		for _, want := range tc.want {
			if !strings.Contains(machineFile, want) {
				t.Errorf("%s: the Machine's file lacks %q:\n%s", name, want, machineFile)
			}
		}
	}
	// Each refuses what it cannot make, in words.
	manifest := installedForRules(t)
	for name, op := range map[string]Op{
		"assigned over a text field":            {Kind: OpAddRule, Machine: "mch_catat", Item: RuleExampleAssigned, Field: "fld_judul"},
		"forward to a status not there":         {Kind: OpAddRule, Machine: "mch_catat", Item: RuleExampleForward, Field: "fld_status", Value: "gone", Roles: []string{"approver"}},
		"forward with no role":                  {Kind: OpAddRule, Machine: "mch_catat", Item: RuleExampleForward, Field: "fld_status", Value: "sent"},
		"forward to a role not declared":        {Kind: OpAddRule, Machine: "mch_catat", Item: RuleExampleForward, Field: "fld_status", Value: "sent", Roles: []string{"janitor"}},
		"remind over a status field":            {Kind: OpAddScheduledRule, Machine: "mch_catat", Item: RuleExampleRemind, Field: "fld_status", Watch: "fld_status", Value: "closed"},
		"close with no last status":             {Kind: OpAddScheduledRule, Machine: "mch_catat", Item: RuleExampleClose, Field: "fld_tenggat", Watch: "fld_status", Value: "nope"},
		"an example that is not one":            {Kind: OpAddRule, Machine: "mch_catat", Item: "teleport", Field: "fld_status"},
		"a scheduled example by the wrong kind": {Kind: OpAddRule, Machine: "mch_catat", Item: RuleExampleClose, Field: "fld_tenggat", Watch: "fld_status", Value: "closed"},
	} {
		if _, err := Review(manifest, appID, []Op{op}); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

// A rule can be switched off and on again -- the one change an existing rule takes -- and a switched-off rule is written as `enabled: false`.
func TestARuleIsSwitchedOffAndOn(t *testing.T) {
	manifest := installedForRules(t)
	if err := Commit(manifest, appID, []Op{{Kind: OpAddRule, Machine: "mch_catat", Item: RuleExampleAssigned, Field: "fld_pemilik"}}, mustVersion(t, manifest), nil); err != nil {
		t.Fatal(err)
	}
	rule := current(t, manifest).Machines[0].Events[0].ID
	if err := Commit(manifest, appID, []Op{{Kind: OpSetRuleEnabled, Machine: "mch_catat", Item: rule, Flag: false}}, mustVersion(t, manifest), nil); err != nil {
		t.Fatal(err)
	}
	if !current(t, manifest).Machines[0].Events[0].Disabled {
		t.Error("the rule was not switched off")
	}
	if err := Commit(manifest, appID, []Op{{Kind: OpSetRuleEnabled, Machine: "mch_catat", Item: rule, Flag: true}}, mustVersion(t, manifest), nil); err != nil {
		t.Fatal(err)
	}
	if current(t, manifest).Machines[0].Events[0].Disabled {
		t.Error("the rule stayed switched off")
	}
	if _, err := Apply(current(t, manifest), []Op{{Kind: OpSetRuleEnabled, Machine: "mch_catat", Item: "evt_nowhere", Flag: false}}); err == nil {
		t.Error("a rule that is not there was switched")
	}
}

// A new rule is never given the id of one the machine already has but this grammar cannot describe (a rule on a date): the second reminder of a Machine is refused for being the same
// rule only if it is, never because the ids collided.
func TestANewRuleDoesNotTakeTheIdOfAHiddenOne(t *testing.T) {
	manifest := installedForRules(t)
	op := Op{Kind: OpAddScheduledRule, Machine: "mch_catat", Item: RuleExampleRemind, Field: "fld_tenggat", Watch: "fld_status", Value: "closed"}
	if err := Commit(manifest, appID, []Op{op}, mustVersion(t, manifest), nil); err != nil {
		t.Fatal(err)
	}
	if err := Commit(manifest, appID, []Op{op}, mustVersion(t, manifest), nil); err != nil {
		t.Fatalf("a second reminder collided with the first one's ids: %v", err)
	}
	for _, id := range []string{"evt_catat_remind_log_2", "evt_catat_remind_2"} {
		found := false
		for path, text := range tree(t, manifest) {
			found = found || (strings.HasSuffix(path, "catat.yaml") && strings.Contains(text, "id: "+id))
		}
		if !found {
			t.Errorf("the second reminder lacks %s", id)
		}
	}
}
