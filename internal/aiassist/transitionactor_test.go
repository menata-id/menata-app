package aiassist

import (
	"slices"
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// withGatedMove adds a person Field and a status move limited by role and by that person to the fixture's second
// Machine, the way the assistant is now allowed to.
func withGatedMove(a *GeneratedApplication, roles []string, actorField string) {
	m := &a.Machines[1]
	m.Fields = append(m.Fields, GeneratedField{ID: "fld_pelapor", Name: "Pelapor", Type: "person"}, GeneratedField{ID: "fld_catatan", Name: "Catatan", Type: "text"})
	m.Transitions = []GeneratedTransition{{ID: "trn_send", Name: "Kirim", Field: "fld_status", From: "draft", To: "sent", Roles: roles, ActorField: actorField}}
}

func TestTransitionActorIsWrittenLoadedAndDescribedBack(t *testing.T) {
	manifestPath := installedWasteApp(t)
	loaded := publish(t, manifestPath, update(t, manifestPath, func(a *GeneratedApplication) {
		withGatedMove(a, []string{"submitter"}, "fld_pelapor")
	}))
	var got domain.Transition
	for _, m := range loaded.Machines {
		if m.ID == "mch_pencatatan_sampah" && len(m.Transitions) == 1 {
			got = m.Transitions[0]
		}
	}
	if !slices.Equal(got.Roles, []string{"submitter"}) || got.ActorField != "fld_pelapor" || got.Action != domain.ActionEdit {
		t.Fatalf("the loaded edge = %+v, want roles [submitter], actor_field fld_pelapor, action edit", got)
	}
	state, _ := stateOf(t, manifestPath)
	back := DescribeApplication("app_pengelolaan_sampah", state.Applications["app_pengelolaan_sampah"])
	tr := back.Machines[1].Transitions
	if len(tr) != 1 || !slices.Equal(tr[0].Roles, []string{"submitter"}) || tr[0].ActorField != "fld_pelapor" {
		t.Fatalf("the described edge = %+v, want the roles and actor_field it was written with", tr)
	}
}

func TestTransitionActorIsHeldToTheLoadersRules(t *testing.T) {
	for _, tc := range []struct {
		name  string
		roles []string
		actor string
		want  string
	}{
		{"a role the application does not declare", []string{"bos"}, "", "bos"},
		{"an actor_field that is not a person", nil, "fld_catatan", "fld_catatan"},
		{"an actor_field the machine lacks", nil, "fld_hilang", "fld_hilang"},
	} {
		manifestPath := installedWasteApp(t)
		change := update(t, manifestPath, func(a *GeneratedApplication) { withGatedMove(a, tc.roles, tc.actor) })
		state, _ := stateOf(t, manifestPath)
		err := Validate(change, state)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: Validate = %v, want an error naming %q", tc.name, err, tc.want)
		}
	}
}

// Who may take an existing step is not changed by an update: the file edit only appends, so a change would be
// reported in the plan and never written.
func TestAnUpdateCannotChangeWhoMayTakeAStep(t *testing.T) {
	manifestPath := installedWasteApp(t)
	publish(t, manifestPath, update(t, manifestPath, func(a *GeneratedApplication) {
		withGatedMove(a, []string{"submitter"}, "fld_pelapor")
	}))
	for name, edit := range map[string]func(*GeneratedTransition){
		"roles":       func(x *GeneratedTransition) { x.Roles = []string{"approver"} },
		"roles gone":  func(x *GeneratedTransition) { x.Roles = nil },
		"actor_field": func(x *GeneratedTransition) { x.ActorField = "" },
	} {
		change := update(t, manifestPath, func(a *GeneratedApplication) { edit(&a.Machines[1].Transitions[0]) })
		state, _ := stateOf(t, manifestPath)
		plan := PlanUpdate(DescribeApplication(change.TargetAppID, state.Applications[change.TargetAppID]), *change.Application)
		if len(plan.Refusals) != 1 || !strings.Contains(plan.Refusals[0], "trn_send") {
			t.Errorf("%s: refusals = %v, want one naming trn_send", name, plan.Refusals)
		}
	}
	same := update(t, manifestPath, func(a *GeneratedApplication) {})
	state, _ := stateOf(t, manifestPath)
	if plan := PlanUpdate(DescribeApplication(same.TargetAppID, state.Applications[same.TargetAppID]), *same.Application); len(plan.Refusals) != 0 {
		t.Errorf("an unchanged step was refused: %v", plan.Refusals)
	}
}

