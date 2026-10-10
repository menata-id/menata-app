package conformance

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"menata.app/internal/domain"
	"menata.app/internal/ir"
	"menata.app/internal/registry"
)

// The two gates here close K17: what a `page:` may write is decided by `ir.allowedProps`, and three things
// had drifted from it because each list was individually valid -- `columns` and `section` were drawn by the
// renderer and declared kinds, `StatusBadge.size` was a contract input, and none of the three could be
// written in YAML. The guide's property table was a fourth hand-kept copy.

// TestEveryDrawnLayoutKindAndContractInputIsWritable asserts the first half: every layout kind the vocabulary
// closes has an `allowedProps` entry (even an empty one, as `panel` has), and every input a registered
// Component's contract declares is a key a node of it may carry.
func TestEveryDrawnLayoutKindAndContractInputIsWritable(t *testing.T) {
	for kind := range domain.KnownLayoutKinds {
		if ir.PermittedProps(ir.NodeLayout, string(kind)) == nil && kind != domain.LayoutPanel {
			t.Errorf("layout kind %q is declared and drawn but no node of it may carry a property -- add layout/%s to ir.allowedProps", kind, kind)
		}
	}
	for typ, c := range registry.Components {
		permitted := ir.PermittedProps(ir.NodeComponent, string(typ))
		for _, in := range c.Contract.Inputs {
			if !slices.Contains(permitted, in.Name) {
				t.Errorf("Component %s declares input %q but a page node of it may not carry it -- add it to ir.allowedProps", typ, in.Name)
			}
		}
	}
}

// TestGuideLayoutTableIsTheAllowedPropsList asserts the second: writing-guide.md §12.1a states every layout
// kind with its properties exactly as `ir.PermittedProps` reports them (kinds and keys sorted), so adding a
// property without telling an author fails here rather than being found by an author who cannot write it.
func TestGuideLayoutTableIsTheAllowedPropsList(t *testing.T) {
	guide := readFile(t, repoRoot()+"/writing-guide.md")

	var kinds []string
	for k := range domain.KnownLayoutKinds {
		kinds = append(kinds, string(k))
	}
	slices.Sort(kinds)

	var names, props []string
	for _, k := range kinds {
		names = append(names, "`"+k+"`")
		keys := ir.PermittedProps(ir.NodeLayout, k)
		if len(keys) == 0 {
			props = append(props, fmt.Sprintf("`%s` none", k))
			continue
		}
		quoted := make([]string, len(keys))
		for i, key := range keys {
			quoted[i] = "`" + key + "`"
		}
		props = append(props, fmt.Sprintf("`%s` %s", k, strings.Join(quoted, ", ")))
	}
	for _, want := range []string{
		"| `layout:` | " + strings.Join(names, ", ") + " |",
		"Properties: " + strings.Join(props, "; ") + " |",
	} {
		if !strings.Contains(guide, want) {
			t.Errorf("writing-guide.md §12.1a does not contain %q -- regenerate the layout row from ir.PermittedProps", want)
		}
	}
}
