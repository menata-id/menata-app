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
	"Avatar":  "drawn by bespoke screens only (members, assignees, the app shell) -- the contract was registered over four measured sites, and no `page:` declares one yet",
	"Callout": "drawn by the account screens only -- registered over six measured sites (2026-10-12); no declared screen shows a message today (writing-guide §12.1a)",
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
	var walkWritten func(n domain.PageNode)
	walkWritten = func(n domain.PageNode) {
		if n.Kind == "component" {
			written[n.Type]++
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
				walkWritten(*item.Page)
				tree, err := ir.Lower(*item.Page, metadata.PlaceholderResolver(app.AllNavigation, datasets))
				if err != nil {
					t.Fatalf("%s/%s/%s does not lower: %v (TestEveryInstalledPageLowersAndValidates reports this too)", slug, app.ID, item.ID, err)
				}
				walkLowered(tree)
			}
		}
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
// **Blind spot, stated:** "installed" is the Workspace manifests under metadata/workspaces/, the way every sweep here
// reads them; a page in the template library that no Workspace has installed depends on nothing. And the count is of
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

// capabilitiesComponentRows is the text of every capabilities.md row whose Kind says it is a registered Component.
func capabilitiesComponentRows(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(), "capabilities.md"))
	if err != nil {
		t.Fatal(err)
	}
	var rows []string
	for _, line := range strings.Split(string(b), "\n") {
		if cells := tableCells(line); len(cells) >= 3 && strings.HasPrefix(cells[1], "registered Component") {
			rows = append(rows, line)
		}
	}
	return rows
}

// TestEveryRegisteredComponentIsInCapabilities holds capabilities.md's "Shared rendering components" table to the
// catalogue: each registered Component is named, by its type or by its renderer in backticks, in a row whose Kind says
// it is a registered Component. `TestCapabilitiesComponentsTableMatchesTempl` holds the first column to the .templ
// files; neither reads Kind, so a Component registered and left described as a "shared fragment" passed both.
//
// **Presence, not accuracy:** it reads the whole row, not one cell, because the Button row names its registered
// renderer (`requestButton`) in prose while its first cell names the four-string `button`. A row can therefore
// mention a Component it does not describe. Reading it is the only check for that.
func TestEveryRegisteredComponentIsInCapabilities(t *testing.T) {
	rows := capabilitiesComponentRows(t)
	if len(rows) < 5 {
		t.Fatalf("found %d `registered Component` rows in capabilities.md -- the Kind column moved or this gate reads the wrong table", len(rows))
	}
	text := strings.Join(rows, "\n")
	for _, typ := range sortedComponentTypes() {
		c := registry.Components[domain.ComponentType(typ)].Contract
		if !strings.Contains(text, "`"+typ+"`") && !strings.Contains(text, "`"+c.Renderer+"`") {
			t.Errorf("%s is registered, and no capabilities.md row of Kind `registered Component` names `%s` or its renderer `%s` -- add the row (or the Kind) in the change that registers it", typ, typ, c.Renderer)
		}
	}
}

// statusTableRow matches one row of the writing-guide's Component status table: `| `Type` | status | ... |`.
var statusTableRow = regexp.MustCompile("^\\| `([A-Za-z]+)` \\| (experimental|stable|deprecated)\\b")

// writingGuideSection12_1a is the text of `### 12.1a` up to the next `### ` heading: the section an author reads to
// write a `page:`.
func writingGuideSection12_1a(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(), "writing-guide.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	start := strings.Index(s, "\n### 12.1a ")
	if start < 0 {
		t.Fatal("writing-guide.md has no `### 12.1a` heading -- the section an author reads to write a page moved")
	}
	rest := s[start+1:]
	if end := strings.Index(rest[4:], "\n### "); end >= 0 {
		rest = rest[:end+4]
	}
	return rest
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
	rows := map[string][]string{} // type -> the lines that name it
	statusOf := map[string]string{}
	for _, line := range strings.Split(section, "\n") {
		if m := statusTableRow.FindStringSubmatch(line); m != nil {
			rows[m[1]] = append(rows[m[1]], line)
			statusOf[m[1]] = m[2]
		}
	}
	if len(rows) == 0 {
		return []string{"writing-guide §12.1a has no Component status table (rows shaped `| `Type` | status | ... |`) -- an author cannot see how far to rely on a Component"}
	}
	var problems []string
	types := make([]string, 0, len(catalogue))
	for typ := range catalogue {
		types = append(types, string(typ))
	}
	sort.Strings(types)
	for _, typ := range types {
		c := catalogue[domain.ComponentType(typ)].Contract
		switch lines := rows[typ]; {
		case len(lines) == 0:
			problems = append(problems, fmt.Sprintf("%s is registered (%s) and writing-guide §12.1a's status table has no row for it", typ, c.Status))
		case len(lines) > 1:
			problems = append(problems, fmt.Sprintf("%s has %d rows in writing-guide §12.1a's status table -- one row per Component", typ, len(lines)))
		case statusOf[typ] != string(c.Status):
			problems = append(problems, fmt.Sprintf("%s is %s in its contract and %s in writing-guide §12.1a's status table -- an author is told the opposite of the truth", typ, c.Status, statusOf[typ]))
		case c.Status == domain.ComponentStatusDeprecated && !strings.Contains(lines[0], "`"+string(c.ReplacedBy)+"`"):
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
// cannot: a deprecated Component, whose row must name its successor.
func TestStatusTableProblems(t *testing.T) {
	cat := map[domain.ComponentType]registry.Component{
		"Old": {Contract: domain.ComponentContract{Type: "Old", Status: domain.ComponentStatusDeprecated, ReplacedBy: "New"}},
		"New": {Contract: domain.ComponentContract{Type: "New", Status: domain.ComponentStatusStable}},
	}
	good := "| `Old` | deprecated | use `New` |\n| `New` | stable | x |\n"
	if got := statusTableProblems(good, cat); len(got) != 0 {
		t.Fatalf("a correct table reported problems: %v", got)
	}
	for _, tc := range []struct{ name, section, want string }{
		{"deprecated row names no successor", "| `Old` | deprecated | use something |\n| `New` | stable | x |\n", "does not name `New`"},
		{"deprecated row names the wrong successor", "| `Old` | deprecated | use `Other` |\n| `New` | stable | x |\n", "does not name `New`"},
		{"status word differs", "| `Old` | deprecated | use `New` |\n| `New` | experimental | x |\n", "opposite of the truth"},
		{"missing row", "| `Old` | deprecated | use `New` |\n", "has no row for it"},
		{"extra row", good + "| `Gone` | stable | x |\n", "which registry.Components does not hold"},
		{"duplicate row", good + "| `New` | stable | again |\n", "one row per Component"},
		{"no table at all", "no table here\n", "has no Component status table"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.Join(statusTableProblems(tc.section, cat), "\n")
			if !strings.Contains(got, tc.want) {
				t.Errorf("want a problem containing %q, got %q", tc.want, got)
			}
		})
	}
}
