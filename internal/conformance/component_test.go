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

	// And the tone vocabulary, held at both ends -- which are no longer the same two ends they were.
	//
	// A tone used to be drawn by a `.templ` switch naming it, so "some .templ names this tone" meant "the
	// renderer can draw it". Since 2026-10-05 a tone resolves through the Workspace's Theme to a *palette*,
	// and it is the palette the renderer draws, so the renderer's half of the check moves to the palettes. A
	// palette no arm draws renders as the fallback colour, and a palette nothing draws is a class in the
	// bundle for nothing.
	//
	// The tone's own half is not "a .templ names it" any more -- `ToneWarn` is named by no `.templ` and is
	// correct, because Composition produces it as data (`composition.toneFor`) and the Page passes `row.Tone`
	// through (007 §4.4). That is **why the old check read as satisfied for ToneWarn**: it was named by the
	// renderer's own switch, which proved the renderer could draw it and said nothing about anyone producing
	// it. Retargeting it to producers measures the thing the old wording only implied. See
	// TestEveryBadgeToneHasAProducer.
	for setName, members := range map[string][]string{
		"KnownTonePalettes":    paletteIdents(),
		"KnownAvatarSizes":     {"domain.AvatarInline", "domain.AvatarLead", "domain.AvatarCompact"},
		"KnownAvatarPresences": {"domain.AvatarPresent", "domain.AvatarPending"},
		"KnownBadgeSizes":      {"domain.BadgeRegular", "domain.BadgeCompact"},
		"KnownButtonVariants":  {"domain.ButtonPrimary", "domain.ButtonSecondary", "domain.ButtonDanger"},
	} {
		for _, ident := range members {
			if !strings.Contains(rendering, ident) {
				t.Errorf("domain.%s declares a member no .templ names (%s) -- a declared value no renderer draws silently renders the default", setName, ident)
			}
		}
	}
	// And the sets may not grow past what this gate enumerates. A range loop gives that for free; a hand-
	// written list does not, so the count is asserted instead.
	if got, want := len(domain.KnownAvatarSizes)+len(domain.KnownAvatarPresences)+len(domain.KnownBadgeSizes)+len(domain.KnownButtonVariants), 10; got != want {
		t.Errorf("the Avatar, Badge and Button parameter sets now hold %d members, this gate enumerates %d -- add the new one above with its renderer arm", got, want)
	}
}

