package aiassist

import (
	"reflect"
	"strings"
	"testing"
)

func categorisedApp() *GeneratedApplication {
	app := designRequestApp()
	app.Machines[1].StatusCategories = &GeneratedStatusCategories{Field: "fld_status", Values: []GeneratedStatusCategory{
		{Option: "baru", Category: "start"}, {Option: "dikerjakan", Category: "active"}, {Option: "selesai", Category: "done"},
	}}
	return app
}

func TestStatusCategories_areWrittenLoadedAndReadBack(t *testing.T) {
	manifest := w2Workspace(t)
	change := GeneratedChange{Kind: KindNewApplication, Application: categorisedApp()}
	state, _ := stateOf(t, manifest)
	if err := Validate(change, state); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if _, err := Write(manifest, change, nil); err != nil {
		t.Fatalf("Write: %v", err)
	}
	state, _ = stateOf(t, manifest)
	got := DescribeApplication("app_w2_request", state.Applications["app_w2_request"])
	for _, m := range got.Machines {
		if m.ID == "mch_w2_request" {
			if !reflect.DeepEqual(m.StatusCategories, categorisedApp().Machines[1].StatusCategories) {
				t.Errorf("read back %+v", m.StatusCategories)
			}
			return
		}
	}
	t.Fatal("no request machine")
}

func TestStatusCategories_areRefusedForTheLoadersReason(t *testing.T) {
	for name, edit := range map[string]func(*GeneratedMachine){
		"a field that is not a status": func(m *GeneratedMachine) { m.StatusCategories.Field = "fld_budget" },
		"an option the field lacks":    func(m *GeneratedMachine) { m.StatusCategories.Values[0].Option = "nope" },
		"an unknown category":          func(m *GeneratedMachine) { m.StatusCategories.Values[0].Category = "purple" },
		"done that completion denies":  func(m *GeneratedMachine) { m.StatusCategories.Values[1].Category = "done" },
		"an option listed twice":       func(m *GeneratedMachine) { m.StatusCategories.Values[1].Option = "baru" },
	} {
		app := categorisedApp()
		edit(&app.Machines[1])
		state, _ := stateOf(t, w2Workspace(t))
		err := Validate(GeneratedChange{Kind: KindNewApplication, Application: app}, state)
		if err == nil {
			t.Errorf("%s was accepted", name)
			continue
		}
		if !strings.Contains(err.Error(), "status_categories") && !strings.Contains(err.Error(), "categor") {
			t.Errorf("%s refused for another reason: %v", name, err)
		}
	}
}

func TestStatusCategories_updateMayAddButNotChange(t *testing.T) {
	manifest := w2Workspace(t)
	if _, err := Write(manifest, GeneratedChange{Kind: KindNewApplication, Application: designRequestApp()}, nil); err != nil {
		t.Fatal(err)
	}
	desire := func(edit func(*GeneratedMachine)) GeneratedChange {
		state, _ := stateOf(t, manifest)
		d := DescribeApplication("app_w2_request", state.Applications["app_w2_request"])
		for i := range d.Machines {
			if d.Machines[i].ID == "mch_w2_request" {
				edit(&d.Machines[i])
			}
		}
		return GeneratedChange{Kind: KindUpdateApplication, TargetAppID: "app_w2_request", Application: &d}
	}
	add := desire(func(m *GeneratedMachine) { m.StatusCategories = categorisedApp().Machines[1].StatusCategories })
	state, _ := stateOf(t, manifest)
	if err := Validate(add, state); err != nil {
		t.Fatalf("adding categories by update was refused: %v", err)
	}
	publish(t, manifest, add)
	change := desire(func(m *GeneratedMachine) { m.StatusCategories.Values[0].Category = "attention" })
	state, _ = stateOf(t, manifest)
	err := Validate(change, state)
	if err == nil || !strings.Contains(err.Error(), "status_categories") {
		t.Errorf("changing a declared category: %v", err)
	}
}
