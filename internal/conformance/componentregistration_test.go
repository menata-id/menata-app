package conformance

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"menata.app/internal/domain"
	"menata.app/internal/ir"
	"menata.app/internal/metadata"
	"menata.app/internal/registry"
)

// G3 of menata-app-document's audits/2026-10-11-rencana-design-system.md (rencana 2.1, 2026-10-13): a registered
// Component is complete only when four things agree about it -- its contract's Status, the installed manifests, the
// capabilities.md row that says what it is, and the writing-guide table that tells an author how far to rely on it.
// Each gate below holds one pair; none of them judges whether a status is *wise*, only whether it is *true*.

// componentsWithoutInstalledUse is every registered Component no installed Workspace's `page:` declares or lowers to,
// with what is true of it instead. Closed both ways: a Component that gains an installed use must leave (and be
// promoted from experimental on the same change), and an entry naming a type nothing registers fails too. An empty map
// stays declared, like any emptied ratchet.
var componentsWithoutInstalledUse = map[string]string{
	"Avatar":           "drawn by bespoke screens only (members, assignees, the app shell) -- the contract was registered over four measured sites, and no `page:` declares one yet",
	"SegmentedControl": "drawn by the record list's view switch only (`viewSwitcher`, every Machine with two or more Views) -- registered over that one site (2026-10-14, rencana 2.5 step 1); no `page:` declares one, because a declared page has no handler that reads the query parameter yet",
	"Disclosure":       "drawn by the Groups and Members screens only (\"+ New group\", \"+ Invite member\") -- registered over those two byte-identical panels (2026-10-14); no declared screen has a panel that opens, and wrapping a Form in one is a visible change to a page the owner has not asked for",
	"Callout":          "drawn by the account screens only -- registered over six measured sites (2026-10-12); no declared screen shows a message today (writing-guide §12.1a)",
}

// installedComponentUse counts, over every installed Workspace's declared `page:`, the Components an author wrote
// (`written`) and the Components that exist once the page is lowered (`lowered`), which is a superset: a Form lowers to
// a Field and an Input around each control, and nobody writes either. "Stable" rests on the second count, because
// changing a Field's inputs breaks every installed Form whether or not a manifest names a Field.
func installedComponentUse(t *testing.T) (written, lowered map[string]int, pages int) {
	t.Helper()
	wss, err := metadata.LoadWorkspaces(filepath.Join(repoRoot(), "metadata", "workspaces"))
	if err != nil {
		t.Fatalf("load workspaces: %v", err)
	}
	ir.RegisterComponentTypes(registry.ComponentTypeNames())
	ir.RegisterSlottedComponentTypes(registry.SlottedComponentTypeNames())

	written, lowered = map[string]int{}, map[string]int{}
	counted := map[string]bool{}
	var walkWritten func(n domain.PageNode)
	walkWritten = func(n domain.PageNode) {
		if n.Kind == "component" {
			written[n.Type]++
		}
		// A layout's `empty:` lowers to an EmptyState only on a day everything it holds is hidden, and the load-time
		// placeholder record means that day never occurs while lowering (the blind spot stated on the test below). The
		// page still depends on the contract, so the declaration counts as a lowered use (owner decision E-1).
		if n.Kind == "layout" && n.Props["empty"] != "" {
			lowered[string(domain.ComponentEmptyState)]++
		}
		for _, c := range n.Children {
			walkWritten(c)
		}
	}
	var walkLowered func(n ir.UINode)
	walkLowered = func(n ir.UINode) {
		if n.Kind == ir.NodeComponent {
			lowered[n.Type]++
		}
		for _, c := range n.Children {
			walkLowered(c)
		}
	}
	for _, slug := range sortedKeys(wss) {
		ws := wss[slug].Workspace
		datasets := map[string]domain.Dataset{}
		for _, m := range ws.Machines {
			for _, ds := range m.Datasets {
				datasets[ds.ID] = ds
			}
		}
		for _, app := range ws.Applications {
			for _, item := range app.AllNavigation {
				if item.Page == nil {
					continue
				}
				pages++
				counted[slug] = true
				walkWritten(*item.Page)
				tree, err := ir.Lower(*item.Page, metadata.PlaceholderResolver(app.AllNavigation, datasets))
				if err != nil {
					t.Fatalf("%s/%s/%s does not lower: %v (TestEveryInstalledPageLowersAndValidates reports this too)", slug, app.ID, item.ID, err)
				}
				walkLowered(tree)
			}
		}
	}
	if len(counted) < 2 {
		t.Fatalf("only %d installed Workspace(s) declare a `page:` -- \"installed use\" is a sweep of every Workspace, and one cannot tell it from a gate reading only `default`", len(counted))
	}
	return written, lowered, pages
}

