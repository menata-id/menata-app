package aiassist

import (
	"strings"
	"testing"
)

// listApp: a menu with the plain entry, a "mine" list and an "all" list over one Machine.
func listApp() *GeneratedApplication {
	app := boardApp()
	app.ID = "app_w5"
	app.Machines[0].ID = "mch_w5_tugas"
	app.Machines[0].Views = nil
	app.Navigation = []GeneratedMenuItem{
		{Label: "Tugas", MachineID: "mch_w5_tugas"},
		{Label: "Tugas saya", MachineID: "mch_w5_tugas", List: &GeneratedList{Scope: ListScopeMine, PersonField: "fld_pic", Limit: 20}},
		{Label: "Semua tugas", MachineID: "mch_w5_tugas", List: &GeneratedList{Scope: ListScopeAll, SortField: "fld_due", Direction: "asc", Limit: 100}},
	}
	return app
}

// TestW5a_aListIsWrittenAsDatasetAndPageThatTheLoaderAccepts: roadmap stage 1, W5-a.
func TestW5a_aListIsWrittenAsDatasetAndPageThatTheLoaderAccepts(t *testing.T) {
	manifest := w2Workspace(t)
	change := GeneratedChange{Kind: KindNewApplication, Application: listApp()}
	state, _ := stateOf(t, manifest)
	if err := Validate(change, state); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if _, err := Write(manifest, change, nil); err != nil {
		t.Fatalf("Write: %v", err)
	}
	_, loaded := stateOf(t, manifest)
	var mine, all bool
	for _, m := range loaded.Machines {
		if m.ID != "mch_w5_tugas" {
			continue
		}
		for _, ds := range m.Datasets {
			switch {
			case ds.ViewerScoped && ds.Where != nil && len(ds.Where.All) == 1 && ds.Where.All[0].Value == "$current_user" && ds.Where.All[0].Field == "fld_pic":
				mine = ds.Limit == 20
			case !ds.ViewerScoped && ds.Where == nil && ds.Select == "records":
				all = ds.Limit == 100 && len(ds.Sort) == 1 && ds.Sort[0].Field == "fld_due" && ds.Sort[0].Direction == "asc"
			}
		}
	}
	if !mine || !all {
		t.Errorf("datasets read back: mine=%v all=%v", mine, all)
	}
	var routes []string
	for _, n := range loaded.Workspace.Applications[0].AllNavigation {
		routes = append(routes, n.Route)
	}
	if got := strings.Join(routes, " "); !strings.Contains(got, "/machines/mch_w5_tugas") || strings.Count(got, "/pages/") != 2 {
		t.Errorf("routes = %s", got)
	}
}