func TestNewApplicationStepRolesMustBeDeclared(t *testing.T) {
	good := validLeaveRequestChange()
	good.Application.Machines[0].Transitions[0].Roles = []string{"Supervisor"}
	if err := Validate(good, ExistingState{}); err != nil {
		t.Fatalf("a declared role on a step was refused: %v", err)
	}
	bad := validLeaveRequestChange()
	bad.Application.Machines[0].Transitions[0].Roles = []string{"Ghost"}
	if err := Validate(bad, ExistingState{}); err == nil || !strings.Contains(err.Error(), "Ghost") {
		t.Fatalf("an undeclared role on a step = %v, want an error naming it", err)
	}
}

// A NEW application takes the other write path (a whole file, not an appended line), so it is held separately.
func TestNewApplicationWritesWhoMayTakeAStep(t *testing.T) {
	manifestPath := installedWasteApp(t)
	change := GeneratedChange{Kind: KindNewApplication, Application: &GeneratedApplication{
		ID: "app_cuti", Name: "Cuti", Roles: []string{"manajer", "karyawan"}, PublisherRole: "manajer",
		Navigation: []GeneratedMenuItem{{Label: "Cuti", MachineID: "mch_cuti"}},
		Machines: []GeneratedMachine{{ID: "mch_cuti", Name: "Cuti",
			Fields: []GeneratedField{
				{ID: "fld_manajer", Name: "Manajer", Type: "person"},
				{ID: "fld_status", Name: "Status", Type: "status", Required: true, Options: []string{"diajukan", "disetujui"}},
			},
			Permissions: []GeneratedPermission{{ID: "prm_cuti_create", Action: "create", Roles: []string{"karyawan"}}},
			Transitions: []GeneratedTransition{{ID: "trn_setujui", Name: "Setujui", Field: "fld_status", From: "diajukan", To: "disetujui", Roles: []string{"manajer"}, ActorField: "fld_manajer"}},
		}},
	}}
	loaded := publish(t, manifestPath, change)
	for _, m := range loaded.Machines {
		if m.ID != "mch_cuti" {
			continue
		}
		got := m.Transitions[0]
		if !slices.Equal(got.Roles, []string{"manajer"}) || got.ActorField != "fld_manajer" {
			t.Fatalf("the written edge = %+v, want roles [manajer] and actor_field fld_manajer", got)
		}
		return
	}
	t.Fatal("the new Machine was not loaded")
}

func withConditionalFields(a *GeneratedApplication) {
	m := &a.Machines[1]
	m.Fields = append(m.Fields,
		GeneratedField{ID: "fld_lampiran", Name: "Lampiran", Type: "text", RequiredWhen: &GeneratedRequiredWhen{Field: "fld_status", Equals: "sent"}},
		GeneratedField{ID: "fld_tenggat", Name: "Tenggat", Type: "date", DefaultFrom: &GeneratedDefaultFrom{Field: "fld_status", Days: []GeneratedDayCount{{Value: "draft", Days: 7}, {Value: "sent", Days: 1}}}})
}

