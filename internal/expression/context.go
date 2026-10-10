package expression

import "strings"

// Context is what a Predicate may resolve besides the record in front of it.
//
// 007 §9.2 names the vocabulary, and this package implements the part of it the runtime has cases
// for:
//
//	record
//	old
//	current_user     <- built
//	today            <- built
//	now
//	parameters       <- built
//
// **`today` is built, `now` is not.** `today` arrived with the ordered operators and a Dataset that
// needs them (documents past their due date): a date Field compared against a date, which is all a
// day-grained value can mean. `now` has no case -- there is no datetime Field type to compare it with.
// The value is injected by the caller, never read here: the same request must give the same answer
// (007 §4.6 Determinism), and this package may not read the clock.
//
// Fields are plain strings because internal/expression may import nothing from internal/ at all
// (boundary_test.go: "a pure evaluator -- no I/O, and no dependency on any other plane"). The caller
// resolves an identity into its record id and hands the string over.
type Context struct {
	// CurrentUser is the viewing identity's own record id, or "" when a caller has none.
	CurrentUser string
	// Parameters are the request's own named values -- a record id from a route, typically.
	Parameters map[string]string
	// Today is the request's date as YYYY-MM-DD, or "" when a caller supplies none.
	Today string
}

// SentinelPrefix marks a Comparison value as a reference to the Context rather than a literal.
const SentinelPrefix = "$"

const (
	// SentinelCurrentUser is 007 §9.2's `current_user`.
	SentinelCurrentUser = "$current_user"
	// SentinelToday is 007 §9.2's `today`: the request's date, resolved by the caller.
	SentinelToday = "$today"
	// SentinelParameterPrefix is `$parameters.<name>`; the name after the dot is the key.
	SentinelParameterPrefix = "$parameters."
)

// IsSentinel reports whether a declared value references the Context.
func IsSentinel(value string) bool { return strings.HasPrefix(value, SentinelPrefix) }

// KnownSentinel reports whether value is one this runtime resolves.
//
// **This is the half that matters**, and 007 §9.2 states it as a requirement rather than a nicety:
// "Access outside this context must fail closed." A `$whatever` the runtime does not know must be a
// load error, never a literal compared against records -- because a filter silently comparing a
// Field against the string "$current_usr" matches nothing, renders an empty list, and looks exactly
// like a screen with no data. That is the same class of silent-empty failure as the /review 404,
// and it is why this is validated at load rather than discovered at runtime.
func KnownSentinel(value string) bool {
	if value == SentinelCurrentUser || value == SentinelToday {
		return true
	}
	return strings.HasPrefix(value, SentinelParameterPrefix) && len(value) > len(SentinelParameterPrefix)
}

// Resolve turns a declared value into the one to compare against. A non-sentinel is returned
// unchanged, so every existing declaration keeps its exact meaning.
//
// An unresolvable sentinel -- `$current_user` with no viewer, `$parameters.x` with no such
// parameter -- returns ok=false rather than an empty string. The distinction is the same one
// domain.Resolution draws: a filter that silently compares against "" would match records whose
// Field is absent, which is a different query than the one declared, and it would do it quietly.
func (c Context) Resolve(value string) (string, bool) {
	if !IsSentinel(value) {
		return value, true
	}
	if value == SentinelCurrentUser {
		return c.CurrentUser, c.CurrentUser != ""
	}
	if value == SentinelToday {
		return c.Today, c.Today != ""
	}
	if name, found := strings.CutPrefix(value, SentinelParameterPrefix); found {
		got, ok := c.Parameters[name]
		return got, ok && got != ""
	}
	return "", false
}
