package aiassist

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

// boardApp is the Desain Request pattern: a status board first, a table second, and the card projection the
// board needs.
func boardApp() *GeneratedApplication {
	return &GeneratedApplication{
		ID: "app_w4", Name: "Tugas", Roles: []string{"staf"}, PublisherRole: "staf",
		Machines: []GeneratedMachine{{ID: "mch_w4_tugas", Name: "Tugas",
			Fields: []GeneratedField{
				{ID: "fld_title", Name: "Judul", Type: "text", Required: true},
				{ID: "fld_status", Name: "Status", Type: "status", Required: true, Options: []string{"rencana", "jalan", "selesai"}},
				{ID: "fld_due", Name: "Tenggat", Type: "date"},
				{ID: "fld_pic", Name: "PIC", Type: "person"},
				{ID: "fld_cost", Name: "Biaya", Type: "money"},
			},
			CardFields: []GeneratedCardField{
				{Field: "fld_title", Role: "title"}, {Field: "fld_status", Role: "status"}, {Field: "fld_due", Role: "date"},
				{Field: "fld_pic", Role: "person"}, {Field: "fld_cost", Role: "money"},
			},
			Views: []GeneratedView{
				{ID: "vw_w4_board", Name: "Papan", Type: "board", GroupBy: "fld_status"},
				{ID: "vw_w4_table", Name: "Tabel", Type: "table"},
			}}},
		Navigation: []GeneratedMenuItem{{Label: "Tugas", MachineID: "mch_w4_tugas"}},
	}
}

// TestW4_aBoardAndItsCardAreValidatedWrittenLoadedAndReadBack: roadmap stage 1, W4.
func TestW4_aBoardAndItsCardAreValidatedWrittenLoadedAndReadBack(t *testing.T) {
	manifest := w2Workspace(t)
	change := GeneratedChange{Kind: KindNewApplication, Application: boardApp()}
	state, _ := stateOf(t, manifest)
	if err := Validate(change, state); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if _, err := Write(manifest, change, nil); err != nil {
		t.Fatalf("Write: %v", err)
	}
	state, loaded := stateOf(t, manifest)
	got := DescribeApplication("app_w4", state.Applications["app_w4"]).Machines[0]
	want := boardApp().Machines[0]
	if !reflect.DeepEqual(got.CardFields, want.CardFields) {
		t.Errorf("card_fields read back %+v, want %+v", got.CardFields, want.CardFields)
	}
	if !reflect.DeepEqual(got.Views, want.Views) {
		t.Errorf("views read back %+v, want %+v", got.Views, want.Views)
	}
	// and the runtime's own model agrees: the first view is the board
	for _, m := range loaded.Machines {
		if m.ID == "mch_w4_tugas" && (len(m.Views) != 2 || m.Views[0].Type != "board" || m.Views[0].GroupBy != "fld_status") {
			t.Errorf("the loaded machine's views are %+v", m.Views)
		}
	}
}

