package web

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"mime/multipart"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"menata.app/internal/action"
	"menata.app/internal/authorization"
	"menata.app/internal/behavior"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/execution"
	"menata.app/internal/experience"
	"menata.app/internal/mail"
	"menata.app/internal/rendering"
	"menata.app/internal/storage"
)

func createRecordForm(store *data.Store, files *storage.Store, mailer mail.Mailer, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machines := machinesFor(req.Context())
		machine, ok := resolveMachine(w, req)
		if !ok {
			return
		}
		if !parseRecordForm(w, req, cfg.MaxUploadBytes()) {
			return
		}

		values := data.ValuesFromForm(machine, req.Form)
		uploaded, err := handleFileUploads(req, machine, files)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		for k, v := range uploaded {
			values[k] = v
		}
		record, actor, ok := createFromValues(w, req, store, files, mailer, cfg, machine, values)
		if !ok {
			return
		}
		// A form that says `then: record` opens what it made: the record's own page.
		if req.Form.Get(domain.FormThenParam) == domain.FormThenRecord {
			w.Header().Set("HX-Redirect", recordPageRoute(machine, record.ID))
			return
		}

		if fromAnotherPage(req, machine) {
			w.Header().Set("HX-Refresh", "true")
			return
		}
		renderMachineBody(w, req, machines, machine, store, actor)
	}
}

// createFromValues is the generic create pipeline every form-driven create goes through: defaults, computed and
// stamped Fields, the create Permission, validation, the write, then the Machine's on_create Events. It answers
// the request itself when any step refuses, so a caller only continues on ok.
func createFromValues(w http.ResponseWriter, req *http.Request, store *data.Store, files *storage.Store, mailer mail.Mailer, cfg config.Config, machine *domain.Machine, values map[string]any) (*data.Record, domain.Actor, bool) {
	record, actor, ok := writeFromValues(w, req, store, cfg, machine, values)
	if !ok {
		return nil, actor, false
	}
	execution.RunCreateEvents(req.Context(), execution.Services{Store: store, Mailer: mailer, Files: files}, machinesFor(req.Context()), machine, record, actor.ID)
	return record, actor, true
}

// writeFromValues is createFromValues without the Machine's on_create Events: the write itself, which joins a transaction
// the request's context already carries. A caller that writes several records as one (a document's proposal: a contract and
// its instalments) writes each through here inside one transaction and runs the Events of all of them once it has committed,
// so a record that was rolled back has announced nothing.
func writeFromValues(w http.ResponseWriter, req *http.Request, store *data.Store, cfg config.Config, machine *domain.Machine, values map[string]any) (*data.Record, domain.Actor, bool) {
	data.ApplyDefaults(machine, values)
	execution.ApplyComputed(req.Context(), store, machine, "", values)

	actor := currentActor(req, store, cfg)
	data.ApplyStamps(machine, values, actor.ID)
	if !allowsRecordCreate(w, req, store, machine, values, actor) {
		return nil, actor, false
	}
	machines := machinesFor(req.Context())
	var record *data.Record
	// The checks and the insert are one transaction under the locks a total over this Machine calls for (writelock.go).
	ok := withWriteLock(w, req, store, machines, machine, "", values, func(req *http.Request) bool {
		// A numbered Field is the runtime's: its number is written here, under its lock, before the record is judged.
		if err := execution.AssignNumbers(req.Context(), store, machine, values, time.Now()); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return false
		}
		// What is derived from the number (a computed Field reading it) is derived from the number just written.
		execution.ApplyComputed(req.Context(), store, machine, "", values)
		if !validRecord(w, req, store, machine, values) || !notFrozen(w, req, store, machine, "", values) || !withinBounds(w, req, store, machine, "", values) {
			return false
		}
		var err error
		if record, err = store.CreateRecord(req.Context(), machine.ID, values); err != nil {
			serverError(w, err)
			return false
		}
		// What the new record changes in a parent's aggregates is written in the same transaction.
		if err := execution.RecomputeParents(req.Context(), store, machines, machine, record.Values); err != nil {
			serverError(w, err)
			return false
		}
		return true
	})
	if !ok {
		return nil, actor, false
	}
	return record, actor, true
}

// recordPageRoute is where a record of machine opens: the page its Application declares (`record_of:`), else the
// generic record route.
func recordPageRoute(machine *domain.Machine, id string) string {
	if machine.RecordPage != "" {
		return machine.RecordPage + "/" + url.PathEscape(id)
	}
	return "/machines/" + machine.ID + "/records/" + url.PathEscape(id)
}

// fromAnotherPage says the request came from a screen that is not this Machine's own page -- a Task's
// checklist adding an item from the Task's detail page -- so the Machine body it would answer with has
// nowhere to be swapped in. A caller that names no page at all (a plain form post, a test) is not
// "another page" and keeps the body.
func fromAnotherPage(req *http.Request, machine *domain.Machine) bool {
	return req.Header.Get("HX-Current-URL") != "" && !onMachinePage(req, machine)
}

// showRecordRow serves three different renderings of the same Record from one route, depending
// on who's asking (development-history.md Phase 8): a direct browser navigation gets the full detail page;
// an HTMX request targeting the detail page's own container gets just that container's view
// fragment (used by the detail page's own Cancel-from-edit); any other HTMX request (a table row
// or board card's Cancel) gets the original RecordRow fragment, unchanged from Phase 1.
func showRecordRow(store *data.Store, files *storage.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machines := machinesFor(req.Context())
		machine, ok := resolveMachine(w, req)
		if !ok {
			return
		}
		// The Loader first, and the record through it: the signature-placement embed below reads this
		// same Document again, and a direct store read leaves nothing to memoize against.
		ld := composition.NewLoader(store, machines).WithFiles(files)
		record, err := ld.Record(req.Context(), machine.ID, chi.URLParam(req, "id"))
		if err != nil {
			recordError(w, err)
			return
		}
		relations, err := ld.RelationOptions(req.Context(), machine)
		if err != nil {
			serverError(w, err)
			return
		}
		groups, err := ld.GroupOptions(req.Context(), machine)
		if err != nil {
			serverError(w, err)
			return
		}
		actor := currentActor(req, store, cfg)

		if req.Header.Get("HX-Request") != "true" {
			children, err := ld.ChildSections(req.Context(), machine, record.ID)
			if err != nil {
				serverError(w, err)
				return
			}
			sigPlacement := documentSignaturePlacementView(req.Context(), ld, files, machines, machine, record.ID, actor)
			extras, err := ld.RecordExtras(req.Context(), machine, record, relations)
			if err != nil {
				serverError(w, err)
				return
			}
			workspaceName, viewer, switchHref, err := pageChrome(req.Context(), req, store, cfg)
			if err != nil {
				serverError(w, err)
				return
			}
			render(req.Context(), w, rendering.RecordDetailPage(machine, record, relations, groups, children, actor, sigPlacement, workspaceName, viewer, switchHref, extras, time.Now()))
			return
		}
		if isDetailContext(req) {
			children, err := ld.ChildSections(req.Context(), machine, record.ID)
			if err != nil {
				serverError(w, err)
				return
			}
			sigPlacement := documentSignaturePlacementView(req.Context(), ld, files, machines, machine, record.ID, actor)
			extras, err := ld.RecordExtras(req.Context(), machine, record, relations)
			if err != nil {
				serverError(w, err)
				return
			}
			render(req.Context(), w, rendering.RecordDetailView(machine, record, relations, groups, children, actor, sigPlacement, extras, time.Now()))
			return
		}
		render(req.Context(), w, rendering.RecordRow(machine, record, relations, groups, actor, time.Now()))
	}
}

