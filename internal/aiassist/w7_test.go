package aiassist

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// machineView finds a step of a loaded Machine by id.
type machineView struct{ *domain.Machine }

func (m machineView) transition(id string) domain.Transition {
	for _, t := range m.Transitions {
		if t.ID == id {
			return t
		}
	}
	return domain.Transition{}
}

// journalApp is Case 9 in the shape the assistant generates: an entry posted only when its lines balance.
func journalApp() *GeneratedApplication {
	return &GeneratedApplication{
		ID: "app_w7_jurnal", Name: "Jurnal", Roles: []string{"akuntan", "penyelia"}, PublisherRole: "penyelia",
		Machines: []GeneratedMachine{
			{ID: "mch_w7_entri", Name: "Entri", Fields: []GeneratedField{
				{ID: "fld_uraian", Name: "Uraian", Type: "text", Required: true},
				{ID: "fld_status", Name: "Status", Type: "status", Required: true, Options: []string{"draf", "diposting"}, Default: "draf"},
			}, Transitions: []GeneratedTransition{{ID: "trn_posting", Name: "Posting", Field: "fld_status", From: "draf", To: "diposting", Roles: []string{"penyelia"},
				Balanced: &GeneratedBalanced{Of: "mch_w7_baris", Via: "fld_entri", Debit: "fld_debit", Credit: "fld_kredit", Reason: "Debit dan kredit belum seimbang."}}}},
			{ID: "mch_w7_baris", Name: "Baris", Fields: []GeneratedField{
				{ID: "fld_entri", Name: "Entri", Type: "relation", Required: true, RelatedMachine: "mch_w7_entri"},
				{ID: "fld_akun", Name: "Akun", Type: "text", Required: true},
				{ID: "fld_debit", Name: "Debit", Type: "number"},
				{ID: "fld_kredit", Name: "Kredit", Type: "number"},
			}},
		},
		Navigation: []GeneratedMenuItem{{Label: "Entri", MachineID: "mch_w7_entri"}},
	}
}

// loanApp is Case 14: disbursing a loan writes its instalments.
func loanApp() *GeneratedApplication {
	return &GeneratedApplication{
		ID: "app_w7_pinjaman", Name: "Pinjaman", Roles: []string{"petugas"}, PublisherRole: "petugas",
		Machines: []GeneratedMachine{
			{ID: "mch_w7_pinjaman", Name: "Pinjaman", Fields: []GeneratedField{
				{ID: "fld_jumlah", Name: "Jumlah", Type: "number", Required: true},
				{ID: "fld_bulan", Name: "Bulan", Type: "number", Required: true},
				{ID: "fld_status", Name: "Status", Type: "status", Required: true, Options: []string{"disetujui", "dicairkan"}, Default: "disetujui"},
			}, Transitions: []GeneratedTransition{{ID: "trn_cair", Name: "Cairkan", Field: "fld_status", From: "disetujui", To: "dicairkan",
				Generates: &GeneratedGenerates{Machine: "mch_w7_angsuran", CountField: "fld_bulan", LinkField: "fld_pinjaman", DueField: "fld_jatuh_tempo", DueEveryDays: 30, AmountField: "fld_nilai", AmountFrom: "fld_jumlah"}}}},
			{ID: "mch_w7_angsuran", Name: "Angsuran", Fields: []GeneratedField{
				{ID: "fld_pinjaman", Name: "Pinjaman", Type: "relation", RelatedMachine: "mch_w7_pinjaman"},
				{ID: "fld_jatuh_tempo", Name: "Jatuh tempo", Type: "date"},
				{ID: "fld_nilai", Name: "Nilai", Type: "number"},
			}},
		},
		Navigation: []GeneratedMenuItem{{Label: "Pinjaman", MachineID: "mch_w7_pinjaman"}},
	}
}