func TestConditionalAndDerivedFieldsAreWrittenLoadedAndDescribedBack(t *testing.T) {
	manifestPath := installedWasteApp(t)
	loaded := publish(t, manifestPath, update(t, manifestPath, withConditionalFields))
	var lampiran, tenggat domain.Field
	for _, m := range loaded.Machines {
		if m.ID == "mch_pencatatan_sampah" {
			lampiran, _ = m.FieldByID("fld_lampiran")
			tenggat, _ = m.FieldByID("fld_tenggat")
		}
	}
	if lampiran.RequiredWhen == nil || lampiran.RequiredWhen.Field != "fld_status" || lampiran.RequiredWhen.Equals != "sent" {
		t.Fatalf("the loaded required_when = %+v", lampiran.RequiredWhen)
	}
	if tenggat.DefaultFrom == nil || tenggat.DefaultFrom.Days["draft"] != 7 || tenggat.DefaultFrom.Days["sent"] != 1 {
		t.Fatalf("the loaded default_from = %+v", tenggat.DefaultFrom)
	}
	state, _ := stateOf(t, manifestPath)
	back := DescribeApplication("app_pengelolaan_sampah", state.Applications["app_pengelolaan_sampah"])
	var gl, gt GeneratedField
	for _, f := range back.Machines[1].Fields {
		switch f.ID {
		case "fld_lampiran":
			gl = f
		case "fld_tenggat":
			gt = f
		}
	}
	if gl.RequiredWhen == nil || gl.RequiredWhen.Equals != "sent" || gt.DefaultFrom == nil || len(gt.DefaultFrom.Days) != 2 || gt.DefaultFrom.Days[0].Value != "draft" {
		t.Fatalf("described back: %+v / %+v", gl.RequiredWhen, gt.DefaultFrom)
	}
	// Describing and resubmitting changes nothing.
	same := update(t, manifestPath, func(a *GeneratedApplication) {})
	if plan := PlanUpdate(DescribeApplication(same.TargetAppID, state.Applications[same.TargetAppID]), *same.Application); len(plan.Refusals) != 0 {
		t.Errorf("an unchanged application was refused: %v", plan.Refusals)
	}
}

func TestConditionalAndDerivedFieldsAreHeldToTheLoadersRulesAndCannotBeChanged(t *testing.T) {
	manifestPath := installedWasteApp(t)
	bad := update(t, manifestPath, func(a *GeneratedApplication) {
		withConditionalFields(a)
		a.Machines[1].Fields[len(a.Machines[1].Fields)-2].RequiredWhen.Equals = "zzz"
	})
	state, _ := stateOf(t, manifestPath)
	if err := Validate(bad, state); err == nil || !strings.Contains(err.Error(), "zzz") {
		t.Fatalf("a required_when on a value that is no option = %v, want an error naming it", err)
	}
	dup := update(t, manifestPath, func(a *GeneratedApplication) {
		withConditionalFields(a)
		d := a.Machines[1].Fields[len(a.Machines[1].Fields)-1].DefaultFrom
		d.Days = append(d.Days, GeneratedDayCount{Value: "draft", Days: 2})
	})
	if err := Validate(dup, state); err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Fatalf("a value counted twice = %v, want a refusal", err)
	}
	publish(t, manifestPath, update(t, manifestPath, withConditionalFields))
	for name, edit := range map[string]func(*GeneratedField){
		"required_when changed": func(f *GeneratedField) {
			if f.RequiredWhen != nil {
				f.RequiredWhen.Equals = "draft"
			}
		},
		"required_when dropped": func(f *GeneratedField) { f.RequiredWhen = nil },
		"a day count added": func(f *GeneratedField) {
			if f.DefaultFrom != nil {
				f.DefaultFrom.Days = append(f.DefaultFrom.Days, GeneratedDayCount{Value: "x", Days: 1})
			}
		},
		"days changed": func(f *GeneratedField) {
			if f.DefaultFrom != nil {
				f.DefaultFrom.Days[0].Days = 9
			}
		},
		"default_from dropped": func(f *GeneratedField) { f.DefaultFrom = nil },
	} {
		change := update(t, manifestPath, func(a *GeneratedApplication) {
			for i := range a.Machines[1].Fields {
				if id := a.Machines[1].Fields[i].ID; id == "fld_lampiran" || id == "fld_tenggat" {
					edit(&a.Machines[1].Fields[i])
				}
			}
		})
		st, _ := stateOf(t, manifestPath)
		plan := PlanUpdate(DescribeApplication(change.TargetAppID, st.Applications[change.TargetAppID]), *change.Application)
		if len(plan.Refusals) != 1 || !strings.Contains(plan.Refusals[0], "cannot change when it is required") {
			t.Errorf("%s: refusals = %v, want exactly the field-source refusal", name, plan.Refusals)
		}
	}
}

