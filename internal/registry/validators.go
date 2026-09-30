package registry

import (
	"fmt"

	"menata.app/internal/domain"
)

// The three per-Service contract validators, moved here from internal/metadata/validate.go on
// 2026-09-30 so a Service's name and its contract live in one place (007 §14).
//
// **Moved verbatim, on purpose.** Each was already a pure function over `*domain.Machine`,
// `domain.Event` and the Machine's own Fields, with an identical signature -- which is what made the
// Service seam the right one to unify first (audit §5.2: four members, fewest consumers). Not one
// message is reworded: a metadata author's error text is part of the interface, and behaviour-preserving
// is what lets internal/metadata's existing tests stand as the proof of the move rather than needing new
// ones written to match a rewrite.
//
// Their one shared helper, `violatesOptions`, did not come with them -- it has eight other callers in
// internal/metadata. It became `domain.Field.ViolatesOptions` instead, because it asks a question about a
// Field and copying it across the plane boundary would have been 001 #8 over a two-line predicate.

// validateRollup checks everything a rollup declaration can be checked against from inside its own
// Machine file: the parent reference it writes through, and that the child values it watches for
// are really values the watched Field can hold. The other half -- that target_field exists on the
// *parent* Machine and that set/default are among its options -- needs both Machines loaded, so it
// lives in application.go beside validateRelationTargets.
// validateComposite is composite_signed_document's own same-file half (Stage C, 2026-09-28): the three
// Fields it names, and that the one on *this* Machine is a reference to the record being composited.
//
// Whether source_field and target_field exist at all is a cross-Machine question -- they live on the
// parent -- so it is answered by validateCompositeTargets once every Machine is loaded, the same split
// validateRollup/validateRollupTargets already uses for exactly the same reason.
//
// Each of these is silent at runtime if it loads: a missing parent_field composites nothing and logs a
// line nobody reads, and a target_field naming no real Field stores a key on the parent that no screen
// ever offers for download.
func validateComposite(m *domain.Machine, e domain.Event, fieldsByID map[string]domain.Field) []string {
	if e.Then.Composite == nil {
		return []string{fmt.Sprintf("event %q: then.service %q requires parent_field/source_field/target_field", e.ID, e.Then.Name)}
	}
	c := *e.Then.Composite

	var issues []string
	parentField, ok := fieldsByID[c.ParentField]
	if !ok {
		issues = append(issues, fmt.Sprintf("event %q: then.parent_field %q is not a field of machine %q", e.ID, c.ParentField, m.ID))
	} else if !parentField.IsReference() {
		issues = append(issues, fmt.Sprintf("event %q: then.parent_field %q must reference the machine being composited (a relation or person field), got %q", e.ID, c.ParentField, parentField.Type))
	}
	for _, f := range []struct{ key, value string }{{"source_field", c.SourceField}, {"target_field", c.TargetField}} {
		if !domain.FieldIDPattern.MatchString(f.value) {
			issues = append(issues, fmt.Sprintf("event %q: then.%s %q must match %s", e.ID, f.key, f.value, domain.FieldIDPattern.String()))
		}
	}
	if c.SourceField != "" && c.SourceField == c.TargetField {
		issues = append(issues, fmt.Sprintf("event %q: then.source_field and then.target_field are both %q -- compositing always starts from the original, so writing the result back over it would make every run composite onto the previous output", e.ID, c.SourceField))
	}
	return issues
}

func validateRollup(m *domain.Machine, e domain.Event, fieldsByID map[string]domain.Field) []string {
	var issues []string

	if e.Then.Rollup == nil {
		return append(issues, fmt.Sprintf("event %q: then.service %q requires parent_field/target_field", e.ID, e.Then.Name))
	}
	r := *e.Then.Rollup

	parentField, ok := fieldsByID[r.ParentField]
	if !ok {
		issues = append(issues, fmt.Sprintf("event %q: then.parent_field %q is not a field of machine %q", e.ID, r.ParentField, m.ID))
	} else if !parentField.IsReference() {
		issues = append(issues, fmt.Sprintf("event %q: then.parent_field %q must reference the parent machine (a relation or person field), got %q", e.ID, r.ParentField, parentField.Type))
	}
	if !domain.FieldIDPattern.MatchString(r.TargetField) {
		issues = append(issues, fmt.Sprintf("event %q: then.target_field %q must match %s", e.ID, r.TargetField, domain.FieldIDPattern.String()))
	}
	if r.Default == "" {
		issues = append(issues, fmt.Sprintf("event %q: then.default is required -- it is what the parent becomes when neither rule fires, including when it has no children yet", e.ID))
	}
	if r.AnyValue == "" && r.AllValue == "" {
		issues = append(issues, fmt.Sprintf("event %q: a rollup declaring neither any nor all can only ever write then.default", e.ID))
	}

	// The values watched for are values of the Event's own on: field -- that is the field whose
	// change triggers this rollup, and whose value every sibling is then read for.
	if onField, onExists := fieldsByID[e.On]; onExists {
		for _, v := range []struct{ key, value string }{{"any.value", r.AnyValue}, {"all.value", r.AllValue}} {
			if v.value != "" && onField.ViolatesOptions(v.value) {
				issues = append(issues, fmt.Sprintf("event %q: then.%s %q is not one of field %q's options %v", e.ID, v.key, v.value, e.On, onField.Options))
			}
		}
	}

	return issues
}

// validateNotify checks a send_notification declaration (Flow 2 gap study Tahap 6): recipient_field
// must be a real Field of this Machine -- no cross-record resolution exists yet (domain.Notify's
// own doc comment), so unlike validateRollup there is no second, cross-Machine pass here -- and
// preference_key must be one of the closed set, each naming a real column on credentials.
func validateNotify(m *domain.Machine, e domain.Event, fieldsByID map[string]domain.Field) []string {
	var issues []string

	if e.Then.Notify == nil {
		return append(issues, fmt.Sprintf("event %q: then.service %q requires recipient_field/preference_key", e.ID, e.Then.Name))
	}
	n := *e.Then.Notify

	if _, ok := fieldsByID[n.RecipientField]; !ok {
		issues = append(issues, fmt.Sprintf("event %q: then.recipient_field %q is not a field of machine %q", e.ID, n.RecipientField, m.ID))
	}
	if !domain.KnownNotificationPreferenceKeys[n.PreferenceKey] {
		issues = append(issues, fmt.Sprintf("event %q: then.preference_key %q is not a preference this runtime knows", e.ID, n.PreferenceKey))
	}

	return issues
}
