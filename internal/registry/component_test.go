package registry

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// TestValidateComponentUse covers the validator the catalogue carries, including the arm that exists for
// §12.3's own sentence about arbitrary properties rather than for symmetry.
func TestValidateComponentUse(t *testing.T) {
	for _, tc := range []struct {
		name   string
		typ    domain.ComponentType
		inputs map[string]string
		want   string // substring the issues must contain; "" means no issue at all
	}{
		{
			name:   "a complete use passes",
			typ:    domain.ComponentStatusBadge,
			inputs: map[string]string{"label": "OVERDUE", "tone": string(domain.ToneBad)},
		},
		{
			name:   "an unset size is allowed, because unset means regular",
			typ:    domain.ComponentStatusBadge,
			inputs: map[string]string{"label": "3", "tone": string(domain.ToneNeutral), "size": string(domain.BadgeCompact)},
		},
		{
			name:   "a size outside the closed set is refused",
			typ:    domain.ComponentStatusBadge,
			inputs: map[string]string{"label": "3", "tone": string(domain.ToneNeutral), "size": "jumbo"},
			want:   `size "jumbo" is not one of the declared sizes`,
		},
		{
			name:   "a missing required input is named",
			typ:    domain.ComponentStatusBadge,
			inputs: map[string]string{"tone": string(domain.ToneGood)},
			want:   `requires input "label"`,
		},
		{
			name:   "a tone outside the closed set is refused",
			typ:    domain.ComponentStatusBadge,
			inputs: map[string]string{"label": "ok", "tone": "chartreuse"},
			want:   `tone "chartreuse" is not one of the declared tones`,
		},
		{
			// The arm that matters for §12.3: an undeclared input is how a bounded Component starts
			// becoming the unbounded GenericComponent that section forbids by name.
			name:   "an undeclared input is refused even when everything required is present",
			typ:    domain.ComponentStatusBadge,
			inputs: map[string]string{"label": "ok", "tone": string(domain.ToneGood), "icon": "clock"},
			want:   `no declared input "icon"`,
		},
		{
			name:   "an unregistered type is refused rather than resolved",
			typ:    domain.ComponentType("SparklineCard"),
			inputs: map[string]string{},
			want:   `"SparklineCard" is not one this runtime realizes`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			issues := ValidateComponentUse(tc.typ, tc.inputs)
			joined := strings.Join(issues, " | ")
			if tc.want == "" {
				if len(issues) != 0 {
					t.Fatalf("ValidateComponentUse() = %v, want no issues", issues)
				}
				return
			}
			if !strings.Contains(joined, tc.want) {
				t.Fatalf("ValidateComponentUse() = %v, want an issue containing %q", issues, tc.want)
			}
		})
	}
}

// TestStatusBadgeContractDeclaresNoDataRequirements asserts the one field the whole slice rests on, rather
// than leaving it to be read.
//
// A `StatusBadge` with a data requirement would be allowed to go and fetch something, which is what
// `rendering.slaBadgePill` did by parsing a date and calling `experience.EvaluateSLA(due, time.Now())`. The
// contract declaring none is what makes `conformance.TestRegisteredComponentsStayBounded` able to refuse a
// renderer signature that takes a record, an `any`, or the clock.
func TestStatusBadgeContractDeclaresNoDataRequirements(t *testing.T) {
	c := Components[domain.ComponentStatusBadge].Contract
	if len(c.DataRequirements) != 0 {
		t.Errorf("StatusBadge declares data requirements %v -- a badge is handed a resolved label and tone; resolution belongs in internal/composition", c.DataRequirements)
	}
	if len(c.Slots) != 0 || len(c.Actions) != 0 {
		t.Errorf("StatusBadge declares slots %v and actions %v -- it is a display-only leaf", c.Slots, c.Actions)
	}
	if c.Accessibility == "" {
		t.Error("StatusBadge declares no accessibility semantics -- §13 lists them, and \"the text is the name\" is an answer worth writing down so the next reader does not add a role it should not have")
	}
}

