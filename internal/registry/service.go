package registry

import (
	"fmt"

	"menata.app/internal/domain"
)

// Service is one member of the Service seam 007 §14 describes: a name, the contract it requires of the
// Event that names it, and the validator that checks that contract at load time.
//
// **It deliberately does not carry the executor, and the reason is a plane boundary rather than taste.**
// §14 sketches one seam running `type → contract → validator → resolver → renderer`, and a single Go
// package cannot hold all of it here: `internal/metadata` must import this catalogue to validate, and
// `internal/metadata` is forbidden from `internal/data`; `internal/execution` must import it to dispatch,
// and `internal/execution` is forbidden from `internal/metadata`. An executor needs the Store and the
// Mailer, so putting it here would make this package un-importable by the plane that validates.
//
// So the seam is one **catalogue of names and contracts** in a package that depends on nothing but
// `internal/domain`, with each later stage held in the plane entitled to it, and a conformance gate
// asserting the stages agree (`TestServiceRegistryAndExecutorsAgree`). That is a weaker claim than "one
// registration point" and it is the true one -- said here rather than discovered by whoever next tries to
// move `Execute` in and finds the import cycle.
//
// What this replaced: `domain.KnownServices` (a `map[string]bool`, names only) plus a four-case
// `switch e.Then.Name` in `internal/metadata/validate.go` and three more in `internal/execution`. The
// names were a closed set and the *contracts* were not catalogued anywhere -- a new Service meant finding
// six places that had to agree, guarded by a conformance test rather than by a single point of
// registration.
type Service struct {
	// Name is the string an Event's `then.service` names.
	Name string
	// Validate checks everything this Service's contract can be checked from inside one Machine file:
	// the keys it requires on `then:`, and that the values it names are ones the Fields can hold.
	// Cross-Machine checks (a rollup's target Field living on the *parent*) stay in
	// internal/metadata's application-level validators, which are the only ones holding both Machines.
	//
	// It returns issues rather than an error so a file reports every problem at once, which is the
	// posture every validator in internal/metadata already has.
	Validate func(m *domain.Machine, e domain.Event, fieldsByID map[string]domain.Field) []string
}

// Services is the closed catalogue. A name absent from it is a load error, which is the whole point of a
// static seam: extension means adding Go code and recompiling, never accepting an unknown type string
// (see this package's doc.go).
var Services = map[string]Service{
	domain.ServiceLogActivity:             {Name: domain.ServiceLogActivity, Validate: validateLogActivity},
	domain.ServiceRollupParentStatus:      {Name: domain.ServiceRollupParentStatus, Validate: validateRollup},
	domain.ServiceSendNotification:        {Name: domain.ServiceSendNotification, Validate: validateNotify},
	domain.ServiceCompositeSignedDocument: {Name: domain.ServiceCompositeSignedDocument, Validate: validateComposite},
}

// ValidateService is the one entry point a loader needs: it resolves the name and runs its contract, or
// reports that the runtime realizes no such Service.
//
// The unknown-name message stays identical to the `default:` arm it replaced, because a metadata author's
// error text is part of the interface and a refactor is not the place to reword it.
func ValidateService(m *domain.Machine, e domain.Event, fieldsByID map[string]domain.Field) []string {
	svc, known := Services[e.Then.Name]
	if !known {
		return []string{fmt.Sprintf("event %q: then.service %q is not a service this runtime realizes", e.ID, e.Then.Name)}
	}
	if svc.Validate == nil {
		return nil
	}
	return svc.Validate(m, e, fieldsByID)
}

// validateLogActivity: summary is log_activity's message, and means nothing to a rollup, which writes a
// Field rather than a sentence. Lifted verbatim from the `case domain.ServiceLogActivity:` arm it
// replaced -- behaviour-preserving by construction, which is what lets the existing metadata tests stand
// as the proof of this move.
func validateLogActivity(_ *domain.Machine, e domain.Event, _ map[string]domain.Field) []string {
	var issues []string
	if e.Then.Summary == "" {
		issues = append(issues, fmt.Sprintf("event %q: then.summary is required", e.ID))
	}
	if (e.Then.SummaryOverrideWhen == "") != (e.Then.SummaryOverride == "") {
		issues = append(issues, fmt.Sprintf("event %q: then.summary_override_when and then.summary_override must be set together or not at all", e.ID))
	}
	return issues
}
