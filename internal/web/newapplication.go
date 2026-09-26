package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/aiassist"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// showNewApplication serves the AI Metadata Assistant's own conversation screen (Flow 2 gap study
// Tahap 8, NewApp.dc.html/M03c-NewApp.dc.html): describe a new application, or a change to one
// already installed, in plain language. requireWorkspaceAdmin-gated, same group as
// /workspace-settings/-members/-groups -- matching the mockup's own "Only workspace admins can add
// applications" and the owner's own "whoever creates a workspace can build an application" (they
// are always that workspace's admin, per registerWorkspace).
//
// Writes nothing: with no ?session=, it renders an empty conversation (no session exists yet --
// internal/conformance.TestGetRoutesDoNotWrite's own discipline, applied here even though its
// specific call-graph walk only names CreateRecord/UpdateRecord/DeleteRecord and would not itself
// catch a GET that created a bespoke row). The session is created lazily by the first
// postNewApplicationMessage call instead, the same "GET never writes, the wizard's own first POST
// does" shape the Document submit wizard already established.
//
// Scope, stated once rather than left to be discovered: this is a plain form POST + redirect flow
// (POST /new-application/message -> redirect back to this same GET), not an htmx fragment swap --
// there is no chat-log precedent anywhere else in this app to extend, and a full round trip per
// message is a reasonable, honest simplification for a screen an admin uses rarely and
// deliberately, not one anyone is optimizing perceived latency on.
func showNewApplication(machines map[string]*domain.Machine, store *data.Store, aiClient aiassist.Client, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if cfg.GeminiAPIKey == "" {
			// A clear, real page rather than a 403 or a broken conversation -- see appshell.templ's
			// own comment on why the entry point stays visible rather than being hidden for this
			// one config-dependent case.
			http.Error(w, "The AI assistant is not configured yet (GEMINI_API_KEY is unset). Ask whoever runs this workspace to set it up.", http.StatusServiceUnavailable)
			return
		}
		ctx := req.Context()
		workspaceID, _ := data.WorkspaceScope(ctx)
		sessionID := req.URL.Query().Get("session")

		chrome, err := resolveChrome(ctx, req, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		actorID := currentActor(req, store, cfg).ID
		_, switchHref := viewerWorkspaceContext(ctx, store, actorID)

		view := rendering.ConversationView{}
		if sessionID != "" {
			session, err := store.GetAISession(ctx, workspaceID, sessionID)
			if err != nil {
				recordError(w, err)
				return
			}
			view, err = buildConversationView(session)
			if err != nil {
				serverError(w, err)
				return
			}
		}
		render(ctx, w, rendering.NewApplicationPage(view, chrome.WorkspaceName, chrome.Viewer(), switchHref))
	}
}

// postNewApplicationMessage creates the session on its first call (empty "session" form value --
// the lazy-create half of showNewApplication's own "GET never writes" discipline) or appends to an
// existing one, then calls the configured aiassist.Client with the full history and the
// capability-tiered system prompt (grounded in this workspace's own currently installed
// applications, so an extend_application request can be evaluated against what is actually real),
// appends the assistant's reply, records a capability gap if the reply names one, and redirects
// back to the conversation screen.
func postNewApplicationMessage(store *data.Store, aiClient aiassist.Client, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}
		ctx := req.Context()
		workspaceID, _ := data.WorkspaceScope(ctx)
		sessionID := req.FormValue("session")
		message := strings.TrimSpace(req.FormValue("message"))
		if message == "" {
			http.Error(w, "message is required", http.StatusUnprocessableEntity)
			return
		}

		var session *data.AISession
		var err error
		if sessionID == "" {
			actor := currentActor(req, store, cfg)
			session, err = store.CreateAISession(ctx, workspaceID, actor.ID, aiassist.KindNewApplication)
		} else {
			session, err = store.GetAISession(ctx, workspaceID, sessionID)
		}
		if err != nil {
			recordError(w, err)
			return
		}
		if err := store.AppendAISessionTurn(ctx, session.ID, "user", message); err != nil {
			serverError(w, err)
			return
		}
		session.Turns = append(session.Turns, data.AISessionTurn{Role: "user", Content: message})

		if err := runAssistantTurn(ctx, store, aiClient, workspaceID, session); err != nil {
			serverError(w, err)
			return
		}
		redirectTo(w, req, "/new-application?session="+session.ID)
	}
}

