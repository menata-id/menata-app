package aiassist

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// copyTree copies src into dst, keeping the relative layout the manifests' "../" paths rely on.
func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// missingKeys lists every mapping key (as a path) present in before and absent from after. A list
// element is matched by its "id" when it has one, so reordering is not a loss. Scalars are not
// compared: the update is allowed to change a value, never to drop a key.
func missingKeys(path string, before, after any) []string {
	var out []string
	switch b := before.(type) {
	case map[string]any:
		a, ok := after.(map[string]any)
		if !ok {
			return []string{path + " (no longer a mapping)"}
		}
		for k, bv := range b {
			av, present := a[k]
			if !present {
				out = append(out, path+"."+k)
				continue
			}
			out = append(out, missingKeys(path+"."+k, bv, av)...)
		}
	case []any:
		a, _ := after.([]any)
		byID := map[string]any{}
		for _, e := range a {
			if m, ok := e.(map[string]any); ok {
				if id, ok := m["id"].(string); ok {
					byID[id] = e
				}
			}
		}
		for i, e := range b {
			m, ok := e.(map[string]any)
			if !ok {
				continue
			}
			id, _ := m["id"].(string)
			if id == "" {
				continue
			}
			ae, present := byID[id]
			if !present {
				out = append(out, path+"["+id+"]")
				continue
			}
			_ = i
			out = append(out, missingKeys(path+"["+id+"]", e, ae)...)
		}
	}
	return out
}

func readGeneric(t *testing.T, p string) any {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var v any
	if err := yaml.Unmarshal(b, &v); err != nil {
		t.Fatalf("%s: %v", p, err)
	}
	return v
}

// TestUpdate_keepsEveryKeyOfAHandWrittenApplication is roadmap stage 1, W1: the assistant is shown
// only the projection DescribeApplication expresses, so the question is whether a real Application
// written by hand (Desain Request in the sample-design-requests Workspace: datasets, card_fields, views, completion,
// send_notification events, `page:` menu items) loses anything when the assistant renames things,
// adds a field and a status option, and rewords a menu item. A key present before and absent after
// is a defect that was alive in production.
func TestUpdate_keepsEveryKeyOfAHandWrittenApplication(t *testing.T) {
	src := filepath.Join("..", "..", "metadata")
	root := t.TempDir()
	copyTree(t, src, root)
	manifest := filepath.Join(root, "samples", "sample-design-requests.yaml")
	wsDir := filepath.Join(root, "samples", "sample-design-requests")
	files := []string{
		filepath.Join(wsDir, "design_request.yaml"),
		filepath.Join(wsDir, "applications", "design_request.yaml"),
	}
	before := make([]any, len(files))
	for i, f := range files {
		before[i] = readGeneric(t, f)
	}

	state, _ := stateOf(t, manifest)
	desired := DescribeApplication("app_design_request", state.Applications["app_design_request"])
	desired.Name = "Desain Request Baru"
	for i := range desired.Machines {
		if desired.Machines[i].ID != "mch_design_request" {
			continue
		}
		desired.Machines[i].Name = "Request Desain Baru"
		desired.Machines[i].Fields = append(desired.Machines[i].Fields, GeneratedField{ID: "fld_catatan_baru", Name: "Catatan Baru", Type: "text"})
		for j := range desired.Machines[i].Fields {
			if desired.Machines[i].Fields[j].Type == "status" {
				desired.Machines[i].Fields[j].Options = append(desired.Machines[i].Fields[j].Options, "ditinjau_ulang")
			}
		}
	}
	for i := range desired.Navigation {
		desired.Navigation[i].Label += " (baru)"
	}
	change := GeneratedChange{Kind: KindUpdateApplication, TargetAppID: "app_design_request", Application: &desired}
	publish(t, manifest, change)

	// The probe is vacuous unless the update really happened.
	appAfter := readGeneric(t, files[1]).(map[string]any)
	if appAfter["name"] != "Desain Request Baru" {
		t.Fatalf("the update did not rename the Application: %v", appAfter["name"])
	}
	machAfter, _ := os.ReadFile(files[0])
	for _, want := range []string{"fld_catatan_baru", "ditinjau_ulang", "Request Desain Baru"} {
		if !strings.Contains(string(machAfter), want) {
			t.Fatalf("the update did not write %q into the Machine file", want)
		}
	}
	for i, f := range files {
		after := readGeneric(t, f)
		if lost := missingKeys(filepath.Base(f), before[i], after); len(lost) > 0 {
			t.Errorf("the update dropped keys from %s:\n  %v", f, lost)
		}
	}
}

