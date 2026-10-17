package web

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/installer"
	"menata.app/internal/mail"
	"menata.app/internal/registry"
	"menata.app/internal/rendering"
)

// actionRunner performs one registry.RuntimeActions entry. It reports a refusal itself, to w, and returns ok=false; on
// success it returns where the person goes next ("" means: stay, with the page re-read).
type actionRunner func(w http.ResponseWriter, req *http.Request, env actionEnv, record string) (next string, ok bool)

// actionEnv is what a runner may use of the process: the store, and the mailer and configuration an invitation needs. The
// runner is handed no more than this, so what an action can reach is what is listed here.
type actionEnv struct {
	store  *data.Store
	mailer mail.Mailer
	cfg    config.Config
	// reload re-reads one Workspace's manifest from disk and swaps its routes (web.Deps.ReloadMetadata); nil in a process
	// with no hook, which an action that needs it reports rather than ignores.
	reload func(slug string) error
}

// runtimeActionRunners are the executors, one per registry.RuntimeActions entry (TestRuntimeActionsAndRunnersAgree).
var runtimeActionRunners = map[string]actionRunner{
	"create_group": runCreateGroup,
	"delete_group": runDeleteGroup,
	// A Group's membership is the whole set ticked: unticking everyone is a meaningful submission, which is why
	// data.SetGroupMembers takes a list and not a sequence of adds and removes.
	"set_group_members":   runSetGroupMembers,
	"set_group_roles":     runSetGroupRoles,
	"invite_member":       runInviteMember,
	"set_member_access":   runSetMemberAccess,
	"deactivate_member":   runDeactivateMember,
	"reactivate_member":   runReactivateMember,
	"set_member_app_role": runSetMemberAppRole,
	"install_application": runInstallApplication,
	"reload_workspace":    runReloadWorkspace,
	"restore_snapshot":    runRestoreSnapshot,
	"archive_workspace":   runArchiveWorkspace,
	"revoke_invite":       runRevokeInvite,
}

// runtimeAction is `POST /runtime-actions/{action}[?record=<id>]`, the one route a runtime page's Form or Button for a
// named operation posts to. It is registered under the Workspace-admin gate, which is every action's gate today: an action
// that is not admin-only would need a route of its own rather than ride this one, so it fails closed here.
//
// The route holds no logic of any operation: it looks the name up in the closed set, checks that a record is named
// exactly when the action is about one, and hands over. What an action may do is what its runner does, and nothing a page
// said.
func runtimeAction(store *data.Store, mailer mail.Mailer, cfg config.Config, reload func(slug string) error) http.HandlerFunc {
	env := actionEnv{store: store, mailer: mailer, cfg: cfg, reload: reload}
	return func(w http.ResponseWriter, req *http.Request) {
		id := chi.URLParam(req, "action")
		a, known := registry.RuntimeActions[id]
		run, hasRunner := runtimeActionRunners[id]
		if !known || !hasRunner {
			http.Error(w, "unknown action", http.StatusNotFound)
			return
		}
		if !a.Admin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		record := req.URL.Query().Get("record")
		if (a.Source != "") != (record != "") {
			http.Error(w, "this action is about a record exactly when it names one", http.StatusBadRequest)
			return
		}
		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}
		next, ok := run(w, req, env, record)
		if !ok {
			return
		}
		switch {
		case next != "":
			redirectTo(w, req, next)
		case req.Header.Get("HX-Request") == "true":
			w.Header().Set("HX-Refresh", "true")
		default:
			http.Redirect(w, req, "/home", http.StatusSeeOther)
		}
	}
}

func runCreateGroup(w http.ResponseWriter, req *http.Request, env actionEnv, _ string) (string, bool) {
	workspaceID, _ := data.WorkspaceScope(req.Context())
	name := strings.TrimSpace(req.FormValue("name"))
	if name == "" {
		http.Error(w, "group name is required", http.StatusUnprocessableEntity)
		return "", false
	}
	if _, err := env.store.CreateGroup(req.Context(), workspaceID, name); err != nil {
		serverError(w, err)
		return "", false
	}
	return "", true
}

func runDeleteGroup(w http.ResponseWriter, req *http.Request, env actionEnv, record string) (string, bool) {
	workspaceID, _ := data.WorkspaceScope(req.Context())
	if err := env.store.DeleteGroup(req.Context(), workspaceID, record); err != nil {
		recordError(w, err)
		return "", false
	}
	return domain.RuntimeScreenRoute("nav_workspace_groups"), true
}