// TestComponentStatusMatchesInstalledUse holds each registered Component's Status to the installed manifests, both
// ways:
//   - `stable` needs an installed page that depends on the contract, written or lowered;
//   - `experimental` may have none, and one that gains a use must be promoted -- the first page using a Component is
//     the moment its contract stops being free to change, and that is a decision, not a drift;
//   - a Component with no installed use is listed in componentsWithoutInstalledUse with its reason, and an entry whose
//     Component has gained a use, or no longer exists, fails.
//
// It is the pair of "a Component arrives with the uses it replaces" for the catalogue's own state: the reverse
// failure is a Status that was true when written and is not now, which nothing else would notice because both
// words render the same.
//
// **Blind spots, stated:**
//   - A page is lowered with the loader's placeholder records, which mark a record `ShapeOnly` so that a node a real
//     record would hide (`when:`) is still validated. A Component that only such a node holds counts as used, so
//     `stable` can rest on a node no real record shows today. No current status is wrong for it (the 3.4-era review
//     checked); the count is of *declared and lowered* shape, which is what a contract change breaks.
//   - "installed" is the Workspace manifests under metadata/workspaces/, the way every sweep here
//     reads them; a page in the template library that no Workspace has installed depends on nothing. And the count is of
//
// declared pages, so a Component drawn only by bespoke Go screens (Avatar, Callout) has no use here by design: that is
// the measurement `experimental` is defined by, not a verdict on how widely it is drawn.
func TestComponentStatusMatchesInstalledUse(t *testing.T) {
	written, lowered, pages := installedComponentUse(t)
	if pages == 0 || len(lowered) < 5 {
		t.Fatalf("only %d installed pages / %d Component types lowered -- this gate is measuring far less than it appears to", pages, len(lowered))
	}
	if problems := registry.CatalogueProblems(); len(problems) > 0 {
		t.Errorf("the catalogue breaks its own status rules:\n  %s", strings.Join(problems, "\n  "))
	}
	for typ := range componentsWithoutInstalledUse {
		if _, ok := registry.Components[domain.ComponentType(typ)]; !ok {
			t.Errorf("componentsWithoutInstalledUse names %q, which registry.Components does not hold -- remove the entry", typ)
		}
	}
	for _, typ := range sortedComponentTypes() {
		c := registry.Components[domain.ComponentType(typ)].Contract
		used := lowered[typ] > 0
		reason, listed := componentsWithoutInstalledUse[typ]
		t.Logf("%-12s %-12s written %3d, lowered %3d", typ, c.Status, written[typ], lowered[typ])
		switch {
		case used && c.Status == domain.ComponentStatusExperimental:
			t.Errorf("%s is experimental, but %d installed page node(s) depend on its contract -- a manifest now breaks if its inputs change, so promote it to stable (a decision to record), or find why the page should not use it", typ, lowered[typ])
		case !used && c.Status == domain.ComponentStatusStable:
			t.Errorf("%s is stable, but no installed page declares it or lowers to it -- stable means a manifest depends on the contract; demote it to experimental, or install the use that makes it true", typ)
		}
		switch {
		case !used && !listed:
			t.Errorf("%s has no installed use and is not in componentsWithoutInstalledUse -- add it with what is true of it instead (a Component arrives with the uses it replaces, 007 §34)", typ)
		case !used && strings.TrimSpace(reason) == "":
			t.Errorf("componentsWithoutInstalledUse lists %s with no reason", typ)
		case used && listed:
			t.Errorf("componentsWithoutInstalledUse lists %s, which %d installed page node(s) now use -- remove the entry, and promote it if it is still experimental", typ, lowered[typ])
		}
	}
}

// tableCells splits one markdown table row into its trimmed cells, or nil if the line is not a row.
func tableCells(line string) []string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") {
		return nil
	}
	cells := strings.Split(line[1:len(line)-1], "|")
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}
	return cells
}

