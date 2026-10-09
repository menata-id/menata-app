package registry

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"menata.app/internal/domain"
)

// Component is one member of the Component Registry seam 007 §14 describes: an identity, the contract it
// declares (§13), and the validator that checks a use of it.
//
// It mirrors `Service` exactly, including what it leaves out -- the **renderer** stays in
// `internal/rendering`, because this package may import `internal/domain` and nothing else. §14's chain
// (`type → contract → validator → resolver → renderer`) is held across three planes with a conformance gate
// asserting they agree, not in one package. Read `Service`'s own comment for why that is a plane boundary
// rather than a preference.
//
// **What a registry buys that a `templ` function does not**, since a reader is entitled to ask: a closed
// catalogue is the only place a *boundedness* rule can be enforced. §12.3 forbids a Component acquiring
// business data its contract does not represent, and that is unenforceable against an ordinary function --
// any signature is legal, so `slaBadgePill(v any)` parsing a date and calling `EvaluateSLA(due, time.Now())`
// looked exactly like a renderer. Against a contract it is a mismatch a test can state.
type Component struct {
	// Contract is what this Component declares (§13).
	Contract domain.ComponentContract
	// Validate checks one *use* of this Component: the inputs a caller supplies against the ones declared.
	// It returns issues rather than an error, the posture every validator in this tree already has.
	//
	// It is not the boundedness check. Boundedness is structural -- it is about the renderer's own
	// signature, not about one call -- and lives in `conformance.TestRegisteredComponentsStayBounded`.
	Validate func(inputs map[string]string) []string
}

// Components is the closed catalogue. A type absent from it is refused rather than resolved, which is what
// makes this a static seam and not a plugin loader (see doc.go).
//
// **One member, and that is the honest starting size.** §12.3 lists nine example Components and this
// registers the one with a measured population: `statusPill` had 8 call sites plus two more renderers of the
// same idea under different names, one of which was unbounded. The other eight examples are either already
// expressible (a `Metric` is `statusBadge`'s sibling, next) or have no site at all -- and 007 §34 plus this
// repo's own three zero-caller layout primitives are why none of them is registered in advance.
var Components = map[domain.ComponentType]Component{
	domain.ComponentStatusBadge: {Contract: statusBadgeContract, Validate: validateStatusBadge},
	domain.ComponentAvatar:      {Contract: avatarContract, Validate: validateAvatar},
	domain.ComponentMetric:      {Contract: metricContract, Validate: validateMetric},
	domain.ComponentCollection:  {Contract: collectionContract, Validate: validateCollection},
	domain.ComponentField:       {Contract: fieldContract, Validate: validateField},
	domain.ComponentButton:      {Contract: buttonContract, Validate: validateButton},
	domain.ComponentTag:         {Contract: tagContract, Validate: validateTag},
	domain.ComponentForm:        {Contract: formContract, Validate: validateForm},
	domain.ComponentFormInput:   {Contract: formInputContract, Validate: validateFormInput},
}

// formContract is §12.3's `Form`, the write side of 007 §11.3 in its narrowest honest form: a submit label, the
// route it posts to, and a `field` slot lowering fills.
//
// **`action` is a declared input an author may not write.** It is required here because a Form with no
// destination submits to the page it is on; `ir.Lower` is what fills it, from the bound Dataset's Machine, and
// refuses a typed one for the reason it refuses a typed `href` (001 #3, #8). The validator pins its shape to the
// runtime's own create route, so a hand-built tree cannot aim a form somewhere else.
//
// What the contract forces that a reviewer would not have asked for: `submit` is required, so a form never has
// a button with no name, and the Component draws the button itself through `Button` -- an author cannot leave
// a form with no way to send it, nor with a differently-styled one.
var formContract = domain.ComponentContract{
	Type: domain.ComponentForm,
	Inputs: []domain.ComponentInput{
		{Name: "submit", Kind: "string", Required: true},
		{Name: "action", Kind: "route", Required: true},
		{Name: "method", Kind: "string"},
	},
	DataRequirements: nil,
	Slots:            []string{"field"},
	Actions:          []string{"submit"},
	Accessibility:    "a real <form> with a real submit <button>, so Enter in a text control sends it without a script; every control inside is labelled by a Field whose `for` names it",
	Renderer:         "form",
}

