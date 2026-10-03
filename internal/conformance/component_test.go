package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"menata.app/internal/domain"
	"menata.app/internal/registry"
)

// TestComponentRegistryAndRenderersAgree binds the two halves of 007 §14's chain that cannot live in one
// package.
//
// `internal/registry` may import `internal/domain` and nothing else, so it holds a Component's identity and
// contract while `internal/rendering` holds the templ function that draws it. The contract names its renderer
// as a **string**, which is exactly as weak as it sounds -- so this is the only thing keeping the name
// honest, the same role `TestServiceRegistryAndExecutorsAgree` plays for Services.
//
// Both directions, because each failure is silent in a different way. A contract naming a renderer that does
// not exist is a registered Component nothing can draw. A registered Component nothing *calls* is the
// shape-before-need that put three zero-caller layout primitives in this tree a day earlier, one of which
// had already leaked a utility class into the shipped CSS bundle.
func TestComponentRegistryAndRenderersAgree(t *testing.T) {
	rendering := renderingSource(t)

	if len(registry.Components) == 0 {
		t.Fatal("registry.Components is empty -- this gate would pass by measuring nothing")
	}
	for typ, c := range registry.Components {
		if c.Contract.Type != typ {
			t.Errorf("registry.Components[%q] carries a contract whose own Type is %q -- the key and the identity must agree", typ, c.Contract.Type)
		}
		if c.Contract.Renderer == "" {
			t.Errorf("%s declares no renderer", typ)
			continue
		}
		if !strings.Contains(rendering, "templ "+c.Contract.Renderer+"(") {
			t.Errorf("%s's contract names renderer %q and no internal/rendering/*.templ declares `templ %s(...)` -- a registered Component nothing can draw", typ, c.Contract.Renderer, c.Contract.Renderer)
		}
		if !strings.Contains(rendering, "@"+c.Contract.Renderer+"(") {
			t.Errorf("%s's renderer %q has no caller -- a Component arrives with the uses it replaces, or it is a name with nothing behind it (007 §34)", typ, c.Contract.Renderer)
		}
		if c.Validate == nil {
			t.Errorf("%s declares no validator -- §14's chain runs type → contract → validator, and a contract nothing checks is a comment", typ)
		}
	}

	// And the tone vocabulary, held the same both-ends way the Layout primitives' is: a tone the renderer
	// cannot draw renders as no colour at all, and a tone nothing names is one more class in the shipped
	// bundle for nothing. `ToneMuted` is the member that proves the second half is worth checking -- it
	// exists for a measured reason (52 of 65 inference rows are correctly "not applicable", and drawing them
	// at full weight buried the two that mattered), not for symmetry.
	for tone := range domain.KnownBadgeTones {
		name := "domain.Tone" + strings.ToUpper(string(tone)[:1]) + string(tone)[1:]
		if !strings.Contains(rendering, name) {
			t.Errorf("domain.KnownBadgeTones declares %q and no .templ names %s -- a tone no renderer draws is a declaration that silently renders no colour", tone, name)
		}
	}
}

