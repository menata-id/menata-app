package composition

import (
	"strings"

	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// recordActions are the actions the "Record actions" section enumerates, in this order.
//
// domain.ActionDecide is deliberately absent: it performs a declared Transition, so it is already
// a row in the Transitions section above, and listing it twice would let one section say
// something the other does not.
var recordActions = []string{domain.ActionCreate, domain.ActionEdit, domain.ActionDelete}

// RoleMatrix composes the Authorization Matrix (ui-sample/case-03-flow1/06b-authorization-matrix.
// html): what each role may do in one Application, and what the Workspace decides instead.
//
// It is a pure projection over already-loaded metadata -- no database, no request, no identity.
// The board's own copy states the constraint: "this page adds no permission model of its own."
// Every cell here has to be derivable from declarations that already gate real writes; anything
// this screen could say that authorization.AllowsAction would not agree with is a bug here, not a
// richer view.
//
// Three sections, because the three answer different questions and conflating them is what made
// the first version of this screen almost empty:
//
//	Transitions     -- Machine.Transitions x Application.Roles, via the Permissions on each
//	                   transition's Action. What board 06 drew, and all it drew.
//	Record actions  -- create/edit/delete on this Application's own Machines. Governed by the same
//	                   Permission primitive and never rendered anywhere until now, which is how
//	                   two of them stayed ungoverned without anyone noticing.
//	Workspace       -- rules that are not an Application's to make: a Machine no Application
//	                   claims, a Permission requiring the Workspace role, an append-only Machine.
//
// `all` is the Workspace's whole Machine list in declaration order -- not the id-keyed map -- so
// the Workspace section's own row order is the manifest's rather than a map's iteration order.
func RoleMatrix(app domain.Application, all []*domain.Machine) rendering.RoleMatrixView {
	byID := make(map[string]*domain.Machine, len(all))
	for _, m := range all {
		byID[m.ID] = m
	}

	v := rendering.RoleMatrixView{
		ApplicationID:   app.ID,
		ApplicationName: app.Name,
		Roles:           app.Roles,
	}
	for _, machineID := range app.Machines {
		m, ok := byID[machineID]
		if !ok {
			continue
		}
		for _, t := range m.Transitions {
			v.Transitions = append(v.Transitions, transitionRow(m, t, app.Roles))
		}
	}
	for _, machineID := range app.Machines {
		if m, ok := byID[machineID]; ok {
			v.RecordActions = append(v.RecordActions, foldIdentical(recordActionRows(m, app.Roles))...)
		}
	}
	for _, m := range all {
		v.WorkspaceRules = append(v.WorkspaceRules, workspaceRules(m)...)
	}
	return v
}

// transitionRow projects one declared edge into one row: its label, the move it makes, and one
// cell per role.
//
// The "system" case is not a styling choice. An edge naming no Action is one no human route can
// perform (domain.Transition.Action), so *no* role grants it and every cell is blank -- rendering
// a row of empty cells with no explanation would read as "nobody has been given this yet", which
// is a different and wrong statement.
func transitionRow(m *domain.Machine, t domain.Transition, roles []string) rendering.RoleMatrixRow {
	row := rendering.RoleMatrixRow{
		MachineName: m.Name,
		Name:        t.Name,
		Detail:      t.From + " → " + t.To,
		System:      t.Action == "",
	}
	if row.System {
		return row
	}
	fillGrants(&row, m, t.Action, roles)
	row.Note = scopeNote(m, t.Action)
	return row
}

// recordActionRows is one row per create/edit/delete on m.
func recordActionRows(m *domain.Machine, roles []string) []rendering.RoleMatrixRow {
	rows := make([]rendering.RoleMatrixRow, 0, len(recordActions))
	for _, act := range recordActions {
		row := rendering.RoleMatrixRow{MachineName: m.Name, Name: m.Name, Detail: act}
		// An append-only Machine refuses edit and delete before any actor is consulted, so the
		// row is "nobody", not "everybody" -- which is what an undeclared action would mean.
		if m.AppendOnly && (act == domain.ActionEdit || act == domain.ActionDelete) {
			row.Granted = make([]bool, len(roles))
			row.Note = m.Name + " records are append-only: never changed or removed once written, by anyone."
			rows = append(rows, row)
			continue
		}
		fillGrants(&row, m, act, roles)
		row.Note = scopeNote(m, act)
		rows = append(rows, row)
	}
	return rows
}

// foldIdentical merges consecutive rows that grant the same roles for the same reason into one,
// so "edit" and "delete" governed by one identical rule read as a single line rather than as two
// the reader has to compare. Only consecutive rows fold, and only on an exact match of every
// rendered value, so the fold can never hide a difference.
func foldIdentical(rows []rendering.RoleMatrixRow) []rendering.RoleMatrixRow {
	var out []rendering.RoleMatrixRow
	for _, row := range rows {
		if n := len(out); n > 0 && sameGrant(out[n-1], row) {
			out[n-1].Detail += ", " + row.Detail
			continue
		}
		out = append(out, row)
	}
	return out
}

func sameGrant(a, b rendering.RoleMatrixRow) bool {
	if a.MachineName != b.MachineName || a.Note != b.Note || a.Unrestricted != b.Unrestricted || len(a.Granted) != len(b.Granted) {
		return false
	}
	for i := range a.Granted {
		if a.Granted[i] != b.Granted[i] {
			return false
		}
	}
	return true
}

// fillGrants sets Unrestricted and one Granted entry per role, in Application.Roles order.
func fillGrants(row *rendering.RoleMatrixRow, m *domain.Machine, action string, roles []string) {
	required, restricted := requiredRoles(m, action)
	row.Unrestricted = !restricted
	for _, role := range roles {
		row.Granted = append(row.Granted, row.Unrestricted || required[role])
	}
}

// scopeNote is the sentence under a row: what the Permissions on this action say *besides* which
// roles may, in the vocabulary of the metadata that says it.
//
// A ✓ answers the role question only, and every rule in this manifest that matters also narrows
// which record. A matrix that showed the ticks and hid that would overstate every grant on it --
// "an approver may reject" is true and "any approver may reject any step" is not.
//
// It also says so when *nothing* is declared. An ungoverned action and a fully granted one render
// identically in the cells, and only one of them is something an administrator would want to know
// about.
func scopeNote(m *domain.Machine, action string) string {
	perms := m.PermissionsFor(action)
	if len(perms) == 0 {
		return "No rule is declared, so any member of this Workspace may."
	}
	var notes []string
	for _, p := range perms {
		switch {
		case p.DynamicActor != nil:
			notes = append(notes, "only the record's own "+fieldName(m, p.DynamicActor.ActorUserField)+
				", or a member of the "+fieldName(m, p.DynamicActor.ActorGroupField)+" it names")
		case p.ActorField != "" && action == domain.ActionCreate:
			// At creation the values checked are the ones being submitted, so this arm reads as a
			// rule about the record you may write rather than one you may reach.
			notes = append(notes, "only in your own name -- the record must name you as its "+fieldName(m, p.ActorField))
		case p.ActorField != "":
			notes = append(notes, "only the record's own "+fieldName(m, p.ActorField))
		case p.WorkspaceRole != "":
			notes = append(notes, "only a Workspace "+p.WorkspaceRole)
		}
	}
	if len(notes) == 0 {
		return ""
	}
	return "Also: " + strings.Join(notes, "; ") + "."
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

// workspaceRules collects the rules that belong to the Workspace rather than to any Application:
// a Permission requiring the Workspace role, and an append-only Machine.
//
// Scoped to Machines *no Application claims*, deliberately. A claimed Machine's own rules already
// appear in the sections above, where they are read against that Application's roles; repeating
// them here would put one declaration in two places and invite the two drifting apart.
func workspaceRules(m *domain.Machine) []rendering.RoleMatrixNote {
	if m.ApplicationID != "" {
		return nil
	}
	var out []rendering.RoleMatrixNote
	if m.AppendOnly {
		out = append(out, rendering.RoleMatrixNote{
			MachineName: m.Name,
			Detail:      "edit, delete",
			Who:         "No one, including a Workspace admin.",
			Why:         "Declared append_only: an audit trail is never changed or removed once written.",
		})
	}
	byWho := map[string][]string{}
	var order []string
	for _, p := range m.Permissions {
		if p.WorkspaceRole == "" {
			continue
		}
		if _, seen := byWho[p.WorkspaceRole]; !seen {
			order = append(order, p.WorkspaceRole)
		}
		byWho[p.WorkspaceRole] = append(byWho[p.WorkspaceRole], p.Action)
	}
	for _, who := range order {
		out = append(out, rendering.RoleMatrixNote{
			MachineName: m.Name,
			Detail:      strings.Join(byWho[who], ", "),
			Who:         "Workspace " + who + " only.",
			Why:         "Not an Application role -- this is decided by the Workspace, so no Application's own vocabulary can express it.",
		})
	}
	return out
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
// restrict by record or by Workspace role, which scopeNote reports in words instead.
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
