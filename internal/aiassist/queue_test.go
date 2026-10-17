package aiassist

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
)

func queueMachine() GeneratedMachine {
	m := boardApp().Machines[0]
	m.ID = "mch_q_tugas"
	m.Views = nil
	m.Completion = &GeneratedCompletion{Field: "fld_status", Done: "selesai"}
	m.StatusCategories = &GeneratedStatusCategories{Field: "fld_status", Values: []GeneratedStatusCategory{
		{Option: "rencana", Category: "start"}, {Option: "jalan", Category: "active"}, {Option: "selesai", Category: "done"},
	}}
	m.Transitions = []GeneratedTransition{
		{ID: "trn_q_mulai", Name: "Start", Field: "fld_status", From: "rencana", To: "jalan"},
		{ID: "trn_q_selesai", Name: "Complete", Field: "fld_status", From: "jalan", To: "selesai"},
	}
	return m
}

func queueApp(q *GeneratedQueue) *GeneratedApplication {
	return &GeneratedApplication{
		ID: "app_q", Name: "Antrean", Roles: []string{"staf"}, PublisherRole: "staf",
		Machines:   []GeneratedMachine{queueMachine()},
		Navigation: []GeneratedMenuItem{{Label: "Antrean", MachineID: "mch_q_tugas", Queue: q}, {Label: "Semua", MachineID: "mch_q_tugas"}},
	}
}

func TestQueue_isWrittenLoadedAndItsPageValidates(t *testing.T) {
	for name, q := range map[string]*GeneratedQueue{
		"dated":   {DateField: "fld_due", Limit: 50},
		"undated": {Limit: 50, Words: GeneratedQueueWords{Empty: "Kosong."}},
	} {
		t.Run(name, func(t *testing.T) {
			manifest := w2Workspace(t)
			change := GeneratedChange{Kind: KindNewApplication, Application: queueApp(q)}
			state, _ := stateOf(t, manifest)
			if err := Validate(change, state); err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if _, err := Write(manifest, change, nil); err != nil {
				t.Fatalf("Write: %v", err)
			}
			_, loaded := stateOf(t, manifest)
			var found bool
			for _, n := range loaded.Workspace.Applications[0].AllNavigation {
				if strings.HasPrefix(n.Route, "/pages/") {
					found = true
					if n.Page == nil {
						t.Fatal("page missing")
					}
				}
			}
			if !found {
				t.Fatal("no queue page in the loaded menu")
			}
		})
	}
}