// TestAvatarContractRequiresAnAccessibleName asserts the clause the second Component existed to find.
//
// `StatusBadge`'s accessibility clause describes an absence -- its text is its name. An avatar's content is
// initials, which are not a name, so `label` is a **required input** rather than an optional nicety. The four
// hand-written sites it replaced carried no accessible name at all.
func TestAvatarContractRequiresAnAccessibleName(t *testing.T) {
	var label *domain.ComponentInput
	for i, in := range Components[domain.ComponentAvatar].Contract.Inputs {
		if in.Name == "label" {
			label = &Components[domain.ComponentAvatar].Contract.Inputs[i]
		}
	}
	if label == nil {
		t.Fatal("Avatar declares no `label` input -- the initials are not an accessible name, so a Component that cannot be given one renders a circle a screen reader announces as two letters")
	}
	if !label.Required {
		t.Error("Avatar's `label` is optional -- an accessibility requirement a caller may skip is not a requirement")
	}
	if issues := ValidateComponentUse(domain.ComponentAvatar, map[string]string{
		"initials": "AP", "size": string(domain.AvatarInline), "presence": string(domain.AvatarPresent),
	}); len(issues) == 0 {
		t.Error("an Avatar use with no label passed validation")
	}
}

// TestBothComponentsShareOneValidatorShape is the answer to the Stage 2 plan's own question -- does the
// contract have to change to hold a second member?
//
// **Structurally it did not**, and that is worth asserting rather than implying: `Inputs` is a slice, so four
// fit where two did, and both validators derive their required/undeclared checks from the declaration through
// one shared helper. A second validator that invented its own conventions would make the catalogue two
// catalogues, which is the failure this checks for.
func TestBothComponentsShareOneValidatorShape(t *testing.T) {
	for typ := range Components {
		undeclared := ValidateComponentUse(typ, map[string]string{"notAThing": "x"})
		var found bool
		for _, s := range undeclared {
			if strings.Contains(s, `no declared input "notAThing"`) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s does not refuse an undeclared input -- every Component must, or boundedness is per-Component taste (007 §12.3)", typ)
		}
	}
	if len(Components) < 2 {
		t.Fatalf("this gate compares Components against each other and the catalogue holds %d", len(Components))
	}
}

// A Metric's optional hint and tone are declared in its contract, and a tone outside the closed set is
// refused -- the same posture StatusBadge takes, since both hand a tone to the Theme's palette lookup.
func TestMetricAcceptsHintAndToneButOnlyDeclaredTones(t *testing.T) {
	if issues := ValidateComponentUse(domain.ComponentMetric, map[string]string{
		"label": "Overdue", "value": "1", "hint": "Past its due date", "tone": string(domain.ToneBad),
	}); len(issues) != 0 {
		t.Errorf("a Metric with hint and a declared tone was refused: %v", issues)
	}
	if issues := ValidateComponentUse(domain.ComponentMetric, map[string]string{
		"label": "Overdue", "value": "1", "tone": "chartreuse",
	}); len(issues) == 0 {
		t.Error("a Metric with an undeclared tone passed validation")
	}
}