func TestW5a_eachMisuseIsRefusedForItsOwnReason(t *testing.T) {
	cases := []struct {
		name string
		edit func(a *GeneratedApplication)
		want string
	}{
		{"mine without a person field", func(a *GeneratedApplication) { a.Navigation[1].List.PersonField = "" }, "needs person_field to be a person field"},
		{"mine over a text field", func(a *GeneratedApplication) { a.Navigation[1].List.PersonField = "fld_title" }, "needs person_field to be a person field"},
		{"all with a person field", func(a *GeneratedApplication) { a.Navigation[2].List.PersonField = "fld_pic" }, "takes no person_field"},
		{"a scope per role", func(a *GeneratedApplication) { a.Navigation[2].List.Scope = "role" }, "scope must be"},
		{"no limit", func(a *GeneratedApplication) { a.Navigation[2].List.Limit = 0 }, "limit must be between 1 and 200"},
		{"a limit past the cap", func(a *GeneratedApplication) { a.Navigation[2].List.Limit = 201 }, "limit must be between 1 and 200"},
		{"a sort field that is not one", func(a *GeneratedApplication) { a.Navigation[2].List.SortField = "fld_nope" }, "sort_field"},
		{"a direction that is not one", func(a *GeneratedApplication) { a.Navigation[2].List.Direction = "up" }, "direction must be"},
		{"a Machine with no title", func(a *GeneratedApplication) { a.Machines[0].CardFields = a.Machines[0].CardFields[1:] }, "needs a card_fields entry with role \"title\""},
		{"two plain entries for one Machine", func(a *GeneratedApplication) {
			a.Navigation = append(a.Navigation, GeneratedMenuItem{Label: "Lagi", MachineID: "mch_w5_tugas"})
		}, "more than one navigation entry"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := listApp()
			tc.edit(app)
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

// An update may add a list as a new menu entry over an existing Machine; it may not give an existing entry one.
func TestW5a_anUpdateAddsAListButNeverGivesAnExistingEntryOne(t *testing.T) {
	manifest := w2Workspace(t)
	app := listApp()
	app.Navigation = app.Navigation[:1]
	if _, err := Write(manifest, GeneratedChange{Kind: KindNewApplication, Application: app}, nil); err != nil {
		t.Fatalf("Write: %v", err)
	}
	desire := func(edit func(a *GeneratedApplication)) GeneratedChange {
		state, _ := stateOf(t, manifest)
		d := DescribeApplication("app_w5", state.Applications["app_w5"])
		edit(&d)
		return GeneratedChange{Kind: KindUpdateApplication, TargetAppID: "app_w5", Application: &d}
	}
	state, _ := stateOf(t, manifest)
	err := Validate(desire(func(a *GeneratedApplication) {
		a.Navigation[0].List = &GeneratedList{Scope: ListScopeAll, Limit: 10}
	}), state)
	if err == nil || !strings.Contains(err.Error(), "cannot be given a list") {
		t.Errorf("giving an existing entry a list: want a refusal saying so, got %v", err)
	}
	publish(t, manifest, desire(func(a *GeneratedApplication) {
		a.Navigation = append(a.Navigation, GeneratedMenuItem{Label: "Tugas saya", MachineID: "mch_w5_tugas",
			List: &GeneratedList{Scope: ListScopeMine, PersonField: "fld_pic", Limit: 20}})
	}))
	_, loaded := stateOf(t, manifest)
	scoped := 0
	for _, m := range loaded.Machines {
		for _, ds := range m.Datasets {
			if ds.ViewerScoped {
				scoped++
			}
		}
	}
	if scoped != 1 {
		t.Errorf("the update wrote %d viewer-scoped datasets, want 1", scoped)
	}
}

func TestW5a_thePromptAsksWhoSeesWhatAndRefusesPerRoleVisibility(t *testing.T) {
	full := SystemPromptFor(nil, nil)
	for _, want := range []string{`"scope": "mine"`, "never choose the scope", "izin baca per peran", `kind "runtime"`} {
		if !strings.Contains(full, want) {
			t.Errorf("the prompt lacks %q", want)
		}
	}
}

func statusApp() *GeneratedApplication {
	app := listApp()
	app.Navigation = []GeneratedMenuItem{
		{Label: "Tugas", MachineID: "mch_w5_tugas"},
		{Label: "Per status", MachineID: "mch_w5_tugas", ByStatus: &GeneratedByStatus{StatusField: "fld_status", DetailLabel: "Tugas dalam status", Limit: 100}},
	}
	return app
}

// TestW6_aStatusFilterIsTwoPagesAndTwoDatasetsTheLoaderAccepts: roadmap stage 1, W6.
func TestW6_aStatusFilterIsTwoPagesAndTwoDatasetsTheLoaderAccepts(t *testing.T) {
	manifest := w2Workspace(t)
	change := GeneratedChange{Kind: KindNewApplication, Application: statusApp()}
	state, _ := stateOf(t, manifest)
	if err := Validate(change, state); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if _, err := Write(manifest, change, nil); err != nil {
		t.Fatalf("Write: %v", err)
	}
	_, loaded := stateOf(t, manifest)
	var counts, records bool
	for _, m := range loaded.Machines {
		for _, ds := range m.Datasets {
			counts = counts || (ds.Dimension == "fld_status" && len(ds.Measures) == 1)
			records = records || (ds.Select == "records" && ds.Where != nil && ds.Where.All[0].Value == "$parameters.status" && ds.Limit == 100)
		}
	}
	if !counts || !records {
		t.Errorf("datasets read back: counts=%v records=%v", counts, records)
	}
	nav := loaded.Workspace.Applications[0].AllNavigation
	if len(nav) != 3 || nav[2].Label != "Tugas dalam status" {
		t.Errorf("menu = %+v", nav)
	}
}

func TestW6_eachMisuseIsRefusedForItsOwnReason(t *testing.T) {
	cases := []struct {
		name string
		edit func(a *GeneratedApplication)
		want string
	}{
		{"a text field", func(a *GeneratedApplication) { a.Navigation[1].ByStatus.StatusField = "fld_title" }, "status_field must be a status field"},
		{"no detail label", func(a *GeneratedApplication) { a.Navigation[1].ByStatus.DetailLabel = " " }, "detail_label"},
		{"no limit", func(a *GeneratedApplication) { a.Navigation[1].ByStatus.Limit = 0 }, "limit must be between"},
		{"no title", func(a *GeneratedApplication) { a.Machines[0].CardFields = a.Machines[0].CardFields[1:] }, "role \"title\""},
		{"a list as well", func(a *GeneratedApplication) {
			a.Navigation[1].List = &GeneratedList{Scope: ListScopeAll, Limit: 5}
		}, "not both"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := statusApp()
			tc.edit(app)
			state, _ := stateOf(t, w2Workspace(t))
			err := Validate(GeneratedChange{Kind: KindNewApplication, Application: app}, state)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("want a refusal saying %q, got %v", tc.want, err)
			}
		})
	}
}

func TestW6_anUpdateAddsAStatusFilterOverAnInstalledMachine(t *testing.T) {
	manifest := w2Workspace(t)
	app := statusApp()
	app.Navigation = app.Navigation[:1]
	if _, err := Write(manifest, GeneratedChange{Kind: KindNewApplication, Application: app}, nil); err != nil {
		t.Fatalf("Write: %v", err)
	}
	state, _ := stateOf(t, manifest)
	d := DescribeApplication("app_w5", state.Applications["app_w5"])
	d.Navigation = append(d.Navigation, statusApp().Navigation[1])
	publish(t, manifest, GeneratedChange{Kind: KindUpdateApplication, TargetAppID: "app_w5", Application: &d})
	_, loaded := stateOf(t, manifest)
	if n := len(loaded.Workspace.Applications[0].AllNavigation); n != 3 {
		t.Errorf("menu has %d entries, want 3", n)
	}
	d = DescribeApplication("app_w5", state.Applications["app_w5"])
	d.Navigation[0].ByStatus = statusApp().Navigation[1].ByStatus
	state, _ = stateOf(t, manifest)
	d = DescribeApplication("app_w5", state.Applications["app_w5"])
	d.Navigation[0].ByStatus = statusApp().Navigation[1].ByStatus
	if err := Validate(GeneratedChange{Kind: KindUpdateApplication, TargetAppID: "app_w5", Application: &d}, state); err == nil || !strings.Contains(err.Error(), "cannot be given a list") {
		t.Errorf("giving an existing entry a status filter: got %v", err)
	}
}

func TestW6_thePromptTeachesTheStatusFilter(t *testing.T) {
	full := SystemPromptFor(nil, nil)
	for _, want := range []string{`"by_status"`, "detail_label", "one tile per"} {
		if !strings.Contains(full, want) {
			t.Errorf("the prompt lacks %q", want)
		}
	}
}
