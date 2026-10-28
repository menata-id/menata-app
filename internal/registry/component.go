package registry

import (
	"fmt"
	"regexp"
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
	domain.ComponentProgress:    {Contract: progressContract, Validate: validateProgress},
	domain.ComponentCallout:     {Contract: calloutContract, Validate: validateCallout},
	domain.ComponentEmptyState:  {Contract: emptyStateContract, Validate: validateEmptyState},
	domain.ComponentDisclosure:  {Contract: disclosureContract, Validate: validateDisclosure},

	domain.ComponentSegmentedControl: {Contract: segmentedControlContract, Validate: validateSegmentedControl},
	domain.ComponentPopover:          {Contract: popoverContract, Validate: validatePopover},
	domain.ComponentTable:            {Contract: tableContract, Validate: validateTable},
	domain.ComponentNavList:          {Contract: navListContract, Validate: validateNavList},
	domain.ComponentSearchBox:        {Contract: searchBoxContract, Validate: validateSearchBox},
	domain.ComponentDiff:             {Contract: diffContract, Validate: validateDiff},
	domain.ComponentFilterChips:      {Contract: filterChipsContract, Validate: validateFilterChips},
	domain.ComponentFilterSelect:     {Contract: filterSelectContract, Validate: validateFilterSelect},
	domain.ComponentActionBar:        {Contract: actionBarContract, Validate: validateActionBar},
	domain.ComponentStepper:          {Contract: stepperContract, Validate: validateStepper},
	domain.ComponentToggle:           {Contract: toggleContract, Validate: validateToggle},
	domain.ComponentIconTile:         {Contract: iconTileContract, Validate: validateIconTile},
	domain.ComponentTree:             {Contract: treeContract, Validate: validateTree},
	domain.ComponentFrame:            {Contract: frameContract, Validate: validateFrame},
}

// toggleContract is a record's completion circle (mockup PM01 and PM08): open (a ring) or finished (a filled circle with a
// check), and pressing it writes the Machine's own `completion:` Field the other way. It is a Component rather than a
// `Button` variant because what it says is a state, not an action: a screen reader hears "pressed" or "not pressed" against
// a name, and sighted people read the ring and the fill.
//
// **Every input but `describes` is derived.** `checked` is whether the record stands at the Machine's completion value,
// and `action`, `name` and `value` are the PATCH route, the status Field and the value to write -- the *other* side of
// where the record is -- so a page declares `component: Toggle, from: {describes: title}` and names no status, Field or
// route (001 #3, #8). `describes` is required for the reason Avatar's `label` is: a bare circle has no name.
var toggleContract = domain.ComponentContract{
	Type:   domain.ComponentToggle,
	Status: domain.ComponentStatusStable,
	Inputs: []domain.ComponentInput{
		{Name: "describes", Kind: "string", Required: true},
		{Name: "checked", Kind: "bool", Required: true},
		{Name: "action", Kind: "string", Required: true},
		{Name: "name", Kind: "string", Required: true},
		{Name: "value", Kind: "string", Required: true},
	},
	Actions:       []string{"transition"},
	Accessibility: "a real <button> with aria-pressed carrying the state; its accessible name says what pressing it does and names the record (`describes`), so no two circles down a list share a bare name; the ring and the filled check differ in lightness and shape as well as colour",
	Renderer:      "toggle",
}

// validateToggle checks a use against the declared inputs and the two rules that are not presence: `checked` is a boolean
// word, and `describes` is not blank (a name of spaces is a circle a screen reader cannot find).
func validateToggle(inputs map[string]string) []string {
	issues := checkDeclaredInputs(toggleContract, inputs, "Toggle")
	if v, ok := inputs["checked"]; ok && v != "" && v != "true" && v != "false" {
		issues = append(issues, fmt.Sprintf("Toggle checked %q is not true or false", v))
	}
	if v, ok := inputs["describes"]; ok && v != "" && strings.TrimSpace(v) == "" {
		issues = append(issues, "Toggle describes is blank -- the circle would carry no name")
	}
	return issues
}

// stepperContract is a record's place in its lifecycle. `label` names it ("Contract status"); `steps` is one
// `label<TAB>state` line per step, state done, current or todo, derived from a status Field and never written.
var stepperContract = domain.ComponentContract{
	Type:   domain.ComponentStepper,
	Status: domain.ComponentStatusStable,
	Inputs: []domain.ComponentInput{
		{Name: "label", Kind: "string", Required: true},
		{Name: "steps", Kind: "string", Required: true},
	},
	Accessibility: "an ordered list named by `label`; the current step carries aria-current=\"step\", and each step's state is in its text for a reader that does not see colour",
	Renderer:      "stepper",
}

func validateStepper(inputs map[string]string) []string {
	issues := checkDeclaredInputs(stepperContract, inputs, "Stepper")
	if v, ok := inputs["label"]; ok && strings.TrimSpace(v) == "" {
		issues = append(issues, "Stepper label is blank -- the list's only accessible name")
	}
	for _, line := range nonEmptyLines(inputs["steps"]) {
		if p := strings.Split(line, "\t"); len(p) != 2 || (p[1] != "done" && p[1] != "current" && p[1] != "todo") {
			issues = append(issues, fmt.Sprintf("Stepper step %q is not a label and done, current or todo", line))
		}
	}
	return issues
}

// actionBarContract is a page's primary actions. `label` names the group for assistive technology ("Contract actions");
// the actions are its children, links and Buttons, each already its own contract.
var actionBarContract = domain.ComponentContract{
	Type:   domain.ComponentActionBar,
	Status: domain.ComponentStatusStable,
	Inputs: []domain.ComponentInput{
		{Name: "label", Kind: "string", Required: true},
	},
	Slots:         []string{"actions"},
	Accessibility: "a group (role=group) named by `label`, holding real links and buttons in reading order",
	Renderer:      "actionBar",
}

func validateActionBar(inputs map[string]string) []string {
	issues := checkDeclaredInputs(actionBarContract, inputs, "ActionBar")
	if v, ok := inputs["label"]; ok && strings.TrimSpace(v) == "" {
		issues = append(issues, "ActionBar label is blank -- the group's only accessible name")
	}
	return issues
}

// filterChipsContract is a row of counted filter chips over one query parameter.
//
// `label` names the group (required: a row of links with no name is a list nobody can find), `query` the parameter the
// chips set, `options` an optional subset of the Dimension's values in the order they are drawn (`value=Label` lines),
// and `all` the words of the chip that clears the parameter. `chips` is what lowering produced from the bound Dimension
// and the request -- one line per chip, its label, count, address and whether it is the current one -- and is never
// written. What the contract forces that a reviewer would not have asked for: every chip is a link that keeps the
// request's other parameters, so choosing "Crew" does not throw away the words in the search box.
var filterChipsContract = domain.ComponentContract{
	Type:   domain.ComponentFilterChips,
	Status: domain.ComponentStatusStable,
	Inputs: []domain.ComponentInput{
		{Name: "label", Kind: "string", Required: true},
		{Name: "query", Kind: "string", Required: true},
		{Name: "options", Kind: "string"},
		{Name: "all", Kind: "string"},
		{Name: "chips", Kind: "string"},
	},
	DataRequirements: []string{"an aggregate Dataset with a dimension and a measure: `binding: {dataset, measure, rows: dimension}`"},
	Accessibility:    "a <nav> named by `label` holding one link per chip; the chip the request selected is a <span aria-current=\"true\">, and each chip's count is part of its text, so it is read with the label",
	Renderer:         "filterChips",
}