func editRecordRow(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machines := machinesFor(req.Context())
		machine, ok := resolveMachine(w, req)
		if !ok {
			return
		}
		id := chi.URLParam(req, "id")
		record, err := store.GetRecord(req.Context(), machine.ID, id)
		if err != nil {
			recordError(w, err)
			return
		}
		actor := currentActor(req, store, cfg)
		if !recordEditAllowed(req.Context(), store, machine, record.Values, actor) {
			http.Error(w, "not allowed to edit this record", http.StatusForbidden)
			return
		}
		ld := composition.NewLoader(store, machines)
		relations, err := ld.RelationOptions(req.Context(), machine)
		if err != nil {
			serverError(w, err)
			return
		}
		groups, err := ld.GroupOptions(req.Context(), machine)
		if err != nil {
			serverError(w, err)
			return
		}
		if isDetailContext(req) {
			render(req.Context(), w, rendering.RecordDetailEdit(machine, record, relations, groups))
			return
		}
		render(req.Context(), w, rendering.RecordEditRow(machine, record, relations, groups))
	}
}

// updateRecordForm applies a form edit to one record: collect the submitted values, run every
// guard the write has to pass, write, log, and re-render.
//
// The guards are named helpers below rather than inline blocks because each is a rule in its own
// right -- one about file inputs, one about who may change a decision, one about Constraints --
// and reading this handler should show the order they run in, which is itself the contract
// (005-runtime-lifecycle.md "Security Ordering").
func updateRecordForm(store *data.Store, files *storage.Store, mailer mail.Mailer, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machines := machinesFor(req.Context())
		machine, ok := resolveMachine(w, req)
		if !ok {
			return
		}
		if refusesAppendOnlyWrite(w, machine) {
			return
		}
		id := chi.URLParam(req, "id")
		actor := currentActor(req, store, cfg)
		if !allowsRecordEdit(w, req, store, machine, id, actor) {
			return
		}

		values, uploaded, ok := submittedValues(w, req, machine, files, cfg.MaxUploadBytes())
		if !ok {
			return
		}
		if !carryForwardFixedFields(w, req, store, machine, id, uploaded, values) {
			return
		}
		record, ok := commitRecordUpdate(w, req, store, files, mailer, machines, machine, id, values, actor)
		if !ok {
			return
		}

		renderRecord(w, req, machines, store, files, machine, record, actor)
	}
}

// commitRecordUpdate is everything between "these are the new values" and "they are stored": computed
// Fields, the declared state model, the write guards, and the Events that fire afterwards. The generic
// edit route and the single-Field one both end here, so a write that skips none of it cannot be written
// by adding a route.
func commitRecordUpdate(w http.ResponseWriter, req *http.Request, store *data.Store, files *storage.Store, mailer mail.Mailer, machines map[string]*domain.Machine, machine *domain.Machine, id string, values map[string]any, actor domain.Actor) (*data.Record, bool) {
	var (
		record    *data.Record
		oldValues map[string]any
		oldOK     bool
		created   []generatedRecord
	)
	// The guards, the write and the schedule it generates are one transaction holding the locks the Machine's totals
	// call for (writelock.go): a rule that judges a total reads what the previous writer committed, and a move whose
	// schedule cannot be written does not happen. Events run afterwards, on what committed.
	ok := withWriteLock(w, req, store, machines, machine, id, values, func(req *http.Request) bool {
		if !applyTransitionSets(w, req, store, machine, id, values) {
			return false
		}
		if !keepsLockedFields(w, req, store, machine, id, values) {
			return false
		}
		execution.ApplyComputed(req.Context(), store, machine, id, values)
		if !allowsTransition(w, req, store, machine, id, values, actor) || !passesWriteGuards(w, req, store, machines, machine, id, values) {
			return false
		}
		batch, ok := planGenerated(w, req, store, machines, machine, id, values)
		if !ok {
			return false
		}
		// oldValues has to be read before the write replaces it, so any declared Event
		// (domain.Machine.Events) can compare what changed once the write succeeds.
		oldValues, oldOK = eventOldValues(req, store, machine, id)
		var err error
		if record, err = store.UpdateRecord(req.Context(), machine.ID, id, values); err != nil {
			recordError(w, err)
			return false
		}
		// Before and after: a record moved to another parent changes both parents' aggregates; and the records that read
		// this one through `parent:` follow it.
		if err := execution.RecomputeParents(req.Context(), store, machines, machine, oldValues, record.Values); err != nil {
			serverError(w, err)
			return false
		}
		if err := execution.RecomputeChildren(req.Context(), store, machines, machine, id, oldValues, record.Values); err != nil {
			serverError(w, err)
			return false
		}
		if created, err = insertGenerated(req.Context(), store, machines, batch); err != nil {
			serverError(w, err)
			return false
		}
		return true
	})
	if !ok {
		return nil, false
	}
	svc := execution.Services{Store: store, Mailer: mailer, Files: files}
	execution.RunEvents(req.Context(), svc, machines, machine, record, actor.ID, oldValues, oldOK)
	runGeneratedEvents(req.Context(), svc, machines, created, actor.ID)
	return record, true
}