func runSetGroupMembers(w http.ResponseWriter, req *http.Request, env actionEnv, record string) (string, bool) {
	ctx := req.Context()
	workspaceID, _ := data.WorkspaceScope(ctx)
	if _, err := env.store.GetGroup(ctx, workspaceID, record); err != nil {
		recordError(w, err)
		return "", false
	}
	if err := env.store.SetGroupMembers(ctx, record, req.Form["member"]); err != nil {
		serverError(w, err)
		return "", false
	}
	return "", true
}

// runSetGroupRoles sets the Group's role in each Application, validated against that Application's own declared roles
// (submittedAppRoles, the reader the member screens use), so an undeclared role is refused here as there.
func runSetGroupRoles(w http.ResponseWriter, req *http.Request, env actionEnv, record string) (string, bool) {
	ctx := req.Context()
	workspaceID, _ := data.WorkspaceScope(ctx)
	if _, err := env.store.GetGroup(ctx, workspaceID, record); err != nil {
		recordError(w, err)
		return "", false
	}
	roles, err := submittedAppRoles(req, rendering.CurrentWorkspace(ctx).Applications)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return "", false
	}
	for appID, role := range roles {
		if err := env.store.SetGroupAppRole(ctx, record, appID, role); err != nil {
			serverError(w, err)
			return "", false
		}
	}
	return "", true
}

// runInviteMember records an invitation and emails its workspace-bound token. It creates no membership and no mch_user
// record: those come into existence when the invitation is *accepted* (submitAcceptInvite), which is the owner's rule from
// 2026-09-22 -- someone who has confirmed nothing is not a member, so they must not appear in a member list, in an approver
// picker, or in any authorization lookup. It asks no name for the invitee for the same reason: a name belongs to whoever owns
// the email. An admin supplies the email and the roles, the two things that are the Workspace's decision.
func runInviteMember(w http.ResponseWriter, req *http.Request, env actionEnv, _ string) (string, bool) {
	ctx := req.Context()
	workspaceID, _ := data.WorkspaceScope(ctx)
	email := normalizeEmail(req.FormValue("email"))
	if email == "" {
		http.Error(w, "email is required", http.StatusUnprocessableEntity)
		return "", false
	}
	appRoles, err := submittedAppRoles(req, rendering.CurrentWorkspace(ctx).Applications)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return "", false
	}
	// Already a member of this Workspace: the invitation would have nothing to grant, and accepting it would try to create a
	// second membership for the same identity.
	if _, ok, err := resolveWorkspaceMembership(ctx, env.store, email, workspaceID); err != nil {
		serverError(w, err)
		return "", false
	} else if ok {
		http.Error(w, "that email is already a member of this workspace", http.StatusUnprocessableEntity)
		return "", false
	}
	invite := data.PendingInvite{WorkspaceID: workspaceID, Email: email, WorkspaceRole: "member", AppRoles: appRoles}
	if err := env.store.CreatePendingInvite(ctx, invite); err != nil {
		serverError(w, err)
		return "", false
	}
	sendInviteEmail(ctx, env.mailer, env.cfg, email, workspaceID)
	return "", true
}

// runRevokeInvite withdraws an invitation that has not been accepted. There is no membership to remove and no record to
// delete, which is what an invitation living in its own table buys: one delete, and the emailed link stops working at once
// because accepting re-checks that the invitation still exists.
func runRevokeInvite(w http.ResponseWriter, req *http.Request, env actionEnv, record string) (string, bool) {
	workspaceID, _ := data.WorkspaceScope(req.Context())
	email := normalizeEmail(record)
	if email == "" {
		http.Error(w, "email is required", http.StatusUnprocessableEntity)
		return "", false
	}
	if err := env.store.DeletePendingInvite(req.Context(), workspaceID, email); err != nil {
		serverError(w, err)
		return "", false
	}
	return "", true
}

