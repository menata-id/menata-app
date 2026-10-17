package behavior

import "menata.app/internal/domain"

// RollupValue decides what a parent record's target Field becomes, given the values its children
// currently hold. Pure -- no I/O, no database, no clock -- the same posture CheckConstraints and
// MatchedEvents already take; internal/web performs the read of the children and the write of the
// result.
//
// Priority is the substance of the rule, not an implementation detail. AnyValue is checked first
// so a single rejection decides the parent the moment it happens, rather than waiting for the
// remaining children to make up their minds -- the discriminator half of the parent-rollup
// pattern. AllValue applies only when every child holds it. Everything else, including a parent
// with no children at all, is Default -- and when no Default is declared, false: the parent keeps
// whatever it holds, so another writer of the same Field (a schedule marking it overdue) is not
// overwritten by a rollup with nothing to say (K-15, menata-app-document
// audits/2026-10-17-rencana-dokter-kecil-kontrak-produksi.md).
//
// An empty AnyValue or AllValue disables that arm rather than matching children that hold no
// value, so a rollup declaring only one of the two behaves as written.
func RollupValue(r domain.Rollup, childValues []string) (string, bool) {
	if r.AnyValue != "" {
		for _, v := range childValues {
			if v == r.AnyValue {
				return r.AnySet, true
			}
		}
	}
	if r.AllValue != "" && len(childValues) > 0 {
		all := true
		for _, v := range childValues {
			if v != r.AllValue {
				all = false
				break
			}
		}
		if all {
			return r.AllSet, true
		}
	}
	return r.Default, r.Default != ""
}
