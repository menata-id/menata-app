package metadata

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"menata.app/internal/domain"
	"menata.app/internal/expression"
)

// viewGroupDoc is one `view_groups:` entry of an Application -- see domain.ViewGroup. The words are written once, here, and every
// number in them is substituted at request time.
type viewGroupDoc struct {
	ID      string `yaml:"id"`
	Machine string `yaml:"machine"`
	Figures struct {
		Dataset  string `yaml:"dataset"`
		Total    string `yaml:"total"`
		Complete string `yaml:"complete"`
	} `yaml:"figures"`
	Summary struct {
		Total struct {
			One   string `yaml:"one"`
			Other string `yaml:"other"`
		} `yaml:"total"`
		Complete string `yaml:"complete"`
		Grouped  string `yaml:"grouped"`
	} `yaml:"summary"`
	Scope *struct {
		Machine   string `yaml:"machine"`
		Field     string `yaml:"field"`
		Dataset   string `yaml:"dataset"`
		Parameter string `yaml:"parameter"`
		Switch    string `yaml:"switch"`
		All       string `yaml:"all"`
		Chosen    string `yaml:"chosen"`
		Overall   string `yaml:"overall"`
	} `yaml:"scope"`
}

func (d viewGroupDoc) toDomain() domain.ViewGroup {
	g := domain.ViewGroup{
		ID: d.ID, Machine: d.Machine,
		Figures: domain.ViewFigures{Dataset: d.Figures.Dataset, Total: d.Figures.Total, Complete: d.Figures.Complete},
		Words:   domain.ViewWords{TotalOne: d.Summary.Total.One, TotalOther: d.Summary.Total.Other, Complete: d.Summary.Complete, Grouped: d.Summary.Grouped},
	}
	if s := d.Scope; s != nil {
		g.Scope = &domain.ViewScope{Machine: s.Machine, Field: s.Field, Dataset: s.Dataset, Parameter: s.Parameter, Switch: s.Switch, All: s.All, Chosen: s.Chosen, Overall: s.Overall}
	}
	return g
}

func toViewGroups(docs []viewGroupDoc) []domain.ViewGroup {
	var out []domain.ViewGroup
	for _, d := range docs {
		out = append(out, d.toDomain())
	}
	return out
}

var viewGroupID = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// validateViewGroupShape is what a group can be checked for from the Application's own file: ids, the words and their
// tokens, and that every navigation item naming a group names one this Application declares. What needs the Machines (the
// Dataset, the Field, the scope parameter every page Dataset must carry) is validateViewGroups, with the pages.
func validateViewGroupShape(doc applicationDoc) []string {
	var issues []string
	declared := map[string]bool{}
	for i, g := range doc.ViewGroups {
		where := fmt.Sprintf("application %q: view_groups[%d]", doc.ID, i)
		switch {
		case !viewGroupID.MatchString(g.ID):
			issues = append(issues, fmt.Sprintf("%s: id %q is lower-case words joined by _", where, g.ID))
			continue
		case declared[g.ID]:
			issues = append(issues, fmt.Sprintf("%s: id %q is declared twice", where, g.ID))
			continue
		}
		declared[g.ID] = true
		if g.Machine == "" || !slices.Contains(doc.Machines, g.Machine) {
			issues = append(issues, fmt.Sprintf("%s: machine %q is not one of this application's own machines", where, g.Machine))
		}
		if g.Figures.Dataset == "" || g.Figures.Total == "" || g.Figures.Complete == "" {
			issues = append(issues, fmt.Sprintf("%s: figures: names the dataset and the two measures (total, complete) the summary is counted from", where))
		}
		if !strings.Contains(g.Summary.Total.One, domain.ViewCountToken) || !strings.Contains(g.Summary.Total.Other, domain.ViewCountToken) {
			issues = append(issues, fmt.Sprintf("%s: summary.total.one and .other each carry %s, or the number is never shown", where, domain.ViewCountToken))
		}
		if !strings.Contains(g.Summary.Complete, domain.ViewCountToken) {
			issues = append(issues, fmt.Sprintf("%s: summary.complete carries %s, or the number is never shown", where, domain.ViewCountToken))
		}
		if !strings.Contains(g.Summary.Grouped, domain.ViewFieldToken) {
			issues = append(issues, fmt.Sprintf("%s: summary.grouped carries %s, or the field is never named", where, domain.ViewFieldToken))
		}
		if s := g.Scope; s != nil {
			switch {
			case s.Machine == "" || !slices.Contains(doc.Machines, s.Machine):
				issues = append(issues, fmt.Sprintf("%s: scope.machine %q is not one of this application's own machines", where, s.Machine))
			case s.Field == "" || s.Dataset == "":
				issues = append(issues, fmt.Sprintf("%s: scope.field names the group machine's relation Field to the scope machine, and scope.dataset the records Dataset of the scope machine the menu lists", where))
			case !regexp.MustCompile(`^[a-z][a-z0-9_]*$`).MatchString(s.Parameter) || domain.ParamReserved(s.Parameter) || s.Parameter == expression.WeekParameter:
				issues = append(issues, fmt.Sprintf("%s: scope.parameter %q is a lower-case word the runtime does not own", where, s.Parameter))
			case s.Switch == "" || s.All == "" || !strings.Contains(s.Chosen, domain.ViewCountToken) || !strings.Contains(s.Overall, domain.ViewCountToken):
				issues = append(issues, fmt.Sprintf("%s: scope words: switch and all are said, chosen and overall each carry %s", where, domain.ViewCountToken))
			}
		}
	}
	for _, n := range doc.Navigation {
		if n.ViewGroup != "" && !declared[n.ViewGroup] {
			issues = append(issues, fmt.Sprintf("application %q: navigation item %q: view_group %q is not declared in this application's view_groups", doc.ID, n.ID, n.ViewGroup))
		}
		if n.Summary != "" && n.Summary != domain.SummaryCounts {
			issues = append(issues, fmt.Sprintf("application %q: navigation item %q: summary: is %q, and the only value is %s", doc.ID, n.ID, n.Summary, domain.SummaryCounts))
		}
		if n.Summary != "" && n.ViewGroup == "" {
			issues = append(issues, fmt.Sprintf("application %q: navigation item %q: summary: words the group's header, so the item names a view_group", doc.ID, n.ID))
		}
	}
	return issues
}

