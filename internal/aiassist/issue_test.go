package aiassist

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"menata.app/internal/domain"
	"menata.app/internal/metadata"
)

// addressShape is what an Issue.Address may be: the application, or a part the Apps Builder's tree names
// (web/builderscreen.go), or a view/transition of a Machine, or a menu entry by id or by its place when it is new.
var addressShape = regexp.MustCompile(`^(application|machines/[^/]+(/(fields|permissions|transitions|events|views)/[^/]+)?|navigation/(#[0-9]+|[^/#]+))$`)

// addressMakers are the only things an Issue's address may be built by.
var addressMakers = map[string]bool{"machineAt": true, "fieldAt": true, "permissionAt": true, "transitionAt": true, "eventAt": true, "viewAt": true, "menuAt": true}

// TestEveryIssueIsMadeWithAnAddress holds the shape of the check that makes a fault: it is made by at, atf or
// Plan.refuse, whose first argument is the address, and that argument is AddressApplication, a name that holds one
// (addr, address) or a call to one of the *At makers -- never a literal, so a new check cannot say a fault without
// saying where it is. An Issue is also never written as a composite literal outside issue.go.
//
// Reads the sources (not the behaviour), so it sees a check no test happens to reach. Mutation-proved 2026-11-19:
// an `atf("", ...)` in validate.go, a bare `Issue{Message: ...}` in lists.go, and a `p.refuse("x", ...)` in plan.go
// each fail it.
func TestEveryIssueIsMadeWithAnAddress(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	checked := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CompositeLit:
				if id, ok := x.Type.(*ast.Ident); ok && id.Name == "Issue" && name != "issue.go" {
					t.Errorf("%s: an Issue is made by at/atf, not written as a literal", fset.Position(x.Pos()))
				}
			case *ast.CallExpr:
				var callee string
				switch f := x.Fun.(type) {
				case *ast.Ident:
					callee = f.Name
				case *ast.SelectorExpr:
					callee = f.Sel.Name
				}
				if callee != "at" && callee != "atf" && callee != "refuse" {
					return true
				}
				if name == "issue.go" && callee != "refuse" {
					return true // the definitions of at/atf and the helpers that build on them
				}
				checked++
				if len(x.Args) == 0 || !addressArgument(x.Args[0]) {
					t.Errorf("%s: the first argument of %s must be an address (AddressApplication, addr, address or an *At call)", fset.Position(x.Pos()), callee)
				}
			}
			return true
		})
	}
	if checked < 100 {
		t.Errorf("only %d issue-making calls were found; the gate no longer reads the checks (was well over 100 on 2026-11-19)", checked)
	}
}

func addressArgument(e ast.Expr) bool {
	switch a := e.(type) {
	case *ast.Ident:
		return a.Name == "AddressApplication" || a.Name == "addr" || a.Name == "address"
	case *ast.CallExpr:
		id, ok := a.Fun.(*ast.Ident)
		return ok && addressMakers[id.Name]
	}
	return false
}

// issuesOf is the issues of an error from Validate.
func issuesOf(t *testing.T, err error) []Issue {
	t.Helper()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("error = %v (%T), want a *ValidationError", err, err)
	}
	return ve.Issues
}

// findIssue is the issue whose message holds the words, failing the test when there is none.
func findIssue(t *testing.T, issues []Issue, words string) Issue {
	t.Helper()
	for _, is := range issues {
		if strings.Contains(is.Message, words) {
			return is
		}
	}
	t.Fatalf("no issue says %q; got %v", words, Messages(issues))
	return Issue{}
}