func validateFilterChips(inputs map[string]string) []string {
	issues := checkDeclaredInputs(filterChipsContract, inputs, "FilterChips")
	if v, ok := inputs["label"]; ok && strings.TrimSpace(v) == "" {
		issues = append(issues, "FilterChips label is blank -- the group's only accessible name")
	}
	issues = append(issues, filterQueryIssues("FilterChips", inputs["query"])...)
	for _, line := range nonEmptyLines(inputs["chips"]) {
		if parts := strings.Split(line, "\t"); len(parts) != 4 || strings.TrimSpace(parts[0]) == "" || !strings.HasPrefix(parts[2], "?") {
			issues = append(issues, fmt.Sprintf("FilterChips chip %q is not a label, a count, a ?query address and a current flag", line))
		}
	}
	return issues
}

// filterSelectContract is the same choice as a drop-down.
//
// `label` names the control (a real <label>, visually hidden), `query` is the parameter, `all` the words of the choice
// that clears it (required: a filter you cannot clear is a trap). `choices` (one `value<TAB>label` line per Dimension
// row), `selected` (the request's value) and `keep` (the request's other parameters, as a query string) are produced by
// lowering and never written.
var filterSelectContract = domain.ComponentContract{
	Type:   domain.ComponentFilterSelect,
	Status: domain.ComponentStatusStable,
	Inputs: []domain.ComponentInput{
		{Name: "label", Kind: "string", Required: true},
		{Name: "query", Kind: "string", Required: true},
		{Name: "all", Kind: "string", Required: true},
		{Name: "choices", Kind: "string"},
		{Name: "selected", Kind: "string"},
		{Name: "keep", Kind: "string"},
	},
	DataRequirements: []string{"an aggregate Dataset with a dimension and a measure: `binding: {dataset, measure, rows: dimension}`"},
	Accessibility:    "a GET form holding one <select> named by a real <label> (visually hidden); it submits on change, and a submit button inside <noscript> does the same without script",
	Renderer:         "filterSelect",
}

func validateFilterSelect(inputs map[string]string) []string {
	issues := checkDeclaredInputs(filterSelectContract, inputs, "FilterSelect")
	if v, ok := inputs["label"]; ok && strings.TrimSpace(v) == "" {
		issues = append(issues, "FilterSelect label is blank -- a select needs a name")
	}
	issues = append(issues, filterQueryIssues("FilterSelect", inputs["query"])...)
	for _, line := range nonEmptyLines(inputs["choices"]) {
		if parts := strings.Split(line, "\t"); len(parts) != 2 || parts[0] == "" || strings.TrimSpace(parts[1]) == "" {
			issues = append(issues, fmt.Sprintf("FilterSelect choice %q is not a value and a label", line))
		}
	}
	return issues
}

// filterQueryIssues is what a filter's parameter must be: a plain word, and not one the runtime owns.
func filterQueryIssues(name, p string) []string {
	if p == "" {
		return nil
	}
	var issues []string
	if !searchParamPattern.MatchString(p) {
		issues = append(issues, fmt.Sprintf("%s query %q is not a plain word (letters, digits and underscore, starting with a letter)", name, p))
	}
	if domain.ParamReserved(p) {
		issues = append(issues, fmt.Sprintf("%s query %q is one the runtime owns", name, p))
	}
	return issues
}