// TestRegisteredComponentsStayBounded is the gate 007 §12.3 asks for by name: *"A Component MUST expose a
// bounded semantic contract. It MUST NOT become 'generic' merely by accepting arbitrary properties or
// silently performing arbitrary data access."*
//
// **Boundedness is a property of the renderer's signature, not of any one call**, which is why this reads the
// signature rather than the call sites. A Component declaring no data requirements may take its declared
// inputs and nothing else -- no `*data.Record` to find a value in, no `any` to parse, no `time.Time` to
// compare against, no `*domain.Machine` to consult.
//
// Measured against the thing it exists to have caught: `slaBadgePill(v any)` drew a status badge while
// parsing a date out of its argument and calling `experience.EvaluateSLA(due, time.Now())`. Every one of
// those three -- the `any`, the parse, the clock -- is invisible to a reviewer reading the call site, and all
// three are refused by a signature check.
func TestRegisteredComponentsStayBounded(t *testing.T) {
	src := renderingSource(t)

	// A parameter type that means "I will go and find the data myself".
	unbounded := map[string]string{
		"any":               "an untyped value the Component would have to interpret",
		"*data.Record":      "a record the Component would have to pick a Field out of (007 §4.4: Composition resolves, a Page renders)",
		"*domain.Machine":   "a Machine definition, which is an Application's vocabulary and not a Component's input",
		"time.Time":         "the clock, which makes the Component's output depend on when it ran (007 §4.6, a MUST)",
		"context.Context":   "ctx, through which any ambient dependency can be reached",
		"RelationOptions":   "a lookup table the Component would resolve names through",
		"[]*data.Record":    "a record set the Component would have to select from",
		"*domain.Workspace": "the installed Workspace, which is ambient state rather than a declared input",
	}

	for typ, c := range registry.Components {
		if len(c.Contract.DataRequirements) > 0 {
			// A Component that declares data requirements is making a different promise, and checking it
			// needs the Dataset ids rather than the signature. None exists yet; when one does, this branch
			// is where its check goes, and leaving it empty-but-named is better than a silent skip.
			t.Logf("%s declares data requirements %v -- this gate checks signatures only, so that promise is unchecked", typ, c.Contract.DataRequirements)
			continue
		}
		sig := rendererSignature(src, c.Contract.Renderer)
		if sig == "" {
			continue // TestComponentRegistryAndRenderersAgree reports the missing renderer
		}
		for bad, why := range unbounded {
			if regexp.MustCompile(`(?:^|[\s,(\[])` + regexp.QuoteMeta(bad) + `(?:[\s,)]|$)`).MatchString(sig) {
				t.Errorf("%s declares no data requirements, but its renderer takes %s (%s): `%s`. Resolve the value before it reaches the Component, or declare the requirement", typ, bad, why, sig)
			}
		}
		// And the positive half: every declared input must actually be a parameter. A contract listing an
		// input the renderer does not take is a contract describing a different function.
		for _, in := range c.Contract.Inputs {
			if !strings.Contains(sig, in.Name) {
				t.Errorf("%s declares input %q and its renderer's signature does not mention it: `%s`", typ, in.Name, sig)
			}
		}
	}
}

// TestRenderingDoesNotReadTheClock holds 007 §4.6 Determinism ("identical input MUST produce identical
// output") against the one way this tree has actually broken it.
//
// A renderer calling `time.Now()` produces different output from the same inputs, and the failure is
// invisible: the page renders, and only a byte-level comparison across a date boundary -- or across
// midnight -- disagrees. Two sites did it (`appshell.templ`'s `slaBadgePill` and `approvalinbox.templ`'s
// `parseSLA`), both evaluating a business rule `internal/composition` was already evaluating correctly four
// times over with an injected `now`.
//
// `time.Time` as a *parameter* is fine and is the fix: the two generic field loops take `now` from their
// handler. What is forbidden is reaching for the clock.
func TestRenderingDoesNotReadTheClock(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}
	comment := regexp.MustCompile(`(?m)^[\t ]*//.*$`)
	checked := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		checked++
		// Comments stripped: the two files that used to do this explain what they used to do.
		code := comment.ReplaceAllString(string(b), "")
		for _, call := range []string{"time.Now()", "time.Since(", "time.Until("} {
			if strings.Contains(code, call) {
				t.Errorf("internal/rendering/%s calls %s -- a renderer whose output depends on when it ran breaks 007 §4.6 Determinism, a MUST. Take the time as a parameter, or resolve the value in internal/composition where `now` already is one", e.Name(), call)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no .templ files read -- this gate would pass by measuring nothing")
	}
}

// renderingSource is every .templ file concatenated. Shared by the gates above, which all ask "does
// internal/rendering contain this" rather than "which file".
func renderingSource(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}
	var b strings.Builder
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		b.Write(body)
	}
	if b.Len() == 0 {
		t.Fatal("internal/rendering has no .templ files")
	}
	return b.String()
}

// rendererSignature returns the parameter list of `templ <name>(...)`, or "" when there is none.
func rendererSignature(src, name string) string {
	m := regexp.MustCompile(`templ ` + regexp.QuoteMeta(name) + `\(([^)]*)\)`).FindStringSubmatch(src)
	if m == nil {
		return ""
	}
	return m[1]
}