func withEnsure(a *GeneratedApplication, atLeast float64) {
	m := &a.Machines[1]
	m.Fields = append(m.Fields, GeneratedField{ID: "fld_jumlah", Name: "Jumlah", Type: "number"}, GeneratedField{ID: "fld_barang", Name: "Barang", Type: "text"})
	m.Ensure = []GeneratedEnsure{{ID: "ens_tidak_negatif", SumOf: "fld_jumlah", Per: "fld_barang", AtLeast: &atLeast, Reason: "Stok tidak boleh negatif."}}
}

func TestEnsureIsWrittenLoadedDescribedBackAndNotChangeable(t *testing.T) {
	manifestPath := installedWasteApp(t)
	loaded := publish(t, manifestPath, update(t, manifestPath, func(a *GeneratedApplication) { withEnsure(a, 0) }))
	var b domain.AggregateBound
	for _, m := range loaded.Machines {
		if m.ID == "mch_pencatatan_sampah" && len(m.Bounds) == 1 {
			b = m.Bounds[0]
		}
	}
	if b.ID != "ens_tidak_negatif" || b.AtLeast == nil || *b.AtLeast != 0 || b.AtMost != nil || b.Reason != "Stok tidak boleh negatif." {
		t.Fatalf("the loaded bound = %+v (zero must survive as a real bound)", b)
	}
	state, _ := stateOf(t, manifestPath)
	back := DescribeApplication("app_pengelolaan_sampah", state.Applications["app_pengelolaan_sampah"])
	if e := back.Machines[1].Ensure; len(e) != 1 || e[0].AtLeast == nil || *e[0].AtLeast != 0 || e[0].SumOf != "fld_jumlah" {
		t.Fatalf("described back: %+v", e)
	}
	same := update(t, manifestPath, func(a *GeneratedApplication) {})
	if plan := PlanUpdate(DescribeApplication(same.TargetAppID, state.Applications[same.TargetAppID]), *same.Application); len(plan.Refusals) != 0 || len(plan.Items) != 0 {
		t.Errorf("an unchanged application planned something: %+v", plan)
	}
	for name, edit := range map[string]func(*GeneratedEnsure){
		"the bound moved":  func(e *GeneratedEnsure) { v := 5.0; e.AtLeast = &v },
		"a bound added":    func(e *GeneratedEnsure) { v := 9.0; e.AtMost = &v },
		"the group moved":  func(e *GeneratedEnsure) { e.Per = "fld_jumlah" },
		"the reason moved": func(e *GeneratedEnsure) { e.Reason = "lain" },
	} {
		change := update(t, manifestPath, func(a *GeneratedApplication) { edit(&a.Machines[1].Ensure[0]) })
		plan := PlanUpdate(DescribeApplication(change.TargetAppID, state.Applications[change.TargetAppID]), *change.Application)
		if len(plan.Refusals) != 1 || !strings.Contains(plan.Refusals[0], "ens_tidak_negatif") {
			t.Errorf("%s: refusals = %v", name, plan.Refusals)
		}
	}
	dropped := update(t, manifestPath, func(a *GeneratedApplication) { a.Machines[1].Ensure = nil })
	if plan := PlanUpdate(DescribeApplication(dropped.TargetAppID, state.Applications[dropped.TargetAppID]), *dropped.Application); len(plan.Refusals) != 1 {
		t.Errorf("dropping a limit: refusals = %v", plan.Refusals)
	}
}

