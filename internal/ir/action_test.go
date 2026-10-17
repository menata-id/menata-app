package ir

import (
	"testing"

	"menata.app/internal/domain"
)

func actionResolver(permitted bool) Resolver {
	return Resolver{
		Action: func(b domain.PageBinding) (FormSpec, error) {
			return FormSpec{Permitted: permitted, Inputs: []domain.FormInput{{FieldID: "name", Label: "Group name", Kind: domain.InputText, Required: true}}}, nil
		},
		Records: func(domain.PageBinding) (RecordSet, error) {
			return RecordSet{Records: []map[string]string{{"title": "a", domain.PageRecordIDRole: "grp_1"}, {"title": "b", domain.PageRecordIDRole: "grp_2"}}}, nil
		},
	}
}

func actionForm() domain.PageNode {
	return formPage(domain.PageNode{Kind: "component", Type: string(domain.ComponentForm), Props: map[string]string{"submit": "Create"},
		Binding: &domain.PageBinding{Write: domain.PageWriteAction, Action: "create_group"}})
}

func TestLower_anActionFormPostsToTheRuntimesRouteAndAsksWhatTheActionAsks(t *testing.T) {
	got, err := Lower(actionForm(), actionResolver(true))
	if err != nil {
		t.Fatal(err)
	}
	f := got.Children[0]
	if f.Type != "Form" || f.Props["action"] != "/runtime-actions/create_group" || len(f.Children) != 1 || f.Children[0].Children[0].Props["name"] != "name" {
		t.Errorf("form = %+v", f)
	}
}

func TestLower_aViewerWhoMayNotPerformAnActionGetsNoNodeForIt(t *testing.T) {
	got, err := Lower(actionForm(), actionResolver(false))
	if err != nil || len(got.Children) != 0 {
		t.Errorf("err=%v children=%d, want none", err, len(got.Children))
	}
}

func TestLower_anActionButtonNamesItsRecordAndMayAskBeforeItDoes(t *testing.T) {
	root := formPage(domain.PageNode{
		Kind: "component", Type: string(domain.ComponentCollection), Binding: &domain.PageBinding{Dataset: "ds_a", Rows: domain.PageRowsRecords},
		Children: []domain.PageNode{{Kind: "component", Type: string(domain.ComponentButton), Props: map[string]string{"label": "Delete", "variant": "danger", "confirm": "Delete this group?"},
			Binding: &domain.PageBinding{Write: domain.PageWriteAction, Action: "delete_group"}}},
	})
	got, err := Lower(root, actionResolver(true))
	if err != nil {
		t.Fatal(err)
	}
	items := got.Children[0].Children
	if len(items) != 2 || items[0].Props["action"] != "/runtime-actions/delete_group?record=grp_1" || items[1].Props["action"] != "/runtime-actions/delete_group?record=grp_2" ||
		items[0].Props["method"] != domain.ButtonMethodPost || items[0].Props["confirm"] != "Delete this group?" {
		t.Errorf("items = %+v", items)
	}
}

func TestLower_anActionTakesNoRouteOrVerbFromThePage(t *testing.T) {
	for _, p := range []string{"action", "method", "name", "value"} {
		n := actionForm()
		n.Children[0].Props[p] = "/somewhere"
		if _, err := Lower(n, actionResolver(true)); err == nil {
			t.Errorf("a typed %q was accepted on an action", p)
		}
	}
}

func TestLower_anActionButtonIsNotDrawnForAViewerWhoMayNotPerformIt(t *testing.T) {
	root := formPage(domain.PageNode{
		Kind: "component", Type: string(domain.ComponentCollection), Binding: &domain.PageBinding{Dataset: "ds_a", Rows: domain.PageRowsRecords},
		Children: []domain.PageNode{{Kind: "component", Type: string(domain.ComponentButton), Props: map[string]string{"label": "Delete", "variant": "danger"},
			Binding: &domain.PageBinding{Write: domain.PageWriteAction, Action: "delete_group"}}},
	})
	got, err := Lower(root, actionResolver(false))
	if err != nil || len(got.Children[0].Children) != 0 {
		t.Errorf("err=%v items=%d, want a Collection of no buttons", err, len(got.Children[0].Children))
	}
}