// runSetMemberAccess sets a member's Workspace role and their direct role in each Application, each checked against what it
// may be: the Workspace role is admin or member, an Application's role is one that Application declares (submittedAppRoles).
// UpdateMemberRole still writes the legacy single app_role column alongside the new rows (migration 008 keeps it so a rollback
// loses nothing), fed Document Approval's role, which is what that column has always held.
func runSetMemberAccess(w http.ResponseWriter, req *http.Request, env actionEnv, record string) (string, bool) {
	ctx := req.Context()
	workspaceID, _ := data.WorkspaceScope(ctx)
	workspaceRole := req.FormValue("workspace_role")
	if workspaceRole != domain.WorkspaceRoleAdmin && workspaceRole != domain.WorkspaceRoleMember {
		http.Error(w, "invalid workspace role", http.StatusUnprocessableEntity)
		return "", false
	}
	appRoles, err := submittedAppRoles(req, rendering.CurrentWorkspace(ctx).Applications)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return "", false
	}
	if err := env.store.UpdateMemberRole(ctx, workspaceID, record, workspaceRole, appRoles[legacyAppRoleApplicationID]); err != nil {
		recordError(w, err)
		return "", false
	}
	for appID, role := range appRoles {
		if err := env.store.SetMemberAppRole(ctx, workspaceID, record, appID, role); err != nil {
			serverError(w, err)
			return "", false
		}
	}
	return "", true
}

// runDeactivateMember gates a member out of this Workspace. Their own mch_user record, and everything it authored, is
// untouched: only their membership row's deactivated_at is set, checked fresh on every later request. A member with an open
// pending Approval Step would otherwise become permanently undecidable by anyone the moment they can no longer be reached, so
// the blocking reasons are asked first and a refusal is a plain 422 saying why.
func runDeactivateMember(w http.ResponseWriter, req *http.Request, env actionEnv, record string) (string, bool) {
	ctx := req.Context()
	workspaceID, _ := data.WorkspaceScope(ctx)
	reasons, err := composition.BlockingReasonsForMemberRemoval(ctx, composition.NewLoader(env.store, machinesFor(ctx)), record)
	if err != nil {
		serverError(w, err)
		return "", false
	}
	if len(reasons) > 0 {
		http.Error(w, "cannot deactivate: "+strings.Join(reasons, "; "), http.StatusUnprocessableEntity)
		return "", false
	}
	if err := env.store.DeactivateMember(ctx, workspaceID, record); err != nil {
		recordError(w, err)
		return "", false
	}
	return "", true
}

// runReactivateMember reverses runDeactivateMember. It has no guard of its own, symmetric to the store having none: undoing a
// restriction never needs the check that imposing one does.
func runReactivateMember(w http.ResponseWriter, req *http.Request, env actionEnv, record string) (string, bool) {
	workspaceID, _ := data.WorkspaceScope(req.Context())
	if err := env.store.ReactivateMember(req.Context(), workspaceID, record); err != nil {
		recordError(w, err)
		return "", false
	}
	return "", true
}

// runReloadWorkspace re-reads this Workspace's manifest from disk -- what to press after fixing a hand edit, or after a
// Workspace was marked unavailable. A manifest that fails to load changes nothing live and answers 422 with the error, which
// names the file. A hand edit is not a write the runtime made, so no snapshot was taken for it: one is taken now, before the
// reload, unless the newest already holds exactly this state.
func runReloadWorkspace(w http.ResponseWriter, req *http.Request, env actionEnv, _ string) (string, bool) {
	if env.reload == nil {
		serverError(w, errors.New("this process has no reload hook configured"))
		return "", false
	}
	if manifestPath, _, err := workspaceInstallation(req.Context(), env.store, env.cfg); err == nil && env.cfg.BackupDir != "" {
		if out, err := installer.SnapshotWorkspaceIfChanged(manifestPath, env.cfg.BackupDir, time.Now()); err != nil {
			log.Printf("snapshot before reloading %s failed: %v", filepath.Base(manifestPath), err)
		} else if out != "" {
			log.Printf("snapshot %s", out)
		}
	}
	if err := reloadCurrentWorkspace(req.Context(), env.store, env.reload); err != nil {
		http.Error(w, fmt.Sprintf("this workspace was not reloaded and keeps serving what it had: %v", err), http.StatusUnprocessableEntity)
		return "", false
	}
	return domain.RuntimeScreenRoute("nav_home"), true
}

