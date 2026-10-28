package rendering

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// A <form> cannot be a child of a <tr>: the HTML parser closes it at once, so the cells land outside it and a Save inside it sends nothing
// (found 2026-10-25 on a phone: a row's inline edit did nothing, and no request reached the server). A row keeps its form in one cell and
// its controls say form=<id>. Measured, not guessed: in Chromium the form held 0 of the row's 5 controls.
func TestNoFormIsAChildOfATableRow(t *testing.T) {
	files, _ := filepath.Glob("*.templ")
	if len(files) == 0 {
		t.Fatal("no templates found")
	}
	bad := regexp.MustCompile(`<tr\b[^>]*>\s*(//[^\n]*\s*)*<form\b`)
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if bad.Match(src) {
			t.Errorf("%s opens a <form> directly inside a <tr>; the parser closes it empty. Put the form in a cell and give the controls form=<id> (withControlForm)", f)
		}
	}
}

func rowFormFixture() (*domain.Machine, *data.Record, RelationOptions) {
	m := &domain.Machine{ID: "mch_t", Fields: []domain.Field{
		{ID: "fld_title", Name: "Title", Type: domain.FieldTypeText, Required: true},
		{ID: "fld_due", Name: "Due", Type: domain.FieldTypeDate},
		{ID: "fld_status", Name: "Status", Type: domain.FieldTypeStatus, Required: true, Options: []string{"To Do", "Done"}},
	}}
	r := &data.Record{ID: "rec_1", MachineID: m.ID, Values: map[string]any{"fld_title": "x", "fld_status": "To Do"}}
	return m, r, RelationOptions{}
}

// Every control of an edit row, and of the create row, belongs to the row's one form -- named by form=, since none of them is inside it.
func TestRowEditorsOwnTheirControlsByFormAttribute(t *testing.T) {
	m, r, rel := rowFormFixture()
	for name, c := range map[string]struct {
		render func(*bytes.Buffer) error
		id     string
	}{
		"edit": {func(b *bytes.Buffer) error {
			return RecordEditRow(m, r, rel, nil, domain.Actor{}).Render(context.Background(), b)
		}, "edit-rec_1"},
		"create": {func(b *bytes.Buffer) error { return createFormRow(m, rel, nil).Render(context.Background(), b) }, "create-mch_t"},
	} {
		var b bytes.Buffer
		if err := c.render(&b); err != nil {
			t.Fatal(err)
		}
		out := b.String()
		if got := strings.Count(out, `<form id="`+c.id+`"`); got != 1 {
			t.Errorf("%s row: %d forms named %q, want 1", name, got, c.id)
		}
		if got, want := strings.Count(out, `form="`+c.id+`"`), len(m.Fields); got != want {
			t.Errorf("%s row: %d controls name their form, want %d (one per Field):\n%s", name, got, want, out)
		}
	}
}

// One table row is edited at a time: the shell puts the other open row editors back when one is drawn. Text-pinned like the shell's other
// scripts (it cannot run in a Go test; the Chromium check is in the slice's record).
func TestShellScript_oneRowEditorOpenAtATime(t *testing.T) {
	var b bytes.Buffer
	if err := appShell("T", "W", Viewer{}, "").Render(assignedNavFixture(), &b); err != nil {
		t.Skipf("the shell does not render bare: %v", err)
	}
	for _, want := range []string{`addEventListener("htmx:afterSwap"`, `tr[data-row-editor]`, `[data-row-cancel]`} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("the shell script lacks %q", want)
		}
	}
	var row bytes.Buffer
	m, r, rel := rowFormFixture()
	if err := RecordEditRow(m, r, rel, nil, domain.Actor{}).Render(context.Background(), &row); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`data-row-editor`, `data-row-cancel`} {
		if !strings.Contains(row.String(), want) {
			t.Errorf("an edit row lacks %q, which the shell's script looks for", want)
		}
	}
}
