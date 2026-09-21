package composition

import (
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// RoleMatrix composes board 06 (ui-sample/case-03-flow1/06-approval-role-matrix.html, Fase 7):
// which role may perform which transition, for one Application.
//
// It is a pure projection over already-loaded metadata -- no database, no request, no identity.
// The board's own copy says why, and it is a design constraint rather than a description: "this
// page doesn't invent a new permission model, it just re-projects [a transition] role-first
// instead of transition-first." Every cell here therefore has to be derivable from declarations
// that already gate real writes; anything this screen could say that authorization.AllowsAction
// would not agree with is a bug in this function, not a richer view.
//
// The join is the whole of it, and it runs in the direction the runtime already reads:
//
//	Machine.Transitions  -> which moves exist, and which Action performs each  (the rows)
//	Application.Roles    -> the vocabulary a member may hold here              (the columns)
//	Machine.Permissions  -> which roles that Action requires                   (the cells)
//
// That is upstream's own derivation for its process map (CAP-W05's extractProcessMap: edges from
// the state model, actors from "every Permission row that grants it"), applied to this runtime's
// shapes. Upstream reads the edge out of a compiled Event because its Events are triggerable;
// here the edge is declared directly (see domain.Transition for why).
//
// Machines are taken in the order the Application declares them, and transitions in declaration
// order within each, so the table's row order is the metadata author's own and never reshuffles.
func RoleMatrix(app domain.Application, machines map[string]*domain.Machine) rendering.RoleMatrixView {
	v := rendering.RoleMatrixView{
		ApplicationID:   app.ID,
		ApplicationName: app.Name,
		Roles:           app.Roles,
	}
	for _, machineID := range app.Machines {
		m, ok := machines[machineID]
		if !ok {
			continue
		}
		for _, t := range m.Transitions {
			v.Rows = append(v.Rows, transitionRow(m, t, app.Roles))
		}
	}
	return v
}

// transitionRow projects one declared edge into one table row: its label, the move it makes, and
// one cell per role.
//
// The "system" case is not a styling choice. An edge naming no Action is one no human route can
// perform (domain.Transition.Action), so *no* role grants it and every cell is blank -- rendering
// a row of empty cells with no explanation would read as "nobody has been given this yet", which
// is a different and wrong statement. Upstream labels the same case "System" for the same reason.
func transitionRow(m *domain.Machine, t domain.Transition, roles []string) rendering.RoleMatrixRow {
	row := rendering.RoleMatrixRow{
		MachineName: m.Name,
		Name:        t.Name,
		From:        t.From,
		To:          t.To,
		Action:      t.Action,
		System:      t.Action == "",
	}
	if row.System {
		return row
	}
	// A Permission that names no role restricts nobody by role (001 Principle #6: metadata
	// describes exceptions, not defaults), so the Action is open to the whole vocabulary. Said
	// out loud in the row rather than left to the reader of six ticks: an all-granted row where
	// no rule was written and an all-granted row where every role was named look identical, and
	// only one of them changes when a role is added to the Application.
	required, restricted := requiredRoles(m, t.Action)
	row.Unrestricted = !restricted
	for _, role := range roles {
		row.Granted = append(row.Granted, row.Unrestricted || required[role])
	}
	return row
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
// restrict by record, not by role, and a record-scoped rule is not something this matrix can or
// should render (the board's own footnote scopes it to "who *can* approve this Machine").
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