// runAssistantTurn is postNewApplicationMessage's own derivation, split out to stay inside
// internal/conformance.TestHandlersStaySmall's budget: calls the configured aiassist.Client with
// this workspace's own capability-tiered system prompt and the session's full history so far,
// stores the raw structured reply as the next turn (buildConversationView/latestChange decode it
// back), records a capability gap if the reply names one, and marks the session "generated" only
// once its own proposed change passes aiassist.Validate -- a change that fails is left as an
// ordinary conversational turn, never surfaced to the review step.
func runAssistantTurn(ctx context.Context, store *data.Store, aiClient aiassist.Client, workspaceID string, session *data.AISession) error {
	ws := rendering.CurrentWorkspace(ctx)
	prompt := aiassist.SystemPromptFor(installedApplicationsFor(ws))
	turns := make([]aiassist.Turn, 0, len(session.Turns))
	for _, t := range session.Turns {
		turns = append(turns, aiassist.Turn{Role: t.Role, Text: t.Content})
	}

	reply, err := aiClient.Generate(ctx, prompt, turns)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(reply)
	if err != nil {
		return err
	}
	if err := store.AppendAISessionTurn(ctx, session.ID, "model", string(raw)); err != nil {
		return err
	}

	if reply.CapabilityGap != nil {
		if err := store.RecordAICapabilityGap(ctx, session.ID, workspaceID, reply.CapabilityGap.Requested, reply.CapabilityGap.Note); err != nil {
			return err
		}
	}
	if reply.Change != nil && aiassist.Validate(*reply.Change, existingStateFor(ws)) == nil {
		if err := store.UpdateAISessionStatus(ctx, session.ID, data.AISessionStatusGenerated); err != nil {
			return err
		}
	}
	return nil
}

// showNewApplicationReview renders NewAppReview.dc.html's shape once a session holds a validated
// GeneratedChange -- re-validates rather than trusting the session's own "generated" status alone,
// since this workspace's installed applications may have changed since that status was set.
func showNewApplicationReview(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		workspaceID, _ := data.WorkspaceScope(ctx)
		sessionID := chi.URLParam(req, "session")

		session, err := store.GetAISession(ctx, workspaceID, sessionID)
		if err != nil {
			recordError(w, err)
			return
		}
		change, _, err := latestChange(session)
		if err != nil {
			serverError(w, err)
			return
		}
		if change == nil {
			http.Error(w, "this conversation has no proposed change ready to review yet", http.StatusUnprocessableEntity)
			return
		}
		ws := rendering.CurrentWorkspace(ctx)
		if err := aiassist.Validate(*change, existingStateFor(ws)); err != nil {
			http.Error(w, "this proposal is no longer valid: "+err.Error(), http.StatusUnprocessableEntity)
			return
		}

		chrome, err := resolveChrome(ctx, req, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		actorID := currentActor(req, store, cfg).ID
		_, switchHref := viewerWorkspaceContext(ctx, store, actorID)
		render(ctx, w, rendering.NewApplicationReviewPage(*change, session.ID, chrome.WorkspaceName, chrome.Viewer(), switchHref))
	}
}

// publishNewApplication re-validates (never trusting a stale review render), writes the change to
// disk via aiassist.Write, and reloads the whole route table live -- Deps.ReloadMetadata, injected
// by cmd/server, since this package may not import internal/metadata itself
// (TestPlaneBoundaries). On any failure at any step nothing partial is left live: aiassist.Write's
// own temp-file-then-rename discipline means a failed write leaves no half-written file, and a
// failed reload leaves the previous route table serving traffic untouched.
func publishNewApplication(machines map[string]*domain.Machine, store *data.Store, cfg config.Config, reload func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		workspaceID, _ := data.WorkspaceScope(ctx)
		sessionID := chi.URLParam(req, "session")

		session, err := store.GetAISession(ctx, workspaceID, sessionID)
		if err != nil {
			recordError(w, err)
			return
		}
		change, _, err := latestChange(session)
		if err != nil {
			serverError(w, err)
			return
		}
		if change == nil {
			http.Error(w, "this conversation has no proposed change ready to publish", http.StatusUnprocessableEntity)
			return
		}
		ws := rendering.CurrentWorkspace(ctx)
		if err := aiassist.Validate(*change, existingStateFor(ws)); err != nil {
			http.Error(w, "this proposal is no longer valid: "+err.Error(), http.StatusUnprocessableEntity)
			return
		}

		manifestPath := filepath.Join(cfg.MetadataPath, ws.Slug+".yaml")
		newAppID, err := aiassist.Write(filepath.Dir(cfg.MetadataPath), manifestPath, *change, aiassist.FileMachineResolver{WorkspaceManifestPath: manifestPath})
		if err != nil {
			serverError(w, fmt.Errorf("write generated metadata: %w", err))
			return
		}
		if reload == nil {
			serverError(w, fmt.Errorf("metadata was written but this process has no reload hook configured -- a restart will pick it up"))
			return
		}
		if err := reload(); err != nil {
			serverError(w, fmt.Errorf("metadata was written but the live reload failed -- a process restart will pick it up: %w", err))
			return
		}
		if err := store.UpdateAISessionStatus(ctx, session.ID, data.AISessionStatusPublished); err != nil {
			serverError(w, err)
			return
		}

		if newAppID != "" {
			redirectTo(w, req, "/machines/"+firstMachineOf(*change))
			return
		}
		redirectTo(w, req, "/home")
	}
}