// validateForm checks a use against the declared inputs and the two shapes `action` may take: a Machine's create
// route (`/machines/<id>/records`) for a post, and one of its records (`/machines/<id>/records/<id>`) for the
// patch that `method` names. Anything else -- a path elsewhere, a verb beyond those two -- is refused, so a
// hand-built tree cannot aim a form somewhere the generic routes do not own.
func validateForm(inputs map[string]string) []string {
	issues := checkDeclaredInputs(formContract, inputs, "Form")
	a := inputs["action"]
	if a == "" {
		return issues
	}
	method := inputs["method"]
	if method != "" && method != domain.FormMethodPatch {
		issues = append(issues, fmt.Sprintf("Form method %q is not one the runtime derives (%s)", method, domain.FormMethodPatch))
	}
	parts := strings.Split(strings.TrimPrefix(a, "/machines/"), "/")
	valid := strings.HasPrefix(a, "/machines/") && parts[0] != "" && len(parts) >= 2 && parts[1] == "records"
	switch {
	case valid && method == domain.FormMethodPatch:
		valid = len(parts) == 3 && parts[2] != ""
	case valid:
		valid = len(parts) == 2
	}
	if !valid {
		issues = append(issues, fmt.Sprintf("Form action %q is not a Machine's create route (/machines/<id>/records) or, with method %s, one of its records (/machines/<id>/records/<id>)", a, domain.FormMethodPatch))
	}
	return issues
}

// formInputContract is the control inside a `Form`'s `Field`. It has no slot and no action: it is a leaf whose
// every property was decided by the Machine's Field, which is the point of it.
//
// **`name` is an input here, and it is the one place a `name=` is allowed to exist** -- because it arrives
// derived, from `Field.ID`, and never typed. `kind` is a closed set; `options` belongs to a select and to
// nothing else, so a text control carrying choices is a declaration that says something the control cannot do.
var formInputContract = domain.ComponentContract{
	Type: domain.ComponentFormInput,
	Inputs: []domain.ComponentInput{
		{Name: "id", Kind: "string", Required: true},
		{Name: "name", Kind: "string", Required: true},
		{Name: "kind", Kind: "InputKind", Required: true},
		{Name: "options", Kind: "string"},
		{Name: "value", Kind: "string"},
		{Name: "required", Kind: "bool"},
	},
	DataRequirements: nil,
	Slots:            nil,
	Actions:          nil,
	Accessibility:    "a native control, named by the Field that labels it; `required` is the native attribute, so a screen reader announces it and the browser refuses an empty submit before the server is asked",
	Renderer:         "formInput",
}

// validateFormInput checks a use against the declared inputs, the closed kind set, and that `options` appears
// only on a select.
func validateFormInput(inputs map[string]string) []string {
	issues := checkDeclaredInputs(formInputContract, inputs, "Input")
	kind := domain.InputKind(inputs["kind"])
	if inputs["kind"] != "" && !domain.KnownInputKinds[kind] {
		issues = append(issues, fmt.Sprintf("Input kind %q is not one of the declared kinds", inputs["kind"]))
	}
	if inputs["options"] != "" && kind != domain.InputSelect {
		issues = append(issues, fmt.Sprintf("Input options belong to a select, not to a %q control", inputs["kind"]))
	}
	if v := inputs["required"]; v != "" && v != "true" && v != "false" {
		issues = append(issues, fmt.Sprintf("Input required %q is not true or false", v))
	}
	return issues
}

// tagContract is §12.3's `Tag`: a label and, optionally, an entry of the closed palette.
//
// **Two inputs, and the second is optional on purpose.** An untagged colour is a real state -- a label whose
// Machine declares no `color` role, or whose value is not in the palette, is drawn in the neutral slate entry --
// so requiring `color` would make a page author state what the data layer already defaults. The validator holds
// the palette instead: a *declared* colour that is not an entry is a mistake, where the renderer's fallback for
// data that arrives unknown is a kindness. Same asymmetry `Avatar`'s `presence` has.
//
// What the contract forces that a reviewer would not have asked for: `label` is **required**. The five sites
// always had one, but the type allowed a `CardTag{}` and drew an empty coloured dot -- an element that says
// nothing to a screen reader (the dot is `aria-hidden`) and nothing to a person who cannot tell the hues apart.
var tagContract = domain.ComponentContract{
	Type: domain.ComponentTag,
	Inputs: []domain.ComponentInput{
		{Name: "label", Kind: "string", Required: true},
		{Name: "color", Kind: "TagColor"},
	},
	DataRequirements: nil,
	Slots:            nil,
	Actions:          nil,
	Accessibility:    "the label is the accessible name and is required; the colour dot is aria-hidden and the palette is the only colour vocabulary, so colour never carries the tag's meaning by itself",
	Renderer:         "tagChip",
}