// patchRecordForm changes only the Fields the request names and keeps the rest of the record as it is.
// updateRecordForm replaces a record from a whole form, which is right for an edit form and wrong for a
// card's completion circle, quick edit or move: each knows one Field and must not blank the others by
// not knowing them. Same permission, state model, guards and Events as that route (commitRecordUpdate).
// Answers with the Machine's body, the fragment a board swaps in, the way create does.
func patchRecordForm(store *data.Store, files *storage.Store, mailer mail.Mailer, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machines := machinesFor(req.Context())
		machine, ok := resolveMachine(w, req)
		if !ok || refusesAppendOnlyWrite(w, machine) {
			return
		}
		id := chi.URLParam(req, "id")
		actor := currentActor(req, store, cfg)
		if !allowsRecordEdit(w, req, store, machine, id, actor) {
			return
		}
		if !parseRecordForm(w, req, cfg.MaxUploadBytes()) {
			return
		}
		existing, err := store.GetRecord(req.Context(), machine.ID, id)
		if err != nil {
			recordError(w, err)
			return
		}
		values, submitted := maps.Clone(existing.Values), data.ValuesFromForm(machine, req.Form)
		var changed int
		for _, f := range machine.Fields {
			if _, named := req.Form[f.ID]; named && f.Type != domain.FieldTypeFile && f.Stamp == "" {
				values[f.ID] = submitted[f.ID]
				changed++
			}
		}
		// A file sent with the change (a proof of payment uploaded as the plan is marked paid) is stored as the edit
		// route stores one.
		uploaded, err := handleFileUploads(req, machine, files)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		for k, v := range uploaded {
			values[k] = v
			changed++
		}
		if changed == 0 {
			http.Error(w, "no field of this machine was named", http.StatusBadRequest)
			return
		}
		position, ok := positionParam(w, req)
		if !ok {
			return
		}
		record, ok := commitRecordUpdate(w, req, store, files, mailer, machines, machine, id, values, actor)
		if !ok || (position != 0 && !placeCard(w, req, store, machine, record, position)) {
			return
		}
		if !onMachinePage(req, machine) {
			w.Header().Set("HX-Refresh", "true")
			return
		}
		renderMachineBody(w, req, machines, machine, store, actor)
	}
}

// fromDeclaredPage says the request was sent from a `page:` screen (`/pages/<nav id>`), which draws its own
// records and has no Machine row to swap an answer into. Narrower than fromAnotherPage on purpose: the three
// hand-written delete sites keep answering an empty body.
// recordPageBack is where a delete sent from a record's own page (`/pages/<nav>/<id>`, `record_of:`) goes once that record
// is gone: the screen the page's Application names with `back_to:`, else the Machine's own list. Not a record page of
// this record -- another record's, a list page, another screen -- is not its business (ok false).
func recordPageBack(req *http.Request, machine *domain.Machine, id string) (string, bool) {
	current, err := url.Parse(req.Header.Get("HX-Current-URL"))
	if err != nil {
		return "", false
	}
	rest, ok := strings.CutPrefix(current.Path, "/pages/")
	navID, recordID, found := strings.Cut(rest, "/")
	if !ok || !found || recordID != id {
		return "", false
	}
	app, ok := applicationForPath(rendering.CurrentWorkspace(req.Context()), current.Path)
	if !ok {
		return "", false
	}
	item, ok := app.NavigationItemByID(navID)
	if !ok || item.RecordOf != machine.ID {
		return "", false
	}
	if back, ok := app.NavigationItemByID(item.BackTo); ok && back.Route != "" {
		return back.Route, true
	}
	return "/machines/" + machine.ID, true
}

func fromDeclaredPage(req *http.Request) bool {
	current, err := url.Parse(req.Header.Get("HX-Current-URL"))
	return err == nil && strings.HasPrefix(current.Path, "/pages/")
}

// onMachinePage says whether the person who sent this fragment request is looking at the Machine's own page
// (HX-Current-URL), the only place the Machine body it would answer with can be swapped in. A write from
// another screen -- a task's circle on My Tasks -- is answered with an HX-Refresh instead, so that screen
// redraws itself from its own composition rather than receiving markup it has no place for.
func onMachinePage(req *http.Request, machine *domain.Machine) bool {
	current, err := url.Parse(req.Header.Get("HX-Current-URL"))
	return err == nil && current.Path == "/machines/"+machine.ID
}

// positionParam reads the optional `position` of a PATCH before anything is written, so a malformed one
// refuses the whole request instead of refusing it after the Fields were saved. 0 means none was sent; a
// position is 1-based, and a number below 1 is a malformed one.
func positionParam(w http.ResponseWriter, req *http.Request) (int, bool) {
	raw := req.FormValue("position")
	if raw == "" {
		return 0, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		http.Error(w, "position must be a number from 1", http.StatusBadRequest)
		return 0, false
	}
	return n, true
}

// placeCard puts the record at the 1-based slot a PATCH's `position` asked for among the cards of the group
// it now belongs to, in the View the person is looking at (resolveView reads it from HX-Current-URL, like
// every other fragment). Position is not a Field and is never stored in the record: order is sort_order's
// business (data.Store.PlaceRecord).
func placeCard(w http.ResponseWriter, req *http.Request, store *data.Store, machine *domain.Machine, record *data.Record, position int) bool {
	v, ok := resolveView(w, machine, req)
	if !ok {
		return false
	}
	if v.GroupBy == "" {
		http.Error(w, "position needs a grouped view", http.StatusBadRequest)
		return false
	}
	records, err := store.ListRecords(req.Context(), machine.ID)
	if err != nil {
		serverError(w, err)
		return false
	}
	before := experience.PlaceBefore(v, records, record.ID, fmt.Sprint(record.Values[v.GroupBy]), position)
	if err := store.PlaceRecord(req.Context(), machine.ID, record.ID, before); err != nil {
		recordError(w, err)
		return false
	}
	return true
}

// recordEditAllowed is the one place that decides whether actor may edit a record given its
// current values -- an unrestricted "yes" the instant no edit Permission is declared
// (001-design-principles.md Principle #6: metadata describes exceptions, not defaults), otherwise
// authorization.AllowsAction's own answer. Both allowsRecordEdit (below, which doesn't have the
// record yet and must fetch it) and editRecordRow (which already fetched it to render the edit
// form) call this instead of each re-stating the same check with its own copy of the error
// string, code-review finding 2026-09-19.
func recordEditAllowed(ctx context.Context, store *data.Store, machine *domain.Machine, values map[string]any, actor domain.Actor) bool {
	return len(machine.PermissionsFor(domain.ActionEdit)) == 0 || allowsActionResolvingParents(ctx, store, machine, domain.ActionEdit, values, actor)
}

// allowsRecordEdit enforces any declared domain.ActionEdit Permission before the generic update
// route writes anything, generalizing decideStep's own authorization.AllowsAction check
// (internal/web/approval.go) from Approve/Reject to every Machine -- the same "generalize on a
// second real case, never the first" discipline deleteAllowed below already follows. Skips the
// fetch entirely when no edit Permission is declared (recordEditAllowed's own fast path).
func allowsRecordEdit(w http.ResponseWriter, req *http.Request, store *data.Store, machine *domain.Machine, id string, actor domain.Actor) bool {
	if len(machine.PermissionsFor(domain.ActionEdit)) == 0 {
		return true
	}
	existing, err := store.GetRecord(req.Context(), machine.ID, id)
	if err != nil {
		recordError(w, err)
		return false
	}
	if !recordEditAllowed(req.Context(), store, machine, existing.Values, actor) {
		http.Error(w, "not allowed to edit this record", http.StatusForbidden)
		return false
	}
	return true
}