func TestEnsureIsHeldToTheLoadersRules(t *testing.T) {
	manifestPath := installedWasteApp(t)
	state, _ := stateOf(t, manifestPath)
	for name, edit := range map[string]func(*GeneratedEnsure){
		"no bound":      func(e *GeneratedEnsure) { e.AtLeast = nil },
		"a text sum":    func(e *GeneratedEnsure) { e.SumOf = "fld_barang" },
		"a bad id":      func(e *GeneratedEnsure) { e.ID = "batas" },
		"a missing per": func(e *GeneratedEnsure) { e.Per = "fld_nope" },
	} {
		change := update(t, manifestPath, func(a *GeneratedApplication) {
			withEnsure(a, 0)
			edit(&a.Machines[1].Ensure[0])
		})
		if err := Validate(change, state); err == nil {
			t.Errorf("%s: Validate accepted it", name)
		}
	}
}

// A NEW Machine takes the other write path (a whole file), so the bound, with its zero, is held there too.
func TestNewApplicationWritesItsLimits(t *testing.T) {
	manifestPath := installedWasteApp(t)
	zero := 0.0
	change := GeneratedChange{Kind: KindNewApplication, Application: &GeneratedApplication{
		ID: "app_gudang", Name: "Gudang", Roles: []string{"petugas"}, PublisherRole: "petugas",
		Navigation: []GeneratedMenuItem{{Label: "Mutasi", MachineID: "mch_mutasi"}},
		Machines: []GeneratedMachine{{ID: "mch_mutasi", Name: "Mutasi",
			Fields: []GeneratedField{{ID: "fld_barang", Name: "Barang", Type: "text", Required: true}, {ID: "fld_delta", Name: "Jumlah", Type: "number", Required: true}},
			Ensure: []GeneratedEnsure{{ID: "ens_stok", SumOf: "fld_delta", Per: "fld_barang", AtLeast: &zero, Reason: "Stok negatif."}}}},
	}}
	loaded := publish(t, manifestPath, change)
	for _, m := range loaded.Machines {
		if m.ID == "mch_mutasi" {
			if len(m.Bounds) != 1 || m.Bounds[0].AtLeast == nil || *m.Bounds[0].AtLeast != 0 || m.Bounds[0].Reason != "Stok negatif." {
				t.Fatalf("the written bound = %+v", m.Bounds)
			}
			return
		}
	}
	t.Fatal("the new Machine was not loaded")
}

func frozenApp(equals string) *GeneratedApplication {
	return &GeneratedApplication{
		ID: "app_kas", Name: "Kas", Roles: []string{"kasir"}, PublisherRole: "kasir",
		Navigation: []GeneratedMenuItem{{Label: "Voucher", MachineID: "mch_kas_voucher"}},
		Machines: []GeneratedMachine{
			{ID: "mch_kas_periode", Name: "Periode", Fields: []GeneratedField{
				{ID: "fld_nama", Name: "Nama", Type: "text", Required: true},
				{ID: "fld_status", Name: "Status", Type: "status", Required: true, Options: []string{"terbuka", "ditutup"}}}},
			{ID: "mch_kas_voucher", Name: "Voucher", Fields: []GeneratedField{
				{ID: "fld_ket", Name: "Keterangan", Type: "text", Required: true},
				{ID: "fld_periode", Name: "Periode", Type: "relation", RelatedMachine: "mch_kas_periode", Required: true}},
				FrozenWhen: &GeneratedFrozenWhen{Via: "fld_periode", Field: "fld_status", Equals: equals}},
		},
	}
}