// paletteIdents turns every declared TonePalette into the Go identifier a .templ would name it by, read out of
// the closed set rather than retyped -- so a palette added to domain and not drawn fails without anyone
// editing this.
func paletteIdents() []string {
	out := make([]string, 0, len(domain.KnownTonePalettes))
	for p := range domain.KnownTonePalettes {
		parts := strings.Split(string(p), "-")
		for i := range parts {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
		out = append(out, "domain.Palette"+strings.Join(parts, ""))
	}
	return out
}

// TestEveryBadgeToneHasAProducer is the half of the tone check that a tone resolved through Theme cannot get
// from the renderer: **who produces this value.**
//
// A tone reaches `statusBadge` two ways -- a `.templ` call site naming it, or Composition resolving it as data
// and the Page passing it through. A grep of `.templ` call sites sees only the first, and read `ToneWarn` as
// having no caller when `composition.toneFor` returns it for `input unavailable`, one of the two statuses
// `TestInstalledCastsExplainWithoutDefects` exists to catch. So this counts a tone as produced when any
// non-test, non-generated source **outside `internal/domain`** names it on a non-comment line.
//
// `internal/domain` is excluded because it is where the closed set and `DefaultTheme` live: a tone named only
// there is declared, not produced, which is the shape-before-need the old check was reaching for.
func TestEveryBadgeToneHasAProducer(t *testing.T) {
	produced := map[string][]string{}
	for tone := range domain.KnownBadgeTones {
		produced[string(tone)] = nil
	}
	root := filepath.Join(repoRoot(), "internal")
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		name := info.Name()
		if strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, "_templ.go") ||
			!(strings.HasSuffix(name, ".go") || strings.HasSuffix(name, ".templ")) {
			return nil
		}
		if rel, _ := filepath.Rel(root, path); strings.HasPrefix(rel, "domain"+string(filepath.Separator)) {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var code strings.Builder
		for _, line := range strings.Split(string(body), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			code.WriteString(line)
			code.WriteByte('\n')
		}
		for tone := range produced {
			ident := "domain.Tone" + strings.ToUpper(tone[:1]) + tone[1:]
			if regexp.MustCompile(regexp.QuoteMeta(ident) + `\b`).MatchString(code.String()) {
				produced[tone] = append(produced[tone], name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal: %v", err)
	}
	for tone, files := range produced {
		if len(files) == 0 {
			t.Errorf("domain.Tone%s is named by no source outside internal/domain -- a tone nothing produces is a Theme entry and a palette arm for nothing; delete it, or find who should be producing it before concluding that", strings.ToUpper(tone[:1])+tone[1:])
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

// TestEveryAcceptedNodeTypeHasAWalkerArm closes the gap the UI IR slice shipped with.
//
// `ir.Validate` accepted `layout/row`, `layout/grid`, `layout/split` and `component/Avatar` -- correctly, they
// are real primitives -- while `rendering.uiNode` had a case for none of them. A valid tree containing any one
// of them rendered **nothing, silently**: no error, no panic, no failing test, just a missing block. That is
// the exact failure class this package gates everywhere else, and the slice that introduced UI IR introduced
// it too.
//
// The two lists are `internal/ir`'s permitted-property table and the walker's switch arms, and they are in
// different packages by design (§15.1's pipeline runs IR → renderer, so the renderer may not be imported
// back). Nothing but this test makes them agree -- the same role `TestServiceRegistryAndExecutorsAgree` and
// `TestComponentRegistryAndRenderersAgree` already play across the other two seams.
//
// **Narrowing `Validate` is not the way to pass.** A tree with a `row` node is not invalid; refusing it would
// make the IR reject legitimate composition to match a renderer that is behind. The walker gets the arm.
func TestEveryAcceptedNodeTypeHasAWalkerArm(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(repoRoot(), "internal", "ir", "ui.go"))
	if err != nil {
		t.Fatalf("read ir/ui.go: %v", err)
	}
	accepted := regexp.MustCompile(`"(layout|static|component)/([A-Za-z]+)"`).FindAllStringSubmatch(string(src), -1)
	if len(accepted) == 0 {
		t.Fatal("found no accepted node types in internal/ir/ui.go -- this gate would pass by measuring nothing")
	}

	walker := renderingSource(t)
	// The identifier each kind's arm must name, derived from the type rather than listed, so a new entry in
	// ir's table is covered the day it is added.
	ident := map[string]string{
		"layout":    "domain.Layout",
		"static":    "domain.Static",
		"component": "domain.Component",
	}
	seen := map[string]bool{}
	for _, m := range accepted {
		kind, typ := m[1], m[2]
		key := kind + "/" + typ
		if seen[key] {
			continue
		}
		seen[key] = true
		name := ident[kind] + strings.ToUpper(typ[:1]) + typ[1:]
		if kind == "component" {
			name = ident[kind] + typ // ComponentStatusBadge, ComponentAvatar
		}
		if !strings.Contains(walker, "case "+name+":") {
			t.Errorf("internal/ir accepts %q and internal/rendering's uiNode has no `case %s:` -- a valid tree containing it renders nothing, silently. Add the arm; do not narrow Validate", key, name)
		}
	}
	if len(seen) < 8 {
		t.Errorf("only %d node types found in ir's permitted-property table -- expected at least 8 (5 layouts, 3 static); has the table moved?", len(seen))
	}
}
