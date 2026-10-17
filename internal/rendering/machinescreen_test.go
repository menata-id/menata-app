package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"menata.app/internal/data"
	"menata.app/internal/domain"
)

func TestStatusOptionToneFollowsDeclaredCategories(t *testing.T) {
	m := &domain.Machine{StatusCategories: &domain.StatusCategories{Field: "fld_status", Values: map[string]domain.StatusCategory{
		"open": domain.StatusActive, "late": domain.StatusAttention, "closed": domain.StatusDone, "dropped": domain.StatusStopped,
	}}}
	for option, want := range map[string]domain.BadgeTone{
		"open": domain.ToneInfo, "late": domain.ToneWarn, "closed": domain.ToneGood, "dropped": domain.ToneBad, "other": domain.ToneNeutral,
	} {
		if got := statusOptionTone(m, "fld_status", option); got != want {
			t.Errorf("%s: tone %q, want %q", option, got, want)
		}
	}
	if got := statusOptionTone(m, "fld_other", "open"); got != domain.ToneNeutral {
		t.Errorf("a category belongs to its own Field only, got %q", got)
	}
	if got := statusOptionTone(&domain.Machine{}, "fld_status", "open"); got != domain.ToneNeutral {
		t.Errorf("no categories declared must stay neutral, got %q", got)
	}
}

func TestMachineSchemaIsShownToAdminsOnly(t *testing.T) {
	if !showMachineSchema(domain.Actor{WorkspaceRole: "admin"}) || showMachineSchema(domain.Actor{WorkspaceRole: "member"}) {
		t.Fatal("the Machine id and Field schema table are for a Workspace admin only")
	}
}

func TestMachineBody_recordsComeBeforeTheFieldSchema(t *testing.T) {
	m := docMachine()
	records := []*data.Record{{ID: "doc_1", Values: map[string]any{"fld_title": "Kontrak A"}}}
	var buf bytes.Buffer
	if err := MachineBody(m, m.Views[1], records, nil, nil, nil, nil, domain.Actor{ID: "usr_ana", WorkspaceRole: "admin"}, time.Now()).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	got := buf.String()
	rec, fields := strings.Index(got, "Records ("), strings.Index(got, ">Fields<")
	if rec < 0 || fields < 0 || rec > fields {
		t.Fatalf("records at %d, schema at %d: an admin's schema table must sit below the records", rec, fields)
	}
}
