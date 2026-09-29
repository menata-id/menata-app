package expression

import "testing"

// TestKnownSentinel_failsClosed is the assertion 007 §9.2 asks for by name: "Access outside this
// context must fail closed."
//
// A sentinel the runtime does not know must be rejected, not compared as a literal. The failure it
// prevents is quiet: a filter comparing a Field against the string "$current_usr" matches nothing,
// renders an empty list, and is indistinguishable from a screen that legitimately has no rows.
func TestKnownSentinel_failsClosed(t *testing.T) {
	known := []string{"$current_user", "$parameters.document", "$parameters.x"}
	for _, v := range known {
		if !KnownSentinel(v) {
			t.Errorf("KnownSentinel(%q) = false, want true", v)
		}
	}

	unknown := []string{
		"$current_usr",     // a typo -- the case this exists for
		"$currentuser",     //
		"$parameters.",     // no name after the dot
		"$parameters",      // the prefix alone names nothing
		"$today",           // named by §9.2, deliberately not built
		"$now",             //
		"$record.fld_x",    // §9.2 names `record`; not built either
		"$",                //
		"$anything_at_all", //
	}
	for _, v := range unknown {
		if KnownSentinel(v) {
			t.Errorf("KnownSentinel(%q) = true -- an unknown sentinel must fail closed, or it becomes "+
				"a literal that silently matches nothing", v)
		}
	}
}

// TestResolve_leavesOrdinaryValuesAlone: every declaration that exists today compares against a
// literal, and none of them may change meaning.
func TestResolve_leavesOrdinaryValuesAlone(t *testing.T) {
	c := Context{CurrentUser: "usr_1"}
	for _, v := range []string{"done", "pending", "", "10", "a value with $ inside"} {
		got, ok := c.Resolve(v)
		if !ok || got != v {
			t.Errorf("Resolve(%q) = %q, %v; want the value unchanged", v, got, ok)
		}
	}
}

func TestResolve_sentinels(t *testing.T) {
	c := Context{CurrentUser: "usr_1", Parameters: map[string]string{"document": "rec_9"}}

	if got, ok := c.Resolve("$current_user"); !ok || got != "usr_1" {
		t.Errorf("Resolve($current_user) = %q, %v; want usr_1, true", got, ok)
	}
	if got, ok := c.Resolve("$parameters.document"); !ok || got != "rec_9" {
		t.Errorf("Resolve($parameters.document) = %q, %v; want rec_9, true", got, ok)
	}
}

// TestResolve_unresolvableIsNotAnEmptyString: a filter comparing against "" is a *different query*
// than the one declared -- it matches records whose Field is absent -- and it would do it quietly.
// The same distinction domain.Resolution draws between "undeclared" and "input unavailable".
func TestResolve_unresolvableIsNotAnEmptyString(t *testing.T) {
	noViewer := Context{}
	if got, ok := noViewer.Resolve("$current_user"); ok {
		t.Errorf("Resolve($current_user) with no viewer = %q, true; want not-ok, so the caller cannot "+
			"silently run a filter against the empty string", got)
	}

	c := Context{CurrentUser: "usr_1", Parameters: map[string]string{"document": ""}}
	if _, ok := c.Resolve("$parameters.document"); ok {
		t.Error("an empty parameter resolved successfully; an absent value must fail rather than filter on \"\"")
	}
	if _, ok := c.Resolve("$parameters.missing"); ok {
		t.Error("a missing parameter resolved successfully")
	}
}