// runRestoreSnapshot puts this Workspace's installation back to a saved snapshot (the record) and reloads it. The restore is
// load-verified and itself snapshotted first (installer.RestoreSnapshot), so pressing it by mistake is undone by restoring the
// newest snapshot; a snapshot that does not load answers 422 and changes nothing.
func runRestoreSnapshot(w http.ResponseWriter, req *http.Request, env actionEnv, record string) (string, bool) {
	ctx := req.Context()
	if env.cfg.BackupDir == "" || env.reload == nil {
		serverError(w, errors.New("this process has no backup directory or reload hook configured"))
		return "", false
	}
	manifestPath, _, err := workspaceInstallation(ctx, env.store, env.cfg)
	if err != nil {
		serverError(w, err)
		return "", false
	}
	if err := installer.RestoreSnapshot(manifestPath, env.cfg.BackupDir, record, time.Now()); err != nil {
		var rejected *installer.RejectedError
		if errors.As(err, &rejected) {
			http.Error(w, fmt.Sprintf("the snapshot was not restored: %v", err), http.StatusUnprocessableEntity)
			return "", false
		}
		serverError(w, err)
		return "", false
	}
	if err := reloadCurrentWorkspace(ctx, env.store, env.reload); err != nil {
		serverError(w, fmt.Errorf("the snapshot was restored on disk but the live reload failed -- a process restart will pick it up: %w", err))
		return "", false
	}
	return domain.RuntimeScreenRoute("nav_home"), true
}

// runArchiveWorkspace archives the current Workspace. Not the Workspace this session was just in: it is now archived, so the
// admin is sent to /switch-workspace, the mid-session screen of whatever else this identity belongs to (/choose-workspace
// resolves identity from a pre-session cookie an authenticated admin never carries).
func runArchiveWorkspace(w http.ResponseWriter, req *http.Request, env actionEnv, _ string) (string, bool) {
	workspaceID, _ := data.WorkspaceScope(req.Context())
	if err := env.store.ArchiveWorkspace(req.Context(), workspaceID); err != nil {
		serverError(w, err)
		return "", false
	}
	return "/switch-workspace", true
}

// snapshotsProvider is the reader of the `snapshots` source: the Workspace's saved installations, newest first as
// installer.ListSnapshots lists them, titled by when they were taken and how large they are. It lives here because the
// backup directory is the process's own configuration.
func snapshotsProvider(store *data.Store, cfg config.Config) composition.RuntimeProvider {
	return func(ctx context.Context, _ *composition.Loader, _ domain.Actor) ([]*data.Record, error) {
		snaps, err := workspaceSnapshots(ctx, store, cfg)
		if err != nil {
			return nil, err
		}
		workspaceID, _ := data.WorkspaceScope(ctx)
		out := make([]*data.Record, 0, len(snaps))
		for _, sn := range snaps {
			out = append(out, &data.Record{ID: sn.ID, MachineID: "mch_snapshot", WorkspaceID: workspaceID, CreatedAt: sn.TakenAt,
				Values: map[string]any{"fld_id": sn.ID, "fld_title": fmt.Sprintf("%s UTC · %d KB", sn.TakenAt.UTC().Format("2006-01-02 15:04:05"), (sn.Size+1023)/1024)}})
		}
		return out, nil
	}
}

// runSetMemberAppRole sets one member's direct role in one Application and nothing else: not their Workspace role, not their roles
// elsewhere. The record names both (`<hub id>:<user record id>`); the value is checked against the Application's own declared
// vocabulary, an empty one clears the row, and a member who is not in this Workspace is a 404 -- SetMemberAppRole alone would
// insert a row for any user id. It leaves the legacy workspace_members.app_role column alone: nothing reads it (the member screen
// still writes it for the Document Approval id, see legacyAppRoleApplicationID), and a second write here would be a second place
// to remember it exists.
func runSetMemberAppRole(w http.ResponseWriter, req *http.Request, env actionEnv, record string) (string, bool) {
	ctx := req.Context()
	hubID, userID, _ := strings.Cut(record, ":")
	app, _, found := composition.SettingsApplication(rendering.CurrentWorkspace(ctx), hubID)
	if !found {
		http.NotFound(w, req)
		return "", false
	}
	role := req.FormValue("role")
	if role != "" && !slices.Contains(app.Roles, role) {
		http.Error(w, "this application does not declare that role", http.StatusUnprocessableEntity)
		return "", false
	}
	workspaceID, _ := data.WorkspaceScope(ctx)
	if _, err := env.store.GetMembership(ctx, workspaceID, userID); err != nil {
		recordError(w, err)
		return "", false
	}
	if err := env.store.SetMemberAppRole(ctx, workspaceID, userID, app.ID, role); err != nil {
		serverError(w, err)
		return "", false
	}
	return "", true
}