// submittedValues reads the form body and merges in any freshly uploaded files. It returns the
// uploaded set separately as well, because "which file fields did this request actually set" is
// what carryForwardFixedFields needs and cannot recover from values alone.
func submittedValues(w http.ResponseWriter, req *http.Request, machine *domain.Machine, files *storage.Store, uploadLimit int64) (values, uploaded map[string]any, ok bool) {
	if !parseRecordForm(w, req, uploadLimit) {
		return nil, nil, false
	}
	values = data.ValuesFromForm(machine, req.Form)
	uploaded, err := handleFileUploads(req, machine, files)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil, nil, false
	}
	for k, v := range uploaded {
		values[k] = v
	}
	return values, uploaded, true
}

// carryForwardFixedFields keeps a file field that this request did not re-upload, and a stamped Field whatever it says. A browser can't
// pre-fill <input type="file">, so "no new upload" must not be read as "clear the file" the way
// an empty text input would be (development-history.md Phase 11).
func carryForwardFixedFields(w http.ResponseWriter, req *http.Request, store *data.Store, machine *domain.Machine, id string, uploaded, values map[string]any) bool {
	if err := carryForwardStored(req.Context(), store, machine, id, uploaded, values); err != nil {
		recordError(w, err)
		return false
	}
	return true
}

// allowsTransition holds the Machine's own declared state model against this write: every status
// Field the request moves must be moving along a declared edge, and that edge must be reserved for
// the Action this route realizes (domain.ActionEdit here -- the generic update route and its JSON
// twin both go through it).
//
// It replaces allowsDecisionChange, which said exactly this for one Machine and one Field by
// hand: "an Approval Step's fld_decision only ever moves through POST .../decide, the edit route
// must not become a bypass." That rule is now three lines of metadata on mch_approval_step
// (`trn_step_approve`/`trn_step_reject`, both `action: decide`), and the same declaration is what
// the Approval Role Matrix draws -- so the screen showing who may approve and the guard refusing
// an edit read one statement instead of agreeing by coincidence.
//
// It also closed a second bypass the hand-written rule never covered, which is the evidence the
// generalization was worth making rather than the claim: nothing stopped a Document's own
// fld_status being set directly through the same generic route, even though that Field is
// *derived* from its steps (evt_step_decision_rollup). Its declared edges name no action at all,
// so this now refuses it.
//
// The fetch is skipped entirely for a Machine that declares no transitions -- the same fast path
// recordEditAllowed takes for an undeclared Permission, and the reason declaring the state model
// of one Machine costs nothing on every other write in the manifest.
func allowsTransition(w http.ResponseWriter, req *http.Request, store *data.Store, machine *domain.Machine, id string, values map[string]any, actor domain.Actor) bool {
	if len(machine.Transitions) == 0 {
		return true
	}
	existing, err := store.GetRecord(req.Context(), machine.ID, id)
	if err != nil {
		recordError(w, err)
		return false
	}
	if err := behavior.CheckTransitions(machine, domain.ActionEdit, existing.Values, values); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return false
	}
	// The edge's own actor arms (roles, actor_field), read against the record as it stands before the move.
	for _, t := range behavior.MovedTransitions(machine, existing.Values, values) {
		if !authorization.AllowsTransition(machine, t, existing.Values, actor) {
			http.Error(w, "not allowed to take the "+t.Name+" step on this record", http.StatusForbidden)
			return false
		}
	}
	return true
}

// frozenReason says why a write is refused when the Machine freezes records under a related record that holds a value
// (`frozen_when:`, domain.FrozenWhen), or "" when it is not. existing is the stored values of the record being edited
// or deleted (nil for a create) and next the values it would hold (nil for a delete): a record is frozen by where it
// sits now, and moving it into a frozen parent is refused too. A Machine declaring none reads nothing.
func frozenReason(ctx context.Context, store *data.Store, machine *domain.Machine, existing, next map[string]any) (string, error) {
	if machine.Frozen == nil {
		return "", nil
	}
	via, ok := machine.FieldByID(machine.Frozen.Via)
	if !ok {
		return "", nil
	}
	seen := map[string]bool{}
	for _, values := range []map[string]any{existing, next} {
		if values == nil {
			continue
		}
		parentID := fmt.Sprint(values[machine.Frozen.Via])
		if parentID == "" || parentID == "<nil>" || seen[parentID] {
			continue
		}
		seen[parentID] = true
		parent, err := store.GetRecord(ctx, via.RelatedMachine, parentID)
		if err != nil {
			if errors.Is(err, data.ErrRecordNotFound) {
				continue // a dangling reference is ValidateRelations' to refuse, not a freeze
			}
			return "", err
		}
		if fmt.Sprint(parent.Values[machine.Frozen.Field]) == machine.Frozen.Equals {
			return fmt.Sprintf("this %s is frozen: its %s is %s", machine.Name, via.Name, machine.Frozen.Equals), nil
		}
	}
	return "", nil
}

// notFrozen answers a refused write with 409 when frozenReason says the record is frozen.
func notFrozen(w http.ResponseWriter, req *http.Request, store *data.Store, machine *domain.Machine, id string, next map[string]any) bool {
	if machine.Frozen == nil {
		return true
	}
	var existing map[string]any
	if id != "" {
		rec, err := store.GetRecord(req.Context(), machine.ID, id)
		if err != nil {
			recordError(w, err)
			return false
		}
		existing = rec.Values
	}
	reason, err := frozenReason(req.Context(), store, machine, existing, next)
	if err != nil {
		serverError(w, err)
		return false
	}
	if reason != "" {
		http.Error(w, reason, http.StatusConflict)
		return false
	}
	return true
}

// planGenerated plans, validates and checks the schedule the edge this write takes generates (`generates:` on a
// transition), before anything is written: a count or amount that is not a number, a schedule already generated for this
// record, or a row the target Machine would refuse each refuse the move, so it never leaves the status moved and the
// schedule missing. nil with ok is the common case, a write that takes no such edge.
func planGenerated(w http.ResponseWriter, req *http.Request, store *data.Store, machines map[string]*domain.Machine, machine *domain.Machine, id string, values map[string]any) (*behavior.GeneratedBatch, bool) {
	if !behavior.HasGenerates(machine) {
		return nil, true
	}
	existing, err := store.GetRecord(req.Context(), machine.ID, id)
	if err != nil {
		recordError(w, err)
		return nil, false
	}
	batch, err := behavior.PlanGenerated(machine, id, existing.Values, values, time.Now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return nil, false
	}
	if batch == nil {
		return nil, true
	}
	target := machines[batch.Machine]
	if target == nil {
		serverError(w, fmt.Errorf("generates: machine %q is not loaded", batch.Machine))
		return nil, false
	}
	have, err := store.ListRecordsBy(req.Context(), target.ID, batch.LinkField, id)
	if err != nil {
		serverError(w, err)
		return nil, false
	}
	if len(have) > 0 {
		http.Error(w, "the schedule was already generated for this record", http.StatusConflict)
		return nil, false
	}
	for _, row := range batch.Rows {
		data.ApplyDefaults(target, row)
		if err := data.ValidateRecord(target, row); err != nil {
			http.Error(w, "the schedule cannot be generated: "+err.Error(), http.StatusUnprocessableEntity)
			return nil, false
		}
	}
	return batch, true
}