// The two edge rules that name another machine are validated, written, loaded and read back: the catalogue's claim that
// the assistant can write them is only as good as this round trip.
func TestW7_balancedAndGeneratesAreWrittenLoadedAndReadBack(t *testing.T) {
	for name, tc := range map[string]struct {
		app  func() *GeneratedApplication
		id   string
		load func(t *testing.T, m machineView)
	}{
		"balanced": {journalApp, "app_w7_jurnal", func(t *testing.T, m machineView) {
			if b := m.transition("trn_posting").Balanced; b == nil || b.Machine != "mch_w7_baris" || b.Via != "fld_entri" || b.Debit != "fld_debit" || b.Credit != "fld_kredit" || b.Reason == "" {
				t.Errorf("the loaded edge's rule = %+v", b)
			}
		}},
		"generates": {loanApp, "app_w7_pinjaman", func(t *testing.T, m machineView) {
			if g := m.transition("trn_cair").Generates; g == nil || g.Machine != "mch_w7_angsuran" || g.CountField != "fld_bulan" || g.DueEveryDays != 30 || g.AmountFrom != "fld_jumlah" {
				t.Errorf("the loaded edge's schedule = %+v", g)
			}
		}},
	} {
		manifest := w2Workspace(t)
		change := GeneratedChange{Kind: KindNewApplication, Application: tc.app()}
		state, _ := stateOf(t, manifest)
		if err := Validate(change, state); err != nil {
			t.Fatalf("%s: Validate: %v", name, err)
		}
		if _, err := Write(manifest, change, nil); err != nil {
			t.Fatalf("%s: Write: %v", name, err)
		}
		state, loaded := stateOf(t, manifest)
		read := DescribeApplication(tc.id, state.Applications[tc.id])
		if !reflect.DeepEqual(read.Machines[0].Transitions, tc.app().Machines[0].Transitions) {
			t.Errorf("%s: read back %+v, want %+v", name, read.Machines[0].Transitions, tc.app().Machines[0].Transitions)
		}
		for _, m := range loaded.Machines {
			if m.ID == tc.app().Machines[0].ID {
				tc.load(t, machineView{m})
			}
		}
	}
}

func TestW7_eachMisuseIsRefusedForItsOwnReason(t *testing.T) {
	for name, tc := range map[string]struct {
		app  func() *GeneratedApplication
		edit func(a *GeneratedApplication)
		want string
	}{
		"lines that do not point back":    {journalApp, func(a *GeneratedApplication) { a.Machines[1].Fields[0].RelatedMachine = "mch_w7_baris" }, "pointing back"},
		"a text debit":                    {journalApp, func(a *GeneratedApplication) { a.Machines[1].Fields[2].Type = "text" }, "debit"},
		"one field for debit and credit":  {journalApp, func(a *GeneratedApplication) { a.Machines[0].Transitions[0].Balanced.Credit = "fld_debit" }, "two different fields"},
		"lines of a machine nobody has":   {journalApp, func(a *GeneratedApplication) { a.Machines[0].Transitions[0].Balanced.Of = "mch_nope" }, "not a machine of this workspace"},
		"a schedule with no spacing":      {loanApp, func(a *GeneratedApplication) { a.Machines[0].Transitions[0].Generates.DueEveryDays = 0 }, "at least 1"},
		"a text due date":                 {loanApp, func(a *GeneratedApplication) { a.Machines[1].Fields[1].Type = "text" }, "due_field"},
		"a schedule that does not return": {loanApp, func(a *GeneratedApplication) { a.Machines[1].Fields[0].RelatedMachine = "mch_w7_angsuran" }, "pointing back"},
		"an unfillable required field": {loanApp, func(a *GeneratedApplication) {
			a.Machines[1].Fields = append(a.Machines[1].Fields, GeneratedField{ID: "fld_catatan", Name: "Catatan", Type: "text", Required: true})
		}, "cannot fill"},
	} {
		app := tc.app()
		tc.edit(app)
		manifest := w2Workspace(t)
		state, _ := stateOf(t, manifest)
		err := Validate(GeneratedChange{Kind: KindNewApplication, Application: app}, state)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want an error containing %q", name, err, tc.want)
		}
	}
}