// validateTag checks a use against the declared inputs and the closed palette.
func validateTag(inputs map[string]string) []string {
	issues := checkDeclaredInputs(tagContract, inputs, "Tag")
	if v := inputs["color"]; v != "" && !domain.IsTagColor(v) {
		issues = append(issues, fmt.Sprintf("Tag color %q is not an entry of the palette", v))
	}
	return issues
}

// buttonContract is §12.3's `Button`, and the first contract whose `Actions` is not empty.
//
// **Four inputs, and two of them are one input.** `name` and `value` are the native pair a submit button posts
// with its form -- `intent=draft`, `decision=approved` -- and they are meaningful only together: a name with
// no value posts an empty string the handler cannot tell from "not clicked", and a value with no name is
// dropped by the browser. The validator holds that; the four hand-written sites that carried the pair never
// did. That is 007 §11.3's Binding in its smallest honest form, not a way to pass arbitrary attributes: there
// is no `hx-*`, no `id`, no `class`, and `checkDeclaredInputs` refuses all three by name.
//
// `type` is not an input. Every site that migrates is a submit, and the ones that are not (`onclick` closing a
// `<details>`, a `data-modal` trigger) are behaviour -- §12.3's `actions` -- which is a capability this
// contract does not claim.
var buttonContract = domain.ComponentContract{
	Type: domain.ComponentButton,
	Inputs: []domain.ComponentInput{
		{Name: "label", Kind: "string", Required: true},
		{Name: "variant", Kind: "ButtonVariant", Required: true},
		{Name: "name", Kind: "string"},
		{Name: "value", Kind: "string"},
		{Name: "confirm", Kind: "string"},
		{Name: "action", Kind: "string"},
		{Name: "method", Kind: "string"},
	},
	DataRequirements: nil,
	Slots:            nil,
	Actions:          []string{"submit", "delete"},
	Accessibility:    "the label is the accessible name, so it is required and never an icon alone; the element is a real <button>, focusable and activated by Enter and Space without a role or a script; a delete states its consequence in `confirm`, which the browser asks before anything is sent",
	Renderer:         "requestButton",
}

// validateButton checks a use against the declared inputs, the closed variant set, and the one cross-input
// rule: `name` and `value` arrive together or not at all.
func validateButton(inputs map[string]string) []string {
	issues := checkDeclaredInputs(buttonContract, inputs, "Button")
	if v := inputs["variant"]; v != "" && !domain.KnownButtonVariants[domain.ButtonVariant(v)] {
		issues = append(issues, fmt.Sprintf("Button variant %q is not one of the declared variants", v))
	}
	if (inputs["name"] == "") != (inputs["value"] == "") {
		issues = append(issues, "Button `name` and `value` are one pair: a name without a value posts an empty string, and a value without a name is dropped by the browser")
	}
	return append(issues, buttonRequestIssues(inputs)...)
}

// buttonRequestIssues holds the rules of the one Button that sends a request instead of submitting a form.
// `action` and `method` are derived by lowering and arrive together; the only verb is delete, because it is the
// only write that asks a person nothing (an edit is a Form). `confirm` is required with them, and meaningless
// without: it is the sentence a person reads before a record is gone.
func buttonRequestIssues(inputs map[string]string) []string {
	action, method, confirm := inputs["action"], inputs["method"], inputs["confirm"]
	var issues []string
	if (action == "") != (method == "") {
		issues = append(issues, "Button `action` and `method` are one pair: a route with no verb, or a verb with no route, sends nothing")
	}
	if action == "" {
		if confirm != "" {
			issues = append(issues, "Button `confirm` asks before a request is sent, and this Button sends none")
		}
		return issues
	}
	if method != domain.ButtonMethodDelete {
		issues = append(issues, fmt.Sprintf("Button method %q is not one the runtime derives (only %q)", method, domain.ButtonMethodDelete))
	}
	if confirm == "" {
		issues = append(issues, "a Button that sends a delete states its consequence in `confirm`")
	}
	if inputs["name"] != "" {
		issues = append(issues, "a Button that sends a request posts no name/value pair")
	}
	if !strings.HasPrefix(action, "/machines/") {
		issues = append(issues, fmt.Sprintf("Button action %q is not a route the runtime derives (/machines/<id>/records/<id>)", action))
	}
	return issues
}