func TestValidateSaysWhereEachFaultIs(t *testing.T) {
	cases := []struct {
		name    string
		edit    func(*GeneratedChange)
		words   string
		address string
		allowed []string // every one must be offered
	}{
		{"a permission naming a role nobody declares", func(c *GeneratedChange) {
			c.Application.Machines[0].Permissions[1].Roles = []string{"Intern"}
		}, `names role "Intern"`, "machines/mch_leave_request/permissions/prm_edit", []string{"Employee", "Supervisor", "HR admin"}},
		{"a step naming a role nobody declares", func(c *GeneratedChange) {
			c.Application.Machines[0].Transitions[0].Roles = []string{"Intern"}
		}, `step "trn_approve" names role`, "machines/mch_leave_request/transitions/trn_approve", []string{"Employee", "HR admin"}},
		{"an unknown icon", func(c *GeneratedChange) { c.Application.Icon = "no-such-icon" }, "is not a known icon", "application", []string{declarableIconEnum()[0]}},
		{"an unknown colour", func(c *GeneratedChange) { c.Application.Color = "no-such-colour" }, "is not a known application color", "application", []string{applicationColorEnum()[0]}},
		{"a publisher role that is not a role", func(c *GeneratedChange) { c.Application.PublisherRole = "Boss" }, "publisher_role", "application", []string{"HR admin"}},
		{"a relation to a machine that will not exist", func(c *GeneratedChange) {
			m := &c.Application.Machines[0]
			m.Fields = append(m.Fields, GeneratedField{ID: "fld_boss", Name: "Boss", Type: "relation", RelatedMachine: "mch_nothing"})
		}, "relation target", "machines/mch_leave_request/fields/fld_boss", []string{"mch_leave_request"}},
		{"a board grouped by a field that is not a status", func(c *GeneratedChange) {
			m := &c.Application.Machines[0]
			m.CardFields = []GeneratedCardField{{Field: "fld_title", Role: "title"}}
			m.Views = []GeneratedView{{ID: "view_board", Name: "Board", Type: "board", GroupBy: "fld_title"}}
		}, "a board is grouped by a status field", "machines/mch_leave_request/views/view_board", []string{"fld_status"}},
		{"an event service nobody may name", func(c *GeneratedChange) {
			c.Application.Machines[0].Events[0].Service = "launch_missiles"
		}, "is not one a generated event may name", "machines/mch_leave_request/events/evt_submitted", []string{"log_activity", "send_notification", "set_field"}},
		{"a menu entry opening a machine nobody declares", func(c *GeneratedChange) {
			c.Application.Navigation = append(c.Application.Navigation, GeneratedMenuItem{Label: "Ghost", MachineID: "mch_ghost"})
		}, "which this application does not declare", "navigation/#2", []string{"mch_leave_request"}},
		{"a list with a scope the runtime lacks", func(c *GeneratedChange) {
			c.Application.Navigation = append(c.Application.Navigation, GeneratedMenuItem{Label: "Mine", MachineID: "mch_leave_request", List: &GeneratedList{Scope: "per-role", Limit: 10}})
		}, "scope must be", "navigation/#2", nil},
		{"two machines with one id", func(c *GeneratedChange) {
			c.Application.Machines = append(c.Application.Machines, c.Application.Machines[0])
		}, "is declared more than once in this application", "machines/mch_leave_request", nil},
		{"no name", func(c *GeneratedChange) { c.Application.Name = " " }, "application name is required", "application", nil},
		{"a field only a role nobody declares may change", func(c *GeneratedChange) {
			c.Application.Machines[0].Fields[0].EditableBy = []string{"Intern"}
		}, "editable_by names role", "machines/mch_leave_request/fields/fld_title", []string{"Employee", "Supervisor", "HR admin"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			change := validLeaveRequestChange()
			tc.edit(&change)
			issues := issuesOf(t, Validate(change, ExistingState{}))
			for _, is := range issues {
				if !addressShape.MatchString(is.Address) {
					t.Errorf("issue %q has address %q, which is not a part of an application", is.Message, is.Address)
				}
			}
			got := findIssue(t, issues, tc.words)
			if got.Address != tc.address {
				t.Errorf("address = %q, want %q (%s)", got.Address, tc.address, got.Message)
			}
			for _, want := range tc.allowed {
				if !slices.Contains(got.Allowed, want) {
					t.Errorf("accepted values %v lack %q", got.Allowed, want)
				}
			}
		})
	}
}

