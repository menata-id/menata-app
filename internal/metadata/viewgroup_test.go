package metadata

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
	"menata.app/internal/expression"
)

func scopeCmp(field string) *expression.Predicate {
	return &expression.Predicate{All: []expression.Comparison{{Field: field, Op: expression.OpEquals, Value: "$parameters.project", Optional: true}}}
}

// groupFixture is a valid view group over a task Machine and a project Machine, the pieces each test then breaks one of.
func groupFixture() (domain.Application, map[string]*domain.Machine, map[string]domain.Dataset) {
	tasks := &domain.Machine{ID: "mch_task", Completion: &domain.Completion{Field: "fld_status", Done: "done"}, Fields: []domain.Field{
		{ID: "fld_status", Type: domain.FieldTypeStatus, Options: []string{"todo", "done"}},
		{ID: "fld_project", Type: domain.FieldTypeRelation, RelatedMachine: "mch_project"},
	}}
	projects := &domain.Machine{ID: "mch_project", Fields: []domain.Field{{ID: "fld_name", Type: domain.FieldTypeText}}}
	datasets := map[string]domain.Dataset{
		"ds_table": {ID: "ds_table", Source: "mch_task", Select: domain.SelectRecords, Limit: 10, Where: scopeCmp("fld_project")},
		"ds_figures": {ID: "ds_figures", Source: "mch_task", Measures: []domain.Measure{
			{ID: "msr_all", Aggregate: domain.AggregateCount, Where: scopeCmp("fld_project")},
			{ID: "msr_done", Aggregate: domain.AggregateCount, Where: &expression.Predicate{All: []expression.Comparison{
				{Field: "fld_status", Op: expression.OpEquals, Value: "$done"},
				{Field: "fld_project", Op: expression.OpEquals, Value: "$parameters.project", Optional: true}}}},
		}},
		"ds_projects": {ID: "ds_projects", Source: "mch_project", Select: domain.SelectRecords, Limit: 10},
	}
	app := domain.Application{ID: "app_x",
		ViewGroups: []domain.ViewGroup{{ID: "vg_tasks", Machine: "mch_task",
			Figures: domain.ViewFigures{Dataset: "ds_figures", Total: "msr_all", Complete: "msr_done"},
			Scope:   &domain.ViewScope{Machine: "mch_project", Field: "fld_project", Dataset: "ds_projects", Parameter: "project"}}},
		AllNavigation: []domain.NavigationItem{
			{ID: "nav_table", ViewGroup: "vg_tasks", Route: "/pages/nav_table", Page: &domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{
				{Kind: "component", Type: "Table", Binding: &domain.PageBinding{Dataset: "ds_table", Rows: domain.PageRowsRecords}}}}},
		}}
	return app, map[string]*domain.Machine{"mch_task": tasks, "mch_project": projects}, datasets
}

func TestAViewGroupThatIsWholeRaisesNoIssue(t *testing.T) {
	app, machines, datasets := groupFixture()
	if got := validateViewGroups([]domain.Application{app}, machines, datasets); len(got) != 0 {
		t.Errorf("a whole group: %v", got)
	}
}

// TestAViewGroupIsRefusedForEachWayItCouldBeDishonest: every arrangement of the group must narrow by the scope the header offers, or
// choosing a project would change one screen and leave another showing the whole set.
func TestAViewGroupIsRefusedForEachWayItCouldBeDishonest(t *testing.T) {
	for name, tc := range map[string]struct {
		break_ func(app *domain.Application, ms map[string]*domain.Machine, ds map[string]domain.Dataset)
		want   string
	}{
		"a page dataset that ignores the scope": {func(_ *domain.Application, _ map[string]*domain.Machine, ds map[string]domain.Dataset) {
			d := ds["ds_table"]
			d.Where = nil
			ds["ds_table"] = d
		}, "does not take the group's scope"},
		"a page dataset whose scope is not optional": {func(_ *domain.Application, _ map[string]*domain.Machine, ds map[string]domain.Dataset) {
			d := ds["ds_table"]
			d.Where = &expression.Predicate{All: []expression.Comparison{{Field: "fld_project", Op: expression.OpEquals, Value: "$parameters.project"}}}
			ds["ds_table"] = d
		}, "does not take the group's scope"},
		"a figure that ignores the scope": {func(_ *domain.Application, _ map[string]*domain.Machine, ds map[string]domain.Dataset) {
			d := ds["ds_figures"]
			d.Measures = []domain.Measure{{ID: "msr_all", Aggregate: domain.AggregateCount}, d.Measures[1]}
			ds["ds_figures"] = d
		}, "takes the scope"},
		"a measure the summary reads that is not there": {func(a *domain.Application, _ map[string]*domain.Machine, _ map[string]domain.Dataset) {
			a.ViewGroups[0].Figures.Complete = "msr_nope"
		}, `declares no measure "msr_nope"`},
		"a Machine that cannot say what is complete": {func(_ *domain.Application, ms map[string]*domain.Machine, _ map[string]domain.Dataset) {
			ms["mch_task"].Completion = nil
		}, "declares a `completion:`"},
		"a scope field that is not a relation to the scope machine": {func(a *domain.Application, _ map[string]*domain.Machine, _ map[string]domain.Dataset) {
			a.ViewGroups[0].Scope.Field = "fld_status"
		}, "is a relation of machine"},
		"a scope dataset of another machine": {func(a *domain.Application, _ map[string]*domain.Machine, _ map[string]domain.Dataset) {
			a.ViewGroups[0].Scope.Dataset = "ds_table"
		}, "is a records Dataset of machine"},
		"figures that are a list": {func(a *domain.Application, _ map[string]*domain.Machine, _ map[string]domain.Dataset) {
			a.ViewGroups[0].Figures.Dataset = "ds_table"
		}, "is an aggregate Dataset"},
	} {
		app, machines, datasets := groupFixture()
		tc.break_(&app, machines, datasets)
		if got := strings.Join(validateViewGroups([]domain.Application{app}, machines, datasets), "\n"); !strings.Contains(got, tc.want) {
			t.Errorf("%s: %q, want it to say %q", name, got, tc.want)
		}
	}
}