func TestQueue_eachMisuseIsRefusedForItsOwnReason(t *testing.T) {
	cases := []struct {
		name string
		q    *GeneratedQueue
		edit func(a *GeneratedApplication)
		want string
	}{
		{"no limit", &GeneratedQueue{}, nil, "limit must be between 1 and 200"},
		{"a limit past the cap", &GeneratedQueue{Limit: 201}, nil, "limit must be between 1 and 200"},
		{"a word with a line break", &GeneratedQueue{Limit: 5, Words: GeneratedQueueWords{Empty: "a\nb"}}, nil, "every word is one line"},
		{"no title", &GeneratedQueue{Limit: 5}, func(a *GeneratedApplication) { a.Machines[0].CardFields = a.Machines[0].CardFields[1:] }, "role \"title\""},
		{"no status_categories", &GeneratedQueue{Limit: 5}, func(a *GeneratedApplication) { a.Machines[0].StatusCategories = nil }, "needs status_categories"},
		{"a date field that is not a date", &GeneratedQueue{Limit: 5, DateField: "fld_title"}, nil, "date_field must be a date field"},
		{"a dated queue with no completion", &GeneratedQueue{Limit: 5, DateField: "fld_due"}, func(a *GeneratedApplication) { a.Machines[0].Completion = nil }, "declare completion"},
		{"a queue and a list on one entry", &GeneratedQueue{Limit: 5}, func(a *GeneratedApplication) {
			a.Navigation[0].List = &GeneratedList{Scope: ListScopeAll, Limit: 5}
		}, "not more than one"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := queueApp(tc.q)
			if tc.edit != nil {
				tc.edit(app)
			}
			state, _ := stateOf(t, w2Workspace(t))
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

func TestQueue_anUpdateAddsOneButNeverGivesAnExistingEntryOne(t *testing.T) {
	manifest := w2Workspace(t)
	app := queueApp(nil)
	app.Navigation = app.Navigation[1:]
	if _, err := Write(manifest, GeneratedChange{Kind: KindNewApplication, Application: app}, nil); err != nil {
		t.Fatalf("Write: %v", err)
	}
	desire := func(edit func(a *GeneratedApplication)) GeneratedChange {
		state, _ := stateOf(t, manifest)
		d := DescribeApplication("app_q", state.Applications["app_q"])
		edit(&d)
		return GeneratedChange{Kind: KindUpdateApplication, TargetAppID: "app_q", Application: &d}
	}
	state, _ := stateOf(t, manifest)
	err := Validate(desire(func(a *GeneratedApplication) { a.Navigation[0].Queue = &GeneratedQueue{Limit: 10} }), state)
	if err == nil || !strings.Contains(err.Error(), "cannot be given a list") {
		t.Errorf("giving an existing entry a queue: want a refusal, got %v", err)
	}
	publish(t, manifest, desire(func(a *GeneratedApplication) {
		a.Navigation = append(a.Navigation, GeneratedMenuItem{Label: "Antrean", MachineID: "mch_q_tugas", Queue: &GeneratedQueue{DateField: "fld_due", Limit: 20}})
	}))
	_, loaded := stateOf(t, manifest)
	pages := 0
	for _, n := range loaded.Workspace.Applications[0].AllNavigation {
		if strings.HasPrefix(n.Route, "/pages/") {
			pages++
		}
	}
	if pages != 1 {
		t.Errorf("the update wrote %d pages, want 1", pages)
	}
}

func TestQueue_theDatedRecipeGuardsEverySectionWithDoneAndGivesOnlyOpenOnesAButton(t *testing.T) {
	m := queueMachine()
	secs := queueSections(&GeneratedQueue{DateField: "fld_due", Limit: 5}, m)
	if len(secs) != 4 {
		t.Fatalf("%d sections, want 4", len(secs))
	}
	for _, s := range secs {
		if s.Where == nil || s.Where.All[0].Op != "not_equals" || s.Where.All[0].Value != "$done" {
			t.Errorf("section %s is not guarded by not_equals $done: %+v", s.Key, s.Where)
		}
	}
	if len(secs[0].Buttons) != 1 || secs[0].Buttons[0].Becomes != "$done" || len(secs[3].Buttons) != 0 {
		t.Errorf("buttons: overdue %+v, later %+v", secs[0].Buttons, secs[3].Buttons)
	}
	und := queueSections(&GeneratedQueue{Limit: 5}, m)
	if len(und) != 2 || und[0].Buttons[0].Becomes != "jalan" || und[1].Buttons[0].Becomes != "selesai" {
		t.Errorf("undated sections = %+v", und)
	}
}

func TestQueue_thePromptTeachesTheRecipeAndItsThreeExamples(t *testing.T) {
	full := SystemPromptFor(nil, nil)
	for _, want := range []string{`"queue"`, "date_field", "helpdesk", "Maintenance", "Lending", "a queue with no transitions is"} {
		if !strings.Contains(full, want) {
			t.Errorf("the prompt lacks %q", want)
		}
	}
}

func queueCorpusMachine(id, name string, statuses []string, cats map[string]string, due bool, edges [][3]string) GeneratedMachine {
	m := GeneratedMachine{ID: id, Name: name,
		Fields: []GeneratedField{
			{ID: "fld_title", Name: "Title", Type: "text", Required: true},
			{ID: "fld_status", Name: "Status", Type: "status", Required: true, Options: statuses, Default: statuses[0]},
		},
		CardFields:       []GeneratedCardField{{Field: "fld_title", Role: "title"}, {Field: "fld_status", Role: "status"}},
		StatusCategories: &GeneratedStatusCategories{Field: "fld_status"},
	}
	for _, s := range statuses {
		m.StatusCategories.Values = append(m.StatusCategories.Values, GeneratedStatusCategory{Option: s, Category: cats[s]})
	}
	if due {
		m.Fields = append(m.Fields, GeneratedField{ID: "fld_due", Name: "Due", Type: "date"})
		m.CardFields = append(m.CardFields, GeneratedCardField{Field: "fld_due", Role: "date"})
		m.Completion = &GeneratedCompletion{Field: "fld_status", Done: statuses[len(statuses)-1]}
	}
	for _, e := range edges {
		m.Transitions = append(m.Transitions, GeneratedTransition{ID: "trn_" + strings.ReplaceAll(strings.ToLower(e[0]), " ", "_"), Name: e[0], Field: "fld_status", From: e[1], To: e[2]})
	}
	return m
}

// TestQueueCorpus_helpdeskMaintenanceAndLendingComeOutAsQueuePages: roadmap stage 3 of the assistant gap. What the
// assistant is taught to emit for the three cases the hand-made Workspace pages cover must pass the same
// validation, write, load through the real loader (which lowers and validates every page) and carry a queue page
// whose buttons are the Machine's own edges.
func TestQueueCorpus_helpdeskMaintenanceAndLendingComeOutAsQueuePages(t *testing.T) {
	cases := map[string]struct {
		m       GeneratedMachine
		q       GeneratedQueue
		buttons int
	}{
		"helpdesk": {queueCorpusMachine("mch_c_ticket", "Ticket", []string{"new", "in_progress", "waiting", "resolved"},
			map[string]string{"new": "start", "in_progress": "active", "waiting": "attention", "resolved": "done"}, false,
			[][3]string{{"Start", "new", "in_progress"}, {"Wait", "in_progress", "waiting"}, {"Resolve", "in_progress", "resolved"}, {"Resume", "waiting", "in_progress"}}),
			GeneratedQueue{Limit: 50}, 4},
		"maintenance": {queueCorpusMachine("mch_c_task", "Task", []string{"scheduled", "in_progress", "done"},
			map[string]string{"scheduled": "start", "in_progress": "active", "done": "done"}, true,
			[][3]string{{"Complete", "in_progress", "done"}}),
			GeneratedQueue{DateField: "fld_due", Limit: 50}, 3},
		"lending": {queueCorpusMachine("mch_c_instalment", "Instalment", []string{"due", "late", "paid"},
			map[string]string{"due": "start", "late": "attention", "paid": "done"}, true,
			[][3]string{{"Record payment", "due", "paid"}, {"Record late payment", "late", "paid"}}),
			GeneratedQueue{DateField: "fld_due", Limit: 50}, 3},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			app := &GeneratedApplication{ID: "app_c_" + name, Name: name, Roles: []string{"staf"}, PublisherRole: "staf",
				Machines:   []GeneratedMachine{tc.m},
				Navigation: []GeneratedMenuItem{{Label: "Queue", MachineID: tc.m.ID, Queue: &tc.q}, {Label: "All", MachineID: tc.m.ID}}}
			manifest := w2Workspace(t)
			change := GeneratedChange{Kind: KindNewApplication, Application: app}
			state, _ := stateOf(t, manifest)
			if err := Validate(change, state); err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if _, err := Write(manifest, change, nil); err != nil {
				t.Fatalf("Write: %v", err)
			}
			_, loaded := stateOf(t, manifest)
			var n int
			for _, nav := range loaded.Workspace.Applications[0].AllNavigation {
				if strings.HasPrefix(nav.Route, "/pages/") && nav.Page != nil {
					n++
					if got := countButtons(*nav.Page); got != tc.buttons {
						t.Errorf("%s: %d transition buttons, want %d", name, got, tc.buttons)
					}
				}
			}
			if n != 1 {
				t.Errorf("%s: %d queue pages, want 1", name, n)
			}
		})
	}
}

func countButtons(n domain.PageNode) int {
	c := 0
	if n.Type == "Button" {
		c++
	}
	for _, ch := range n.Children {
		c += countButtons(ch)
	}
	return c
}
