package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"menata.app/internal/domain"
)

func renderDataTable(t *testing.T, ctx context.Context, label string, cols []TableColumn, rows templ.Component) string {
	t.Helper()
	var buf bytes.Buffer
	if err := dataTable(label, cols).Render(templ.WithChildren(ctx, rows), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return buf.String()
}

// TestDataTable_namesItselfAndItsColumns holds what the contract forces: an accessible name on the table, a
// `scope="col"` header per column, the end edge for a figure column, and the rows exactly where the caller put them.
func TestDataTable_namesItselfAndItsColumns(t *testing.T) {
	ctx := context.Background()
	rows := templ.Raw(`<tr><td>Atlas</td><td>4</td></tr>`)
	got := renderDataTable(t, ctx, "Tasks by project", []TableColumn{{Header: "Project"}, {Header: "Total", Align: domain.TableAlignEnd}}, rows)

	for _, want := range []string{
		`aria-label="Tasks by project"`,
		`<th scope="col" class="border-b border-slate-200 px-3 py-2 text-left text-2xs font-medium tracking-wide text-slate-500 uppercase">Project</th>`,
		`<th scope="col" class="border-b border-slate-200 px-3 py-2 text-right text-2xs font-medium tracking-wide text-slate-500 uppercase">Total</th>`,
		`<tbody><tr><td>Atlas</td><td>4</td></tr></tbody>`,
		`<div class="overflow-x-auto">`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "rounded-lg") {
		t.Errorf("a table draws no frame of its own -- the caller's panel may already have one:\n%s", got)
	}
}

// TestDataTable_followsTheWorkspaceTheme proves the head reads its rule from the Theme like every other
// hand-written table did: a Workspace declaring a different surface border moves the header's.
func TestDataTable_followsTheWorkspaceTheme(t *testing.T) {
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Theme: domain.Theme{Border: map[domain.BorderRole]domain.BorderShade{domain.BorderSurface: domain.BorderDefined}}}, "Acme", false)
	got := renderDataTable(t, ctx, "x", []TableColumn{{Header: "A"}}, templ.Raw(""))
	if strings.Contains(got, "border-slate-200") || !strings.Contains(got, "border-slate-300") {
		t.Errorf("header rule did not follow theme.border.surface:\n%s", got)
	}
}

func TestRecordTableColumns_oneNamedPerFieldThenAnUnnamedActionsColumn(t *testing.T) {
	cols := recordTableColumns([]domain.Field{{Name: "Title"}, {Name: "Due"}})
	if len(cols) != 3 || cols[0].Header != "Title" || cols[1].Header != "Due" || cols[2].Header != "" {
		t.Errorf("columns = %+v", cols)
	}
}