// discardNewApplication marks a session discarded. No file was ever written for a session that
// never reached publish, so there is nothing on disk to clean up -- discarding is purely a status
// flag on the session row.
func discardNewApplication(store *data.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		sessionID := chi.URLParam(req, "session")
		if err := store.UpdateAISessionStatus(ctx, sessionID, data.AISessionStatusDiscarded); err != nil {
			recordError(w, err)
			return
		}
		redirectTo(w, req, "/home")
	}
}

// --- shared helpers ------------------------------------------------------------------------------

func installedApplicationsFor(ws domain.Workspace) []aiassist.InstalledApplication {
	out := make([]aiassist.InstalledApplication, 0, len(ws.Applications))
	for _, app := range ws.Applications {
		var machineSummaries []string
		for _, mID := range app.Machines {
			// Field names are not available here without the full Machine map; the id alone still
			// grounds the assistant in what exists, which is what matters for an extend_application
			// request naming a machine to add an option to.
			machineSummaries = append(machineSummaries, mID)
		}
		out = append(out, aiassist.InstalledApplication{
			ID: app.ID, Name: app.Name, Description: app.Description,
			Roles: app.Roles, MachineSummaries: machineSummaries,
		})
	}
	return out
}

func existingStateFor(ws domain.Workspace) aiassist.ExistingState {
	state := aiassist.ExistingState{
		MachineIDs:     map[string]bool{},
		ApplicationIDs: map[string]bool{},
		Applications:   map[string]aiassist.ExistingApplicationState{},
	}
	for _, id := range ws.MachineIDs {
		state.MachineIDs[id] = true
	}
	for _, app := range ws.Applications {
		state.ApplicationIDs[app.ID] = true
	}
	return state
}

// buildConversationView decodes every stored turn for display: a user turn renders as-is; a model
// turn is the raw JSON aiassist.Reply this handler stored, decoded back to show its own Message
// (never the raw JSON) to the person reading the conversation.
func buildConversationView(session *data.AISession) (rendering.ConversationView, error) {
	view := rendering.ConversationView{SessionID: session.ID}
	for _, t := range session.Turns {
		if t.Role == "user" {
			view.Turns = append(view.Turns, rendering.ConversationTurn{FromUser: true, Message: t.Content})
			continue
		}
		var reply aiassist.Reply
		if err := json.Unmarshal([]byte(t.Content), &reply); err != nil {
			return rendering.ConversationView{}, fmt.Errorf("decode stored assistant turn: %w", err)
		}
		view.Turns = append(view.Turns, rendering.ConversationTurn{Message: reply.Message})
		if reply.CapabilityGap != nil {
			view.Turns[len(view.Turns)-1].CapabilityGap = reply.CapabilityGap.Requested
		}
	}
	change, hasChange, err := latestChange(session)
	if err != nil {
		return rendering.ConversationView{}, err
	}
	view.ReadyToReview = hasChange
	if change != nil {
		view.Summary = *change
	}
	return view, nil
}

// latestChange scans a session's own turns, most recent first, for the last assistant reply that
// carried a change -- what the review/publish handlers act on. Returns (nil, false, nil) when none
// exists yet.
func latestChange(session *data.AISession) (*aiassist.GeneratedChange, bool, error) {
	for i := len(session.Turns) - 1; i >= 0; i-- {
		t := session.Turns[i]
		if t.Role != "model" {
			continue
		}
		var reply aiassist.Reply
		if err := json.Unmarshal([]byte(t.Content), &reply); err != nil {
			return nil, false, fmt.Errorf("decode stored assistant turn: %w", err)
		}
		if reply.Change != nil {
			return reply.Change, true, nil
		}
	}
	return nil, false, nil
}

func firstMachineOf(change aiassist.GeneratedChange) string {
	if change.Application != nil && len(change.Application.Machines) > 0 {
		return change.Application.Machines[0].ID
	}
	return ""
}