// TestUpdate_refusesToDropAPageMenuItem: a menu item that opens a declared `page:` is shown to the
// model with no Machine (it opens "a screen that could not be added back"), so a model that leaves
// it out of the desired menu is refused rather than deleting a screen the assistant cannot rewrite.
func TestUpdate_refusesToDropAPageMenuItem(t *testing.T) {
	root := t.TempDir()
	copyTree(t, filepath.Join("..", "..", "metadata"), root)
	manifest := filepath.Join(root, "samples", "sample-design-requests.yaml")
	state, _ := stateOf(t, manifest)
	desired := DescribeApplication("app_design_request", state.Applications["app_design_request"])
	var kept []GeneratedMenuItem
	for _, n := range desired.Navigation {
		if n.MachineID != "" {
			kept = append(kept, n)
		}
	}
	if len(kept) == len(desired.Navigation) {
		t.Fatal("the fixture has no page menu item, so the test proves nothing")
	}
	desired.Navigation = kept
	change := GeneratedChange{Kind: KindUpdateApplication, TargetAppID: "app_design_request", Application: &desired}
	if plan := PlanUpdate(DescribeApplication("app_design_request", state.Applications["app_design_request"]), desired); len(plan.Refusals) == 0 {
		t.Fatalf("dropping page menu items was not refused: %+v", plan)
	}
	if _, err := Write(manifest, change, FileMachineResolver{WorkspaceManifestPath: manifest}); err == nil {
		t.Fatal("Write accepted an update that drops page menu items")
	}
}

// TestDesainRequestRoundTripsThroughTheAssistant is roadmap stage 1's closing measurement for the reference
// Application. Describing it and handing the description back changes nothing (an empty plan, no refusal), and the
// keys the assistant learned later (a step's roles, a conditionally required field, a status Field's derived or
// stamped values) are in the description, so a model that returns it unchanged cannot drop them.
func TestDesainRequestRoundTripsThroughTheAssistant(t *testing.T) {
	root := t.TempDir()
	copyTree(t, filepath.Join("..", "..", "metadata"), root)
	manifest := filepath.Join(root, "samples", "sample-design-requests.yaml")
	state, _ := stateOf(t, manifest)
	current := DescribeApplication("app_design_request", state.Applications["app_design_request"])
	if plan := PlanUpdate(current, current); len(plan.Items) != 0 || len(plan.Refusals) != 0 {
		t.Fatalf("returning the description unchanged is not an empty plan: %+v", plan)
	}
	var m GeneratedMachine
	for _, x := range current.Machines {
		if x.ID == "mch_design_request" {
			m = x
		}
	}
	gated := 0
	for _, tr := range m.Transitions {
		if len(tr.Roles) > 0 {
			gated++
		}
	}
	if gated != len(m.Transitions) || gated == 0 {
		t.Errorf("%d of %d status moves carry their roles in the description, want all", gated, len(m.Transitions))
	}
	var ref GeneratedField
	for _, f := range m.Fields {
		if f.ID == "fld_reference_file" {
			ref = f
		}
	}
	if ref.RequiredWhen == nil || ref.RequiredWhen.Field != "fld_format" || ref.RequiredWhen.Equals != "banner_2_1" {
		t.Errorf("the reference file's required_when is not described: %+v", ref.RequiredWhen)
	}
}

// What the assistant cannot yet write for Desain Request, measured by generating a twin from the description and
// listing the hand-written file's keys (at any depth) the twin lacks. Shrink-only: a key leaving this list must come out of the
// map, and a new one means something the assistant did not lose before is lost now.
var desainRequestHandOnly = map[string]string{
	"datasets": "free-form Datasets (counts by status, a status filter) are written only as the list or status page behind a menu entry",
}