// validateViewGroups is the half that needs the Workspace's Machines and the loaded pages: each group's Machine and Dataset
// exist and say what the header reads, the scope Field is a relation to the scope Machine, and -- the point of the declaration --
// every records Dataset a page of the group reads over the group's Machine carries the scope comparison, so choosing a project
// narrows every arrangement and none can be written that forgets to.
func validateViewGroups(applications []domain.Application, byID map[string]*domain.Machine, datasets map[string]domain.Dataset) []string {
	var issues []string
	for _, app := range applications {
		for _, g := range app.ViewGroups {
			where := fmt.Sprintf("application %q: view group %q", app.ID, g.ID)
			m := byID[g.Machine]
			if m == nil {
				issues = append(issues, fmt.Sprintf("%s: machine %q is not installed in this workspace", where, g.Machine))
				continue
			}
			if m.Completion == nil {
				issues = append(issues, fmt.Sprintf("%s: the summary counts what is complete, so machine %q declares a `completion:`", where, m.ID))
			}
			ds, ok := datasets[g.Figures.Dataset]
			switch {
			case !ok || ds.Source != m.ID || ds.Select == domain.SelectRecords:
				issues = append(issues, fmt.Sprintf("%s: figures.dataset %q is an aggregate Dataset of machine %q", where, g.Figures.Dataset, m.ID))
			default:
				for _, id := range []string{g.Figures.Total, g.Figures.Complete} {
					if !hasMeasure(ds, id) {
						issues = append(issues, fmt.Sprintf("%s: figures.dataset %q declares no measure %q", where, ds.ID, id))
					}
				}
			}
			if g.Scope != nil {
				sm, f := byID[g.Scope.Machine], domain.Field{}
				if sm == nil {
					issues = append(issues, fmt.Sprintf("%s: scope.machine %q is not installed in this workspace", where, g.Scope.Machine))
				}
				if sds, ok := datasets[g.Scope.Dataset]; !ok || sds.Source != g.Scope.Machine || sds.Select != domain.SelectRecords {
					issues = append(issues, fmt.Sprintf("%s: scope.dataset %q is a records Dataset of machine %q", where, g.Scope.Dataset, g.Scope.Machine))
				}
				if fld, found := m.FieldByID(g.Scope.Field); found {
					f = fld
				}
				if f.Type != domain.FieldTypeRelation || f.RelatedMachine != g.Scope.Machine {
					issues = append(issues, fmt.Sprintf("%s: scope.field %q is a relation of machine %q to machine %q", where, g.Scope.Field, m.ID, g.Scope.Machine))
				}
				if ds, ok := datasets[g.Figures.Dataset]; ok && !carriesScope(ds, *g.Scope, g.Figures.Total, g.Figures.Complete) {
					issues = append(issues, fmt.Sprintf("%s: figures.dataset %q takes the scope: the two measures the summary reads each carry an optional `%s equals $parameters.%s` comparison", where, ds.ID, g.Scope.Field, g.Scope.Parameter))
				}
			}
			for _, item := range app.ViewGroupItems(g.ID) {
				if item.Page == nil {
					continue
				}
				if g.Scope == nil {
					continue
				}
				for _, id := range boundDatasets(*item.Page) {
					ds, ok := datasets[id]
					if !ok || ds.Source != g.Machine || ds.Select != domain.SelectRecords {
						continue
					}
					if !carriesScope(ds, *g.Scope) {
						issues = append(issues, fmt.Sprintf("%s: navigation item %q reads dataset %q, which does not take the group's scope -- add `{field: %s, op: equals, value: $parameters.%s, optional: true}` to its where:", where, item.ID, id, g.Scope.Field, g.Scope.Parameter))
					}
				}
			}
		}
	}
	return issues
}

func hasMeasure(ds domain.Dataset, id string) bool {
	for _, ms := range ds.Measures {
		if ms.ID == id {
			return true
		}
	}
	return false
}

// carriesScope says a Dataset narrows itself by the group's scope: a records Dataset has the optional comparison in its where:,
// an aggregate one in the where: of each Measure the summary reads (measures).
func carriesScope(ds domain.Dataset, s domain.ViewScope, measures ...string) bool {
	has := func(p *expression.Predicate) bool {
		if p == nil {
			return false
		}
		for _, c := range p.Comparisons() {
			if c.Field == s.Field && c.Op == expression.OpEquals && c.Value == expression.SentinelParameterPrefix+s.Parameter && c.Optional {
				return true
			}
		}
		return false
	}
	if ds.Select == domain.SelectRecords {
		return has(ds.Where)
	}
	for _, id := range measures {
		found := false
		for _, ms := range ds.Measures {
			if ms.ID == id {
				found = has(ms.Where)
			}
		}
		if !found {
			return false
		}
	}
	return len(measures) > 0
}
