package web

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/a-h/templ"

	"menata.app/internal/action"
	"menata.app/internal/authorization"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
	"menata.app/internal/storage"
)

// showDeleteAccount renders the confirmation page for account deletion. It reads nothing beyond
// chrome: whether the deletion can proceed is decided on submit, where the answer is acted on,
// rather than computed on every view of a page most people only read.
func showDeleteAccount(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		page, err := deleteAccountPageAfter(store, cfg, req, nil, "")
		if err != nil {
			serverError(w, err)
			return
		}
		render(req.Context(), w, page)
	}
}

// showDeleteAccountInfo is the public page Google Play's account-deletion policy asks for: a URL
// that explains how to request deletion without installing or opening the app. Deletion itself
// needs the signed-in person's password, so this page points there rather than offering a form
// that would act on an unauthenticated request.
func showDeleteAccountInfo(w http.ResponseWriter, req *http.Request) {
	render(req.Context(), w, rendering.DeleteAccountInfoPage(req.URL.Query().Get("done") == "1"))
}

func deleteAccountPageAfter(store *data.Store, cfg config.Config, req *http.Request, blockers []string, passwordErr string) (templ.Component, error) {
	ctx := req.Context()
	chrome, err := resolveChrome(ctx, req, store, cfg)
	if err != nil {
		return nil, err
	}
	userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
	_, switchHref := viewerWorkspaceContext(ctx, store, userID)
	return rendering.DeleteAccountPage(chrome.WorkspaceName, chrome.Viewer(), switchHref, blockers, passwordErr), nil
}

// accountDeletionBlockers lists why email cannot be deleted right now, each phrased for the page.
// Two kinds, and both exist so deletion cannot strand other people's work:
//   - being the only active admin of a live Workspace, which would leave nobody able to administer it;
//   - whatever the Workspace's own Machines declare through member_removal blocks (an open Approval
//     Step assigned to this person), the same check deactivating a member already makes.
func accountDeletionBlockers(ctx context.Context, store *data.Store, workspaces map[string]domain.Workspace, email string, memberships []data.Membership) ([]string, error) {
	var out []string
	sole, err := store.SoleAdminOf(ctx, email)
	if err != nil {
		return nil, err
	}
	for _, name := range sole {
		out = append(out, fmt.Sprintf("You are the only admin of %q. Make someone else an admin, or archive the workspace, first.", name))
	}
	for _, m := range memberships {
		if m.Deactivated {
			continue
		}
		machines := machinesByID(workspaces[m.WorkspaceSlug].Machines)
		reasons, err := composition.BlockingReasonsForMemberRemoval(data.WithWorkspaceScope(ctx, m.WorkspaceID), composition.NewLoader(store, machines), m.UserRecordID)
		if err != nil {
			return nil, err
		}
		for _, r := range reasons {
			out = append(out, fmt.Sprintf("%s: %s", m.WorkspaceName, r))
		}
	}
	return out, nil
}

func machinesByID(list []*domain.Machine) map[string]*domain.Machine {
	out := make(map[string]*domain.Machine, len(list))
	for _, m := range list {
		out[m.ID] = m
	}
	return out
}

// removeSavedSignatures deletes the person's reusable signature images, record and file, in every
// Workspace they belonged to. Which Machine holds them and which Fields say whose they are comes
// from the signature_store declaration (action.StoreFields); a Workspace that casts no such Machine
// has nothing to remove. A signature already composited into a finished PDF is part of that
// document and stays with it.
func removeSavedSignatures(ctx context.Context, store *data.Store, files *storage.Store, workspaces map[string]domain.Workspace, memberships []data.Membership) error {
	for _, m := range memberships {
		wsCtx := data.WithWorkspaceScope(ctx, m.WorkspaceID)
		for _, sm := range workspaces[m.WorkspaceSlug].MachinesInWorkflowRole(domain.WorkflowEngineDocumentApproval, domain.WorkflowRoleSignature) {
			fields := action.StoreFields(sm)
			if fields.OwnerField == "" {
				continue
			}
			records, err := store.ListRecordsBy(wsCtx, sm.ID, fields.OwnerField, m.UserRecordID)
			if err != nil {
				return err
			}
			for _, r := range records {
				if key, _ := r.Values[fields.ImageField].(string); key != "" && files != nil {
					if err := files.Remove(key); err != nil {
						log.Printf("account deletion: removing signature file %q: %v", key, err)
					}
				}
				if err := store.DeleteRecord(wsCtx, sm.ID, r.ID); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// submitDeleteAccount anonymizes the signed-in identity (data.AnonymizeIdentity) after the password
// is confirmed and no blocker applies, then ends the session.
func submitDeleteAccount(store *data.Store, files *storage.Store, workspaces map[string]domain.Workspace, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}
		email, ok := currentUserEmail(ctx, store, req, cfg)
		if !ok {
			http.Error(w, "no workspace membership for this identity", http.StatusForbidden)
			return
		}
		cred, err := store.GetCredential(ctx, email)
		if err != nil {
			serverError(w, err)
			return
		}
		if !authorization.VerifyPassword(req.FormValue("password"), cred.PasswordHash) {
			respondDeleteAccount(store, cfg, w, req, nil, "Password is incorrect.")
			return
		}
		memberships, err := store.ListMemberships(ctx, email)
		if err != nil {
			serverError(w, err)
			return
		}
		blockers, err := accountDeletionBlockers(ctx, store, workspaces, email, memberships)
		if err != nil {
			serverError(w, err)
			return
		}
		if len(blockers) > 0 {
			respondDeleteAccount(store, cfg, w, req, blockers, "")
			return
		}
		if err := removeSavedSignatures(ctx, store, files, workspaces, memberships); err != nil {
			serverError(w, err)
			return
		}
		if _, err := store.AnonymizeIdentity(ctx, email); err != nil {
			if errors.Is(err, data.ErrSoleWorkspaceAdmin) {
				respondDeleteAccount(store, cfg, w, req, []string{"Another admin changed while you were deleting: you are now the only admin of a workspace."}, "")
				return
			}
			serverError(w, err)
			return
		}
		authorization.ClearSessionCookie(w, cfg.SecureCookies)
		http.Redirect(w, req, "/delete-account?done=1", http.StatusSeeOther)
	}
}

func respondDeleteAccount(store *data.Store, cfg config.Config, w http.ResponseWriter, req *http.Request, blockers []string, passwordErr string) {
	page, err := deleteAccountPageAfter(store, cfg, req, blockers, passwordErr)
	if err != nil {
		serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusUnprocessableEntity)
	render(req.Context(), w, page)
}
