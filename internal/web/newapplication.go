package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/aiassist"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/installer"
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
func showNewApplication(store *data.Store, aiClient aiassist.Client, cfg config.Config) http.HandlerFunc {
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
		} else {
			// Workspace Home's own "Add an application" box/chips (Flow 2 canvas re-audit,
			// ROADMAP.md, 2026-09-27; auto-send reversal 2026-09-27) -- a plain GET query param.
			// This handler itself still performs no write: PrefillIdea only pre-fills the message
			// input. What sends it is client-side (rendering.NewApplicationPage's own form carries
			// an "on load" hyperscript that calls requestSubmit() when the input already holds a
			// value), so a fresh page load with ?idea= set turns into a real POST to
			// postNewApplicationMessage moments later, without the person clicking Send. No session
			// exists to prefill into once one has started, which is also what stops that auto-submit
			// from firing a second time: the redirect back to this handler lands with a session id
			// and an empty PrefillIdea.
			view.PrefillIdea = req.URL.Query().Get("idea")
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

		if err := runAssistantTurn(ctx, store, aiClient, cfg, workspaceID, session); err != nil {
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
// once its own proposed change passes aiassist.Validate.
//
// The Workspace is read from disk (workspaceInstallation), not from ctx, for the reason publish
// does: a Workspace created since the last reload is the zero Workspace on ctx, and the model would
// be told nothing is installed.
//
// A proposed change that does not validate gets one automatic correction round: the issues go back
// to the model as a turn and it answers once more. Without it the model's "your change is ready"
// stood in the conversation beside no Review button, and nobody told it why (2026-09-30: an
// extend_application with no additions, twice). One round, not a loop -- a model that cannot fix it
// once is talking to a person who can see its message and answer.
func runAssistantTurn(ctx context.Context, store *data.Store, aiClient aiassist.Client, cfg config.Config, workspaceID string, session *data.AISession) error {
	_, ws, err := workspaceInstallation(ctx, store, cfg)
	if err != nil {
		return err
	}
	prompt := aiassist.SystemPromptFor(installedApplicationsFor(ws), ws.MachineIDs)
	existing := existingStateFor(ws)
	for attempt := 0; attempt < 2; attempt++ {
		reply, err := generateReply(ctx, aiClient, prompt, session)
		if err != nil {
			return err
		}
		if err := storeModelTurn(ctx, store, workspaceID, session, reply); err != nil {
			return err
		}
		if reply.Change == nil {
			return nil
		}
		problem := aiassist.Validate(*reply.Change, existing)
		if problem == nil {
			return store.UpdateAISessionStatus(ctx, workspaceID, session.ID, data.AISessionStatusGenerated)
		}
		if attempt == 1 {
			return nil
		}
		report := "Automatic check: the change in your last reply does not validate, so it cannot be reviewed yet:\n\n" +
			problem.Error() + "\n\nCorrect it, or ask me what you need to know."
		if err := store.AppendAISessionTurn(ctx, session.ID, "user", report); err != nil {
			return err
		}
		session.Turns = append(session.Turns, data.AISessionTurn{Role: "user", Content: report})
	}
	return nil
}

// generateReply calls the model with the session's history. A failure the browser did not cause --
// most commonly aiassist.GeminiClient's own 60s timeout, observed live on schema-constrained calls --
// becomes an ordinary assistant turn asking to retry, rather than a plain-text error page landing
// on someone still watching the form they submitted.
func generateReply(ctx context.Context, aiClient aiassist.Client, prompt string, session *data.AISession) (aiassist.Reply, error) {
	turns := make([]aiassist.Turn, 0, len(session.Turns))
	for _, t := range session.Turns {
		turns = append(turns, aiassist.Turn{Role: t.Role, Text: t.Content})
	}
	reply, err := aiClient.Generate(ctx, prompt, turns)
	if err != nil {
		if ctx.Err() != nil {
			return aiassist.Reply{}, err
		}
		log.Printf("assistant turn failed, showing a retry message instead of an error page: %v", err)
		reply = aiassist.Reply{Message: "Menata didn't get a response in time. Please try sending your message again."}
	}
	return reply, nil
}

func storeModelTurn(ctx context.Context, store *data.Store, workspaceID string, session *data.AISession, reply aiassist.Reply) error {
	raw, err := json.Marshal(reply)
	if err != nil {
		return err
	}
	if err := store.AppendAISessionTurn(ctx, session.ID, "model", string(raw)); err != nil {
		return err
	}
	session.Turns = append(session.Turns, data.AISessionTurn{Role: "model", Content: string(raw)})
	if reply.CapabilityGap != nil {
		return store.RecordAICapabilityGap(ctx, session.ID, workspaceID, reply.CapabilityGap.Requested, reply.CapabilityGap.Note)
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
func publishNewApplication(store *data.Store, aiClient aiassist.Client, cfg config.Config, reload func() error) http.HandlerFunc {
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
		manifestPath, ws, err := workspaceInstallation(ctx, store, cfg)
		if err != nil {
			publishEnvironmentError(w, err)
			return
		}
		if err := aiassist.Validate(*change, existingStateFor(ws)); err != nil {
			returnToConversation(ctx, w, req, store, aiClient, cfg, session, err)
			return
		}

		newAppID, err := aiassist.Write(manifestPath, *change, aiassist.FileMachineResolver{WorkspaceManifestPath: manifestPath})
		if err != nil {
			failPublish(ctx, w, req, store, aiClient, cfg, session, err)
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
		if err := store.UpdateAISessionStatus(ctx, workspaceID, session.ID, data.AISessionStatusPublished); err != nil {
			serverError(w, err)
			return
		}

		// Grants the conversation's own answer to "which role will you hold yourself"
		// (aiassist.GeneratedApplication.PublisherRole, validated to be one of the Application's
		// own roles) -- without this, nobody held any role in a brand-new Application the moment
		// it existed, including its own creator, and the redirect below sent them straight into
		// requireApplicationAccess's 403 (found 2026-09-27 chasing a real conversation that hit
		// exactly that). Skipped when the Application declares no roles at all, matching
		// validateNewApplication's own reasoning: nothing gates on holding one there.
		if newAppID != "" && change.Application != nil && change.Application.PublisherRole != "" {
			actor := currentActor(req, store, cfg)
			if err := store.SetMemberAppRole(ctx, workspaceID, actor.ID, newAppID, change.Application.PublisherRole); err != nil {
				serverError(w, fmt.Errorf("metadata was published but granting your own role failed -- add it yourself from this application's Settings: %w", err))
				return
			}
		}

		if newAppID != "" {
			redirectTo(w, req, "/machines/"+firstMachineOf(*change))
			return
		}
		redirectTo(w, req, "/home")
	}
}

// failPublish routes a failed aiassist.Write by whose problem it is. A rejection goes back to the
// assistant to correct -- Write rolled its own writes back, so the tree is untouched. Anything else
// is the environment, which no corrected proposal can fix.
func failPublish(ctx context.Context, w http.ResponseWriter, req *http.Request, store *data.Store, aiClient aiassist.Client, cfg config.Config, session *data.AISession, err error) {
	var rejected *installer.RejectedError
	if errors.As(err, &rejected) {
		returnToConversation(ctx, w, req, store, aiClient, cfg, session, err)
		return
	}
	publishEnvironmentError(w, err)
}

// publishEnvironmentError is a publish that failed for a reason no change to the proposal can fix: a
// manifest that cannot be read or written, a disk write that failed. It is shown to the person and
// leaves the conversation alone -- no turn is added and the session keeps its status, so Publish can
// simply be pressed again once the cause is gone.
//
// Handing these to returnToConversation is what it did until 2026-09-30, when a missing manifest
// reached the model as a mistake to correct and the model answered "please try again", which could
// never succeed.
func publishEnvironmentError(w http.ResponseWriter, err error) {
	log.Printf("publish failed on the server, not on the proposal: %v", err)
	http.Error(w, "Publishing failed on the server, not because of the proposed application: "+err.Error()+
		"\n\nYour proposal is kept. Go back and press Publish again once this is fixed.", http.StatusInternalServerError)
}

// returnToConversation is what a failed publish does instead of a raw error page: it hands the
// problem back to the conversation the proposal came from, as a turn the assistant can read, and
// lets it answer with a corrected change.
//
// A publish failure is almost always a *fixable* one -- a proposal that stopped validating, or
// metadata that would not load (aiassist.Write rolls its own writes back before returning either,
// so nothing is half-applied by the time this runs). Those are exactly the things the assistant
// can repair, and it is the only participant that knows how: the person who clicked Publish did
// not write the metadata and has no way to correct it themselves. Showing them
// `http.Error(422, "1 issue(s): ...")` made a dead end out of something the conversation was
// already equipped to solve -- the same reasoning that made a Gemini timeout an ordinary
// assistant turn earlier the same day (runAssistantTurn's own doc comment).
//
// The session drops back to "open": the proposal that just failed is no longer offerable, and
// runAssistantTurn will mark it "generated" again if the assistant's next reply carries a change
// that actually validates.
func returnToConversation(ctx context.Context, w http.ResponseWriter, req *http.Request, store *data.Store, aiClient aiassist.Client, cfg config.Config, session *data.AISession, problem error) {
	// Phrased as the person reporting it, because that is the role the model's own history format
	// has for "here is what happened when we tried": a model turn would claim the assistant said
	// it, and the schema has no third voice.
	report := "Publishing this failed with:\n\n" + problem.Error() +
		"\n\nPlease correct the metadata so it passes, staying inside what you can actually generate."
	if err := store.AppendAISessionTurn(ctx, session.ID, "user", report); err != nil {
		serverError(w, err)
		return
	}
	session.Turns = append(session.Turns, data.AISessionTurn{Role: "user", Content: report})
	workspaceID, _ := data.WorkspaceScope(ctx)
	if err := store.UpdateAISessionStatus(ctx, workspaceID, session.ID, data.AISessionStatusOpen); err != nil {
		serverError(w, err)
		return
	}
	if err := runAssistantTurn(ctx, store, aiClient, cfg, workspaceID, session); err != nil {
		serverError(w, err)
		return
	}
	redirectTo(w, req, "/new-application?session="+session.ID)
}

// discardNewApplication marks a session discarded. No file was ever written for a session that
// never reached publish, so there is nothing on disk to clean up -- discarding is purely a status
// flag on the session row.
func discardNewApplication(store *data.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		workspaceID, _ := data.WorkspaceScope(ctx)

		// Resolved through this Workspace's own scope first, exactly as every sibling handler here does
		// (lines 63, 118, 204, 247). This one did not until 2026-09-29: it passed the URL parameter
		// straight to UpdateAISessionStatus, whose statement had no Workspace predicate either, so a
		// Workspace admin could discard *another* Workspace's draft Application by id. Both halves are
		// fixed -- the store carries the predicate now -- and this stays because a handler that reads
		// like its four siblings is how the next one gets written correctly.
		session, err := store.GetAISession(ctx, workspaceID, chi.URLParam(req, "session"))
		if err != nil {
			recordError(w, err)
			return
		}
		if err := store.UpdateAISessionStatus(ctx, workspaceID, session.ID, data.AISessionStatusDiscarded); err != nil {
			recordError(w, err)
			return
		}
		redirectTo(w, req, "/home")
	}
}

// --- shared helpers ------------------------------------------------------------------------------

// installedApplicationsFor is what the model is told about each installed Application: every
// Machine with its Fields (so a new relation or computed field can name real ids), and the menu in
// order with each item's id (so a relabel or reorder can name one). It used to pass bare Machine ids
// and no menu, which left the model unable to relate to an existing Machine's data or to name a menu
// item -- and it told the person both were impossible.
func installedApplicationsFor(ws domain.Workspace) []aiassist.InstalledApplication {
	byID := make(map[string]*domain.Machine, len(ws.Machines))
	for _, m := range ws.Machines {
		byID[m.ID] = m
	}
	out := make([]aiassist.InstalledApplication, 0, len(ws.Applications))
	for _, app := range ws.Applications {
		var machineSummaries []string
		for _, mID := range app.Machines {
			m, ok := byID[mID]
			if !ok {
				machineSummaries = append(machineSummaries, mID)
				continue
			}
			fields := make([]string, 0, len(m.Fields))
			for _, f := range m.Fields {
				fields = append(fields, fmt.Sprintf("%s %s: %s", f.ID, f.Name, f.Type))
			}
			machineSummaries = append(machineSummaries, fmt.Sprintf("%s %s (%s)", m.ID, m.Name, strings.Join(fields, ", ")))
		}
		navItems := make([]string, 0, len(app.AllNavigation))
		for _, n := range app.AllNavigation {
			navItems = append(navItems, fmt.Sprintf("%s: %s -> %s", n.ID, n.Label, n.Route))
		}
		out = append(out, aiassist.InstalledApplication{
			ID: app.ID, Name: app.Name, Description: app.Description,
			Roles: app.Roles, MachineSummaries: machineSummaries, NavItems: navItems,
		})
	}
	return out
}

// existingStateFor builds what aiassist.Validate checks a proposal against: the ids already taken
// *in this Workspace*, which is the whole scope that matters now.
//
// Between 2026-09-27 morning and the Workspace-isolation change later the same day this also
// seeded every Machine id in the process, because back then ids genuinely were a global namespace:
// cmd/server deduped Machines by id across Workspaces, and a generated Machine's file was named
// from its id alone into one shared directory -- which is how a generated Application for the
// empty "Dokter Kecil" Workspace came to overwrite the real Document Approval mch_document
// installed in "default". Both causes are gone: a Workspace holds its own Machines, and a
// generated Application is written into its own metadata/workspaces/<slug>/ directory. Widening
// the check would now *refuse a legitimate proposal* -- a new Workspace naming its own
// mch_document is exactly what isolation makes correct.
//
// aiassist.refuseIfExists still stands behind this at the filesystem, and is the guard that holds
// under either model: whatever the caller believed about collisions, no write may land on a file
// that already exists.
func existingStateFor(ws domain.Workspace) aiassist.ExistingState {
	state := aiassist.ExistingState{
		MachineIDs:     map[string]bool{},
		ApplicationIDs: map[string]bool{},
		Applications:   map[string]aiassist.ExistingApplicationState{},
		NavIDs:         map[string]bool{},
	}
	for _, n := range ws.Navigation {
		state.NavIDs[n.ID] = true
	}
	for _, id := range ws.MachineIDs {
		state.MachineIDs[id] = true
	}
	byID := make(map[string]*domain.Machine, len(ws.Machines))
	for _, m := range ws.Machines {
		byID[m.ID] = m
	}
	for _, app := range ws.Applications {
		state.ApplicationIDs[app.ID] = true
		// Applications was declared but never populated here since this package's own first
		// commit, which meant aiassist.Validate's extend_application path always failed --
		// existing.Applications[change.TargetAppID] could never be found, so every extend request
		// was rejected with "application X is not installed in this workspace" even when it plainly
		// was. Found 2026-09-27 chasing a conversation where the assistant tried exactly that path
		// (after a generated Application's own creator hit "you have no role in it" -- see
		// publishNewApplication's own doc comment) and could not get past this.
		claimed := make(map[string]*domain.Machine, len(app.Machines))
		for _, mID := range app.Machines {
			if m, ok := byID[mID]; ok {
				claimed[mID] = m
			}
		}
		nav := make([]aiassist.ExistingNavItem, 0, len(app.AllNavigation))
		for _, n := range app.AllNavigation {
			nav = append(nav, aiassist.ExistingNavItem{ID: n.ID, Label: n.Label})
			state.NavIDs[n.ID] = true
		}
		state.Applications[app.ID] = aiassist.ExistingApplicationState{Name: app.Name, Roles: app.Roles, Machines: claimed, Navigation: nav}
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
	// Only a change that validated (runAssistantTurn sets "generated" for exactly that) is offered for
	// review. hasChange alone put a Review button under a change the automatic check had just refused,
	// and the review page then answered 422 (2026-09-30).
	view.ReadyToReview = hasChange && session.Status == data.AISessionStatusGenerated
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

// showHomeDraftApplications serves Workspace Home's own lazily-fetched "draft Application" row(s)
// (Flow 2 canvas re-audit, ROADMAP.md, 2026-09-27, workspacehome.templ's own hx-get placeholder) --
// every ai_sessions row in this Workspace whose status is "generated" (a validated proposal
// exists, not yet published or discarded), resolved via latestChange, the same pure decode of
// already-stored turns showNewApplicationReview already uses -- no Gemini call. Not threaded
// through showWorkspaceHome's own eager render: /home is already at this repo's hard query-cost
// ceiling (maxQueriesPerAuthenticatedPage), so this query is paid here instead, only when the
// admin-gated section that triggers it is even reachable.
func showHomeDraftApplications(store *data.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		workspaceID, _ := data.WorkspaceScope(ctx)

		sessions, err := store.ListAISessionsByStatus(ctx, workspaceID, aiassist.KindNewApplication, data.AISessionStatusGenerated)
		if err != nil {
			serverError(w, err)
			return
		}
		rows := make([]rendering.HomeDraftApplicationRow, 0, len(sessions))
		for _, s := range sessions {
			full, err := store.GetAISession(ctx, workspaceID, s.ID)
			if err != nil {
				serverError(w, err)
				return
			}
			change, ok, err := latestChange(full)
			if err != nil || !ok || change.Application == nil {
				continue
			}
			rows = append(rows, rendering.HomeDraftApplicationRow{
				Name:        change.Application.Name,
				Description: change.Application.Description,
				ReviewHref:  "/new-application/" + s.ID + "/review",
			})
		}
		render(ctx, w, rendering.HomeDraftApplicationRows(rows))
	}
}