func sortedComponentTypes() []string {
	out := make([]string, 0, len(registry.Components))
	for typ := range registry.Components {
		out = append(out, string(typ))
	}
	sort.Strings(out)
	return out
}

// capabilitiesRegisteredKind is what a capabilities.md row's Kind cell starts with when the row is a registered Component.
const capabilitiesRegisteredKind = "registered Component"

// capabilitiesFirstCells returns, for every row of a table in capabilities.md that has a Kind column, the backticked names
// in its first cell and its Kind. Rows are read cell by cell: pooling a row's prose with the others' was the first
// version of this gate, and the independent review emptied the Button row's Kind with nothing failing.
func capabilitiesFirstCells(t *testing.T) (names [][]string, kinds []string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(), "capabilities.md"))
	if err != nil {
		t.Fatal(err)
	}
	backticked := regexp.MustCompile("`([A-Za-z.]+)`")
	for _, line := range strings.Split(string(b), "\n") {
		cells := tableCells(line)
		if len(cells) < 3 || !strings.HasPrefix(line, "| `") {
			continue
		}
		var n []string
		for _, m := range backticked.FindAllStringSubmatch(cells[0], -1) {
			n = append(n, m[1])
		}
		names, kinds = append(names, n), append(kinds, cells[1])
	}
	return names, kinds
}

// TestEveryRegisteredComponentIsInCapabilities holds capabilities.md's "Shared rendering components" table to the
// catalogue in both directions, by the first cell (the column `TestCapabilitiesComponentsTableMatchesTempl` already
// holds to the .templ files, which neither it nor anything else reads the Kind of):
//   - each registered Component's renderer is named in the first cell of a row whose Kind starts with
//     `registered Component`;
//   - every row whose first cell names a registered renderer has that Kind, so a Component cannot be described as a
//     "shared fragment" in a second row.
func TestEveryRegisteredComponentIsInCapabilities(t *testing.T) {
	names, kinds := capabilitiesFirstCells(t)
	registered := 0
	for _, k := range kinds {
		if strings.HasPrefix(k, capabilitiesRegisteredKind) {
			registered++
		}
	}
	if registered < 5 {
		t.Fatalf("found %d `%s` rows in capabilities.md -- the Kind column moved or this gate reads the wrong table", registered, capabilitiesRegisteredKind)
	}
	renderers := map[string]string{} // renderer -> type
	for _, typ := range sortedComponentTypes() {
		renderers[registry.Components[domain.ComponentType(typ)].Contract.Renderer] = typ
	}
	named := map[string]bool{}
	for i, row := range names {
		for _, n := range row {
			typ, isRenderer := renderers[n]
			if !isRenderer {
				continue
			}
			if !strings.HasPrefix(kinds[i], capabilitiesRegisteredKind) {
				t.Errorf("capabilities.md row naming `%s` (the renderer of %s) has Kind %q -- a registered Component's row is Kind `%s`", n, typ, kinds[i], capabilitiesRegisteredKind)
				continue
			}
			named[typ] = true
		}
	}
	for _, typ := range sortedComponentTypes() {
		if !named[typ] {
			t.Errorf("%s is registered and no capabilities.md row of Kind `%s` names its renderer `%s` in the first cell -- add the row (or the name) in the change that registers it", typ, capabilitiesRegisteredKind, registry.Components[domain.ComponentType(typ)].Contract.Renderer)
		}
	}
}

// statusTableHeader is the cells of the writing-guide status table's header row. The table is the run of rows after it,
// so a row-shaped line anywhere else in the section (a code fence, a quote, prose) is not the table.
var statusTableHeader = []string{"Component", "Status", "What that rests on"}

// statusTableRow matches one row of the table: `| `Type` | status |`, and the status word must be a whole cell, so
// `stable-ish` or `stable (for now)` is not `stable`.
var statusTableRow = regexp.MustCompile("^\\| `([A-Za-z]+)` \\| (experimental|stable|deprecated) \\|")

// writingGuideSection12_1a is the text of `### 12.1a` up to the next heading of any level outside a code fence: the
// section an author reads to write a `page:`. The first version stopped only at the next `### `, which made the "section"
// 715 lines long and a row in a fence near its end count as the table.
func writingGuideSection12_1a(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(), "writing-guide.md"))
	if err != nil {
		t.Fatal(err)
	}
	return sectionAfterHeading(t, string(b), "### 12.1a ")
}