// runInstallApplication installs one library template (the record) into this Workspace as that Workspace's own copy and reloads
// the route table live (Deps.ReloadMetadata, injected because this package may not import internal/metadata itself).
//
// Nothing partial survives a failure: installer.Install rolls its own writes back before returning, and a failed reload leaves
// the previous route table serving traffic while the files on disk are correct for the next restart. What the page showed
// (renames, references added, refusals) came from installer.PlanInstall, the function this runs, so the screen cannot promise
// something the write then declines; reaching a refusal here means the Workspace changed underneath the page, which is a 422
// with the reasons and not a generic failure.
func runInstallApplication(w http.ResponseWriter, req *http.Request, env actionEnv, record string) (string, bool) {
	ctx := req.Context()
	manifestPath, ws, err := workspaceInstallation(ctx, env.store, env.cfg)
	if err != nil {
		serverError(w, err)
		return "", false
	}
	template, err := installer.TemplateByID(env.cfg.TemplatePath, record)
	if err != nil {
		http.Error(w, "no such template", http.StatusNotFound)
		return "", false
	}
	plan := installer.PlanInstall(template, ws)
	if !plan.OK() {
		http.Error(w, "this application cannot be installed here: "+strings.Join(plan.Refusals, "; "), http.StatusUnprocessableEntity)
		return "", false
	}
	snapshotBeforeWrite(env.cfg, manifestPath)
	appID, err := installer.Install(plan, env.cfg.TemplatePath, manifestPath)
	if err != nil {
		serverError(w, err)
		return "", false
	}
	if env.reload == nil {
		serverError(w, fmt.Errorf("%s was installed but this process has no reload hook configured -- a restart will pick it up", appID))
		return "", false
	}
	if err := reloadCurrentWorkspace(ctx, env.store, env.reload); err != nil {
		serverError(w, fmt.Errorf("%s was installed but the live reload failed -- a process restart will pick it up: %w", appID, err))
		return "", false
	}
	if err := grantInstallerRole(ctx, req, env.store, env.cfg, appID, req.FormValue("role")); err != nil {
		serverError(w, err)
		return "", false
	}
	return installedHomeRoute(plan), true
}

// templatesProvider is the reader of the `templates` source: each library template with what installing it into this Workspace
// would do, from installer.PlanInstall. It lives here because the library's location is the process's configuration. A template
// that cannot be installed here carries its reasons as its summary and is not offered an Install; one that can carries what will
// change on the way in (an id this Workspace already uses is renamed, a shared Machine it lacks is referenced).
func templatesProvider(cfg config.Config) composition.RuntimeProvider {
	return func(ctx context.Context, _ *composition.Loader, _ domain.Actor) ([]*data.Record, error) {
		templates, err := installer.Templates(cfg.TemplatePath)
		if err != nil {
			return nil, err
		}
		workspaceID, _ := data.WorkspaceScope(ctx)
		ws := rendering.CurrentWorkspace(ctx)
		out := make([]*data.Record, 0, len(templates))
		for _, t := range templates {
			plan := installer.PlanInstall(t, ws)
			var machines []string
			for _, tm := range t.Machines {
				machines = append(machines, tm.Machine.Name)
			}
			status, summary := "Ready", installChanges(plan)
			if !plan.OK() {
				status, summary = "Blocked", strings.Join(plan.Refusals, " ")
			}
			out = append(out, &data.Record{ID: t.Application.ID, MachineID: "mch_template", WorkspaceID: workspaceID, Values: map[string]any{
				"fld_id": t.Application.ID, "fld_title": t.Application.Name, "fld_description": t.Application.Description,
				"fld_machines": machineLine(machines), "fld_status": status, "fld_changes": summary,
				"fld_roles": strings.Join(t.Application.Roles, "\n"),
			}})
		}
		return out, nil
	}
}

// machineLine is the one line of what a template brings, by name and not id, count first so the shape of the Application is
// readable before the list is.
func machineLine(names []string) string {
	if len(names) == 0 {
		return ""
	}
	noun := "data models"
	if len(names) == 1 {
		noun = "data model"
	}
	return fmt.Sprintf("%d %s: %s", len(names), noun, strings.Join(names, ", "))
}

// installChanges says what installing a ready template into this Workspace will change on the way in, or nothing when it
// changes nothing.
func installChanges(plan installer.Plan) string {
	var parts []string
	for _, from := range sortedRenameKeys(plan.Renames) {
		parts = append(parts, fmt.Sprintf("This workspace already uses %s, so the copy will be called %s.", from, plan.Renames[from]))
	}
	if len(plan.AddShared) > 0 {
		parts = append(parts, fmt.Sprintf("Adds the shared %s this application needs.", strings.Join(plan.AddShared, " and ")))
	}
	return strings.Join(parts, " ")
}