// generatedRecord is a row an edge's `generates:` wrote, with the Machine it belongs to, kept to run its create Events
// once the transaction that wrote it has committed.
type generatedRecord struct {
	machine *domain.Machine
	record  *data.Record
}

// insertGenerated writes the planned records inside the edge's own transaction: all of them or, if one fails, none and
// not the move either. nil batch (an edge that generates nothing) writes nothing.
func insertGenerated(ctx context.Context, store *data.Store, machines map[string]*domain.Machine, batch *behavior.GeneratedBatch) ([]generatedRecord, error) {
	if batch == nil {
		return nil, nil
	}
	target := machines[batch.Machine]
	out := make([]generatedRecord, 0, len(batch.Rows))
	for i, row := range batch.Rows {
		// A generated row is a record like any: its computed Fields derived, and what it adds up to on its parent kept.
		if err := execution.AssignNumbers(ctx, store, target, row, time.Now()); err != nil {
			return nil, fmt.Errorf("generates: row %d of %s: %w", i+1, target.ID, err)
		}
		execution.ApplyComputed(ctx, store, target, "", row)
		rec, err := store.CreateRecord(ctx, target.ID, row)
		if err != nil {
			return nil, fmt.Errorf("generates: row %d of %s: %w", i+1, target.ID, err)
		}
		if err := execution.RecomputeParents(ctx, store, machines, target, rec.Values); err != nil {
			return nil, err
		}
		out = append(out, generatedRecord{machine: target, record: rec})
	}
	return out, nil
}

// runGeneratedEvents runs each generated row's create Events, as any create does, after the commit.
func runGeneratedEvents(ctx context.Context, svc execution.Services, machines map[string]*domain.Machine, rows []generatedRecord, actorID string) {
	for _, g := range rows {
		execution.RunCreateEvents(ctx, svc, machines, g.machine, g.record, actorID)
	}
}

// applyTransitionSets stamps the Fields the edge this write takes sets from the clock (`sets:` on a transition), before
// computed Fields are derived so a date computed from one (the next due date from the completion date) sees it. It reads
// the stored record only for a Machine with such an edge.
func applyTransitionSets(w http.ResponseWriter, req *http.Request, store *data.Store, machine *domain.Machine, id string, values map[string]any) bool {
	if !behavior.HasTransitionSets(machine) {
		return true
	}
	existing, err := store.GetRecord(req.Context(), machine.ID, id)
	if err != nil {
		recordError(w, err)
		return false
	}
	behavior.ApplyTransitionSets(machine, existing.Values, values, time.Now())
	return true
}

// isBalanced holds an edge that must balance (`balanced:` on a transition, domain.BalanceRule): the lines pointing at the
// record are read and their debit and credit totals must be equal before the status moves. A write that takes no such
// edge, and a Machine declaring none, read nothing.
func isBalanced(w http.ResponseWriter, req *http.Request, store *data.Store, machine *domain.Machine, id string, values map[string]any) bool {
	if id == "" || !behavior.HasBalanced(machine) {
		return true
	}
	existing, err := store.GetRecord(req.Context(), machine.ID, id)
	if err != nil {
		recordError(w, err)
		return false
	}
	for _, t := range behavior.BalancedEdges(machine, existing.Values, values) {
		recs, err := store.ListRecordsBy(req.Context(), t.Balanced.Machine, t.Balanced.Via, id)
		if err != nil {
			serverError(w, err)
			return false
		}
		lines := make([]map[string]any, len(recs))
		for i, r := range recs {
			lines[i] = r.Values
		}
		if err := behavior.CheckBalance(t.Balanced, lines); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return false
		}
	}
	return true
}

// withinBounds holds a write against the Machine's aggregate bounds (`ensure:`, domain.AggregateBound): the records
// sharing each bound's group are read, and the write is judged as the total it would leave. id is "" for a create. Only
// the groups the write touches are read, one query per bound. A Machine declaring none costs nothing.
func withinBounds(w http.ResponseWriter, req *http.Request, store *data.Store, machine *domain.Machine, id string, values map[string]any) bool {
	if len(machine.Bounds) == 0 {
		return true
	}
	var siblings []*data.Record
	seen := map[string]bool{}
	for _, b := range machine.Bounds {
		group := fmt.Sprint(values[b.Per])
		if group == "" || group == "<nil>" {
			continue
		}
		rows, err := store.ListRecordsBy(req.Context(), machine.ID, b.Per, group)
		if err != nil {
			serverError(w, err)
			return false
		}
		for _, r := range rows {
			if !seen[r.ID] {
				seen[r.ID] = true
				siblings = append(siblings, r)
			}
		}
	}
	// A bound taken from the group's parent (`at_most: {parent: fld_contract.fld_value}`) is read here, once per group,
	// and checked as the literal it stands for: the parent's value now.
	judged := machine
	for i, b := range machine.Bounds {
		p := b.AtMostParent
		if p == nil {
			continue
		}
		if judged == machine {
			clone := *machine
			clone.Bounds = slices.Clone(machine.Bounds)
			judged = &clone
		}
		via, _ := machine.FieldByID(p.Via)
		parent, err := store.GetRecord(req.Context(), via.RelatedMachine, fmt.Sprint(values[p.Via]))
		if err != nil {
			if errors.Is(err, data.ErrRecordNotFound) {
				continue // a missing parent is ValidateRelations' to refuse
			}
			serverError(w, err)
			return false
		}
		if n, ok := domain.FigureOf(parent.Values[p.Field]); ok {
			judged.Bounds[i].AtMost = &n
		}
	}
	if err := behavior.CheckBounds(judged, id, values, siblings); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return false
	}
	return true
}

// passesWriteGuards runs the three checks every write has to clear: the record's own shape, the
// existence of anything it points at, and the Machine's declared Constraints.
func passesWriteGuards(w http.ResponseWriter, req *http.Request, store *data.Store, machines map[string]*domain.Machine, machine *domain.Machine, id string, values map[string]any) bool {
	if !validRecord(w, req, store, machine, values) {
		return false
	}
	if !notFrozen(w, req, store, machine, id, values) || !withinBounds(w, req, store, machine, id, values) || !isBalanced(w, req, store, machine, id, values) {
		return false
	}
	relatedRecords, err := composition.NewLoader(store, machines).ConstraintRelatedRecords(req.Context(), machine)
	if err != nil {
		serverError(w, err)
		return false
	}
	if err := behavior.CheckConstraints(machine, id, values, relatedRecords); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return false
	}
	return true
}