// sectionAfterHeading returns the lines from the first one starting with heading to the next heading line (any level,
// `#` to `######` and a space) that is not inside a ``` fence, so a YAML comment in an example does not end it.
func sectionAfterHeading(t *testing.T, doc, heading string) string {
	t.Helper()
	heads := regexp.MustCompile(`^#{1,6} `)
	lines := strings.Split(doc, "\n")
	start := -1
	fenced := false
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			fenced = !fenced
		}
		if start < 0 {
			if !fenced && strings.HasPrefix(l, heading) {
				start = i
			}
			continue
		}
		if !fenced && heads.MatchString(l) {
			return strings.Join(lines[start:i], "\n")
		}
	}
	if start < 0 {
		t.Fatalf("the document has no %q heading -- the section moved", strings.TrimSpace(heading))
	}
	return strings.Join(lines[start:], "\n")
}

// TestSectionAfterHeading pins the section scanner: it stops at a deeper heading, ignores a `#` comment in a fence, and
// stops at the end of the document.
func TestSectionAfterHeading(t *testing.T) {
	doc := "intro\n### A \nalpha\n```yaml\n# a comment, not a heading\n### nor this\n```\n#### B\nbeta\n### C\ngamma\n"
	if got := sectionAfterHeading(t, doc, "### A "); got != "### A \nalpha\n```yaml\n# a comment, not a heading\n### nor this\n```" {
		t.Errorf("section A = %q", got)
	}
	if got := sectionAfterHeading(t, doc, "### C"); got != "### C\ngamma\n" {
		t.Errorf("section C = %q", got)
	}
}

// TestWritingGuideShowsEachComponentsStatus holds the table in writing-guide §12.1a to the catalogue, both ways: every
// registered Component has exactly one row, its status word is the contract's, a deprecated one names its successor in
// backticks, and a row naming a type nothing registers fails. The guide is the only file a metadata author reads, and
// the status is for that reader -- a contract that says `stable` while the guide says `experimental` tells an author the
// opposite of the truth.
func TestWritingGuideShowsEachComponentsStatus(t *testing.T) {
	section := writingGuideSection12_1a(t)
	for _, p := range statusTableProblems(section, registry.Components) {
		t.Error(p)
	}
}

// statusTableProblems is the comparison behind TestWritingGuideShowsEachComponentsStatus, taking the catalogue as an
// argument so TestStatusTableProblems can feed it one with a deprecated member: no real Component is deprecated, and
// a branch no real input reaches is a branch nothing has shown can fail.
func statusTableProblems(section string, catalogue map[domain.ComponentType]registry.Component) []string {
	lines := strings.Split(section, "\n")
	header, fenced := -1, false
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		if cells := tableCells(l); len(cells) == len(statusTableHeader) && strings.Join(cells, "|") == strings.Join(statusTableHeader, "|") {
			if header >= 0 {
				return []string{"writing-guide §12.1a has two Component status tables (two `| Component | Status | What that rests on |` headers) -- one table"}
			}
			header = i
		}
	}
	if header < 0 {
		return []string{"writing-guide §12.1a has no Component status table (a `| Component | Status | What that rests on |` header) -- an author cannot see how far to rely on a Component"}
	}
	var problems []string
	i := header + 1
	if i >= len(lines) || tableCells(lines[i]) == nil || !strings.Contains(lines[i], "---") {
		problems = append(problems, "the Component status table's header is not followed by a separator row (`|---|---|---|`)")
	} else {
		i++
	}
	rows := map[string][]string{} // type -> the lines that name it
	statusOf := map[string]string{}
	for ; i < len(lines) && tableCells(lines[i]) != nil; i++ {
		m := statusTableRow.FindStringSubmatch(lines[i])
		if m == nil {
			problems = append(problems, fmt.Sprintf("a row of the Component status table is not `| `Type` | experimental|stable|deprecated | ... |`: %q", lines[i]))
			continue
		}
		rows[m[1]] = append(rows[m[1]], lines[i])
		statusOf[m[1]] = m[2]
	}
	types := make([]string, 0, len(catalogue))
	for typ := range catalogue {
		types = append(types, string(typ))
	}
	sort.Strings(types)
	for _, typ := range types {
		c := catalogue[domain.ComponentType(typ)].Contract
		switch rs := rows[typ]; {
		case len(rs) == 0:
			problems = append(problems, fmt.Sprintf("%s is registered (%s) and writing-guide §12.1a's status table has no row for it", typ, c.Status))
		case len(rs) > 1:
			problems = append(problems, fmt.Sprintf("%s has %d rows in writing-guide §12.1a's status table -- one row per Component", typ, len(rs)))
		case statusOf[typ] != string(c.Status):
			problems = append(problems, fmt.Sprintf("%s is %s in its contract and %s in writing-guide §12.1a's status table -- an author is told the opposite of the truth", typ, c.Status, statusOf[typ]))
		case c.Status == domain.ComponentStatusDeprecated && !strings.Contains(rs[0], "`"+string(c.ReplacedBy)+"`"):
			problems = append(problems, fmt.Sprintf("%s is deprecated in favour of %s, and its writing-guide row does not name `%s`", typ, c.ReplacedBy, c.ReplacedBy))
		}
	}
	extra := make([]string, 0, len(rows))
	for typ := range rows {
		if _, ok := catalogue[domain.ComponentType(typ)]; !ok {
			extra = append(extra, typ)
		}
	}
	sort.Strings(extra)
	for _, typ := range extra {
		problems = append(problems, fmt.Sprintf("writing-guide §12.1a's status table has a row for `%s`, which registry.Components does not hold -- remove it", typ))
	}
	return problems
}

