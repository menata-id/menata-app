package composition

import (
	"time"

	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// RecordCards projects every record of a cards View (domain.ViewCards) through m's own
// card_fields, so the renderer receives a display-ready list and never resolves a value itself.
// Returns nil for any other View kind, the same "the caller supplies only what this arrangement
// needs" shape Loader.BoardColumns already has.
//
// This is the consumer Projection was missing. Its only previous one (PendingApprovalCard)
// projects a list whose role-compatible Fields hold the same value on every row, so the primitive
// could run without any of its output varying; here the projection is the card.
func RecordCards(m *domain.Machine, v domain.View, records []*data.Record, relations rendering.RelationOptions) []rendering.RecordCard {
	if v.EffectiveType() != domain.ViewCards {
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