// renderRecord re-renders one record after a write, as the detail view or as a table/board row
// depending on what the HTMX request targeted.
func renderRecord(w http.ResponseWriter, req *http.Request, machines map[string]*domain.Machine, store *data.Store, files *storage.Store, machine *domain.Machine, record *data.Record, actor domain.Actor) {
	ld := composition.NewLoader(store, machines).WithFiles(files)
	relations, err := ld.RelationOptions(req.Context(), machine)
	if err != nil {
		serverError(w, err)
		return
	}
	groups, err := ld.GroupOptions(req.Context(), machine)
	if err != nil {
		serverError(w, err)
		return
	}
	if isDetailContext(req) {
		children, err := ld.ChildSections(req.Context(), machine, record.ID)
		if err != nil {
			serverError(w, err)
			return
		}
		sigPlacement := documentSignaturePlacementView(req.Context(), ld, files, machines, machine, record.ID, actor)
		extras, err := ld.RecordExtras(req.Context(), machine, record, relations)
		if err != nil {
			serverError(w, err)
			return
		}
		render(req.Context(), w, rendering.RecordDetailView(machine, record, relations, groups, children, actor, sigPlacement, extras, time.Now()))
		return
	}
	render(req.Context(), w, rendering.RecordRow(machine, record, relations, groups, actor, time.Now()))
}

// deleteLocked is the delete both routes share: the permission and freeze check and the delete itself, in one transaction
// under the locks a total over this Machine calls for (a line removed under an entry being posted waits for it).
func deleteLocked(w http.ResponseWriter, req *http.Request, store *data.Store, machine *domain.Machine, id string, actor domain.Actor) bool {
	return withWriteLock(w, req, store, machinesFor(req.Context()), machine, id, nil, func(req *http.Request) bool {
		allowed, status, reason, err := deleteAllowed(req.Context(), store, machine, id, actor)
		if err != nil {
			serverError(w, err)
			return false
		}
		if !allowed {
			http.Error(w, reason, status)
			return false
		}
		existing, err := store.GetRecord(req.Context(), machine.ID, id)
		if err != nil {
			recordError(w, err)
			return false
		}
		if err := store.DeleteRecord(req.Context(), machine.ID, id); err != nil {
			serverError(w, err)
			return false
		}
		if err := execution.RecomputeParents(req.Context(), store, machinesFor(req.Context()), machine, existing.Values); err != nil {
			serverError(w, err)
			return false
		}
		return true
	})
}

func deleteRecord(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machine, ok := resolveMachine(w, req)
		if !ok {
			return
		}
		if refusesAppendOnlyWrite(w, machine) {
			return
		}
		id := chi.URLParam(req, "id")
		actor := currentActor(req, store, cfg)
		if !deleteLocked(w, req, store, machine, id, actor) {
			return
		}
		if isDetailContext(req) {
			// The record is gone; nothing left on this page to show. Send the visitor back to
			// the Machine's own list/board.
			w.Header().Set("HX-Redirect", "/machines/"+machine.ID)
			return
		}
		if back, ok := recordPageBack(req, machine, id); ok {
			// The record this page is about is gone: leave it for the screen its Application names (`back_to:`), or the
			// Machine's own list when it names none.
			w.Header().Set("HX-Redirect", back)
			return
		}
		if fromDeclaredPage(req) {
			// A delete Button on a declared page has no row to remove: the page re-reads and draws the list
			// without the record, the way the create and patch routes answer the same page.
			w.Header().Set("HX-Refresh", "true")
			return
		}
		// Empty response: HTMX swaps the row's outerHTML with nothing, removing it.
	}
}

// moveRecord is the generic route that moves one record a step in its Machine's order
// (`POST .../records/{id}/move?direction=up|down`), for a `write: move` Button on a declared page. The neighbour
// is found here, at request time, from the Machine's own order -- the page names a direction and never an id, so
// a stale page cannot aim a record at a position that has since changed. Moving is an edit of the record's
// place: it asks the `edit` Permission and an append-only Machine refuses it. At an end of the order it does
// nothing and still answers, so a page that went stale redraws itself.
func moveRecord(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machine, ok := resolveMachine(w, req)
		if !ok || refusesAppendOnlyWrite(w, machine) {
			return
		}
		direction := req.URL.Query().Get("direction")
		if direction != domain.MoveUp && direction != domain.MoveDown {
			http.Error(w, "direction must be up or down", http.StatusBadRequest)
			return
		}
		id := chi.URLParam(req, "id")
		if !allowsRecordEdit(w, req, store, machine, id, currentActor(req, store, cfg)) {
			return
		}
		records, err := store.ListRecords(req.Context(), machine.ID)
		if err != nil {
			serverError(w, err)
			return
		}
		ids := make([]string, len(records))
		for i, r := range records {
			ids[i] = r.ID
		}
		before, found, moves := moveTarget(ids, id, direction)
		if !found {
			http.Error(w, data.ErrRecordNotFound.Error(), http.StatusNotFound)
			return
		}
		if moves {
			if err := store.PlaceRecord(req.Context(), machine.ID, id, before); err != nil {
				recordError(w, err)
				return
			}
		}
		if fromDeclaredPage(req) {
			w.Header().Set("HX-Refresh", "true")
		}
	}
}

// moveTarget answers where id goes when it moves one step in direction through ids (the Machine's order): the id
// it should sit immediately before, "" meaning last. found is false when id is not in ids; moves is false at the
// end of the order the direction points away from.
func moveTarget(ids []string, id, direction string) (before string, found, moves bool) {
	i := slices.Index(ids, id)
	switch {
	case i < 0:
		return "", false, false
	case direction == domain.MoveUp && i > 0:
		return ids[i-1], true, true
	case direction == domain.MoveDown && i < len(ids)-1:
		if i+2 < len(ids) {
			return ids[i+2], true, true
		}
		return "", true, true
	}
	return "", true, false
}

