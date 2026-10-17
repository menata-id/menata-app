package rendering

import (
	"testing"

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
