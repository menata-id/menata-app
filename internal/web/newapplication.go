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

		if err := runAssistantTurn(ctx, store, aiClient, cfg, workspaceID, session, currentActor(req, store, cfg)); err != nil {
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
func runAssistantTurn(ctx context.Context, store *data.Store, aiClient aiassist.Client, cfg config.Config, workspaceID string, session *data.AISession, actor domain.Actor) error {
	_, ws, err := workspaceInstallation(ctx, store, cfg)
	if err != nil {
		return err
	}
	installed := installedApplicationsFor(ws)
	prompt := aiassist.SystemPromptFor(installed, ws.MachineIDs) + aiassist.ViewerRolesSection(installed, actor.Roles)
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
			if attempt == 1 {
				logAIReject(ctx, session.ID, "check", "recovered", 2, nil)
			}
			return store.UpdateAISessionStatus(ctx, workspaceID, session.ID, data.AISessionStatusGenerated)
		}
		if attempt == 1 {
			logAIReject(ctx, session.ID, "check", "unresolved", 2, problem)
			return nil
		}
		logAIReject(ctx, session.ID, "check", "pending", 1, problem)
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
	return recordCapabilityGaps(ctx, store, workspaceID, session.ID, reply)
}

// recordCapabilityGaps writes one row and one AI-GAP log line per gap a reply names. The log line is
// the reader (owner decision 2026-10-15: gaps feed the development process, not a screen), so it is
// a stable key=value line and carries the request text quoted, never the conversation. The fields that
// existed first keep their places; the specification fields (plan 2026-10-25, A1) follow, and a reader
// picks every one by name.
func recordCapabilityGaps(ctx context.Context, store *data.Store, workspaceID, sessionID string, reply aiassist.Reply) error {
	for _, g := range reply.Gaps() {
		if err := store.RecordAICapabilityGap(ctx, sessionID, workspaceID, data.AICapabilityGap{
			Kind: g.Kind, Requested: g.Requested, Note: g.Note,
			CapabilityKey: g.CapabilityKey, Class: g.Class, IdealCapability: g.IdealCapability,
			WantedShape: g.WantedShape, Where: g.Where, Intent: g.Intent, Benchmark: g.Benchmark,
		}); err != nil {
			return err
		}
		log.Printf("AI-GAP kind=%s workspace=%s session=%s requested=%q note=%q key=%s class=%s ideal=%q shape=%q where=%q intent=%q benchmark=%q",
			g.Kind, workspaceID, sessionID, g.Requested, g.Note, g.CapabilityKey, g.Class, g.IdealCapability, g.WantedShape, g.Where, g.Intent, g.Benchmark)
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
		render(ctx, w, rendering.NewApplicationReviewPage(*change, reviewPlan(*change, ws), generatedFiles(ctx, store, cfg, *change), reviewAccess(ctx, store, *change), unsupportedOf(session), session.ID, chrome.WorkspaceName, chrome.Viewer(), switchHref))
	}
}

// generatedFiles is aiassist.Preview for the review page: the writer's own output against a staged copy, never a second
// rendering of the change. A process with no metadata directory has nothing to stage and shows no panel; a change the
// writer refuses shows its reason in place of the files.
func generatedFiles(ctx context.Context, store *data.Store, cfg config.Config, change aiassist.GeneratedChange) rendering.GeneratedFilesView {
	if cfg.MetadataPath == "" {
		return rendering.GeneratedFilesView{}
	}
	manifestPath, _, err := workspaceInstallation(ctx, store, cfg)
	if err != nil {
		return rendering.GeneratedFilesView{Reason: err.Error()}
	}
	changes, err := aiassist.Preview(manifestPath, change)
	if err != nil {
		return rendering.GeneratedFilesView{Reason: err.Error()}
	}
	view := rendering.GeneratedFilesView{}
	for _, f := range changes {
		view.Files = append(view.Files, rendering.FileDiffView{Path: f.Path, Changed: true, Diff: f.Diff})
	}
	return view
}

// publishNewApplication re-validates (never trusting a stale review render), writes the change to
// disk via aiassist.Write, and reloads the whole route table live -- Deps.ReloadMetadata, injected
// by cmd/server, since this package may not import internal/metadata itself
// (TestPlaneBoundaries). On any failure at any step nothing partial is left live: aiassist.Write's
// own temp-file-then-rename discipline means a failed write leaves no half-written file, and a
// failed reload leaves the previous route table serving traffic untouched.
func publishNewApplication(store *data.Store, aiClient aiassist.Client, cfg config.Config, reload func(slug string) error) http.HandlerFunc {
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

		if !confirmRemovals(w, req, change, ws) {
			return
		}
		access, ok := readAccess(w, req, store, workspaceID, change.Application)
		if !ok {
			return
		}
		plan, isUpdate := publishedPlan(*change, existingStateFor(ws))
		snapshotBeforeWrite(cfg, manifestPath)
		newAppID, err := aiassist.Write(manifestPath, *change, aiassist.FileMachineResolver{WorkspaceManifestPath: manifestPath})
		if err != nil {
			failPublish(ctx, w, req, store, aiClient, cfg, session, err)
			return
		}
		if reload == nil {
			serverError(w, fmt.Errorf("metadata was written but this process has no reload hook configured -- a restart will pick it up"))
			return
		}
		if err := reloadCurrentWorkspace(ctx, store, reload); err != nil {
			serverError(w, fmt.Errorf("metadata was written but the live reload failed -- a process restart will pick it up: %w", err))
			return
		}
		if err := markPublished(ctx, store, workspaceID, session.ID, *change, newAppID, plan, isUpdate); err != nil {
			serverError(w, err)
			return
		}

		if !grantPublishedRoles(ctx, w, req, store, cfg, workspaceID, *change, newAppID, access) {
			return
		}

		redirectTo(w, req, publishedLanding())
	}
}