// The update path says where too: the refusals of the policy are Issues, and so are the checks run on the whole
// application the model returns.
func TestPlanRefusalsSayWhereTheyAre(t *testing.T) {
	current := GeneratedApplication{
		ID: "app_x", Name: "X", Roles: []string{"a", "b"},
		Machines: []GeneratedMachine{{
			ID: "mch_x", Name: "X",
			Fields: []GeneratedField{
				{ID: "fld_t", Name: "T", Type: "text"},
				{ID: "fld_s", Name: "S", Type: "status", Options: []string{"new", "done"}},
			},
			Permissions: []GeneratedPermission{{ID: "prm_e", Action: "edit", Roles: []string{"a"}}},
		}},
		Navigation: []GeneratedMenuItem{{ID: "nav_x", Label: "X", MachineID: "mch_x"}},
	}
	retype := func() GeneratedApplication {
		d := current
		d.Machines = []GeneratedMachine{current.Machines[0]}
		d.Machines[0].Fields = []GeneratedField{{ID: "fld_t", Name: "T", Type: "number"}, current.Machines[0].Fields[1]}
		return d
	}
	dropOption := func() GeneratedApplication {
		d := current
		d.Machines = []GeneratedMachine{current.Machines[0]}
		d.Machines[0].Fields = []GeneratedField{current.Machines[0].Fields[0], {ID: "fld_s", Name: "S", Type: "status", Options: []string{"new"}}}
		return d
	}
	dropPermission := func() GeneratedApplication {
		d := current
		d.Machines = []GeneratedMachine{current.Machines[0]}
		d.Machines[0].Permissions = nil
		return d
	}
	dropMachine := func() GeneratedApplication {
		d := current
		d.Machines = nil
		return d
	}
	dropRole := func() GeneratedApplication {
		d := current
		d.Roles = []string{"a"}
		return d
	}
	for _, tc := range []struct {
		name    string
		desired GeneratedApplication
		words   string
		address string
	}{
		{"retyping a field", retype(), "cannot change type", "machines/mch_x/fields/fld_t"},
		{"dropping an option", dropOption(), "cannot be removed: records may hold it", "machines/mch_x/fields/fld_s"},
		{"dropping a permission", dropPermission(), "would become open to everyone", "machines/mch_x/permissions/prm_e"},
		{"dropping a machine", dropMachine(), "cannot be removed", "machines/mch_x"},
		{"dropping a role", dropRole(), "everyone holding it would lose access", "application"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := PlanUpdate(current, tc.desired)
			got := findIssue(t, plan.Refusals, tc.words)
			if got.Address != tc.address {
				t.Errorf("address = %q, want %q (%s)", got.Address, tc.address, got.Message)
			}
		})
	}
}

// The turn the automatic check sends names each fault's place and what would have been accepted, and still says the
// faults in the words the model has always been sent.
func TestCheckReportNamesPlaceAndAcceptedValues(t *testing.T) {
	change := validLeaveRequestChange()
	change.Application.Machines[0].Permissions[1].Roles = []string{"Intern"}
	err := Validate(change, ExistingState{})
	report := CheckReport(err)
	for _, want := range []string{
		"Automatic check:",
		"- machines/mch_leave_request/permissions/prm_edit: ",
		`names role "Intern"`,
		"(accepted: Employee, Supervisor, HR admin)",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report lacks %q:\n%s", want, report)
		}
	}
	// A plain error (not a ValidationError) is reported by its own words, not lost.
	if got := CheckReport(os_err("the change is empty")); !strings.Contains(got, "the change is empty") {
		t.Errorf("a plain error vanished from the report: %s", got)
	}
}

type os_err string

func (e os_err) Error() string { return string(e) }

// An Issue copies the values it offers, so a later edit of the caller's slice cannot change an error already made.
func TestIssueCopiesWhatItOffers(t *testing.T) {
	roles := []string{"a", "b"}
	is := at(AddressApplication, "x").allowing(roles...)
	roles[0] = "changed"
	if is.Allowed[0] != "a" {
		t.Errorf("an issue aliased the caller's slice: %v", is.Allowed)
	}
}