// deleteAllowed guards the generic delete route two ways, business-state and identity, evaluated
// independently: a hardcoded, Case-3-scoped business-state check for the two Machines whose
// records are an audit trail Phase 16/17 exist to protect
// (action.CanDeleteApprovalStep/CanDeleteDocument, same narrow posture as
// action.CanDecide/CompositeSignatures, not a generic Machine-level permission engine on its own),
// ANDed with any declared domain.ActionDelete Permission (authorization.AllowsAction,
// generalizing decideStep's identity check the same way allowsRecordEdit above does for edits).
// A Machine with neither -- the common case -- stays fully unrestricted, same as the generic
// route always was (ROADMAP.md's own Method: generalize on a second real case, never the first).
func deleteAllowed(ctx context.Context, store *data.Store, machine *domain.Machine, id string, actor domain.Actor) (ok bool, status int, reason string, err error) {
	// stateGoverned/permissionGoverned decide, before any fetch, whether this Machine needs one
	// at all -- the common case (neither) stays a zero-query no-op, same as the generic route
	// always was.
	stateGoverned := action.IsStep(machine) || action.IsDocument(machine) || len(machine.DeletableUnless) > 0
	permissionGoverned := len(machine.PermissionsFor(domain.ActionDelete)) > 0
	restricting := domain.RestrictingChildren(slices.Collect(maps.Values(machinesFor(ctx))), machine.ID)
	if !stateGoverned && !permissionGoverned && machine.Frozen == nil && len(restricting) == 0 {
		return true, 0, "", nil
	}

	existing, err := store.GetRecord(ctx, machine.ID, id)
	if err != nil {
		return false, 0, "", err
	}
	if reason, err := frozenReason(ctx, store, machine, existing.Values, nil); err != nil {
		return false, 0, "", err
	} else if reason != "" {
		return false, http.StatusConflict, reason, nil
	}

	if stateGoverned {
		var steps []*data.Record
		if action.IsDocument(machine) {
			stepMachine := approvalMachine(ctx, domain.WorkflowRoleStep)
			steps, err = store.ListRecordsBy(ctx, stepMachine.ID, action.DeclaredFields(stepMachine, machine).Parent, id)
			if err != nil {
				return false, 0, "", err
			}
		}
		// action.CanDelete is the single source of truth for this business-state check --
		// internal/rendering's canDeleteInView (detail.templ) and RecordRow (machine.templ) call
		// the exact same function to decide whether to offer the Delete button in the first
		// place, so all three can't drift out of sync with each other (code-review finding,
		// 2026-09-19).
		if ok, reason := action.CanDelete(machine, existing.Values, steps); !ok {
			return false, http.StatusUnprocessableEntity, reason, nil
		}
	}

	if permissionGoverned && !allowsActionResolvingParents(ctx, store, machine, domain.ActionDelete, existing.Values, actor) {
		return false, http.StatusForbidden, "not allowed to delete this record", nil
	}
	// `on_parent_delete: restrict`: a record other records still point at stays, so none of them is left pointing at
	// nothing (gap K-13). Asked last, because it is the one check that reads another Machine.
	for _, link := range restricting {
		children, err := store.ListRecordsBy(ctx, link.Machine.ID, link.Field, id)
		if err != nil {
			return false, 0, "", err
		}
		if n := len(children); n > 0 {
			return false, http.StatusConflict, fmt.Sprintf("cannot be deleted while %d %s record(s) point at it -- delete or move them first", n, link.Machine.Name), nil
		}
	}
	return true, 0, "", nil
}

// sameStoredValue says whether a submitted value leaves a stored one as it is: equal as written, or both empty (a form
// sends "" for a Field the record never had).
func sameStoredValue(submitted, stored any) bool {
	blank := func(v any) bool { return v == nil || fmt.Sprint(v) == "" }
	if blank(submitted) && blank(stored) {
		return true
	}
	// A figure is the same figure however it is written ("54000.00" and 54000).
	a, aErr := strconv.ParseFloat(fmt.Sprint(submitted), 64)
	b, bErr := strconv.ParseFloat(fmt.Sprint(stored), 64)
	if aErr == nil && bErr == nil {
		return a == b
	}
	return fmt.Sprint(submitted) == fmt.Sprint(stored)
}

// keepsLockedFields refuses an update that changes a Field its Machine locks for the record as stored (`locked_when:`,
// `set_once:`, owner decision K1): a paid instalment's gross, a contract's number. A locked Field the request leaves out
// keeps its stored value -- the whole-record routes write what they are sent, so leaving a Field out would otherwise erase
// it (the review of step 1 did exactly that to a paid instalment's VAT). It runs before computed Fields are derived, so
// they are derived from the kept values. A Field sent unchanged is not judged, so a form that sends the record back whole
// still saves. No read for a Machine that locks nothing.
func keepsLockedFields(w http.ResponseWriter, req *http.Request, store *data.Store, machine *domain.Machine, id string, values map[string]any) bool {
	locks := false
	for _, f := range machine.Fields {
		locks = locks || f.SetOnce || f.LockedWhen != nil
	}
	if !locks {
		return true
	}
	existing, err := store.GetRecord(req.Context(), machine.ID, id)
	if err != nil {
		recordError(w, err)
		return false
	}
	for _, f := range machine.Fields {
		locked, why := machine.LockedField(f, existing.Values)
		if !locked {
			continue
		}
		v, sent := values[f.ID]
		if !sent {
			if stored, ok := existing.Values[f.ID]; ok {
				values[f.ID] = stored
			}
			continue
		}
		if !sameStoredValue(v, existing.Values[f.ID]) {
			http.Error(w, why, http.StatusUnprocessableEntity)
			return false
		}
	}
	return true
}

// parseRecordForm parses a create/update request body that may be multipart/form-data (needed
// for a FieldTypeFile upload -- every form sets hx-encoding for this, development-history.md Phase 11) or a
// plain url-encoded body (any other client, e.g. a direct API caller). ParseMultipartForm always
// runs ParseForm first regardless of content type, so http.ErrNotMultipart here just means "no
// file part was present, req.Form is already populated correctly" -- not a real failure.
//
// The body is capped at limit (config.Config.MaxUploadBytes, MAX_UPLOAD_MB) before it is read. ParseMultipartForm's own argument is only how much it keeps
// in memory before spilling file parts to temporary files on disk, so on its own it bounded nothing (the plan for
// Dokter Kecil, Tahap 0.3). A body over the cap is answered 413 and never reaches the disk. It answers the request
// itself and returns false when the body is refused.
func parseRecordForm(w http.ResponseWriter, req *http.Request, limit int64) bool {
	req.Body = http.MaxBytesReader(w, req.Body, limit)
	err := req.ParseMultipartForm(limit)
	var tooLarge *http.MaxBytesError
	switch {
	case err == nil || errors.Is(err, http.ErrNotMultipart):
		return true
	case errors.As(err, &tooLarge):
		http.Error(w, fmt.Sprintf("the upload is larger than the %d MB limit", limit>>20), http.StatusRequestEntityTooLarge)
	default:
		http.Error(w, "invalid form body", http.StatusBadRequest)
	}
	return false
}