// grantPublishedRoles gives, once an Application is published, the publisher the role the conversation asked them to hold and then the
// others the access the review chose (grantPublishedAccess). It answers the failure itself and returns false.
func grantPublishedRoles(ctx context.Context, w http.ResponseWriter, req *http.Request, store *data.Store, cfg config.Config, workspaceID string, change aiassist.GeneratedChange, newAppID string, access accessGrant) bool {
	actor := currentActor(req, store, cfg)
	// The conversation's own answer to "which role will you hold yourself" (aiassist.GeneratedApplication.PublisherRole, validated to be
	// one of the Application's own roles). Without it nobody held any role in a brand-new Application the moment it existed, its creator
	// included, and the redirect after publishing sent them straight into requireApplicationAccess's 403 (found 2026-09-27). Skipped when
	// the Application declares no roles at all, matching validateNewApplication: nothing gates on holding one there.
	if newAppID != "" && change.Application != nil && change.Application.PublisherRole != "" {
		if err := store.SetMemberAppRole(ctx, workspaceID, actor.ID, newAppID, change.Application.PublisherRole); err != nil {
			serverError(w, fmt.Errorf("metadata was published but granting your own role failed -- add it yourself from this application's Settings: %w", err))
			return false
		}
	}
	if newAppID != "" {
		if err := grantPublishedAccess(ctx, store, workspaceID, newAppID, actor.ID, access); err != nil {
			serverError(w, fmt.Errorf("metadata was published but giving the others their role failed -- set it from this application's Members & roles: %w", err))
			return false
		}
	}
	return true
}

// publishedLanding is where a publication leaves the person: Workspace Home, where the new Application is the first of the list and, for ten
// minutes, a "Take back" is offered beside it (N02: a publication can be undone). It used to open the new Application's first Machine, which left
// no place to say that.
func publishedLanding() string { return "/home" }

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
	logAIReject(ctx, session.ID, "publish", "returned", 1, problem)
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
	if err := runAssistantTurn(ctx, store, aiClient, cfg, workspaceID, session, currentActor(req, store, cfg)); err != nil {
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

// installedApplicationsFor is what the model is told about each installed Application: its current
// definition in the exact shape an update returns (aiassist.DescribeApplication), so updating is
// editing what it was shown rather than reconstructing it from a summary.
func installedApplicationsFor(ws domain.Workspace) []aiassist.InstalledApplication {
	state := existingStateFor(ws)
	out := make([]aiassist.InstalledApplication, 0, len(ws.Applications))
	for _, app := range ws.Applications {
		out = append(out, aiassist.InstalledApplication{Current: aiassist.DescribeApplication(app.ID, state.Applications[app.ID])})
	}
	return out
}

// reviewPlan is an update's plan against the installed Application, for the review page; empty for
// a new Application, whose review shows the Application itself.
func reviewPlan(change aiassist.GeneratedChange, ws domain.Workspace) aiassist.Plan {
	if change.Kind != aiassist.KindUpdateApplication || change.Application == nil {
		return aiassist.Plan{}
	}
	current := existingStateFor(ws).Applications[change.TargetAppID]
	return aiassist.PlanUpdate(aiassist.DescribeApplication(change.TargetAppID, current), *change.Application)
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
	return aiassist.ExistingStateFrom(ws)
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
		var named []string
		for _, g := range reply.Gaps() {
			named = append(named, g.Requested)
		}
		view.Turns[len(view.Turns)-1].CapabilityGap = strings.Join(named, "; ")
	}
	view.Unsupported = unsupportedOf(session)
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

// confirmRemovals reads the removals the owner ticked at review into the change and answers 422 itself when
// the plan asks for one that is not ticked. The model cannot tick them: ConfirmedRemovals is not in its schema.
func confirmRemovals(w http.ResponseWriter, req *http.Request, change *aiassist.GeneratedChange, ws domain.Workspace) bool {
	if err := req.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return false
	}
	change.ConfirmedRemovals = req.Form["confirm_remove"]
	if open := reviewPlan(*change, ws).Unconfirmed(change.ConfirmedRemovals); len(open) > 0 {
		http.Error(w, "this update removes something the owner has not confirmed: "+strings.Join(open, ", "), http.StatusUnprocessableEntity)
		return false
	}
	return true
}

// unsupportedOf is every capability gap the assistant named across a conversation, once each and in the order they came: what the person asked for
// that Menata cannot build yet. It reads the same stored replies the gap log is written from, so the screen and the log cannot disagree.
func unsupportedOf(session *data.AISession) []string {
	var out []string
	seen := map[string]bool{}
	for _, t := range session.Turns {
		if t.Role == "user" {
			continue
		}
		var reply aiassist.Reply
		if json.Unmarshal([]byte(t.Content), &reply) != nil {
			continue
		}
		for _, g := range reply.Gaps() {
			key := strings.ToLower(strings.TrimSpace(g.Requested))
			if key != "" && !seen[key] {
				seen[key] = true
				out = append(out, strings.TrimSpace(g.Requested))
			}
		}
	}
	return out
}