// fieldContract is §12.3's `Field`: a label bound to one control, which fills the slot.
//
// **Two inputs, not six.** `authField` took `(id, name, label, kind, autocomplete, autofocus)`; five of those
// describe the *control*, not the labelling, and a Component that accepted them would be deciding what sort
// of input exists. The slot moves that to the caller, which is also why `wizardField` -- already this shape
// with `{ children... }` -- needed no parameters beyond these two.
//
// `for` is the control's own id, so the label and the control are bound in the DOM rather than by proximity.
// That is the accessibility clause doing work again: a `<label>` with no `for` and no nesting labels nothing.
var fieldContract = domain.ComponentContract{
	Type: domain.ComponentField,
	Inputs: []domain.ComponentInput{
		{Name: "label", Kind: "string", Required: true},
		{Name: "for", Kind: "string", Required: true},
	},
	DataRequirements: nil,
	Slots:            []string{"control"},
	Actions:          nil,
	Accessibility:    "`for` must name the control's own id: a <label> bound neither by `for` nor by nesting labels nothing, and this Component cannot nest because the control arrives through a slot",
	Renderer:         "field",
}

func validateField(inputs map[string]string) []string {
	return checkDeclaredInputs(fieldContract, inputs, "Field")
}

// collectionContract is §12.3's `Collection`, and **the first contract here with a slot** -- so it is the
// first time `Slots` has held anything, after three contracts left it nil.
//
// `item` is the slot each child fills. The Component declares no data requirements and never will: items
// arrive already composed, so it cannot select, sort or format. That is what keeps a list generic without
// becoming the unbounded escape hatch §12.3 forbids -- 21 of the 21 written use cases need a list, and they
// need 21 different row shapes, which is a slot's job rather than a parameter's.
var collectionContract = domain.ComponentContract{
	Type: domain.ComponentCollection,
	Inputs: []domain.ComponentInput{
		{Name: "gap", Kind: "Gap", Required: true},
		{Name: "empty", Kind: "string"},
		{Name: "ordered", Kind: "bool"},
		{Name: "divided", Kind: "bool"},
		{Name: "truncated", Kind: "int"},
	},
	DataRequirements: nil,
	Slots:            []string{"item"},
	Actions:          nil,
	Accessibility:    "renders a real <ul>/<li>, or an <ol>/<li> when `ordered` is true, so the list and its length (and, ordered, each item's position) are announced without an explicit role (`divided` changes the look only: the same list element, the rule between rows is a border and not an element) -- the drawn ordinal is aria-hidden for that reason, never the only carrier of the position; with no items and an `empty` text it renders that text in place of an empty list, so a screen-reader user hears why nothing is there rather than an empty list; with `truncated` (the bound that cut the list short) it adds a paragraph after the list naming that bound, in words, so a reader who cannot see the list's end is told it is not the end",
	Renderer:         "collection",
}

func validateCollection(inputs map[string]string) []string {
	issues := checkDeclaredInputs(collectionContract, inputs, "Collection")
	if v := inputs["ordered"]; v != "" && v != "true" && v != "false" {
		issues = append(issues, fmt.Sprintf("Collection ordered %q is not true or false", v))
	}
	if v := inputs["divided"]; v != "" && v != "true" && v != "false" {
		issues = append(issues, fmt.Sprintf("Collection divided %q is not true or false", v))
	}
	if v := inputs["truncated"]; v != "" {
		if n, err := strconv.Atoi(v); err != nil || n <= 0 {
			issues = append(issues, fmt.Sprintf("Collection truncated %q is not a positive whole number", v))
		}
	}
	return issues
}