// TestValidateButton holds the contract's one cross-input rule and its closed set. `name` and `value` are the
// native pair a submit button posts, and half a pair is a defect with a different symptom each way: a name with
// no value posts an empty string the handler cannot tell from "not clicked", and a value with no name is
// dropped by the browser. A validator that only checked required inputs would pass both.
func TestValidateButton(t *testing.T) {
	ok := map[string]string{"label": "Save", "variant": "primary"}
	if issues := ValidateComponentUse(domain.ComponentButton, ok); len(issues) != 0 {
		t.Errorf("a label and a variant is a valid Button, got %v", issues)
	}
	pair := map[string]string{"label": "Reject", "variant": "danger", "name": "decision", "value": "rejected"}
	if issues := ValidateComponentUse(domain.ComponentButton, pair); len(issues) != 0 {
		t.Errorf("a complete name/value pair is valid, got %v", issues)
	}
	for name, in := range map[string]map[string]string{
		"name without value": {"label": "x", "variant": "primary", "name": "intent"},
		"value without name": {"label": "x", "variant": "primary", "value": "draft"},
		"unknown variant":    {"label": "x", "variant": "ghost"},
		"no label":           {"variant": "primary"},
		"no variant":         {"label": "x"},
		"hx attribute":       {"label": "x", "variant": "primary", "hx-get": "/x"},
		"class":              {"label": "x", "variant": "primary", "class": "mr-1"},
		"action no method":   {"label": "x", "variant": "secondary", "action": "/machines/m/records/r", "confirm": "Sure?"},
		"method no action":   {"label": "x", "variant": "secondary", "method": "delete", "confirm": "Sure?"},
		"no confirm":         {"label": "x", "variant": "secondary", "action": "/machines/m/records/r", "method": "delete"},
		"confirm no request": {"label": "x", "variant": "secondary", "confirm": "Sure?"},
		"unknown verb":       {"label": "x", "variant": "secondary", "action": "/machines/m/records/r", "method": "put", "confirm": "Sure?"},
		"move that asks":     {"label": "x", "variant": "secondary", "action": "/machines/m/records/r/move?direction=up", "method": "post", "confirm": "Sure?"},
		"foreign route":      {"label": "x", "variant": "secondary", "action": "/elsewhere", "method": "delete", "confirm": "Sure?"},
		"patch that asks":    {"label": "x", "variant": "secondary", "action": "/machines/m/records/r", "method": "patch", "name": "f", "value": "v", "confirm": "Sure?"},
		"patch no pair":      {"label": "x", "variant": "secondary", "action": "/machines/m/records/r", "method": "patch"},
		"patch name only":    {"label": "x", "variant": "secondary", "action": "/machines/m/records/r", "method": "patch", "name": "f"},
		"post with a pair":   {"label": "x", "variant": "secondary", "action": "/machines/m/records/r/move?direction=up", "method": "post", "name": "f", "value": "v"},
		"request and name":   {"label": "x", "variant": "secondary", "action": "/machines/m/records/r", "method": "delete", "confirm": "Sure?", "name": "a", "value": "b"},
	} {
		if issues := ValidateComponentUse(domain.ComponentButton, in); len(issues) == 0 {
			t.Errorf("%s must be refused, but validated clean", name)
		}
	}
	move := map[string]string{"label": "Move up", "variant": "secondary", "action": "/machines/m/records/r/move?direction=up", "method": "post"}
	if issues := ValidateComponentUse(domain.ComponentButton, move); len(issues) != 0 {
		t.Errorf("a lowered move Button is valid, got %v", issues)
	}
	transition := map[string]string{"label": "Mark done", "variant": "secondary", "action": "/machines/m/records/r", "method": "patch", "name": "fld_status", "value": "done"}
	if issues := ValidateComponentUse(domain.ComponentButton, transition); len(issues) != 0 {
		t.Errorf("a lowered transition Button is valid, got %v", issues)
	}
	del := map[string]string{"label": "Delete", "variant": "secondary", "action": "/machines/m/records/r", "method": "delete", "confirm": "Delete this record?"}
	if issues := ValidateComponentUse(domain.ComponentButton, del); len(issues) != 0 {
		t.Errorf("a lowered delete Button is valid, got %v", issues)
	}
}

// TestValidateTag holds the palette and the one required input. `color` is optional because an untagged colour
// is a real state the renderer draws in the neutral entry; a *declared* colour outside the palette is a
// mistake, and a validator that checked only required inputs would let `chartreuse` through to be drawn slate.
func TestValidateTag(t *testing.T) {
	for name, in := range map[string]map[string]string{
		"label alone":       {"label": "Bug"},
		"label and palette": {"label": "Bug", "color": "rose"},
	} {
		if issues := ValidateComponentUse(domain.ComponentTag, in); len(issues) != 0 {
			t.Errorf("%s is a valid Tag, got %v", name, issues)
		}
	}
	for _, c := range domain.KnownTagColors {
		if issues := ValidateComponentUse(domain.ComponentTag, map[string]string{"label": "x", "color": string(c)}); len(issues) != 0 {
			t.Errorf("palette entry %q must validate, got %v", c, issues)
		}
	}
	for name, in := range map[string]map[string]string{
		"no label":      {"color": "blue"},
		"unknown color": {"label": "x", "color": "chartreuse"},
		"a hex value":   {"label": "x", "color": "#ff0000"},
		"class":         {"label": "x", "class": "mr-1"},
		"tone":          {"label": "x", "tone": "info"},
	} {
		if issues := ValidateComponentUse(domain.ComponentTag, in); len(issues) == 0 {
			t.Errorf("%s must be refused, but validated clean", name)
		}
	}
}