func shapeDoc() applicationDoc {
	d := applicationDoc{ID: "app_x", Machines: []string{"mch_task", "mch_project"}}
	g := viewGroupDoc{ID: "vg_tasks", Machine: "mch_task"}
	g.Figures.Dataset, g.Figures.Total, g.Figures.Complete = "ds", "a", "b"
	g.Summary.Total.One, g.Summary.Total.Other, g.Summary.Complete, g.Summary.Grouped = "{n} task", "{n} tasks", "{n} done", "by {field}"
	g.Scope = &struct {
		Machine   string `yaml:"machine"`
		Field     string `yaml:"field"`
		Dataset   string `yaml:"dataset"`
		Parameter string `yaml:"parameter"`
		Switch    string `yaml:"switch"`
		All       string `yaml:"all"`
		Chosen    string `yaml:"chosen"`
		Overall   string `yaml:"overall"`
	}{"mch_project", "fld_project", "ds_p", "project", "Switch", "All", "one of {n}", "{n} in all"}
	d.ViewGroups = []viewGroupDoc{g}
	d.Navigation = []navItemDoc{{ID: "nav_a", ViewGroup: "vg_tasks", Summary: "counts"}}
	return d
}

func TestAViewGroupsWordsAndNamesAreChecked(t *testing.T) {
	if got := validateViewGroupShape(shapeDoc()); len(got) != 0 {
		t.Fatalf("a whole group: %v", got)
	}
	for name, tc := range map[string]struct {
		edit func(d *applicationDoc)
		want string
	}{
		"a number the words never show":      {func(d *applicationDoc) { d.ViewGroups[0].Summary.Total.Other = "tasks" }, "carry {n}"},
		"a field the words never name":       {func(d *applicationDoc) { d.ViewGroups[0].Summary.Grouped = "grouped" }, "{field}"},
		"a machine of another application":   {func(d *applicationDoc) { d.ViewGroups[0].Machine = "mch_other" }, "not one of this application's own"},
		"a parameter the runtime owns":       {func(d *applicationDoc) { d.ViewGroups[0].Scope.Parameter = "week" }, "does not own"},
		"a reserved record. parameter":       {func(d *applicationDoc) { d.ViewGroups[0].Scope.Parameter = "record.x" }, "does not own"},
		"an item of a group nobody declared": {func(d *applicationDoc) { d.Navigation[0].ViewGroup = "vg_nope" }, "is not declared"},
		"a summary that is not counts":       {func(d *applicationDoc) { d.Navigation[0].Summary = "figures" }, "the only value is counts"},
		"a summary with no group":            {func(d *applicationDoc) { d.Navigation[0].ViewGroup = "" }, "names a view_group"},
		"a group declared twice":             {func(d *applicationDoc) { d.ViewGroups = append(d.ViewGroups, d.ViewGroups[0]) }, "declared twice"},
		"a scope with no dataset":            {func(d *applicationDoc) { d.ViewGroups[0].Scope.Dataset = "" }, "scope.dataset"},
	} {
		d := shapeDoc()
		tc.edit(&d)
		if got := strings.Join(validateViewGroupShape(d), "\n"); !strings.Contains(got, tc.want) {
			t.Errorf("%s: %q, want it to say %q", name, got, tc.want)
		}
	}
}