// metricContract is §12.3's `Metric`: a resolved number and the question it answers.
//
// **`hint` and `tone` are optional, added 2026-10-05 for the Project Management Dashboard (Case 19 PM04),
// whose four tiles each carry a one-line qualifier ("Past its due date") and whose Overdue and Completed
// values are drawn as a signal.** Both are resolved by the caller: the hint is text it already composed, the
// tone is the same closed `BadgeTone` set a status badge takes, so the Component still decides no business
// meaning and the renderer still draws a colour only through the Workspace's theme. Neither is required, so
// the three screens that predate them are unchanged.
//
// **Value is a string, deliberately.** A Component that took an `int` would be taking a half-resolved value --
// the caller already decided the format (`fmt.Sprint`, a percentage, "3 of 7"), and asking the Component to
// format would be asking it to decide presentation from data it does not have. §4.4's division again: resolve,
// then render.
var metricContract = domain.ComponentContract{
	Type: domain.ComponentMetric,
	Inputs: []domain.ComponentInput{
		{Name: "label", Kind: "string", Required: true},
		{Name: "value", Kind: "string", Required: true},
		{Name: "hint", Kind: "string"},
		{Name: "tone", Kind: "BadgeTone"},
		// Optional: a Metric that links is a resolved destination, never a typed one -- `ir.Lower` builds it from
		// a navigation item and a row's value, and refuses an author-written href.
		{Name: "href", Kind: "string"},
	},
	DataRequirements: nil,
	Slots:            nil,
	Actions:          nil,
	Accessibility:    "the value, its label and its hint are adjacent text in reading order, so the group is its own accessible description; tone is colour only and carries no meaning a reader needs, so it is never the only carrier of one; it is not a live region",
	Renderer:         "metric",
}

func validateMetric(inputs map[string]string) []string {
	issues := checkDeclaredInputs(metricContract, inputs, "Metric")
	if tone := inputs["tone"]; tone != "" && !domain.KnownBadgeTones[domain.BadgeTone(tone)] {
		issues = append(issues, fmt.Sprintf("Metric tone %q is not one of the declared tones", tone))
	}
	if href := inputs["href"]; href != "" && (!strings.HasPrefix(href, "/") || strings.HasPrefix(href, "//")) {
		issues = append(issues, fmt.Sprintf("Metric href %q is not a route of this application (it starts with a single /)", href))
	}
	return issues
}

// statusBadgeContract is declared separately from the catalogue, not for tidiness: `validateStatusBadge`
// reads its own declared inputs rather than retyping them, and a validator reaching back into `Components`
// to find them is an initialization cycle Go rejects outright. Naming the contract is what lets the
// validator be derived from it instead of agreeing with it by hand -- the same reason
// `conformance.TestEveryDerivationIsOwedBySomeRole` reads constants out of the source.
var statusBadgeContract = domain.ComponentContract{
	Type: domain.ComponentStatusBadge,
	Inputs: []domain.ComponentInput{
		{Name: "label", Kind: "string", Required: true},
		{Name: "tone", Kind: "BadgeTone", Required: true},
		// Optional, unlike Avatar's: an unset size is `regular`, which is what every badge was before the
		// input existed, so a composed node written earlier keeps meaning what it meant.
		{Name: "size", Kind: "BadgeSize", Required: false},
	},
	// Empty, and this is the load-bearing line of the whole slice. See
	// domain.ComponentContract.DataRequirements.
	DataRequirements: nil,
	Slots:            nil,
	Actions:          nil,
	// Measured rather than invented: no pill in this corpus carries an ARIA attribute, and the text inside
	// the span is the state. Saying so is what stops the next reader adding `role="status"` to a label that
	// is not a live region.
	Accessibility: "the badge's own text is its accessible name; it is not a live region, so it carries no role",
	Renderer:      "statusBadge",
}

// avatarContract is the **second** registered Component, and it is here to answer the question the Stage 2
// plan asked: does the contract have to change to hold a second member?
//
// **Structurally, no.** `Inputs` is a slice, so four inputs fit where two did, and `Validate` derives from
// the declaration rather than agreeing with it. Saying that plainly is more useful than implying the second
// member was a stress test it was not.
//
// **What it did expose is `Accessibility`.** For `StatusBadge` that field describes an absence -- the badge's
// text *is* its accessible name, so there is nothing for the renderer to do. An avatar's content is "AP",
// which is not a name, and none of the four hand-written sites carried one. So this is the first contract
// whose accessibility clause **demands markup**, and honouring it adds a `title` and an `aria-label` to four
// screens that had neither. A field that only ever described absences would have been decoration; this is
// what made it a requirement.
var avatarContract = domain.ComponentContract{
	Type: domain.ComponentAvatar,
	Inputs: []domain.ComponentInput{
		{Name: "initials", Kind: "string", Required: true},
		// Required, and this is the accessibility clause as an input rather than as prose. An avatar with no
		// name is a circle a screen reader announces as two letters.
		{Name: "label", Kind: "string", Required: true},
		{Name: "size", Kind: "AvatarSize", Required: true},
		{Name: "presence", Kind: "AvatarPresence", Required: true},
	},
	DataRequirements: nil,
	Slots:            nil,
	Actions:          nil,
	Accessibility:    "the initials are not an accessible name: the renderer MUST carry the person's name or email as `aria-label`, and as `title` so a sighted reader can hover",
	Renderer:         "avatar",
}