func TestTagContractRequiresAnAccessibleName(t *testing.T) {
	for _, in := range tagContract.Inputs {
		if in.Name == "label" && !in.Required {
			t.Error("a Tag's label is its accessible name (the dot is aria-hidden); it must be required")
		}
		if in.Name == "color" && in.Required {
			t.Error("color is optional: an uncoloured tag is drawn in the neutral palette entry")
		}
	}
}

// TestCollectionOrderedIsABooleanNotAFreeString: `ordered` is true or false (or absent), so a typo such as
// `ordered: yes` is refused at load instead of silently drawing an unordered list.
func TestCollectionOrderedIsABooleanNotAFreeString(t *testing.T) {
	for _, v := range []string{"", "true", "false"} {
		if issues := ValidateComponentUse(domain.ComponentCollection, map[string]string{"gap": "tight", "ordered": v}); len(issues) != 0 {
			t.Errorf("ordered %q refused: %v", v, issues)
		}
	}
	if issues := ValidateComponentUse(domain.ComponentCollection, map[string]string{"gap": "tight", "ordered": "yes"}); len(issues) == 0 {
		t.Error("ordered: yes passed validation")
	}
}

// TestCollectionDividedIsABooleanNotAFreeString: same posture as `ordered`, so `divided: yes` is refused at load
// instead of silently drawing the floating list.
func TestCollectionDividedIsABooleanNotAFreeString(t *testing.T) {
	for _, v := range []string{"", "true", "false"} {
		if issues := ValidateComponentUse(domain.ComponentCollection, map[string]string{"gap": "tight", "divided": v}); len(issues) != 0 {
			t.Errorf("divided %q refused: %v", v, issues)
		}
	}
	if issues := ValidateComponentUse(domain.ComponentCollection, map[string]string{"gap": "tight", "divided": "yes"}); len(issues) == 0 {
		t.Error("divided: yes passed validation")
	}
}

// TestCollectionTruncatedIsAPositiveWholeNumber: it is the bound a list was cut at, so zero, a negative or
// words name no bound and are refused rather than drawn as a notice about nothing.
func TestCollectionTruncatedIsAPositiveWholeNumber(t *testing.T) {
	for _, v := range []string{"", "1", "200"} {
		if issues := ValidateComponentUse(domain.ComponentCollection, map[string]string{"gap": "tight", "truncated": v}); len(issues) != 0 {
			t.Errorf("truncated %q refused: %v", v, issues)
		}
	}
	for _, v := range []string{"0", "-3", "many", "2.5"} {
		if issues := ValidateComponentUse(domain.ComponentCollection, map[string]string{"gap": "tight", "truncated": v}); len(issues) == 0 {
			t.Errorf("truncated %q passed validation", v)
		}
	}
}

// A Metric that links carries a resolved route, never anything else: a scheme-bearing or protocol-relative
// href would send a viewer off the application from a tile that looks like part of it.
func TestMetricHrefIsARouteOfThisApplication(t *testing.T) {
	base := map[string]string{"label": "draft", "value": "3"}
	with := func(href string) map[string]string {
		m := map[string]string{"href": href}
		for k, v := range base {
			m[k] = v
		}
		return m
	}
	if issues := ValidateComponentUse(domain.ComponentMetric, with("/pages/nav_x?status=draft")); len(issues) != 0 {
		t.Errorf("a route with a query was refused: %v", issues)
	}
	for _, bad := range []string{"https://example.com", "//example.com", "javascript:alert(1)", "pages/x"} {
		if issues := ValidateComponentUse(domain.ComponentMetric, with(bad)); len(issues) == 0 {
			t.Errorf("Metric href %q passed validation", bad)
		}
	}
}

