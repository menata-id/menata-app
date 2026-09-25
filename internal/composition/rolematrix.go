package composition

import (
	"fmt"
	"strings"

	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// recordActions are the actions a Machine's own block enumerates, in this order.
//
// domain.ActionDecide is deliberately absent: it performs a declared Transition, which already
// has a row of its own, and listing it twice would let one row say something the other does not.
var recordActions = []string{domain.ActionCreate, domain.ActionEdit, domain.ActionDelete}

// RoleMatrix composes the Authorization Matrix (ui-sample/case-03-flow1/06b-authorization-matrix.
// html): who can do what here, written for someone who does not read YAML.
//
// Two sections, Workspace then Applications, which is the owner's own structure (2026-09-21) and
// a correction of the first draft's. That draft split by mechanism -- "Transitions" and "Record
// actions" -- which asked the reader to know what a transition is before they could find out what
// they may do, and buried the two rows that matter under six a person can never be granted.
// Splitting by *scope* instead matches how the rules are actually decided, and matches how the
// owner's own member-role-detail.html already presents a person's access: a Workspace role, then
// one role per Application.
//
// It is a pure projection over already-loaded metadata -- no database, no request, no identity.
// Every cell has to be derivable from a declaration that already gates a real write; anything
// this screen could say that authorization.AllowsAction would not agree with is a bug here rather
// than a richer view. The one row that is *not* such a projection is marked as such on the page
// (see workspaceSection).
//
// `all` is the Workspace's whole Machine list in declaration order -- not the id-keyed map -- so
// row order is the manifest's rather than a map's iteration order.
func RoleMatrix(apps []domain.Application, all []*domain.Machine) rendering.RoleMatrixView {
	byID := make(map[string]*domain.Machine, len(all))
	for _, m := range all {
		byID[m.ID] = m
	}
	v := rendering.RoleMatrixView{Workspace: workspaceSection(all)}
	for _, app := range apps {
		v.Applications = append(v.Applications, applicationBlock(app, byID))
	}
	return v
}

// applicationBlock is one Application's own card: its roles, what each may do grouped by the thing
// being acted on, and a plain sentence for the status changes nobody performs.
func applicationBlock(app domain.Application, byID map[string]*domain.Machine) rendering.RoleMatrixApp {
	block := rendering.RoleMatrixApp{
		ID:           app.ID,
		Name:         app.Name,
		Description:  app.Description,
		Roles:        app.Roles,
		RolesSummary: rolesSummary(app.Roles),
	}
	if len(app.Roles) == 0 {
		return block
	}
	// Entry first, because it is the only row that says what a role lets you *see*, and without it
	// a role granted nothing else reads as a role granted nothing at all -- which is how the first
	// version of this page showed `reviewer`: an empty column, for a role whose whole point is
	// looking.
	//
	// It is derived rather than typed: an Application declaring `roles:` is gated on holding one
	// (internal/web.requireApplicationAccess), so this row exists exactly when that list is
	// non-empty, and every declared role grants it.
	block.Groups = append(block.Groups, rendering.RoleMatrixGroup{
		Label: "Access",
		Rows: []rendering.RoleMatrixRow{{
			Action:  "See " + app.Name + " at all",
			Actions: []string{"enter"},
			Qualifier: "Any role here. Someone with no role in " + app.Name +
				" cannot open a single one of its screens, including looking at a document.",
			Who: "All roles",
		}},
	})
	for _, machineID := range app.Machines {
		m, ok := byID[machineID]
		if !ok {
			continue
		}
		group := rendering.RoleMatrixGroup{Label: m.Name}
		for _, t := range m.Transitions {
			if t.Action == "" {
				// A move no Action performs is collected into the block's own sentence below
				// instead of being a row: six rows nobody can ever be granted drowned the two
				// that matter, which is what the first draft of this screen actually looked like.
				block.Automatic = append(block.Automatic, automaticNote(m))
				continue
			}
			group.Rows = append(group.Rows, actionRow(m, t.Name, t.Action, []string{t.Action}, app.Roles))
		}
		group.Rows = append(group.Rows, foldIdentical(recordActionRows(m, app.Roles))...)
		if len(group.Rows) > 0 {
			block.Groups = append(block.Groups, group)
		}
	}
	block.Automatic = dedupe(block.Automatic)
	return block
}

// recordActionRows is one row per create/edit/delete on m, before folding.
func recordActionRows(m *domain.Machine, roles []string) []rendering.RoleMatrixRow {
	rows := make([]rendering.RoleMatrixRow, 0, len(recordActions))
	for _, act := range recordActions {
		if m.AppendOnly && (act == domain.ActionEdit || act == domain.ActionDelete) {
			rows = append(rows, rendering.RoleMatrixRow{
				Action:    verbFor([]string{act}),
				Actions:   []string{act},
				Qualifier: "No one, not even an admin — records here are added, never changed or removed.",
				Who:       "No one",
			})
			continue
		}
		rows = append(rows, actionRow(m, verbFor([]string{act}), act, []string{act}, roles))
	}
	return rows
}

// actionRow builds one row: what you can do, who may, and what else the rule says.
func actionRow(m *domain.Machine, label, action string, actions, roles []string) rendering.RoleMatrixRow {
	row := rendering.RoleMatrixRow{Action: label, Actions: actions}
	required, restricted := requiredRoles(m, action)
	row.Who = whoText(roles, required, restricted)
	row.Qualifier, row.Open = restrictionText(m, action, !restricted)
	return row
}

// whoText is a row's own "who can do it" phrase: the role names requiredRoles found to satisfy
// every role-bearing Permission, in this Application's own declared order, or the two edge cases
// requiredRoles' own doc comment names as distinct from "some roles" -- no role-bearing Permission
// at all ("All roles"), and two Permissions leaving nothing in common ("No one").
func whoText(roles []string, required map[string]bool, restricted bool) string {
	if !restricted {
		return "All roles"
	}
	var matched []string
	for _, r := range roles {
		if required[r] {
			matched = append(matched, capitalizeRole(r))
		}
	}
	if len(matched) == 0 {
		return "No one"
	}
	return strings.Join(matched, ", ")
}

// rolesSummary renders an Application's whole role vocabulary for display -- "Approver, Submitter,
// Reviewer" -- capitalized and joined once here so rolematrix.templ only ever renders an
// already-resolved string, the same rule its own top doc comment states for everything else on
// this page.
func rolesSummary(roles []string) string {
	capitalized := make([]string, len(roles))
	for i, r := range roles {
		capitalized[i] = capitalizeRole(r)
	}
	return strings.Join(capitalized, ", ")
}

// capitalizeRole renders a declared role id ("approver") the way a person reads it ("Approver") --
// metadata's own vocabulary stays lower-case (member-role-detail.html's own dropdown options do
// the same lower-case-in-YAML, capitalized-on-screen split), so this is presentation only.
func capitalizeRole(role string) string {
	if role == "" {
		return role
	}
	return strings.ToUpper(role[:1]) + role[1:]
}

// restrictionText is the line under a row: what the rule says *besides* which roles may, in the
// words the screens use rather than in field ids. whoText already carries the role half, so this
// never repeats it the way this function's own predecessor (qualifier) did before the Flow 2
// mockup gave "who may" its own column (ROADMAP.md "Application Settings hub").
//
// It also says so, and says so in amber, when *nothing* is declared. An ungoverned action and a
// fully granted one read identically in Who ("All roles" either way), and only one of them is
// something an administrator would want to know about -- in a YAML file they look identical too,
// which is exactly how three of them stayed ungoverned until someone went looking.
func restrictionText(m *domain.Machine, action string, unrestricted bool) (text string, open bool) {
	perms := m.PermissionsFor(action)
	if len(perms) == 0 {
		return "No restriction set yet — anyone in this workspace can do this to anyone's " + strings.ToLower(m.Name) + ".", true
	}
	var parts []string
	for _, p := range perms {
		switch {
		case p.DynamicActor != nil:
			parts = append(parts, "the person it is assigned to, or a member of the group it names")
		case p.ActorField != "" && action == domain.ActionCreate:
			// At creation the values checked are the ones being submitted, so this arm is a rule
			// about the record you may write rather than one you may reach.
			parts = append(parts, "in your own name")
		case p.ActorField != "":
			parts = append(parts, "the person named as "+strings.ToLower(fieldName(m, p.ActorField)))
		case p.WorkspaceRole != "":
			parts = append(parts, "a workspace "+p.WorkspaceRole)
		}
	}
	switch {
	case len(parts) == 0 && unrestricted:
		// A Permission exists but declares no restriction of any kind -- unconditional, and
		// genuinely distinct from "no Permission at all" above, which is why this isn't folded
		// into that amber branch.
		return "Anyone in this workspace.", false
	case len(parts) == 0:
		// Who already names exactly which roles; there is nothing left for this line to add.
		return "", false
	case unrestricted:
		return "Anyone, but only " + strings.Join(parts, "; and only ") + ".", false
	default:
		return "Only " + strings.Join(parts, "; and only ") + ".", false
	}
}

// automaticNote is the sentence standing in for every move of a Machine's status that no Action
// performs. One per Machine rather than one per edge: the edges are real and are still what the
// runtime enforces, but a reader of this page needs to know the value is not theirs to set, not
// to audit six ordered pairs.
func automaticNote(m *domain.Machine) string {
	// The *destinations*, not the ordered pairs. Six pairs written out inline was the first
	// version and it read as noise -- a person wants to know the value is not theirs to set and
	// which values it can end up at, not to audit the edge list. The edges are still real and
	// still enforced; they are simply not what this sentence is for.
	var states []string
	seen := map[string]bool{}
	for _, t := range m.Transitions {
		if t.Action == "" && !seen[t.To] {
			seen[t.To] = true
			states = append(states, t.To)
		}
	}
	return fmt.Sprintf("A %s's status changes on its own — it becomes %s depending on its approval steps. Nobody sets it by hand, not even an admin, so it is not something a role can be given.",
		strings.ToLower(m.Name), joinWithOr(states))
}

// joinWithOr renders a value list the way a sentence needs it: "a", "a or b", "a, b or c".
func joinWithOr(values []string) string {
	switch len(values) {
	case 0:
		return "another value"
	case 1:
		return values[0]
	default:
		return strings.Join(values[:len(values)-1], ", ") + " or " + values[len(values)-1]
	}
}

// workspaceSection is the rules no Application decides: a Machine no Application claims, an
// append-only Machine, a Permission requiring the Workspace role.
//
// Its first entry is deliberately NOT a projection of metadata, and says so on the page. Workspace
// administration (members, groups, this screen itself) is gated by internal/web's
// requireWorkspaceAdmin middleware on the route, and route-level gating is not something metadata
// can declare here -- the missing capability is a declared `requires_role:` on a navigation item
// (ROADMAP.md's deferral table, "a second real case, not a phase"). Omitting it would leave the
// most familiar rule in the Workspace off the one page meant to answer "who can do what", so it
// is included and labelled rather than dropped or silently implied to be declared.
func workspaceSection(all []*domain.Machine) []rendering.RoleMatrixNote {
	notes := []rendering.RoleMatrixNote{{
		Rule:       "Manage members, groups and who gets which role",
		Who:        "Workspace admins only.",
		Why:        "This page is part of that — a plain member cannot open it.",
		Undeclared: true,
	}}
	for _, m := range all {
		if m.ApplicationID != "" {
			// A claimed Machine's rules already appear in that Application's own block, read
			// against its roles; repeating them here would put one declaration in two places.
			continue
		}
		if m.AppendOnly {
			notes = append(notes, rendering.RoleMatrixNote{
				Rule: "Change the " + strings.ToLower(m.Name) + " history",
				Who:  "No one, not even an admin.",
				Why:  "It is a permanent record of what happened. Entries are added, never edited or removed.",
			})
		}
		if actions := workspaceRoleActions(m); len(actions) > 0 {
			notes = append(notes, rendering.RoleMatrixNote{
				Rule: verbFor(actions) + " a " + strings.ToLower(m.Name),
				Who:  "Workspace admins only.",
				Why:  "This is the workspace's own directory, so changing it is administration — not something any application decides.",
			})
		}
	}
	return notes
}

// workspaceRoleActions is the actions on m that require a Workspace role, in KnownActions order
// rather than declaration order so two Machines never phrase the same pair differently.
func workspaceRoleActions(m *domain.Machine) []string {
	var out []string
	for _, act := range recordActions {
		for _, p := range m.PermissionsFor(act) {
			if p.WorkspaceRole != "" {
				out = append(out, act)
				break
			}
		}
	}
	return out
}

// verbFor renders an action list the way a person says it: "Create", "Edit or delete".
func verbFor(actions []string) string {
	joined := strings.Join(actions, " or ")
	return strings.ToUpper(joined[:1]) + joined[1:]
}

// foldIdentical merges consecutive rows granting the same roles for the same reason into one, so
// "edit" and "delete" under one identical rule read as a single line rather than as two the
// reader has to compare. Only consecutive rows fold, and only on an exact match of every rendered
// value, so the fold can never hide a difference.
func foldIdentical(rows []rendering.RoleMatrixRow) []rendering.RoleMatrixRow {
	var out []rendering.RoleMatrixRow
	for _, row := range rows {
		if n := len(out); n > 0 && sameGrant(out[n-1], row) {
			out[n-1].Actions = append(out[n-1].Actions, row.Actions...)
			out[n-1].Action = verbFor(out[n-1].Actions)
			continue
		}
		out = append(out, row)
	}
	return out
}

func sameGrant(a, b rendering.RoleMatrixRow) bool {
	return a.Who == b.Who && a.Qualifier == b.Qualifier && a.Open == b.Open
}

func dedupe(in []string) []string {
	seen := make(map[string]bool, len(in))
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// fieldName renders a Field by the name a person reads, falling back to the id for a Field that is
// somehow not declared -- an id on screen is ugly and honest; an empty string would silently drop
// half a sentence about who may act.
func fieldName(m *domain.Machine, id string) string {
	if f, ok := m.FieldByID(id); ok {
		return f.Name
	}
	return id
}

// requiredRoles is the set of roles that satisfy *every* role-bearing Permission governing action,
// plus whether any such Permission exists at all.
//
// The intersection, not the union, and that is the one subtle thing in this file. AllowsAction's
// contract is that several Permissions on one Action are requirements while several roles within
// one Permission are alternatives -- so a role only reaches the Action if it satisfies each
// role-bearing Permission separately. Taking the union would draw a tick for a role the server
// refuses, which is exactly the "the button is shown but the request is denied" disagreement
// authorization.AllowsAction's own doc comment exists to prevent.
//
// `restricted` is returned separately rather than inferred from an empty set, and that
// distinction is load-bearing: two Permissions naming *disjoint* role sets intersect to nothing,
// which means nobody may -- the exact opposite of no Permission naming a role at all, which means
// anybody may (Principle #6). Reading "empty" as "unrestricted" would draw a full row of ticks
// for an Action the server refuses for everyone, and it would do so precisely on the Machine
// whose rules are most tangled.
//
// Permissions that name no role are skipped rather than treated as granting nothing: they
// restrict by record or by Workspace role, which qualifier reports in words instead.
func requiredRoles(m *domain.Machine, action string) (required map[string]bool, restricted bool) {
	for _, p := range m.PermissionsFor(action) {
		if len(p.Roles) == 0 {
			continue
		}
		allowed := make(map[string]bool, len(p.Roles))
		for _, r := range p.Roles {
			allowed[r] = true
		}
		if !restricted {
			required, restricted = allowed, true
			continue
		}
		for r := range required {
			if !allowed[r] {
				delete(required, r)
			}
		}
	}
	return required, restricted
}