// validateAvatar checks a use against the declared inputs, including both closed sets. Same shape as
// validateStatusBadge, deliberately -- a second validator that invented its own conventions would make the
// catalogue two catalogues.
func validateAvatar(inputs map[string]string) []string {
	issues := checkDeclaredInputs(avatarContract, inputs, "Avatar")
	if v := inputs["size"]; v != "" && !domain.KnownAvatarSizes[domain.AvatarSize(v)] {
		issues = append(issues, fmt.Sprintf("Avatar size %q is not one of the declared sizes", v))
	}
	if v := inputs["presence"]; v != "" && !domain.KnownAvatarPresences[domain.AvatarPresence(v)] {
		issues = append(issues, fmt.Sprintf("Avatar presence %q is not one of the declared presences", v))
	}
	return issues
}

// checkDeclaredInputs is the half both validators share: every required input present, and no undeclared one
// supplied. Extracted when the second Component arrived rather than in advance -- one validator is a screen's
// own detail, two is a shape (CLAUDE.md's decision path, step 3).
func checkDeclaredInputs(c domain.ComponentContract, inputs map[string]string, name string) []string {
	var issues []string
	declared := map[string]bool{}
	for _, in := range c.Inputs {
		declared[in.Name] = true
		if in.Required && inputs[in.Name] == "" {
			issues = append(issues, fmt.Sprintf("%s requires input %q", name, in.Name))
		}
	}
	for k := range inputs {
		if !declared[k] {
			issues = append(issues, fmt.Sprintf("%s has no declared input %q -- a Component does not grow a property to suit one caller (007 §12.3)", name, k))
		}
	}
	return issues
}

// ValidateComponentUse is the entry point: it resolves the type and runs its contract, or reports that the
// runtime realizes no such Component.
func ValidateComponentUse(t domain.ComponentType, inputs map[string]string) []string {
	c, known := Components[t]
	if !known {
		return []string{fmt.Sprintf("component type %q is not one this runtime realizes", t)}
	}
	return c.Validate(inputs)
}

// validateStatusBadge checks a use against the declared inputs: every required one present, no undeclared
// one supplied, and `tone` naming a member of the closed set.
//
// The undeclared-input half is not symmetry for its own sake. An extra input is how a bounded Component
// starts becoming the unbounded `GenericComponent` §12.3 forbids by name -- one caller passes something the
// contract does not mention, the renderer grows a parameter for it, and the contract is now a comment.
func validateStatusBadge(inputs map[string]string) []string {
	issues := checkDeclaredInputs(statusBadgeContract, inputs, "StatusBadge")
	if tone := inputs["tone"]; tone != "" && !domain.KnownBadgeTones[domain.BadgeTone(tone)] {
		issues = append(issues, fmt.Sprintf("StatusBadge tone %q is not one of the declared tones", tone))
	}
	if size := inputs["size"]; size != "" && !domain.KnownBadgeSizes[domain.BadgeSize(size)] {
		issues = append(issues, fmt.Sprintf("StatusBadge size %q is not one of the declared sizes", size))
	}
	return issues
}

// ComponentTypeNames is the catalogue's keys as strings, for a consumer that must know which types exist but
// may not depend on this package's types -- `internal/ir`, which validates a tree against them (007 §15.3)
// and is upstream of the renderer in §15.1's pipeline.
//
// Sorted, because the caller may log or compare it and 007 §4.6 makes identical input producing identical
// output a MUST -- a map range would make that false for no reason.
func ComponentTypeNames() []string {
	out := make([]string, 0, len(Components))
	for t := range Components {
		out = append(out, string(t))
	}
	sort.Strings(out)
	return out
}

// SlottedComponentTypeNames is the subset of the catalogue whose contract declares at least one child slot,
// for `internal/ir` to validate children against (007 §15.3's slot/type mismatch). Sorted, per §4.6.
func SlottedComponentTypeNames() []string {
	out := []string{}
	for t, c := range Components {
		if len(c.Contract.Slots) > 0 {
			out = append(out, string(t))
		}
	}
	sort.Strings(out)
	return out
}