func TestValidateForm(t *testing.T) {
	ok := map[string]string{"submit": "Add label", "action": "/machines/mch_label/records"}
	if issues := ValidateComponentUse(domain.ComponentForm, ok); len(issues) != 0 {
		t.Fatalf("a complete Form was refused: %v", issues)
	}
	with := func(k, v string) map[string]string {
		m := map[string]string{}
		for a, b := range ok {
			m[a] = b
		}
		if v == "" {
			delete(m, k)
		} else {
			m[k] = v
		}
		return m
	}
	for name, tc := range map[string]struct {
		inputs map[string]string
		want   string
	}{
		"no submit label":             {with("submit", ""), `requires input "submit"`},
		"no destination":              {with("action", ""), `requires input "action"`},
		"an external action":          {with("action", "https://example.com/x"), "not a Machine's create route"},
		"a protocol-relative one":     {with("action", "//example.com/machines/m/records"), "not a Machine's create route"},
		"another route of the app":    {with("action", "/machines/mch_label/records/rec_1"), "not a Machine's create route"},
		"a nested machine path":       {with("action", "/machines/a/b/records"), "not a Machine's create route"},
		"an empty machine id":         {with("action", "/machines//records"), "not a Machine's create route"},
		"an undeclared hx property":   {with("hx-post", "/x"), `no declared input "hx-post"`},
		"a patch of the create route": {with("method", "patch"), "not a Machine's create route"},
		"a verb that is not derived":  {with("method", "delete"), "is not one the runtime derives"},
		"a patch of a nested path":    {map[string]string{"submit": "Rename", "method": "patch", "action": "/machines/m/records/r/x"}, "not a Machine's create route"},
		"a patch with no record id":   {map[string]string{"submit": "Rename", "method": "patch", "action": "/machines/m/records/"}, "not a Machine's create route"},
	} {
		t.Run(name, func(t *testing.T) {
			issues := ValidateComponentUse(domain.ComponentForm, tc.inputs)
			if !strings.Contains(strings.Join(issues, "\n"), tc.want) {
				t.Errorf("want an issue containing %q, got %v", tc.want, issues)
			}
		})
	}
}

func TestValidateFormAcceptsAPatchOfOneRecord(t *testing.T) {
	in := map[string]string{"submit": "Rename", "method": "patch", "action": "/machines/mch_label/records/rec_1"}
	if issues := ValidateComponentUse(domain.ComponentForm, in); len(issues) != 0 {
		t.Errorf("a record's patch route was refused: %v", issues)
	}
}

func TestValidateFormInput(t *testing.T) {
	good := []map[string]string{
		{"id": "c0", "name": "fld_name", "kind": "text", "required": "true"},
		{"id": "c1", "name": "fld_color", "kind": "select", "options": "blue\npurple", "value": "blue"},
		{"id": "c2", "name": "fld_done", "kind": "boolean", "value": "true"},
	}
	for _, in := range good {
		if issues := ValidateComponentUse(domain.ComponentFormInput, in); len(issues) != 0 {
			t.Errorf("%v was refused: %v", in, issues)
		}
	}
	for name, tc := range map[string]struct {
		inputs map[string]string
		want   string
	}{
		"no name":                     {map[string]string{"id": "c", "kind": "text"}, `requires input "name"`},
		"no kind":                     {map[string]string{"id": "c", "name": "f"}, `requires input "kind"`},
		"a kind outside the set":      {map[string]string{"id": "c", "name": "f", "kind": "color"}, "is not one of the declared kinds"},
		"options on a text control":   {map[string]string{"id": "c", "name": "f", "kind": "text", "options": "a\nb"}, "belong to a select"},
		"required that is not a bool": {map[string]string{"id": "c", "name": "f", "kind": "text", "required": "yes"}, "is not true or false"},
		"a class":                     {map[string]string{"id": "c", "name": "f", "kind": "text", "class": "x"}, `no declared input "class"`},
		"no id":                       {map[string]string{"name": "f", "kind": "text"}, `requires input "id"`},
	} {
		t.Run(name, func(t *testing.T) {
			issues := ValidateComponentUse(domain.ComponentFormInput, tc.inputs)
			if !strings.Contains(strings.Join(issues, "\n"), tc.want) {
				t.Errorf("want an issue containing %q, got %v", tc.want, issues)
			}
		})
	}
}

func TestFormIsSlottedAndInputIsALeaf(t *testing.T) {
	if got := Components[domain.ComponentForm].Contract.Slots; len(got) != 1 || got[0] != "field" {
		t.Errorf("Form slots = %v, want [field]", got)
	}
	if got := Components[domain.ComponentFormInput].Contract.Slots; len(got) != 0 {
		t.Errorf("Input is a leaf and declares slots %v", got)
	}
}