// TestStatusTableProblems feeds the comparison a catalogue it can fail on, including the one case the real catalogue
// cannot -- a deprecated Component, whose row must name its successor -- and the shapes the independent review passed
// through the first version: a row in a code fence, a missing header, and a status word with a suffix.
func TestStatusTableProblems(t *testing.T) {
	cat := map[domain.ComponentType]registry.Component{
		"Old": {Contract: domain.ComponentContract{Type: "Old", Status: domain.ComponentStatusDeprecated, ReplacedBy: "New"}},
		"New": {Contract: domain.ComponentContract{Type: "New", Status: domain.ComponentStatusStable}},
	}
	head := "| Component | Status | What that rests on |\n|---|---|---|\n"
	good := head + "| `Old` | deprecated | use `New` |\n| `New` | stable | x |\n"
	if got := statusTableProblems("intro\n\n"+good+"\nafter\n", cat); len(got) != 0 {
		t.Fatalf("a correct table reported problems: %v", got)
	}
	for _, tc := range []struct{ name, section, want string }{
		{"deprecated row names no successor", head + "| `Old` | deprecated | use something |\n| `New` | stable | x |\n", "does not name `New`"},
		{"deprecated row names the wrong successor", head + "| `Old` | deprecated | use `Other` |\n| `New` | stable | x |\n", "does not name `New`"},
		{"status word differs", head + "| `Old` | deprecated | use `New` |\n| `New` | experimental | x |\n", "opposite of the truth"},
		{"missing row", head + "| `Old` | deprecated | use `New` |\n", "has no row for it"},
		{"extra row", good + "| `Gone` | stable | x |\n", "which registry.Components does not hold"},
		{"duplicate row", good + "| `New` | stable | again |\n", "one row per Component"},
		{"no table at all", "no table here\n", "has no Component status table"},
		{"row moved into a code fence", head + "| `Old` | deprecated | use `New` |\n\n```\n| `New` | stable | x |\n```\n", "New is registered (stable)"},
		{"header deleted", "| `Old` | deprecated | use `New` |\n| `New` | stable | x |\n", "has no Component status table"},
		{"status word with a suffix", head + "| `Old` | deprecated | use `New` |\n| `New` | stable-ish | x |\n", "is not `| `Type` |"},
		{"row detached from the table by a blank line", head + "| `Old` | deprecated | use `New` |\n\n| `New` | stable | x |\n", "no row for it"},
		{"a second table", good + "\n" + good, "two Component status tables"},
		{"no separator row", "| Component | Status | What that rests on |\n| `Old` | deprecated | use `New` |\n| `New` | stable | x |\n", "separator row"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.Join(statusTableProblems(tc.section, cat), "\n")
			if !strings.Contains(got, tc.want) {
				t.Errorf("want a problem containing %q, got %q", tc.want, got)
			}
		})
	}
}