func nonEmptyTokens(s string) []string {
	var out []string
	for _, t := range strings.Split(s, ",") {
		if strings.TrimSpace(t) != "" {
			out = append(out, strings.TrimSpace(t))
		}
	}
	return out
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// tableContract is a records table, the nameable form of `dataTable`.
//
// **Three inputs the author never writes the last two of.** `label` is the table's accessible name (required: a screen
// reader reaching "table, 3 columns" must be told which), `empty` is what a table with no records says in place of a
// header over nothing. `headers` and `aligns` are produced by lowering from the page's `columns:` -- one header per line,
// one alignment per header -- and are inputs because the renderer takes them and a boundedness gate reads its signature;
// `ir.Lower` refuses a page that writes them. The cells are the `cell` slot, row by row, already composed: the Table
// selects, sorts and formats nothing.
var tableContract = domain.ComponentContract{
	Type:   domain.ComponentTable,
	Status: domain.ComponentStatusStable,
	Inputs: []domain.ComponentInput{
		{Name: "label", Kind: "string", Required: true},
		{Name: "empty", Kind: "string"},
		{Name: "headers", Kind: "string", Required: true},
		{Name: "aligns", Kind: "string"},
		// hidden is lowered from the columns' `hide_header: true`: the places (0-based, comma-joined) whose header words are kept
		// for assistive technology and not drawn. Never written.
		{Name: "hidden", Kind: "string"},
		// truncated is lowered from `complete: true` when the Dataset's bound cut the list: "100", or "100/340" when the
		// count it was cut from is known. Never written.
		{Name: "truncated", Kind: "string"},
		// framed draws the table on a surface (the bordered card the mockups put a list in); absent, it is unframed and
		// the layout around it decides.
		{Name: "framed", Kind: "boolean"},
		// footer is the total row lowering produced from the columns' `total:` (and the Table's `totals:` Dataset, which
		// lowering reads and drops), one tab-separated cell per column, never written.
		{Name: "footer", Kind: "string"},
	},
	DataRequirements: nil,
	Slots:            []string{"cell"},
	Actions:          nil,
	Accessibility:    "a real <table> with a caption-equivalent aria-label from `label`, a header row of <th scope=\"col\"> cells, and one <tr> per record so a screen reader announces the row and column of every cell; with no records and an `empty` text it draws that text in place of a header over nothing; the scroll box is the table's own, so a table wider than a phone scrolls itself rather than the page",
	Renderer:         "table",
}

// validateTable checks a use against the declared inputs, a non-blank label, and that the headers and alignments agree
// (one alignment per header, each from the closed set).
func validateTable(inputs map[string]string) []string {
	issues := checkDeclaredInputs(tableContract, inputs, "Table")
	if v, ok := inputs["label"]; ok && strings.TrimSpace(v) == "" {
		issues = append(issues, "Table label is blank -- a table with no name is announced as only \"table\"")
	}
	if v, ok := inputs["framed"]; ok && v != "true" && v != "false" {
		issues = append(issues, fmt.Sprintf("Table framed %q is not true or false", v))
	}
	headers := strings.Split(inputs["headers"], "\n")
	if inputs["headers"] == "" {
		return issues
	}
	for i, h := range headers {
		// The last column may be unnamed: it is the row actions' (ir.lowerTable), whose buttons name themselves. A column
		// a page declares always has a header -- lowering refuses a blank one before this runs.
		if strings.TrimSpace(h) == "" && i < len(headers)-1 {
			issues = append(issues, "Table has a blank header")
			break
		}
	}
	for _, h := range nonEmptyTokens(inputs["hidden"]) {
		if i, err := strconv.Atoi(h); err != nil || i < 0 || i >= len(headers) {
			issues = append(issues, fmt.Sprintf("Table hidden %q is not the place of one of its %d columns", h, len(headers)))
		}
	}
	if a := inputs["aligns"]; a != "" {
		parts := strings.Split(a, ",")
		if len(parts) != len(headers) {
			issues = append(issues, fmt.Sprintf("Table has %d headers and %d alignments", len(headers), len(parts)))
		}
		for _, p := range parts {
			if !domain.KnownTableAligns[domain.TableAlign(p)] {
				issues = append(issues, fmt.Sprintf("Table alignment %q is not start or end", p))
			}
		}
	}
	return issues
}

// popoverContract is a floating panel under a trigger.
//
// **Five inputs, one slot, one action.** `label` is the trigger's words (required: a trigger with no words is a control
// with no name), `title` is its accessible name when the label is a glyph ("..." announces as nothing), and the three
// closed sets are what the two existing sites differ in -- the trigger's look, the side the panel opens to, and the room
// it gives its body. `body` is the slot, composed by the caller; the panel never knows it holds a form. `toggle` is the
// same native `<details>` action Disclosure declares: no script, no request, nothing a page may attach.
//
// What the contract forces that a reviewer would not have asked for: a glyph label with no `title` is refused (the
// hand-written "..." button carried `aria-label` by luck, and nothing would have caught the next one without it).
var popoverContract = domain.ComponentContract{
	Type:   domain.ComponentPopover,
	Status: domain.ComponentStatusStable,
	Inputs: []domain.ComponentInput{
		{Name: "label", Kind: "string", Required: true},
		{Name: "title", Kind: "string"},
		{Name: "trigger", Kind: "PopoverTrigger", Required: true},
		{Name: "align", Kind: "PopoverAlign", Required: true},
		{Name: "density", Kind: "PopoverDensity", Required: true},
	},
	DataRequirements: nil,
	Slots:            []string{"body"},
	Actions:          []string{"toggle"},
	Accessibility:    "a native <details> with a <summary>: Enter and Space open and close it and the browser announces the state; a trigger whose label is a glyph must carry a `title`, which becomes its aria-label",
	Renderer:         "popover",
}

// validatePopover checks a use against the declared inputs, the three closed sets and the glyph rule.
func validatePopover(inputs map[string]string) []string {
	issues := checkDeclaredInputs(popoverContract, inputs, "Popover")
	if v, ok := inputs["label"]; ok && strings.TrimSpace(v) == "" {
		issues = append(issues, "Popover label is blank -- a trigger with no words is a control with no name")
	}
	if v := inputs["trigger"]; v != "" && !domain.KnownPopoverTriggers[domain.PopoverTrigger(v)] {
		issues = append(issues, fmt.Sprintf("Popover trigger %q is not one of the declared looks", v))
	}
	if v := inputs["align"]; v != "" && !domain.KnownPopoverAligns[domain.PopoverAlign(v)] {
		issues = append(issues, fmt.Sprintf("Popover align %q is not one of the declared sides", v))
	}
	if v := inputs["density"]; v != "" && !domain.KnownPopoverDensities[domain.PopoverDensity(v)] {
		issues = append(issues, fmt.Sprintf("Popover density %q is not one of the declared densities", v))
	}
	if l := inputs["label"]; l != "" && strings.TrimSpace(inputs["title"]) == "" && !strings.ContainsAny(l, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789") {
		issues = append(issues, fmt.Sprintf("Popover label %q is a glyph with no title -- give it a `title`, its accessible name", l))
	}
	return issues
}

// segmentedControlContract is a short row of mutually exclusive choices of the page itself, one marked.
//
// **Four required inputs, no slot, no action.** `label` names the group (the only name a screen-reader user gets for
// "Board / List / Calendar"), `query` is the query parameter on the page's own route the choice travels in, `options`
// lists `value=Label` pairs, and `selected` is one of the values. A choice is a link to the same route with `?query=value`,
// so the control never holds a route of its own (007 §12.3: nothing typed that metadata declares) and needs no script;
// the page decides what the parameter means. **That is the navigation shape only.** The Builder mockup's
// `role="radiogroup"` is a choice held in the page and switched by script, which needs `actions` and is not registered.
//
// What the contract forces that a reviewer would not have asked for: a selected value outside the options is refused
// (a control with nothing marked looks like one with no state), and so is a lone option (one choice is not a choice;
// `viewSwitcher` already drew nothing below two).
var segmentedControlContract = domain.ComponentContract{
	Type:   domain.ComponentSegmentedControl,
	Status: domain.ComponentStatusStable,
	Inputs: []domain.ComponentInput{
		{Name: "label", Kind: "string", Required: true},
		{Name: "query", Kind: "string", Required: true},
		{Name: "options", Kind: "string", Required: true},
		// selected and keep are what a page's lowering reads off the request (the value the request carried, the other parameters it
		// carried); a caller that draws one directly passes the selected value itself.
		{Name: "selected", Kind: "string"},
		{Name: "keep", Kind: "string"},
	},
	DataRequirements: nil,
	Slots:            nil,
	Actions:          nil,
	Accessibility:    "a <nav> named by `label`; the selected option is a <span aria-current=\"true\"> and the others are links, so the keyboard reaches every other choice and the current one is announced; a blank label is refused because it is the group's only name",
	Renderer:         "segmentedControl",
}

// validateSegmentedControl checks a use against the declared inputs, then the shape of each.
func validateSegmentedControl(inputs map[string]string) []string {
	issues := checkDeclaredInputs(segmentedControlContract, inputs, "SegmentedControl")
	if v, ok := inputs["label"]; ok && strings.TrimSpace(v) == "" {
		issues = append(issues, "SegmentedControl label is blank -- the group's only accessible name")
	}
	if p, ok := inputs["query"]; ok && p != "" {
		for i, r := range p {
			if !(r >= 'a' && r <= 'z' || r == '_' || i > 0 && r >= '0' && r <= '9') {
				issues = append(issues, fmt.Sprintf("SegmentedControl query %q must be lowercase letters, digits and underscores, starting with a letter", p))
				break
			}
		}
	}
	opts := domain.ParseSegmentOptions(inputs["options"])
	if inputs["options"] != "" && len(opts) < 2 {
		issues = append(issues, "SegmentedControl options hold fewer than two choices -- one choice is not a choice")
	}
	seen := map[string]bool{}
	for _, o := range opts {
		switch {
		case strings.TrimSpace(o.Value) == "" || o.Value != strings.TrimSpace(o.Value) || strings.ContainsAny(o.Value, "&?#% "):
			issues = append(issues, fmt.Sprintf("SegmentedControl option value %q must be non-blank with no spaces or URL punctuation -- it travels in a query string", o.Value))
		case strings.TrimSpace(o.Label) == "":
			issues = append(issues, fmt.Sprintf("SegmentedControl option %q has no label -- write it as value=Label", o.Value))
		case seen[o.Value]:
			issues = append(issues, fmt.Sprintf("SegmentedControl option value %q appears twice", o.Value))
		}
		seen[o.Value] = true
	}
	if sel := inputs["selected"]; sel != "" && !seen[sel] {
		issues = append(issues, fmt.Sprintf("SegmentedControl selected %q is not one of its options", sel))
	}
	return issues
}

// disclosureContract is a panel that opens to show what its summary promises.
//
// **One input, required, and one slot.** The summary is the whole of the closed state, so a disclosure with no words
// is a bar that opens onto something unnamed and is refused. What opens is the `body` slot, composed by the caller:
// the Component never knows it holds a form. **It declares `toggle`, the first Component action that is not a
// write**, and that is the honest size of 007 §12.3's `actions` for a native element: opening and closing are the
// browser's own, so there is no event for a page to wire and nothing a page may attach (`hx-*`, a script hook).
// The native Popover API is declarative too (`popovertarget`, six app-shell triggers, no script), so popovers and the
// menus built on them are the next candidates, as a Component of their own; only a modal `<dialog>` needs script
// (`showModal()`). None of those is covered here.
//
// What the contract forces that a reviewer would not have asked for: the control is a real `<summary>`, so the
// keyboard (Enter, Space) and the open state reach assistive technology without an `aria-expanded` anyone must keep
// in sync.
var disclosureContract = domain.ComponentContract{
	Type:   domain.ComponentDisclosure,
	Status: domain.ComponentStatusStable,
	Inputs: []domain.ComponentInput{
		{Name: "summary", Kind: "string", Required: true},
	},
	DataRequirements: nil,
	Slots:            []string{"body"},
	Actions:          []string{"toggle"},
	Accessibility:    "a native <details> with a <summary>: Enter and Space toggle it, the browser announces the expanded state, and a blank summary is refused because it is the only name the closed panel has",
	Renderer:         "disclosure",
}

// validateDisclosure checks a use against the declared input and refuses a blank summary.
func validateDisclosure(inputs map[string]string) []string {
	issues := checkDeclaredInputs(disclosureContract, inputs, "Disclosure")
	if v, ok := inputs["summary"]; ok && strings.TrimSpace(v) == "" {
		issues = append(issues, "Disclosure summary is blank -- a closed panel with no words is a bar that opens onto something unnamed")
	}
	return issues
}

// emptyStateContract is the message a list shows when it holds nothing, in a dashed box.
//
// **One input, and it is required.** A title, a description and an action are what empty states grow in other design
// systems. Two Workspace Home cards do carry a title and a description, but both sit on one screen, so that second
// shape stays a hand-written floor in `conformance.emptyStateExempt` until another screen needs it. An input with no
// caller is the shape-before-need this registry refuses.
//
// What the contract forces that a reviewer would not have asked for: **a message is required, so an empty state
// cannot be a blank dashed box.** The dashed edge is decoration, not information (colour and an outline never carry
// the meaning), so the words are the whole of it. Both are the Accessibility clause doing work rather than describing
// an absence.
var emptyStateContract = domain.ComponentContract{
	Type:   domain.ComponentEmptyState,
	Status: domain.ComponentStatusStable,
	Inputs: []domain.ComponentInput{
		{Name: "text", Kind: "string", Required: true},
	},
	DataRequirements: nil,
	Slots:            nil,
	Actions:          nil,
	Accessibility:    "the message is the whole of it: the dashed edge is decoration, so neither an outline nor a colour carries the meaning, and a blank message is refused",
	Renderer:         "emptyState",
}

// validateEmptyState checks a use against the declared input and refuses a blank message, which would draw an empty
// dashed box that says nothing.
func validateEmptyState(inputs map[string]string) []string {
	issues := checkDeclaredInputs(emptyStateContract, inputs, "EmptyState")
	if v, ok := inputs["text"]; ok && strings.TrimSpace(v) == "" {
		issues = append(issues, "EmptyState text is blank -- an empty state with no words is a dashed box that says nothing")
	}
	return issues
}

// CatalogueProblems checks what only the whole catalogue can: each contract's own status fields
// (`ComponentContract.StatusProblems`), and that a deprecated Component's successor is a Component this catalogue
// holds and is not itself deprecated -- a chain of "use that one instead" ends at something an author can use.
// Sorted by type, per 007 §4.6.
func CatalogueProblems() []string {
	types := make([]string, 0, len(Components))
	for t := range Components {
		types = append(types, string(t))
	}
	sort.Strings(types)
	var issues []string
	for _, name := range types {
		c := Components[domain.ComponentType(name)].Contract
		issues = append(issues, c.StatusProblems()...)
		if c.Status != domain.ComponentStatusDeprecated || c.ReplacedBy == "" {
			continue
		}
		next, ok := Components[c.ReplacedBy]
		switch {
		case !ok:
			issues = append(issues, fmt.Sprintf("%s is replaced by %q, which is not a registered Component", c.Type, c.ReplacedBy))
		case next.Contract.Status == domain.ComponentStatusDeprecated:
			issues = append(issues, fmt.Sprintf("%s is replaced by %s, which is itself deprecated -- name the Component an author can use", c.Type, c.ReplacedBy))
		}
	}
	return issues
}

// calloutContract is one message in a tinted box: its words and the tone that says what kind of message it is.
//
// **Two inputs, both required.** A title, a list of reasons and a dismiss button are what callouts grow
// elsewhere. None of the six sites has one, and an input with no caller is the shape-before-need this
// registry refuses. The one tinted box that does carry a title and a list (Delete account's blockers) stays
// hand-written, a floor in `conformance.calloutExempt`, until a second one makes the shape real.
//
// What the contract forces that a reviewer would not have asked for: **the live-region role is the tone's,
// not the site's.** Before, one of the six sites said `role="status"`, and the other five said nothing, so a
// screen reader heard "Wrong password." only if it happened to land on it. Now `bad` is `role="alert"` and the
// other three tones are `role="status"`.
var calloutContract = domain.ComponentContract{
	Type:   domain.ComponentCallout,
	Status: domain.ComponentStatusStable,
	Inputs: []domain.ComponentInput{
		{Name: "text", Kind: "string", Required: true},
		{Name: "tone", Kind: "BadgeTone", Required: true},
	},
	DataRequirements: nil,
	Slots:            nil,
	Actions:          nil,
	Accessibility:    `a live region whose role the tone decides: bad is role="alert", info/good/warn are role="status"; the text is the whole message, so colour never carries it alone`,
	Renderer:         "callout",
}

// validateCallout checks a use against the declared inputs and the tones a message may take.
func validateCallout(inputs map[string]string) []string {
	issues := checkDeclaredInputs(calloutContract, inputs, "Callout")
	if v := inputs["tone"]; v != "" && !domain.KnownCalloutTones[domain.BadgeTone(v)] {
		issues = append(issues, fmt.Sprintf("Callout tone %q is not one a message takes (info, good, bad, warn)", v))
	}
	return issues
}

// progressContract is a part of a whole drawn as a bar. `value` and `max` are strings for the reason a Metric's
// value is: the caller resolved them (a Measure, already formatted), and the Component only turns their ratio into
// a width. What the contract forces: a bar always carries its words ("2 of 3") and its label, so the fill is never
// the only thing saying how much -- a percentage-only bar tells a screen-reader user nothing about how many.
var progressContract = domain.ComponentContract{
	Type:   domain.ComponentProgress,
	Status: domain.ComponentStatusStable,
	Inputs: []domain.ComponentInput{
		{Name: "label", Kind: "string", Required: true},
		{Name: "value", Kind: "number", Required: true},
		{Name: "max", Kind: "number", Required: true},
		// size: compact draws the bar and "value/max" only, for a Table cell where the column's header names it (K01b's
		// Instalments "1/3"); the label still names the bar for assistive technology.
		{Name: "size", Kind: "string"},
	},
	DataRequirements: nil,
	Slots:            nil,
	Actions:          nil,
	Accessibility:    `role="progressbar" named by the label, with aria-valuetext "<value> of <max>"; the same words are drawn beside the bar`,
	Renderer:         "progressRow",
}

// plainNumber is a number of zero or more written plainly: digits, optionally a decimal part. strconv.ParseFloat
// would also take NaN, Inf, hex and underscores, each of which would draw a bar that says nothing true.
var plainNumber = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)

