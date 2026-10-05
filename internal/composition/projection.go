package composition

import (
	"time"

	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// RecordCards projects every record of a cards or board View (domain.ViewCards, domain.ViewBoard) through m's own
// card_fields, so the renderer receives a display-ready list and never resolves a value itself.
// A board draws each record as a card, so it needs the same projection. Returns nil for any other View kind, the same "the caller supplies only what this arrangement
// needs" shape Loader.BoardColumns already has.
//
// This is the consumer Projection was missing. Its only previous one (PendingApprovalCard)
// projects a list whose role-compatible Fields hold the same value on every row, so the primitive
// could run without any of its output varying; here the projection is the card.
func RecordCards(m *domain.Machine, v domain.View, records []*data.Record, relations rendering.RelationOptions) []rendering.RecordCard {
	if t := v.EffectiveType(); t != domain.ViewCards && t != domain.ViewBoard {
		return nil
	}
	cards := make([]rendering.RecordCard, 0, len(records))
	for _, r := range records {
		cards = append(cards, rendering.RecordCard{Record: r, Fields: ProjectCardFields(m, r, relations)})
	}
	return cards
}

// ProjectCardFields resolves m's own card_fields (domain.CardField, 007 §7.6 Projection)
// against record r's stored values into the display-ready rendering.ProjectedField list a
// composed card can render generically -- the same "composition resolves, rendering displays"
// split PendingApprovalCard's other fields (Submitter, Mode, ...) already follow. This is the
// composable-runtime kajian's Fase 1 pilot: a composed card's own field list becomes metadata
// (card_fields:), not a hardcoded struct + .templ edit.
//
// A CardField naming a Field that doesn't actually exist on m (stale metadata after a Field was
// renamed/removed) is skipped silently rather than erroring -- internal/metadata.Validate already
// rejects this at load time, so a live record only hits this path if metadata changed underneath
// an already-running process; skipping degrades the card gracefully instead of 500ing it.
func ProjectCardFields(m *domain.Machine, r *data.Record, relations rendering.RelationOptions) []rendering.ProjectedField {
	var out []rendering.ProjectedField
	for _, cf := range m.CardFields {
		f, ok := m.FieldByID(cf.Field)
		if !ok {
			continue
		}
		out = append(out, rendering.ProjectedField{
			Label:   f.Name,
			Role:    string(cf.Role),
			Display: projectedFieldDisplay(f, r.Values[cf.Field], relations),
		})
	}
	return out
}

// projectedFieldDisplay resolves one Field's stored value to a display string, the same shape
// RecordDetailView's own read-only field dispatch already uses (internal/rendering/detail.templ):
// a reference field resolves through relations, a date field formats the same way buildInbox
// already formats SubmittedAt ("2 Jan 2006"), everything else is DisplayString as-is.
func projectedFieldDisplay(f domain.Field, value any, relations rendering.RelationOptions) string {
	if f.IsReference() {
		return rendering.RelationLabel(relations, f.RelatedMachine, DisplayString(value))
	}
	if f.Type == domain.FieldTypeDate {
		if due, err := time.Parse("2006-01-02", DisplayString(value)); err == nil {
			return due.Format("2 Jan 2006")
		}
	}
	return DisplayString(value)
}

// ProjectedByRole is ProjectCardFields keyed by role, for a screen that renders a record's own shape
// in its *own* markup rather than as a card -- a table cell, a calendar entry, a list row.
//
// It exists because Projection and cards are not the same thing (007 §7.6 declares a record's display
// shape; §12 governs how a Page arranges it). The Case 19 screens each render a bespoke layout matched
// to a ui-sample mockup, and pushing them through a card component to stop them reading
// Values["fld_title"] would have been a visual change disguised as a refactor. What they needed was
// the *declaration* -- which Field is the title, which the status, which the date -- and their own
// markup around it.
//
// A role a Machine declares more than once keeps its first entry, matching the order card_fields is
// written in; a role it declares not at all is absent, and callers render nothing rather than a blank
// they cannot explain.
func ProjectedByRole(m *domain.Machine, r *data.Record, relations rendering.RelationOptions) map[string]string {
	out := map[string]string{}
	for _, pf := range ProjectCardFields(m, r, relations) {
		if _, taken := out[pf.Role]; !taken {
			out[pf.Role] = pf.Display
		}
	}
	return out
}

// FieldForRole is the *id* of the Field a Machine declares in one card_fields role, for the caller
// that needs the stored value rather than its display -- a date handed to an SLA badge, which parses
// it, where the projected "2 Jan 2006" string would be the wrong input.
//
// Reading `record.Values[FieldForRole(m, "date")]` is generic access, the pattern
// internal/conformance's own projection gate names as the target: the Field id comes from a
// declaration, not from the caller. approvalstepper.templ left that ratchet the same way, taking its
// sequence and decision from domain.Sequencing's own OrderField/StateField.
//
// Empty when the Machine declares no such role, and a caller reading Values[""] gets nil, which every
// one of them already handles as "not set".
func FieldForRole(m *domain.Machine, role domain.CardFieldRole) string {
	for _, cf := range m.CardFields {
		if cf.Role == role {
			return cf.Field
		}
	}
	return ""
}
