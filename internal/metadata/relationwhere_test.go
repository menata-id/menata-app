package metadata

import (
	"path/filepath"
	"strings"
	"testing"
)

// relationWhereManifest writes a Workspace of a parent Machine whose Dataset declares one Relation carrying the
// given `where:` block, over a child Machine with a status Field, a text Field and (optionally) a completion.
func relationWhereManifest(t *testing.T, where string, completion bool) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "parent.yaml", `id: mch_parent
name: Parent
datasets:
  - id: ds_parents
    select: records
    limit: 10
    relations:
      - id: rel_open
        machine: mch_child
        via: fld_parent
`+where)
	tail := ""
	if completion {
		tail = "completion: { field: fld_status, done: done }\n"
	}
	writeFile(t, dir, "child.yaml", `id: mch_child
name: Child
fields:
  - id: fld_parent
    name: Parent
    type: relation
    machine: mch_parent
  - id: fld_status
    name: Status
    type: status
    options: [todo, done]
    default: todo
  - id: fld_note
    name: Note
    type: text
  - id: fld_due
    name: Due
    type: date
`+tail)
	writeFile(t, dir, "app.yaml", "workspace: default\nmachines:\n  - parent.yaml\n  - child.yaml\napplications:\n  - app-main.yaml\n")
	writeFile(t, dir, "app-main.yaml", "id: app_rw\nname: RW\nmachines:\n  - mch_parent\n  - mch_child\n")
	return filepath.Join(dir, "app.yaml")
}

func TestRelationWhere_acceptsTheDeclarationsItCanResolve(t *testing.T) {
	for name, where := range map[string]string{
		"$done on the completion field": "        where: { field: fld_status, op: not_equals, value: $done }\n",
		"a literal option":              "        where: { field: fld_status, op: equals, value: todo }\n",
		"no where at all":               "",
		"$today on a date field":        "        where: { field: fld_due, op: lte, value: $today }\n",
		"$today+n on a date field":      "        where: { field: fld_due, op: lte, value: $today+7 }\n",
		"an any: group":                 "        where:\n          any:\n            - { field: fld_status, op: equals, value: todo }\n            - { field: fld_due, op: lt, value: $today }\n",
		"is_empty without a value":      "        where: { field: fld_due, op: is_empty }\n",
	} {
		if _, err := LoadApplication(relationWhereManifest(t, where, true)); err != nil {
			t.Errorf("%s: LoadApplication() = %v, want it to load", name, err)
		}
	}
}

// Every refusal is a filter that would otherwise load and silently keep the wrong children -- a count that is
// plausible and wrong, which is the failure no screen shows.
func TestRelationWhere_refusesWhatItCannotResolve(t *testing.T) {
	for name, tc := range map[string]struct {
		where      string
		completion bool
		want       string
	}{
		"a field the child does not have": {"        where: { field: fld_nope, op: equals, value: x }\n", true, "not a field of machine"},
		"an unknown operator":             {"        where: { field: fld_note, op: resembles, value: x }\n", true, "not a known operator"},
		"an option the field lacks":       {"        where: { field: fld_status, op: equals, value: later }\n", true, "options"},
		"$done on a child without completion": {
			"        where: { field: fld_status, op: not_equals, value: $done }\n", false, "completion"},
		"$done on a field that is not the completion field": {
			"        where: { field: fld_note, op: equals, value: $done }\n", true, "completion"},
		"is_empty given a value":                 {"        where: { field: fld_due, op: is_empty, value: x }\n", true, "takes none"},
		"an ordered op on a text field":          {"        where: { field: fld_note, op: lt, value: x }\n", true, "orders a number or a date"},
		"$today on a field that is not a date":   {"        where: { field: fld_note, op: equals, value: $today }\n", true, "is a date"},
		"$week, which needs a request parameter": {"        where: { field: fld_due, op: gte, value: $week }\n", true, "cannot resolve"},
		"$done with an ordered op":               {"        where: { field: fld_status, op: lt, value: $done }\n", true, "is a state"},
		"$current_user, which a relation cannot resolve": {
			"        where: { field: fld_note, op: equals, value: $current_user }\n", true, "cannot resolve"},
	} {
		_, err := LoadApplication(relationWhereManifest(t, tc.where, tc.completion))
		if err == nil {
			t.Errorf("%s: LoadApplication() = nil, want an error", name)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error = %v, want it to mention %q", name, err, tc.want)
		}
	}
}