func validateProgress(inputs map[string]string) []string {
	issues := checkDeclaredInputs(progressContract, inputs, "Progress")
	if s, ok := inputs["size"]; ok && s != "compact" {
		issues = append(issues, fmt.Sprintf("Progress size %q is not compact", s))
	}
	for _, k := range []string{"value", "max"} {
		v, ok := inputs[k]
		if !ok {
			continue
		}
		if !plainNumber.MatchString(v) {
			issues = append(issues, fmt.Sprintf("Progress %s %q is not a number of zero or more", k, v))
		}
	}
	return issues
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
	Type:   domain.ComponentForm,
	Status: domain.ComponentStatusStable,
	Inputs: []domain.ComponentInput{
		{Name: "submit", Kind: "string", Required: true},
		{Name: "action", Kind: "route", Required: true},
		{Name: "method", Kind: "string"},
		// multipart is lowered when the form asks for a file; carries is a transition form's move (`field=to`), sent as
		// a hidden input. Neither is written.
		{Name: "multipart", Kind: "bool"},
		{Name: "carries", Kind: "string"},
		// then: record opens the record a create Form made (its record page when its Machine has one) instead of
		// redrawing the form.
		{Name: "then", Kind: "string"},
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
	if t, ok := inputs["then"]; ok && t != domain.FormThenRecord {
		issues = append(issues, fmt.Sprintf("Form then %q is not %q -- the one place a create can open is the record it made", t, domain.FormThenRecord))
	}
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
	// A runtime page's Form performs a named operation: a post to /runtime-actions/<name> (with the record it is about).
	if strings.HasPrefix(a, "/runtime-actions/") && method == "" {
		valid = strings.Trim(strings.SplitN(strings.TrimPrefix(a, "/runtime-actions/"), "?", 2)[0], "/") != ""
	}
	// A page's own Form that reads a document (`extract:`) posts to the page: /pages/<nav>/extract sends the document,
	// /pages/<nav>/extract/save submits the form it filled.
	if rest, ok := strings.CutPrefix(a, "/pages/"); ok && method == "" {
		nav, tail, found := strings.Cut(rest, "/")
		valid = found && nav != "" && (tail == strings.TrimPrefix(domain.ExtractRouteSuffix, "/") || tail == strings.TrimPrefix(domain.ExtractSaveSuffix, "/"))
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
	Type:   domain.ComponentFormInput,
	Status: domain.ComponentStatusStable,
	Inputs: []domain.ComponentInput{
		{Name: "id", Kind: "string", Required: true},
		{Name: "name", Kind: "string", Required: true},
		{Name: "kind", Kind: "InputKind", Required: true},
		{Name: "options", Kind: "string"},
		{Name: "value", Kind: "string"},
		{Name: "required", Kind: "bool"},
		// fixed is a value the request supplied (a Form's `values:`): drawn as words in a hidden input, never chosen.
		{Name: "fixed", Kind: "bool"},
		// label names a control that sits under a column header instead of a Field's label (a table of proposed rows): drawn
		// as a visually hidden <label>, so a screen reader still hears what the cell is.
		{Name: "label", Kind: "string"},
		// money is a money Field's control: typed with the Workspace's thousands and decimal marks, sent as a plain decimal.
		{Name: "money", Kind: "bool"},
	},
	DataRequirements: nil,
	Slots:            nil,
	Actions:          nil,
	Accessibility:    "a native control, named by the Field that labels it (or, in a table, by its own hidden `label`); `required` is the native attribute, so a screen reader announces it and the browser refuses an empty submit before the server is asked",
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
	if inputs["options"] != "" && kind != domain.InputSelect && kind != domain.InputMultiSelect {
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
	Type:   domain.ComponentTag,
	Status: domain.ComponentStatusStable,
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
	Type:   domain.ComponentButton,
	Status: domain.ComponentStatusStable,
	Inputs: []domain.ComponentInput{
		{Name: "label", Kind: "string", Required: true},
		{Name: "variant", Kind: "ButtonVariant", Required: true},
		{Name: "name", Kind: "string"},
		{Name: "value", Kind: "string"},
		{Name: "confirm", Kind: "string"},
		{Name: "action", Kind: "string"},
		{Name: "method", Kind: "string"},
		{Name: "describes", Kind: "string"},
	},
	DataRequirements: nil,
	Slots:            nil,
	Actions:          []string{"submit", "delete", "move", "transition"},
	Accessibility:    "the label is the accessible name, so it is required and never an icon alone; the element is a real <button>, focusable and activated by Enter and Space without a role or a script; a delete states its consequence in `confirm`, which the browser asks before anything is sent; a move's label names the direction in words, since an arrow alone is not a name; a transition's label names the status it moves to; a request Button repeated down a list takes `describes`, the record's own name, and the renderer spends it on `aria-label` so no two buttons on a page share a bare name",
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
// `action` and `method` are derived by lowering and arrive together; the verbs are delete, post (a move) and patch (a
// status transition), the writes that ask a person nothing (an edit is a Form). `confirm` is required with a delete,
// and meaningless without one: it is the sentence a person reads before a record is gone. A move and a transition
// are undone by their opposite button, so they ask nothing. A patch carries the Field it sets as the Button's own
// name/value pair, and is the one request Button that does.
func buttonRequestIssues(inputs map[string]string) []string {
	action, method, confirm := inputs["action"], inputs["method"], inputs["confirm"]
	var issues []string
	if method == domain.ButtonMethodPrint || method == domain.ButtonMethodPDF {
		// Printing sends nothing and a PDF asks for the page's own address: no route, no question, no record to name, no pair to post.
		for _, in := range []string{"action", "confirm", "describes", "name", "value"} {
			if inputs[in] != "" {
				issues = append(issues, fmt.Sprintf("a Button that prints sends no request, so it takes no `%s`", in))
			}
		}
		return issues
	}
	if (action == "") != (method == "") {
		issues = append(issues, "Button `action` and `method` are one pair: a route with no verb, or a verb with no route, sends nothing")
	}
	if action == "" {
		if inputs["describes"] != "" {
			issues = append(issues, "Button `describes` names the record a request acts on, and this Button sends none")
		}
		if confirm != "" {
			issues = append(issues, "Button `confirm` asks before a request is sent, and this Button sends none")
		}
		return issues
	}
	switch method {
	case domain.ButtonMethodDelete:
		if confirm == "" {
			issues = append(issues, "a Button that sends a delete states its consequence in `confirm`")
		}
	case domain.ButtonMethodPost:
		// A move asks nothing; a runtime action may ask before it does what cannot be undone.
		if confirm != "" && !strings.HasPrefix(action, "/runtime-actions/") {
			issues = append(issues, "a Button that sends a move asks nothing, so it takes no `confirm`")
		}
	case domain.ButtonMethodPatch:
		// `confirm` is optional: a transition that cannot be taken back (nothing leads out of the status it reaches) asks first.
		if inputs["name"] == "" {
			issues = append(issues, "a Button that sends a status transition carries the Field it sets and the value as its name/value pair")
		}
	default:
		issues = append(issues, fmt.Sprintf("Button method %q is not one the runtime derives (%q, %q or %q)", method, domain.ButtonMethodDelete, domain.ButtonMethodPost, domain.ButtonMethodPatch))
	}
	if inputs["name"] != "" && method != domain.ButtonMethodPatch {
		issues = append(issues, "a Button that sends a delete or a move posts no name/value pair")
	}
	if !strings.HasPrefix(action, "/machines/") && !(strings.HasPrefix(action, "/runtime-actions/") && method == domain.ButtonMethodPost) {
		issues = append(issues, fmt.Sprintf("Button action %q is not a route the runtime derives (/machines/<id>/records/<id>[/move] or a post to /runtime-actions/<name>)", action))
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
	Type:   domain.ComponentField,
	Status: domain.ComponentStatusStable,
	Inputs: []domain.ComponentInput{
		{Name: "label", Kind: "string", Required: true},
		{Name: "for", Kind: "string", Required: true},
		// note and flag belong to a proposed value (a document reader's answer): where it was read, and the words of a
		// warning when it was inferred rather than quoted. Lowering derives them; nobody types them.
		{Name: "note", Kind: "string"},
		{Name: "flag", Kind: "string"},
		// node is the metadata path of the Field the control edits; lowering writes it for a declared form, and a builder preview names the part by it.
		{Name: "node", Kind: "string"},
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
	Type:   domain.ComponentCollection,
	Status: domain.ComponentStatusStable,
	Inputs: []domain.ComponentInput{
		{Name: "gap", Kind: "Gap", Required: true},
		{Name: "empty", Kind: "string"},
		{Name: "ordered", Kind: "bool"},
		{Name: "divided", Kind: "bool"},
		{Name: "block", Kind: "bool"},
		{Name: "inline", Kind: "bool"},
		{Name: "density", Kind: "ListDensity"},
		{Name: "truncated", Kind: "int"},
		{Name: "heading", Kind: "string"},
		{Name: "hint", Kind: "string"},
		{Name: "move_routes", Kind: "string"},
	},
	DataRequirements: nil,
	Slots:            []string{"item"},
	Actions:          nil,
	Accessibility:    "renders a real <ul>/<li>, or an <ol>/<li> when `ordered` is true, so the list and its length (and, ordered, each item's position) are announced without an explicit role (`block` makes each item one full-width block, still a <li>; `divided` changes the look only: the same list element, the rule between rows is a border and not an element) -- the drawn ordinal is aria-hidden for that reason, never the only carrier of the position; with no items and an `empty` text it renders that text in place of an empty list, so a screen-reader user hears why nothing is there rather than an empty list; with a `heading` it first draws that title as a real heading element followed by the number of items as a count badge and the optional `hint`, in that reading order, so a section's size is read out next to its name and a section with no items and no `empty` text is not drawn at all (lowering decides that, `hide_empty`, so a heading never stands over nothing); with `reorder` each row carries a grip that is a real control named for the row: dragging it onto another row, or the Up and Down arrow keys while it has focus, moves the row, and the page that comes back is the proof it saved; with `truncated` (the bound that cut the list short) it adds a paragraph after the list naming that bound, in words, so a reader who cannot see the list's end is told it is not the end",
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
	if v := inputs["block"]; v != "" && v != "true" && v != "false" {
		issues = append(issues, fmt.Sprintf("Collection block %q is not true or false", v))
	}
	if v := inputs["inline"]; v != "" && v != "true" && v != "false" {
		issues = append(issues, fmt.Sprintf("Collection inline %q is not true or false", v))
	}
	if v := inputs["density"]; v != "" {
		if !domain.KnownListDensities[domain.ListDensity(v)] {
			issues = append(issues, fmt.Sprintf("Collection density %q is not one of the declared densities (regular, roomy)", v))
		} else if inputs["divided"] != "true" {
			issues = append(issues, "Collection density sets the padding of a divided list's rows, so it needs divided: true")
		}
	}
	if inputs["inline"] == "true" && (inputs["ordered"] == "true" || inputs["divided"] == "true" || inputs["block"] == "true") {
		issues = append(issues, "Collection inline lays its items in one wrapping run (a row of chips), so it is not also ordered, divided or block")
	}
	if inputs["block"] == "true" && inputs["ordered"] == "true" {
		issues = append(issues, "Collection block draws each item as one full-width block, so it is not also ordered (an ordinal sits beside the item)")
	}
	if v := inputs["truncated"]; v != "" {
		if n, err := strconv.Atoi(v); err != nil || n <= 0 {
			issues = append(issues, fmt.Sprintf("Collection truncated %q is not a positive whole number", v))
		}
	}
	if inputs["hint"] != "" && inputs["heading"] == "" {
		issues = append(issues, "Collection hint qualifies a heading, so it needs one")
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
	Type:   domain.ComponentMetric,
	Status: domain.ComponentStatusStable,
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
	Type:   domain.ComponentStatusBadge,
	Status: domain.ComponentStatusStable,
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
	Type:   domain.ComponentAvatar,
	Status: domain.ComponentStatusStable,
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
	// Required means present; whitespace is present. A label of spaces is an accessible name of nothing, which is the
	// defect the contract's accessibility clause exists to prevent.
	for _, k := range []string{"label", "initials"} {
		if v, ok := inputs[k]; ok && v != "" && strings.TrimSpace(v) == "" {
			issues = append(issues, fmt.Sprintf("Avatar %s is blank -- the circle would carry no name", k))
		}
	}
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

// navListContract is a grouped list of the app's own destinations with one marked as where the viewer is: the settings
// sidebar and the settings hub.
//
// **The rows are an input, and the page never writes them.** A page names the `section`; lowering reads the navigation
// items that declare themselves part of it and produces `rows` (one row per line, five tab-separated cells: group, label,
// description, href, state), so the sidebar, the hub and every screen's own title agree about what exists and a destination
// is declared once. `home` is the hub's route, for the sidebar's way back.
//
// What the contract forces that a reviewer would not have asked for: a **row for a screen that is not built carries no
// href**, so it is drawn as not built and cannot be a link to nowhere (the hand-written sidebar did this by a separate
// function, and nothing held the two to the same rule), and a list with no accessible name is refused, since two navs on one
// page are told apart by it.
var navListContract = domain.ComponentContract{
	Type:   domain.ComponentNavList,
	Status: domain.ComponentStatusStable,
	Inputs: []domain.ComponentInput{
		{Name: "label", Kind: "string", Required: true},
		{Name: "variant", Kind: "string", Required: true},
		{Name: "home", Kind: "string"},
		{Name: "rows", Kind: "string", Required: true},
	},
	DataRequirements: nil,
	Slots:            nil,
	Actions:          nil,
	Accessibility:    "a <nav> named by `label`; the current row carries aria-current=\"page\"; a row for a screen that is not built is plain text with a tag saying so and is not a link; the sidebar variant is not drawn below the sm breakpoint, where the page-wide list is the way in",
	Renderer:         "navList",
}

// validateNavList checks a use against the declared inputs, a non-blank label, a closed variant, and that every row line
// has its five cells, a link unless it is a placeholder, and at most one current row.
func validateNavList(inputs map[string]string) []string {
	issues := checkDeclaredInputs(navListContract, inputs, "NavList")
	if v, ok := inputs["label"]; ok && strings.TrimSpace(v) == "" {
		issues = append(issues, "NavList label is blank -- a navigation with no name is announced as only \"navigation\"")
	}
	if v := inputs["variant"]; v != "" && !domain.KnownNavListVariants[v] {
		issues = append(issues, fmt.Sprintf("NavList variant %q is not sidebar or page", v))
	}
	current := 0
	for _, line := range strings.Split(inputs["rows"], "\n") {
		if line == "" {
			continue
		}
		cells := strings.Split(line, "\t")
		if len(cells) != 5 {
			issues = append(issues, "NavList has a row without its five cells (group, label, description, href, state)")
			continue
		}
		switch cells[4] {
		case "":
		case "current":
			current++
		case "placeholder":
			if cells[3] != "" {
				issues = append(issues, fmt.Sprintf("NavList row %q is not built yet and so has no href", cells[1]))
			}
			continue
		default:
			issues = append(issues, fmt.Sprintf("NavList row %q has the state %q, which is not current or placeholder", cells[1], cells[4]))
		}
		if cells[3] == "" {
			issues = append(issues, fmt.Sprintf("NavList row %q has no href and is not a placeholder", cells[1]))
		}
		if strings.TrimSpace(cells[1]) == "" {
			issues = append(issues, "NavList has a row with no label")
		}
	}
	if current > 1 {
		issues = append(issues, "NavList marks more than one row as current")
	}
	return issues
}

// searchBoxContract is a one-field GET form that narrows the page it sits on.
//
// `label` is the field's accessible name (required: a search field announced as only "search" is the placeholder's
// problem, and a placeholder is not a name), `query` is the query key the words travel under, `placeholder` is the hint,
// and `value` is the words the request already carried, produced by lowering and never written, so a narrowed list never
// shows an empty box over a filtered result. Nothing here narrows anything: the Dataset or runtime source the page reads
// does, from the parameter.
//
// What the contract forces that a reviewer would not have asked for: the key may not be one the runtime owns
// (`application`, `id`, `week`), because a search that overwrote a record page's id would open a different record.
var searchBoxContract = domain.ComponentContract{
	Type:   domain.ComponentSearchBox,
	Status: domain.ComponentStatusStable,
	Inputs: []domain.ComponentInput{
		{Name: "label", Kind: "string", Required: true},
		{Name: "query", Kind: "string", Required: true},
		{Name: "placeholder", Kind: "string"},
		{Name: "value", Kind: "string"},
		{Name: "action", Kind: "string"},
		{Name: "submit", Kind: "string"},
		{Name: "keep", Kind: "string"},
	},
	DataRequirements: nil,
	Slots:            nil,
	Actions:          nil,
	Accessibility:    "a form with role=search around one type=search input named by `label` through a real <label> (visually hidden), submitted with Enter in every browser with no script; the words already in the request fill the field",
	Renderer:         "searchField",
}

// validateSearchBox checks a use against the declared inputs, a non-blank label, and a parameter name that is a plain
// word the runtime does not own.
func validateSearchBox(inputs map[string]string) []string {
	issues := checkDeclaredInputs(searchBoxContract, inputs, "SearchBox")
	if v, ok := inputs["label"]; ok && strings.TrimSpace(v) == "" {
		issues = append(issues, "SearchBox label is blank -- a placeholder is not a name")
	}
	if p := inputs["query"]; p != "" {
		if !searchParamPattern.MatchString(p) {
			issues = append(issues, fmt.Sprintf("SearchBox query %q is not a plain word (letters, digits and underscore, starting with a letter)", p))
		}
		if domain.ParamReserved(p) {
			issues = append(issues, fmt.Sprintf("SearchBox query %q is one the runtime owns", p))
		}
	}
	return issues
}

var searchParamPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// diffContract is one file's unified diff.
//
// `label` is the file's path (required: a diff that does not say which file is a wall of coloured lines) and `text` is the unified
// diff the writer produced; a blank text is a file that would not change, drawn as a sentence and not as an empty box. The Component reads no file and no record; lines are coloured by their first character,
// which is also what says added or removed to a reader who does not see colour.
var diffContract = domain.ComponentContract{
	Type:   domain.ComponentDiff,
	Status: domain.ComponentStatusStable,
	Inputs: []domain.ComponentInput{
		{Name: "label", Kind: "string", Required: true},
		{Name: "text", Kind: "string"},
	},
	DataRequirements: nil,
	Slots:            nil,
	Actions:          nil,
	Accessibility:    "the file's path as text above a <pre> with one row per line; an added line starts with + and a removed one with -, so the change is in the text and not only in the colour; a file that would not change is a sentence, not an empty box",
	Renderer:         "diffView",
}

// validateDiff checks a use against the declared inputs, and a non-blank label.
func validateDiff(inputs map[string]string) []string {
	issues := checkDeclaredInputs(diffContract, inputs, "Diff")
	if v, ok := inputs["label"]; ok && strings.TrimSpace(v) == "" {
		issues = append(issues, "Diff label is blank -- a diff must say which file")
	}
	return issues
}

// iconTileContract is the small square that holds one icon on an Application's tint (Workspace Home's list, the application menu,
// the launcher, the "Add an application" heading). It is a Component because four hand-written `span`s drew it, differing in a size
// and a corner, and because a page could not otherwise put one at the head of a row.
//
// `icon` and `color` are names of closed sets (`domain.KnownIcons`, `domain.KnownApplicationColors`), so a page names a face and
// never a class. `color` is optional and means slate when unset, as an Application that declares none does; `draft` draws the
// dashed outline of an Application nobody has published, a state and not a colour for the reason `AvatarPresence` is. `size`
// is required, because every site chose one and a default would hide a choice.
var iconTileContract = domain.ComponentContract{
	Type:   domain.ComponentIconTile,
	Status: domain.ComponentStatusStable,
	Inputs: []domain.ComponentInput{
		{Name: "icon", Kind: "string", Required: true},
		{Name: "color", Kind: "string"},
		{Name: "size", Kind: "TileSize", Required: true},
		{Name: "state", Kind: "TileState"},
	},
	DataRequirements: nil,
	Slots:            nil,
	Actions:          nil,
	Accessibility:    "decoration: the tile is drawn aria-hidden and carries no name, because the Application's own name is drawn beside it; a use that stands alone without a visible name is a defect of the page, not something the tile can repair",
	Renderer:         "iconTile",
}

func validateIconTile(inputs map[string]string) []string {
	issues := checkDeclaredInputs(iconTileContract, inputs, "IconTile")
	if v := inputs["icon"]; v != "" && !domain.KnownIcons[v] {
		issues = append(issues, fmt.Sprintf("IconTile icon %q is not one of the declared icons", v))
	}
	if v := inputs["color"]; v != "" && !domain.KnownApplicationColors[v] {
		issues = append(issues, fmt.Sprintf("IconTile color %q is not one of the declared Application colours", v))
	}
	if v := inputs["size"]; v != "" && !domain.KnownTileSizes[domain.TileSize(v)] {
		issues = append(issues, fmt.Sprintf("IconTile size %q is not one of the declared sizes", v))
	}
	if v := inputs["state"]; v != "" && !domain.KnownTileStates[domain.TileState(v)] {
		issues = append(issues, fmt.Sprintf("IconTile state %q is not one of the declared states (draft, assistant)", v))
	}
	if inputs["state"] != "" && inputs["color"] != "" {
		issues = append(issues, "IconTile color is an Application's tint, and a tile in a state (draft, assistant) is drawn in the generated-draft indigo, so it takes none")
	}
	return issues
}

// treeContract is §12.3's Selection and Tree in one Component: records listed under the folders their title names, the row the request chose
// marked. Its first reader is the Workspace files screen (a file's path is its place in the tree, `?path=` its selection); the Application
// Builder's tree of an application (Machine → Field, views, menu, workflow) is its next, and nothing about either is in the Component.
//
// **Rows arrive composed** (`item` slot), as a Collection's do, so a row may be a link, a link and a Tag, anything the page can draw. **The
// outline is not written**: lowering reads each record's `title` as a path and writes one line per row, `folder<TAB>selected`, and the
// Component draws a folded group per folder, the rows beneath it, and `aria-current` on the selected one. What forces a contract here that a
// reviewer would not have asked for: a Tree that names no `query` has no selection to mark, and an outline that marks two rows would
// announce two current places, so both are refused.
var treeContract = domain.ComponentContract{
	Type:   domain.ComponentTree,
	Status: domain.ComponentStatusStable,
	Inputs: []domain.ComponentInput{
		{Name: "label", Kind: "string", Required: true},
		{Name: "query", Kind: "string", Required: true},
		{Name: "outline", Kind: "string"},
		{Name: "empty", Kind: "string"},
	},
	DataRequirements: nil,
	Slots:            []string{"item"},
	Actions:          nil,
	Accessibility:    "a <nav> named by `label` holding nested <ul>/<li>; a folder is a native <details> whose <summary> names it, open by default, so the keyboard folds and unfolds it and the list's depth is announced by the nesting; the selected row's <li> carries aria-current=\"true\" and is also marked by weight and fill, never by colour alone; with no rows and an `empty` text it draws that text in place of an empty list",
	Renderer:         "tree",
}

// validateTree checks a use against the declared inputs and that the outline, when present, is a folder and a 0 or 1 per line with at most one 1.
func validateTree(inputs map[string]string) []string {
	issues := checkDeclaredInputs(treeContract, inputs, "Tree")
	selected := 0
	for _, line := range strings.Split(inputs["outline"], "\n") {
		if line == "" {
			continue
		}
		p := strings.Split(line, "\t")
		if len(p) != 2 || (p[1] != "0" && p[1] != "1") {
			issues = append(issues, fmt.Sprintf("Tree outline line %q is not a folder and a 0 or 1", line))
			continue
		}
		if p[1] == "1" {
			selected++
		}
	}
	if selected > 1 {
		issues = append(issues, "Tree outline marks more than one row as selected")
	}
	return issues
}

// frameContract is another page of this host drawn inside the page: the Application Builder's preview of an Application's own screen, the page
// the person is building, shown as it runs. **It is bounded to this host**: `src` must be a path on it (one slash, not two, no scheme), so the
// Component cannot be made to draw another site, and a page that sets it from a record reads a route the runtime produced, not text a person typed.
// `query` names the parameter the frame's own page reports a choice in (a click on a part that names itself, `data-node-path`), and the Component's
// listener turns that report into a request to this page with the parameter set -- the selection of a Tree beside it.
// What the contract forces that a reviewer would not have asked for: the frame has a required accessible title, an address that leaves the host is
// refused, and a `query` is required because a frame that reports nothing back is a picture.
var frameContract = domain.ComponentContract{
	Type:   domain.ComponentFrame,
	Status: domain.ComponentStatusStable,
	Inputs: []domain.ComponentInput{
		{Name: "label", Kind: "string", Required: true},
		{Name: "src", Kind: "string", Required: true},
		{Name: "query", Kind: "string", Required: true},
	},
	DataRequirements: nil,
	Slots:            nil,
	Actions:          nil,
	Accessibility:    "an <iframe> titled by `label`, so a screen reader announces what it holds; the frame's own page is an ordinary page, reached by the keyboard like any other, and a part chosen inside it is chosen again in the list beside it, which is a real list of links",
	Renderer:         "frame",
}

// validateFrame checks the inputs and that `src` stays on this host.
func validateFrame(inputs map[string]string) []string {
	issues := checkDeclaredInputs(frameContract, inputs, "Frame")
	if src := inputs["src"]; src != "" && (!strings.HasPrefix(src, "/") || strings.HasPrefix(src, "//") || strings.Contains(src, "\\")) {
		issues = append(issues, fmt.Sprintf("Frame src %q is not a path on this host", src))
	}
	return issues
}
