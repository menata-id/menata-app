package domain

import "testing"

func TestFindChildCollections(t *testing.T) {
	project := &Machine{ID: "mch_project", Fields: []Field{{ID: "fld_name", Type: FieldTypeText}}}
	user := &Machine{ID: "mch_user", Fields: []Field{{ID: "fld_name", Type: FieldTypeText}}}
	task := &Machine{
		ID: "mch_task",
		Fields: []Field{
			{ID: "fld_title", Type: FieldTypeText},
			{ID: "fld_project", Type: FieldTypeRelation, RelatedMachine: "mch_project"},
			{ID: "fld_assignee", Type: FieldTypePerson, RelatedMachine: "mch_user"},
		},
	}
	machines := []*Machine{project, user, task}

	projectChildren := FindChildCollections(machines, "mch_project")
	if len(projectChildren) != 1 || projectChildren[0].Machine.ID != "mch_task" || projectChildren[0].Field.ID != "fld_project" {
		t.Fatalf("FindChildCollections(mch_project) = %+v, want one entry: mch_task via fld_project", projectChildren)
	}

	userChildren := FindChildCollections(machines, "mch_user")
	if len(userChildren) != 1 || userChildren[0].Machine.ID != "mch_task" || userChildren[0].Field.ID != "fld_assignee" {
		t.Fatalf("FindChildCollections(mch_user) = %+v, want one entry: mch_task via fld_assignee (Person counts too)", userChildren)
	}

	taskChildren := FindChildCollections(machines, "mch_task")
	if len(taskChildren) != 0 {
		t.Errorf("FindChildCollections(mch_task) = %+v, want none: nothing references Task", taskChildren)
	}
}