func TestDesainRequestTwinLacksOnlyWhatIsListed(t *testing.T) {
	root := t.TempDir()
	copyTree(t, filepath.Join("..", "..", "metadata"), root)
	manifest := filepath.Join(root, "samples", "sample-design-requests.yaml")
	state, _ := stateOf(t, manifest)
	desc := DescribeApplication("app_design_request", state.Applications["app_design_request"])
	desc.ID = "app_design_request_twin"
	desc.Name = "Twin"
	desc.PublisherRole = desc.Roles[0]
	for i := range desc.Machines {
		desc.Machines[i].ID += "_twin"
	}
	var nav []GeneratedMenuItem
	for _, n := range desc.Navigation {
		if n.MachineID == "" {
			continue // a hand-written page: menu item; the assistant cannot add one back
		}
		n.MachineID += "_twin"
		n.ID = ""
		nav = append(nav, n)
	}
	desc.Navigation = nav
	for i := range desc.Machines {
		for j := range desc.Machines[i].Fields {
			if f := &desc.Machines[i].Fields[j]; f.RelatedMachine != "" && f.RelatedMachine != "mch_user" {
				f.RelatedMachine += "_twin"
			}
		}
	}
	change := GeneratedChange{Kind: KindNewApplication, Application: &desc}
	publish(t, manifest, change)

	hand := readGeneric(t, filepath.Join(root, "samples", "sample-design-requests", "design_request.yaml")).(map[string]any)
	twin := readGeneric(t, filepath.Join(root, "samples", "sample-design-requests", "design_request_twin.yaml")).(map[string]any)
	// Compared at every depth, not only at the top: a step's roles or an event's recipient lost inside a kept block
	// would show here as a path.
	got := map[string]bool{}
	for _, lost := range missingKeys("design_request", hand, twin) {
		got[strings.TrimPrefix(lost, "design_request.")] = true
	}
	for k := range got {
		if desainRequestHandOnly[k] == "" {
			t.Errorf("the twin generated from the description lacks %q, which is not a known gap", k)
		}
	}
	for k := range desainRequestHandOnly {
		if !got[k] {
			t.Errorf("%q is listed as a gap but the twin has it now -- remove the entry", k)
		}
	}
}

// TestUpdate_keepsATreeComputeAndAParentBound (the review of step 3): Production Contracts declares computed values as
// trees (`{subtract: [...]}`, `sum_children`) and an ensure bounded by the parent, which the assistant's flat shapes cannot
// carry. An update that changes something else is accepted, and the file keeps both exactly as written.
func TestUpdate_keepsATreeComputeAndAParentBound(t *testing.T) {
	root := t.TempDir()
	copyTree(t, filepath.Join("..", "..", "metadata"), root)
	manifest := filepath.Join(root, "samples", "sample-contracts.yaml")
	wsDir := filepath.Join(root, "samples", "sample-contracts")
	files := []string{filepath.Join(wsDir, "contract.yaml"), filepath.Join(wsDir, "contract_term.yaml")}
	before := make([]any, len(files))
	for i, f := range files {
		before[i] = readGeneric(t, f)
	}
	state, _ := stateOf(t, manifest)
	desired := DescribeApplication("app_production_contracts", state.Applications["app_production_contracts"])
	desired.Description = "Contracts, their instalments, and what is paid."
	change := GeneratedChange{Kind: KindUpdateApplication, TargetAppID: "app_production_contracts", Application: &desired}
	publish(t, manifest, change)
	for i, f := range files {
		after := readGeneric(t, f)
		if lost := missingKeys(filepath.Base(f), before[i], after); len(lost) > 0 {
			t.Errorf("the update dropped keys from %s:\n  %v", f, lost)
		}
	}
	term, _ := os.ReadFile(files[1])
	if !strings.Contains(string(term), "subtract: [fld_gross, fld_vat, fld_wht]") || !strings.Contains(string(term), "parent: fld_contract.fld_value") {
		t.Errorf("the instalment's tree compute or parent bound changed:\n%s", term)
	}
}