func TestFrozenWhenIsWrittenLoadedDescribedBackAndNotChangeable(t *testing.T) {
	manifestPath := installedWasteApp(t)
	loaded := publish(t, manifestPath, GeneratedChange{Kind: KindNewApplication, Application: frozenApp("ditutup")})
	var f *domain.FrozenWhen
	for _, m := range loaded.Machines {
		if m.ID == "mch_kas_voucher" {
			f = m.Frozen
		}
	}
	if f == nil || f.Via != "fld_periode" || f.Field != "fld_status" || f.Equals != "ditutup" {
		t.Fatalf("the loaded freeze = %+v", f)
	}
	st, _ := stateOf(t, manifestPath)
	cur := DescribeApplication("app_kas", st.Applications["app_kas"])
	var v GeneratedMachine
	for _, m := range cur.Machines {
		if m.ID == "mch_kas_voucher" {
			v = m
		}
	}
	if v.FrozenWhen == nil || v.FrozenWhen.Equals != "ditutup" {
		t.Fatalf("described back: %+v", v.FrozenWhen)
	}
	if plan := PlanUpdate(cur, cur); len(plan.Items) != 0 || len(plan.Refusals) != 0 {
		t.Errorf("an unchanged application planned something: %+v", plan)
	}
	for name, edit := range map[string]func(*GeneratedMachine){
		"changed": func(m *GeneratedMachine) {
			m.FrozenWhen = &GeneratedFrozenWhen{Via: "fld_periode", Field: "fld_status", Equals: "terbuka"}
		},
		"removed": func(m *GeneratedMachine) { m.FrozenWhen = nil },
	} {
		desired := DescribeApplication("app_kas", st.Applications["app_kas"])
		for i := range desired.Machines {
			if desired.Machines[i].ID == "mch_kas_voucher" {
				edit(&desired.Machines[i])
			}
		}
		if plan := PlanUpdate(cur, desired); len(plan.Refusals) != 1 {
			t.Errorf("%s: refusals = %v", name, plan.Refusals)
		}
	}
}

func TestFrozenWhenIsHeldToItsTargetInTheSameChange(t *testing.T) {
	manifestPath := installedWasteApp(t)
	state, _ := stateOf(t, manifestPath)
	if err := Validate(GeneratedChange{Kind: KindNewApplication, Application: frozenApp("ditutup")}, state); err != nil {
		t.Fatalf("a sound freeze was refused: %v", err)
	}
	if err := Validate(GeneratedChange{Kind: KindNewApplication, Application: frozenApp("tutup")}, state); err == nil || !strings.Contains(err.Error(), "tutup") {
		t.Errorf("an equals that is no option = %v, want an error naming it", err)
	}
	bad := frozenApp("ditutup")
	bad.Machines[1].FrozenWhen.Field = "fld_nama"
	if err := Validate(GeneratedChange{Kind: KindNewApplication, Application: bad}, state); err == nil || !strings.Contains(err.Error(), "must be a status field") {
		t.Errorf("a text field = %v", err)
	}
	bad = frozenApp("ditutup")
	bad.Machines[1].FrozenWhen.Field = "fld_nope"
	if err := Validate(GeneratedChange{Kind: KindNewApplication, Application: bad}, state); err == nil || !strings.Contains(err.Error(), "not a field of") {
		t.Errorf("a missing field = %v", err)
	}
	bad = frozenApp("ditutup")
	bad.Machines[1].FrozenWhen.Via = "fld_ket"
	if err := Validate(GeneratedChange{Kind: KindNewApplication, Application: bad}, state); err == nil || !strings.Contains(err.Error(), "relation") {
		t.Errorf("a via that is not a relation = %v", err)
	}
}

func withClockWrites(a *GeneratedApplication) {
	m := &a.Machines[1]
	m.Fields = append(m.Fields,
		GeneratedField{ID: "fld_selesai", Name: "Selesai", Type: "date"},
		GeneratedField{ID: "fld_tiap", Name: "Tiap (hari)", Type: "number"},
		GeneratedField{ID: "fld_berikut", Name: "Berikutnya", Type: "date", Compute: &GeneratedCompute{Op: "add_days", Fields: []string{"fld_selesai", "fld_tiap"}}})
	m.Transitions = []GeneratedTransition{{ID: "trn_kirim", Name: "Kirim", Field: "fld_status", From: "draft", To: "sent", Sets: []GeneratedSet{{Field: "fld_selesai", Source: "today"}}}}
}