// handleFileUploads saves any file actually submitted for one of machine's FieldTypeFile fields,
// returning fieldID -> storage key for just those fields. A field with no file in this request
// (http.ErrMissingFile) is simply absent from the result -- not an error, since a file input left
// untouched on an edit form submits nothing. Same for http.ErrNotMultipart: a genuinely
// non-multipart body (a direct API caller posting url-encoded, per parseRecordForm's own promise
// above) by definition submitted no file for *any* field, not just the one FormFile call happened
// to hit first -- previously only ErrMissingFile was tolerated here, so this exact case still 400'd
// despite parseRecordForm's comment claiming it worked (ROADMAP.md Operational backlog, pre-existing
// since Phase 19).
func handleFileUploads(req *http.Request, machine *domain.Machine, files *storage.Store) (map[string]any, error) {
	uploaded := map[string]any{}
	for _, f := range machine.Fields {
		if f.Type != domain.FieldTypeFile {
			continue
		}
		file, header, err := req.FormFile(f.ID)
		if err != nil {
			if errors.Is(err, http.ErrMissingFile) || errors.Is(err, http.ErrNotMultipart) {
				continue
			}
			return nil, fmt.Errorf("read upload for %s: %w", f.ID, err)
		}
		content, sniffErr := rejectDangerousUpload(file)
		if sniffErr != nil {
			_ = file.Close()
			return nil, fmt.Errorf("upload for %s: %w", f.ID, sniffErr)
		}
		key, saveErr := files.Save(machine.ID, f.ID, header.Filename, content)
		_ = file.Close() // read handle on the uploaded part; nothing to act on if this fails
		if saveErr != nil {
			return nil, saveErr
		}
		uploaded[f.ID] = key
		noteUpload(req.Context(), key)
	}
	return uploaded, nil
}

// dangerousUploadContentTypes denylists content a browser will execute as active content if it's
// ever served back, rather than allowlisting one "correct" type -- FieldTypeFile is a generic,
// composable Field Type already used for genuinely different content (mch_document.fld_file's
// PDFs, mch_task.fld_attachment's free-form attachments, capabilities.md's own Field Types table),
// so an allowlist would need updating every time a new attachment use case appears; a denylist of
// "things a browser executes" doesn't (security audit 2026-09-19, H2).
var dangerousUploadContentTypes = []string{
	"text/html",
	"image/svg+xml",
	"text/xml",
	"application/xml",
	"text/javascript",
	"application/javascript",
	"application/ecmascript",
}

// rejectDangerousUpload sniffs the real content of an upload (http.DetectContentType, the same
// mechanism a browser's own MIME-sniffing would use) rather than trusting the client-declared
// filename/extension, and returns an error for anything on the execute-as-active-content
// denylist -- this is what actually stops an "evil.svg"/"evil.html" payload at upload time,
// independent of the nosniff header (secureheaders.go) and serveUpload's own disposition fix,
// which are this bug's other two layers. It returns a reader that still yields the upload's full
// bytes (the sniffed prefix plus the rest of the stream), since the up-to-512-byte read this needs
// would otherwise be lost before files.Save gets to see it.
//
// http.DetectContentType alone isn't enough: its signature table (WHATWG MIME sniffing, a fixed
// list of HTML tag names plus "<?xml") has no entry for "<svg", so an SVG file that doesn't happen
// to open with one of those exact HTML tags sniffs as plain text/xml rather than
// "image/svg+xml" -- an SVG's own <script> would still be denylisted via the "<script" HTML
// signature if it's near the very start, but a real-world SVG (xmlns declaration, etc. before the
// payload) would sail past DetectContentType alone. A direct substring check for "<svg" in the
// sniffed prefix closes that gap regardless of where DetectContentType lands.
func rejectDangerousUpload(file multipart.File) (io.Reader, error) {
	buf := make([]byte, 512)
	n, err := io.ReadFull(file, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, fmt.Errorf("read upload: %w", err)
	}
	buf = buf[:n]

	if bytes.Contains(bytes.ToLower(buf), []byte("<svg")) {
		return nil, fmt.Errorf("SVG uploads are not allowed")
	}
	contentType := http.DetectContentType(buf)
	for _, bad := range dangerousUploadContentTypes {
		if strings.HasPrefix(contentType, bad) {
			return nil, fmt.Errorf("uploads of type %q are not allowed", contentType)
		}
	}
	return io.MultiReader(bytes.NewReader(buf), file), nil
}

// carryForwardStored fills values with the stored value of every Field an edit must not change by
// omission or by saying otherwise: each FieldTypeFile field uploaded didn't just set -- see
// handleFileUploads' caller for why -- and each stamped Field, which an edit may never overwrite (a
// submitted value for it is replaced with the stored one, or dropped if the record never had one). A
// no-op (and no fetch) when machine has neither.
func carryForwardStored(ctx context.Context, store *data.Store, machine *domain.Machine, recordID string, uploaded map[string]any, values map[string]any) error {
	fixed := false
	for _, f := range machine.Fields {
		if f.Type == domain.FieldTypeFile || f.Stamp != "" || f.Numbering != nil {
			fixed = true
			break
		}
	}
	if !fixed {
		return nil
	}

	existing, err := store.GetRecord(ctx, machine.ID, recordID)
	if err != nil {
		return err
	}
	for _, f := range machine.Fields {
		if f.Stamp != "" || f.Numbering != nil {
			delete(values, f.ID)
		} else if f.Type != domain.FieldTypeFile {
			continue
		} else if _, justUploaded := uploaded[f.ID]; justUploaded {
			continue
		}
		if v, ok := existing.Values[f.ID]; ok {
			values[f.ID] = v
		}
	}
	return nil
}

// allowsRecordCreate enforces any declared domain.ActionCreate Permission before the generic
// create routes write anything (2026-09-21 authorization review).
//
// Creation was the one write path in this runtime with no authorization check of any kind, on
// every Machine. The reason was real and is recorded on domain.ActionCreate: a record-scoped rule
// needs a record, and at creation there is none. What changed is that a Permission can now state
// a rule that reads no record at all (roles:, workspace_role:), so the blocker applied to one
// arm rather than to the Action.
//
// `values` are the ones being submitted, which is what gives ActorField its only possible meaning
// here -- "the value you submit for this field must be you" -- and is what stops a record being
// written in somebody else's name. mch_signature is the case that forced it: fld_owner selects
// whose signature gets composited onto an approved PDF, and anyone could create a record naming
// anyone.
//
// Skips the check entirely when no create Permission is declared, the same fast path
// recordEditAllowed takes, so declaring one on one Machine costs nothing on every other create.
func allowsRecordCreate(w http.ResponseWriter, req *http.Request, store *data.Store, machine *domain.Machine, values map[string]any, actor domain.Actor) bool {
	if len(machine.PermissionsFor(domain.ActionCreate)) == 0 {
		return true
	}
	if !allowsActionResolvingParents(req.Context(), store, machine, domain.ActionCreate, values, actor) {
		http.Error(w, "not allowed to create this record", http.StatusForbidden)
		return false
	}
	return true
}

// refusesAppendOnlyWrite refuses an update or delete of a Machine declared append_only -- for
// everyone, with no actor consulted at all, which is the point (domain.Machine.AppendOnly).
//
// It is checked before the Permission arms rather than alongside them, because it is not an
// authorization question: there is no actor who may edit an audit trail, so asking "who is this"
// first would imply one exists.
func refusesAppendOnlyWrite(w http.ResponseWriter, machine *domain.Machine) bool {
	if !machine.AppendOnly {
		return false
	}
	http.Error(w, machine.Name+" records are append-only: they are never changed or removed once written", http.StatusUnprocessableEntity)
	return true
}