// An existing step's balance or schedule is not changed by an update: the file edit only appends.
func TestW7_anUpdateCannotChangeAnExistingEdgeRule(t *testing.T) {
	manifest := w2Workspace(t)
	if _, err := Write(manifest, GeneratedChange{Kind: KindNewApplication, Application: journalApp()}, nil); err != nil {
		t.Fatal(err)
	}
	state, _ := stateOf(t, manifest)
	current := DescribeApplication("app_w7_jurnal", state.Applications["app_w7_jurnal"])
	changed := DescribeApplication("app_w7_jurnal", state.Applications["app_w7_jurnal"])
	changed.Machines[0].Transitions[0].Balanced.Credit = "fld_akun"
	if plan := PlanUpdate(current, changed); len(plan.Refusals) != 1 || !strings.Contains(plan.Refusals[0], "trn_posting") {
		t.Errorf("a changed balance rule: refusals = %v, want one naming trn_posting", plan.Refusals)
	}
	removed := DescribeApplication("app_w7_jurnal", state.Applications["app_w7_jurnal"])
	removed.Machines[0].Transitions[0].Balanced = nil
	if plan := PlanUpdate(current, removed); len(plan.Refusals) != 1 {
		t.Errorf("a removed balance rule: refusals = %v, want one", plan.Refusals)
	}
	if plan := PlanUpdate(current, current); len(plan.Refusals) != 0 {
		t.Errorf("an unchanged rule was refused: %v", plan.Refusals)
	}
}

// The response schema is what the model is told it may emit; a slot the struct has and the schema lacks (or the reverse)
// is a capability the catalogue calls writable that the model can never produce.
func TestW7_theSchemaOffersExactlyTheStructsSlots(t *testing.T) {
	check := func(name string, schema geminiSchema, v any) {
		t.Helper()
		var want []string
		rt := reflect.TypeOf(v)
		for i := 0; i < rt.NumField(); i++ {
			want = append(want, strings.Split(rt.Field(i).Tag.Get("json"), ",")[0])
		}
		var got []string
		for k := range schema.Properties {
			got = append(got, k)
		}
		sort.Strings(want)
		sort.Strings(got)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: schema offers %v, struct has %v", name, got, want)
		}
	}
	check("transition", generatedTransitionSchema, GeneratedTransition{})
	check("transition.balanced", generatedTransitionSchema.Properties["balanced"], GeneratedBalanced{})
	check("transition.generates", generatedTransitionSchema.Properties["generates"], GeneratedGenerates{})
}

// A step added to a machine already installed carries its rule through the extend path too (a different writer from the
// new-application one).
func TestW7_aStepAddedToAnInstalledMachineKeepsItsBalanceRule(t *testing.T) {
	manifest := w2Workspace(t)
	base := journalApp()
	base.Machines[0].Transitions = nil
	if _, err := Write(manifest, GeneratedChange{Kind: KindNewApplication, Application: base}, nil); err != nil {
		t.Fatal(err)
	}
	state, _ := stateOf(t, manifest)
	desired := DescribeApplication("app_w7_jurnal", state.Applications["app_w7_jurnal"])
	desired.Machines[0].Transitions = journalApp().Machines[0].Transitions
	loaded := publish(t, manifest, GeneratedChange{Kind: KindUpdateApplication, TargetAppID: "app_w7_jurnal", Application: &desired})
	found := false
	for _, m := range loaded.Machines {
		if m.ID == "mch_w7_entri" {
			found = true
			if b := (machineView{m}).transition("trn_posting").Balanced; b == nil || b.Machine != "mch_w7_baris" {
				t.Errorf("the added step's rule = %+v", b)
			}
		}
	}
	if !found {
		t.Fatal("the machine is not in the loaded workspace")
	}
}