func TestClockWritesAndAddDaysAreWrittenLoadedDescribedBackAndNotChangeable(t *testing.T) {
	manifestPath := installedWasteApp(t)
	loaded := publish(t, manifestPath, update(t, manifestPath, withClockWrites))
	for _, m := range loaded.Machines {
		if m.ID != "mch_pencatatan_sampah" {
			continue
		}
		if len(m.Transitions) != 1 || m.Transitions[0].Sets["fld_selesai"] != "today" {
			t.Fatalf("the loaded step = %+v", m.Transitions)
		}
		f, _ := m.FieldByID("fld_berikut")
		if f.Compute == nil || f.Compute.Op != domain.ComputeAddDays {
			t.Fatalf("the loaded computed date = %+v", f.Compute)
		}
	}
	st, _ := stateOf(t, manifestPath)
	cur := DescribeApplication("app_pengelolaan_sampah", st.Applications["app_pengelolaan_sampah"])
	if s := cur.Machines[1].Transitions[0].Sets; len(s) != 1 || s[0].Field != "fld_selesai" || s[0].Source != "today" {
		t.Fatalf("described back: %+v", s)
	}
	if plan := PlanUpdate(cur, cur); len(plan.Items) != 0 || len(plan.Refusals) != 0 {
		t.Errorf("an unchanged application planned something: %+v", plan)
	}
	desired := DescribeApplication("app_pengelolaan_sampah", st.Applications["app_pengelolaan_sampah"])
	desired.Machines[1].Transitions[0].Sets = nil
	if plan := PlanUpdate(cur, desired); len(plan.Refusals) != 1 {
		t.Errorf("dropping a clock write: refusals = %v", plan.Refusals)
	}
}

func TestClockWritesAreHeldToTheLoadersRules(t *testing.T) {
	manifestPath := installedWasteApp(t)
	state, _ := stateOf(t, manifestPath)
	for name, edit := range map[string]func(*GeneratedTransition){
		"a field that is not a date": func(tr *GeneratedTransition) { tr.Sets[0].Field = "fld_tiap" },
		"a missing field":            func(tr *GeneratedTransition) { tr.Sets[0].Field = "fld_nope" },
		"a source that is not known": func(tr *GeneratedTransition) { tr.Sets[0].Source = "tomorrow" },
		"a field named twice":        func(tr *GeneratedTransition) { tr.Sets = append(tr.Sets, tr.Sets[0]) },
	} {
		change := update(t, manifestPath, func(a *GeneratedApplication) {
			withClockWrites(a)
			edit(&a.Machines[1].Transitions[0])
		})
		if err := Validate(change, state); err == nil {
			t.Errorf("%s: Validate accepted it", name)
		}
	}
}

func TestNewApplicationWritesItsClockWrites(t *testing.T) {
	manifestPath := installedWasteApp(t)
	change := GeneratedChange{Kind: KindNewApplication, Application: &GeneratedApplication{
		ID: "app_rawat", Name: "Rawat", Roles: []string{"teknisi"}, PublisherRole: "teknisi",
		Navigation: []GeneratedMenuItem{{Label: "Tugas", MachineID: "mch_tugas"}},
		Machines: []GeneratedMachine{{ID: "mch_tugas", Name: "Tugas",
			Fields: []GeneratedField{
				{ID: "fld_status", Name: "Status", Type: "status", Required: true, Options: []string{"jadwal", "selesai"}, Default: "jadwal"},
				{ID: "fld_selesai", Name: "Selesai", Type: "date"}},
			Transitions: []GeneratedTransition{{ID: "trn_selesai", Name: "Selesai", Field: "fld_status", From: "jadwal", To: "selesai", Sets: []GeneratedSet{{Field: "fld_selesai", Source: "today"}}}}}},
	}}
	loaded := publish(t, manifestPath, change)
	for _, m := range loaded.Machines {
		if m.ID == "mch_tugas" {
			if m.Transitions[0].Sets["fld_selesai"] != "today" {
				t.Fatalf("the written step = %+v", m.Transitions[0])
			}
			return
		}
	}
	t.Fatal("the new Machine was not loaded")
}
