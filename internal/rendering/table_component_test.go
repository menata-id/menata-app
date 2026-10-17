package rendering

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"menata.app/internal/domain"
)

func cellText(s string) templ.Component {
	return templ.ComponentFunc(func(_ context.Context, w io.Writer) error { _, err := w.Write([]byte(s)); return err })
}

func renderTable(t *testing.T, label, empty, headers, aligns string, cells ...string) string {
	t.Helper()
	cs := make([]templ.Component, len(cells))
	for i, c := range cells {
		cs[i] = cellText(c)
	}
	var b bytes.Buffer
	if err := table(label, empty, headers, aligns, Truncation{}, cs).Render(assignedNavFixture(), &b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestTable_drawsTheHeaderRowAndOneRowPerRecord(t *testing.T) {
	html := renderTable(t, "Tickets", "", "Ticket\nStatus\nDue", "start,start,end", "a1", "a2", "a3", "b1", "b2", "b3")
	if !strings.Contains(html, `aria-label="Tickets"`) {
		t.Errorf("the table has no accessible name: %s", html)
	}
	for _, h := range []string{">Ticket</th>", ">Status</th>", ">Due</th>"} {
		if !strings.Contains(html, h) {
			t.Errorf("no header %s", h)
		}
	}
	body := html[strings.Index(html, "<tbody>"):]
	if rows := strings.Count(body, "<tr>"); rows != 2 {
		t.Errorf("%d body rows, want 2", rows)
	}
	// Row-major: the first row holds a1..a3 in that order, then the second b1..b3.
	if !(strings.Index(body, "a1") < strings.Index(body, "a3") && strings.Index(body, "a3") < strings.Index(body, "b1") && strings.Index(body, "b1") < strings.Index(body, "b3")) {
		t.Errorf("cells are not row-major:\n%s", body)
	}
	// The third column's header and each of its cells sit on the end edge; the first column's on the start edge.
	if strings.Count(html, "text-right") != 3 {
		t.Errorf("%d end-aligned cells (header and two rows), want 3", strings.Count(html, "text-right"))
	}
}

func TestTable_withNoRecordsSaysSoOrKeepsItsHeader(t *testing.T) {
	said := renderTable(t, "Tickets", "No tickets yet.", "Ticket", "start")
	if !strings.Contains(said, "No tickets yet.") || strings.Contains(said, "<table") {
		t.Errorf("an empty table with an empty text should say it and draw no table: %s", said)
	}
	bare := renderTable(t, "Tickets", "", "Ticket", "start")
	if !strings.Contains(bare, "<table") || !strings.Contains(bare, ">Ticket</th>") || strings.Contains(bare, "<td") {
		t.Errorf("an empty table with no text keeps its header and has no cells: %s", bare)
	}
}

func TestTable_escapesAHeaderAndNeverPanicsOnShortProps(t *testing.T) {
	html := renderTable(t, "T", "", `A<b>`+"\nB", "", "x", "y")
	if strings.Contains(html, "<b>") || !strings.Contains(html, "A&lt;b&gt;") {
		t.Errorf("a header was not escaped: %s", html)
	}
	// A cell count that is not a whole number of rows draws only the whole rows, rather than reading past the cells.
	if got := renderTable(t, "T", "", "A\nB", "start,start", "x", "y", "z"); strings.Count(got, "<tr>") != 2 {
		t.Errorf("a ragged cell list drew %d rows, want the header and one whole row", strings.Count(got, "<tr>"))
	}
	if cols := tableColumnsFromProps("", ""); cols != nil {
		t.Errorf("no headers = %v, want nil", cols)
	}
	if cols := tableColumnsFromProps("A\nB", "end"); cols[0].Align != domain.TableAlignEnd || cols[1].Align == domain.TableAlignEnd {
		t.Errorf("a short alignment list = %+v", cols)
	}
}

// TestTableSaysWhatACutListWasCutFrom: a Table whose Dataset's bound cut it says so under the rows, with the count it was
// cut from (owner decision K2), and says nothing when the bound did not bite.
func TestTableSaysWhatACutListWasCutFrom(t *testing.T) {
	var b strings.Builder
	cell := templ.Raw("<span>a</span>")
	if err := table("Contracts", "", "Contract", "start", truncationFromProp("100/340"), []templ.Component{cell}).Render(assignedNavFixture(), &b); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "Showing the first 100 of 340") {
		t.Errorf("a cut list does not say what it was cut from: %s", b.String())
	}
	b.Reset()
	if err := table("Contracts", "", "Contract", "start", truncationFromProp(""), []templ.Component{cell}).Render(assignedNavFixture(), &b); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "Showing the first") {
		t.Errorf("a list the bound did not cut says it was: %s", b.String())
	}
}