// editable_by names roles the Application declares. The loader refuses an undeclared one, but only at load -- after the
// review the person approved (measured 2026-11-19: Validate let it through) -- so the assistant is told in the conversation,
// on a new Application and on an update alike, and a declared role is not refused.
func TestEditableByIsJudgedWhereTheAssistantCanStillFixIt(t *testing.T) {
	change := validLeaveRequestChange()
	change.Application.Machines[0].Fields[0].EditableBy = []string{"Supervisor"}
	if err := Validate(change, ExistingState{}); err != nil {
		t.Fatalf("a declared role was refused: %v", err)
	}

	manifest := installedWasteApp(t)
	state, _ := stateOf(t, manifest)
	ok := update(t, manifest, func(a *GeneratedApplication) {
		if len(a.Roles) > 0 {
			a.Machines[0].Fields[0].EditableBy = []string{a.Roles[0]}
		}
	})
	if err := Validate(ok, state); err != nil && strings.Contains(err.Error(), "editable_by names role") {
		t.Errorf("a declared role was refused on an update: %v", err)
	}
	bad := update(t, manifest, func(a *GeneratedApplication) { a.Machines[0].Fields[0].EditableBy = []string{"no-such-role"} })
	got := findIssue(t, issuesOf(t, Validate(bad, state)), "editable_by names role")
	if want := "machines/" + bad.Application.Machines[0].ID + "/fields/" + bad.Application.Machines[0].Fields[0].ID; got.Address != want {
		t.Errorf("address = %q, want %q", got.Address, want)
	}
	if !slices.Equal(got.Allowed, bad.Application.Roles) {
		t.Errorf("accepted values %v, want the application's roles %v", got.Allowed, bad.Application.Roles)
	}
}

// TestLoaderFaultsAreSplitAndAddressed holds what LoaderIssues does with what the loader says: one Issue per sentence
// (not the whole error wrapped into one), each at the part its opening names, and every sentence naming its Machine.
// Real loader faults, not made-up sentences, so a change to the loader's wording that the reader no longer understands
// fails here. Mutation-proved 2026-11-19 (see development-history.md).
func TestLoaderFaultsAreSplitAndAddressed(t *testing.T) {
	m := &domain.Machine{ID: "mch_x", Name: "X", Fields: []domain.Field{
		{ID: "fld_a", Name: "A", Type: domain.FieldTypeStatus},
		{ID: "fld_b", Name: "B", Type: "no_such_type"},
	}, Views: []domain.View{{ID: "vw_z", Name: "Z", Type: "no_such_view"}}}
	issues := LoaderIssues(m.ID, metadata.Validate(m))
	want := map[string]string{
		"machines/mch_x/fields/fld_a": "requires at least one option",
		"machines/mch_x/fields/fld_b": "unknown type",
		"machines/mch_x/views/vw_z":   "unknown type",
	}
	got := map[string]string{}
	for _, is := range issues {
		if !addressShape.MatchString(is.Address) {
			t.Errorf("address %q is not a part of the tree", is.Address)
		}
		if !strings.HasPrefix(is.Message, `machine "mch_x": `) {
			t.Errorf("%q does not name its Machine", is.Message)
		}
		if strings.Contains(is.Message, "invalid metadata") {
			t.Errorf("the loader's wrapper was not split: %q", is.Message)
		}
		got[is.Address] = is.Message
	}
	for address, words := range want {
		if !strings.Contains(got[address], words) {
			t.Errorf("%s: want a fault saying %q, got %q (all: %v)", address, words, got[address], got)
		}
	}
	// An error that is not the loader's own list is one Issue at the Machine.
	one := LoaderIssues("mch_x", errors.New("boom"))
	if len(one) != 1 || one[0].Address != "machines/mch_x" || !strings.Contains(one[0].Message, "boom") {
		t.Errorf("a plain error: %+v", one)
	}
}
