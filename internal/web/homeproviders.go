package web

import (
	"context"
	"errors"
	"fmt"
	"log"

	"menata.app/internal/aiassist"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// applicationsProvider is the reader of the `applications` source: one record for each Application the viewer can open (the
// same question the gate answers, canOpenApplication, so Home never offers a card whose link would answer 403), in the
// Workspace's own order. Each says where it opens (its declared HomeRoute, else /home, which is always a valid destination), the
// role the viewer holds *there* (their own entry in the membership, so each card shows the role held in that Application and not
// one Workspace-wide value) and, as its summary, either the pending-approval count (only on an Application that declares that
// badge: it is Document Approval's number, and putting it on another card would invent a meaning for it) or its declared summary
// Machine's record count labelled by that Machine's name. A counting failure degrades to no count and not to no screen: Home's
// job is to list the Applications, and losing one number is not worth losing it.
func applicationsProvider(store *data.Store, cfg config.Config) composition.RuntimeProvider {
	return func(ctx context.Context, _ *composition.Loader, viewer domain.Actor) ([]*data.Record, error) {
		workspaceID, _ := data.WorkspaceScope(ctx)
		membership, err := membershipFor(ctx, store, workspaceID, viewer.ID)
		if err != nil && !errors.Is(err, data.ErrRecordNotFound) {
			return nil, err
		}
		if membership == nil {
			membership = &data.Membership{}
		}
		machines := machinesFor(ctx)
		apps := openableApplications(rendering.CurrentWorkspace(ctx), viewer, cfg)
		pending, perr := pendingTotalFor(ctx, store, apps, viewer.ID)
		if perr != nil {
			return nil, perr
		}
		// Every summary Machine counted in one statement: the page's cost does not grow with the Applications installed.
		var summaryIDs []string
		for _, app := range apps {
			if _, ok := machines[app.SummaryMachine]; ok {
				summaryIDs = append(summaryIDs, app.SummaryMachine)
			}
		}
		counts, cerr := store.CountRecordsByMachine(ctx, summaryIDs)
		if cerr != nil {
			log.Printf("workspace home: count summary machines: %v", cerr)
		}
		out := make([]*data.Record, 0, len(apps))
		for _, app := range apps {
			route := app.HomeRoute
			if route == "" {
				route = domain.RuntimeScreenRoute("nav_home")
			}
			role := domain.AppRoleLabel(membership.AppRoles[app.ID])
			if role == "" {
				role = "—"
			}
			status, summary := "None", ""
			for _, item := range app.AllNavigation {
				if item.Badge == domain.NavBadgeApprovalInboxPending {
					if pending > 0 {
						status, summary = "Pending", fmt.Sprintf("%d pending", pending)
					}
					break
				}
			}
			if m, ok := machines[app.SummaryMachine]; ok && status == "None" && cerr == nil {
				n := counts[app.SummaryMachine]
				status, summary = "Count", fmt.Sprintf("%d %s%s", n, m.Name, plural(n))
			}
			out = append(out, &data.Record{ID: app.ID, MachineID: "mch_home_application", WorkspaceID: workspaceID, Values: map[string]any{
				"fld_id": app.ID, "fld_title": app.Name, "fld_description": app.Description, "fld_role": "Your role: " + role,
				"fld_status": status, "fld_summary": summary, "fld_link": route,
			}})
		}
		// A Workspace's admins also see the drafts the assistant has generated, as rows of the same list: a draft is an Application
		// that is not published yet, and Home lists Applications. Only an admin can review one, so only an admin is offered it.
		if viewer.WorkspaceRole == domain.WorkspaceRoleAdmin {
			drafts, err := draftApplications(ctx, store, workspaceID)
			if err != nil {
				return nil, err
			}
			for _, d := range drafts {
				out = append(out, &data.Record{ID: d.id, MachineID: "mch_home_application", WorkspaceID: workspaceID, Values: map[string]any{
					"fld_id": d.id, "fld_title": d.name, "fld_description": draftApplicationNote, "fld_role": "Not published yet",
					"fld_status": "Draft", "fld_summary": "Draft", "fld_link": d.reviewRoute,
				}})
			}
		}
		return out, nil
	}
}

// pendingTotalFor is the pending-approval number, read only when some Application the viewer sees declares the badge it belongs
// to: Home pays for it once, across every approval Application installed here, and not at all when none would show it.
func pendingTotalFor(ctx context.Context, store *data.Store, apps []domain.Application, userID string) (int, error) {
	for _, app := range apps {
		for _, item := range app.AllNavigation {
			if item.Badge == domain.NavBadgeApprovalInboxPending {
				return pendingApprovalTotal(ctx, store, userID)
			}
		}
	}
	return 0, nil
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// draftApplicationNote is what a draft says of itself on Home.
const draftApplicationNote = "Generated from your description. Review it before anyone else can open it."

// draftApplication is one Application the assistant has generated and nobody has published: its name and where to review it.
type draftApplication struct{ id, name, reviewRoute string }

// draftApplications is every assistant session of this Workspace whose status is "generated" (a validated proposal exists, not yet
// published or discarded), resolved by latestChange, the same pure decode of already-stored turns the review screen uses -- no model
// call. Home reads it for a Workspace's admins only (applicationsProvider).
func draftApplications(ctx context.Context, store *data.Store, workspaceID string) ([]draftApplication, error) {
	sessions, err := store.ListAISessionsByStatus(ctx, workspaceID, aiassist.KindNewApplication, data.AISessionStatusGenerated)
	if err != nil {
		return nil, err
	}
	out := make([]draftApplication, 0, len(sessions))
	for _, s := range sessions {
		full, err := store.GetAISession(ctx, workspaceID, s.ID)
		if err != nil {
			return nil, err
		}
		change, ok, err := latestChange(full)
		if err != nil || !ok || change.Application == nil {
			continue
		}
		out = append(out, draftApplication{id: s.ID, name: change.Application.Name, reviewRoute: "/new-application/" + s.ID + "/review"})
	}
	return out, nil
}

// currentWorkspaceProvider is the reader of the `current_workspace` source: the Workspace the request is in, as one record. The
// shared admin credential has no membership and passes the admin gate by id, so it is the admin here too: what a page says of
// the viewer must agree with what the gate lets them do.
func currentWorkspaceProvider(store *data.Store) composition.RuntimeProvider {
	return func(ctx context.Context, _ *composition.Loader, viewer domain.Actor) ([]*data.Record, error) {
		workspaceID, _ := data.WorkspaceScope(ctx)
		row, ok := currentWorkspaceRow(ctx, store)
		if !ok {
			return nil, errors.New("this request is in no workspace")
		}
		role := viewer.WorkspaceRole
		if role == "" {
			role = domain.WorkspaceRoleAdmin
		}
		return []*data.Record{{ID: workspaceID, MachineID: "mch_current_workspace", WorkspaceID: workspaceID, Values: map[string]any{
			"fld_id": workspaceID, "fld_title": row.Name, "fld_note": "You manage " + row.Name, "fld_role": role,
		}}}, nil
	}
}