func TestW4_eachMisuseIsRefusedForItsOwnReason(t *testing.T) {
	cases := []struct {
		name string
		edit func(m *GeneratedMachine)
		want string
	}{
		{"a board grouped by a text field", func(m *GeneratedMachine) { m.Views[0].GroupBy = "fld_title" }, "grouped by a status field"},
		{"a board grouped by nothing", func(m *GeneratedMachine) { m.Views[0].GroupBy = "" }, "group_by"},
		{"a table with a group_by", func(m *GeneratedMachine) { m.Views[1].GroupBy = "fld_status" }, "belongs to a board"},
		{"a stepper view", func(m *GeneratedMachine) { m.Views[1].Type = "stepper" }, "not one a generated machine may use"},
		{"a board with no card projection", func(m *GeneratedMachine) { m.CardFields = nil }, "needs card_fields including a title"},
		{"a card with no title", func(m *GeneratedMachine) { m.CardFields = m.CardFields[1:] }, "needs card_fields including a title"},
		{"a date role over a text field", func(m *GeneratedMachine) { m.CardFields[2].Field = "fld_title" }, "role \"date\" needs a field of type"},
		{"a person role over a number", func(m *GeneratedMachine) { m.CardFields[3].Field = "fld_cost" }, "role \"person\" needs a field of type"},
		{"a card field that is not a field", func(m *GeneratedMachine) { m.CardFields[1].Field = "fld_nope" }, "fld_nope"},
		{"a view id without the prefix", func(m *GeneratedMachine) { m.Views[1].ID = "table" }, "view id"},
		{"two views with one id", func(m *GeneratedMachine) { m.Views[1].ID = m.Views[0].ID }, "more than once"},
		{"a view name with a line break", func(m *GeneratedMachine) { m.Views[0].Name = "Pa\npan" }, "line break"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manifest := w2Workspace(t)
			app := boardApp()
			tc.edit(&app.Machines[0])
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

// An update may add a view (after the existing ones) and a card entry and rename a view; it may not retype
// or regroup a view or remove them all, put a new one first, or drop or re-role a card entry. Everything hand-set survives.
func TestW4_updatesAddRenameAndRefuseTheRightThings(t *testing.T) {
	manifest := w2Workspace(t)
	app := boardApp()
	m := &app.Machines[0]
	m.Views = m.Views[:1]           // only the board; the update adds the table
	m.CardFields = m.CardFields[:2] // title, status; the update adds a date
	if _, err := Write(manifest, GeneratedChange{Kind: KindNewApplication, Application: app}, nil); err != nil {
		t.Fatalf("Write: %v", err)
	}
	desire := func(edit func(m *GeneratedMachine)) GeneratedChange {
		state, _ := stateOf(t, manifest)
		d := DescribeApplication("app_w4", state.Applications["app_w4"])
		edit(&d.Machines[0])
		return GeneratedChange{Kind: KindUpdateApplication, TargetAppID: "app_w4", Application: &d}
	}
	refused := map[string]struct {
		edit func(m *GeneratedMachine)
		why  string
	}{
		"retyping a view":     {func(m *GeneratedMachine) { m.Views[0].Type = "table"; m.Views[0].GroupBy = "" }, "cannot change its type or grouping"},
		"regrouping a view":   {func(m *GeneratedMachine) { m.Views[0].GroupBy = "fld_title" }, "cannot change its type or grouping"},
		"removing every view": {func(m *GeneratedMachine) { m.Views = nil }, "needs at least one view"},
		"a new view first": {func(m *GeneratedMachine) {
			m.Views = append([]GeneratedView{{ID: "vw_w4_t", Name: "T", Type: "table"}}, m.Views...)
		}, "must come after"},
		"dropping a card role": {func(m *GeneratedMachine) { m.CardFields = m.CardFields[:1] }, "cannot be removed or re-roled"},
	}
	for name, tc := range refused {
		state, _ := stateOf(t, manifest)
		err := Validate(desire(tc.edit), state)
		if err == nil || !strings.Contains(err.Error(), tc.why) {
			t.Errorf("%s: want a refusal saying %q, got %v", name, tc.why, err)
		}
	}
	publish(t, manifest, desire(func(m *GeneratedMachine) {
		m.Views[0].Name = "Kanban"
		m.Views = append(m.Views, GeneratedView{ID: "vw_w4_table", Name: "Tabel", Type: "table"})
		m.CardFields = append(m.CardFields, GeneratedCardField{Field: "fld_due", Role: "date"})
	}))
	state, _ := stateOf(t, manifest)
	got := DescribeApplication("app_w4", state.Applications["app_w4"]).Machines[0]
	if len(got.Views) != 2 || got.Views[0].Name != "Kanban" || got.Views[0].Type != "board" || got.Views[1].Type != "table" || len(got.CardFields) != 3 {
		t.Errorf("after the update: views %+v, card_fields %+v", got.Views, got.CardFields)
	}
}

// A person may ask for "only the board": an update removes a view (one must remain) and reorders the rest, and
// the file is left with exactly those views in that order.
func TestW4_anUpdateRemovesAndReordersViews(t *testing.T) {
	manifest := w2Workspace(t)
	app := boardApp()
	if _, err := Write(manifest, GeneratedChange{Kind: KindNewApplication, Application: app}, nil); err != nil {
		t.Fatalf("Write: %v", err)
	}
	desire := func(edit func(m *GeneratedMachine)) GeneratedChange {
		state, _ := stateOf(t, manifest)
		d := DescribeApplication("app_w4", state.Applications["app_w4"])
		edit(&d.Machines[0])
		return GeneratedChange{Kind: KindUpdateApplication, TargetAppID: "app_w4", Application: &d}
	}
	state, _ := stateOf(t, manifest)
	before := DescribeApplication("app_w4", state.Applications["app_w4"]).Machines[0].Views
	if len(before) < 2 {
		t.Fatalf("fixture needs two views, has %+v", before)
	}
	publish(t, manifest, desire(func(m *GeneratedMachine) { slices.Reverse(m.Views) }))
	state, _ = stateOf(t, manifest)
	got := DescribeApplication("app_w4", state.Applications["app_w4"]).Machines[0].Views
	if got[0].ID != before[len(before)-1].ID || got[len(got)-1].ID != before[0].ID {
		t.Errorf("reordered views = %+v, was %+v", got, before)
	}
	publish(t, manifest, desire(func(m *GeneratedMachine) { m.Views = m.Views[:1] }))
	state, _ = stateOf(t, manifest)
	got = DescribeApplication("app_w4", state.Applications["app_w4"]).Machines[0].Views
	if len(got) != 1 || got[0].ID != before[len(before)-1].ID {
		t.Errorf("after removing the others: %+v", got)
	}
}

func TestW4_theSchemaOffersOnlyWhatIsGeneratable(t *testing.T) {
	if got := strings.Join(generatedViewSchema.Properties["type"].Enum, ","); got != "board,cards,table" {
		t.Errorf("view type enum = %q", got)
	}
	if got := strings.Join(generatedCardFieldSchema.Properties["role"].Enum, ","); got != "title,status,date,person,money" {
		t.Errorf("card role enum = %q", got)
	}
}

func TestPrompt_teachesBoardsAndCards(t *testing.T) {
	full := SystemPromptFor(nil, nil)
	for _, want := range []string{`"board" (one column per option of a STATUS field`, "kanban", `"card_fields"`, "The first view is", "do not guess"} {
		if !strings.Contains(full, want) {
			t.Errorf("the prompt lacks %q", want)
		}
	}
}
