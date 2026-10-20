package web

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/experience"
	"menata.app/internal/rendering"
)

func showMachineList(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		workspaceName, viewer, switchHref, err := pageChrome(ctx, req, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		render(ctx, w, rendering.MachineList(installedMachines(ctx), workspaceName, viewer, switchHref))
	}
}

// installedMachines is this request's own Machines, in the order its Workspace's manifest names
// them (2026-09-22). A Workspace with no manifest gets an empty list, which is the point -- before
// that it got every Machine in the process, which made a freshly created Workspace look like it
// already had an application's worth of data model in it.
//
// It read the process-wide list and filtered it by ws.HasMachine until 2026-09-27. There is no
// process-wide list any more: the Workspace carries its own loaded Machines
// (domain.Workspace.Machines), so filtering a union is exactly the step that disappeared.
func installedMachines(ctx context.Context) []*domain.Machine {
	return rendering.CurrentWorkspace(ctx).Machines
}

// machinesFor is installedMachines keyed by id -- the per-request replacement for what used to be
// web.Deps' own process-wide map, built per call rather than held, since a Workspace has a dozen
// Machines and the map dies with the request.
//
// Both of these resolve from the Workspace already on ctx (internal/web.currentWorkspace put it
// there from an in-memory, slug-keyed lookup), so neither costs a query -- which is what keeps
// TestAuthenticatedPageQueryCost and friends unmoved by any of this.
func machinesFor(ctx context.Context) map[string]*domain.Machine {
	list := installedMachines(ctx)
	out := make(map[string]*domain.Machine, len(list))
	for _, m := range list {
		out[m.ID] = m
	}
	return out
}

// approvalMachine is how a handler asks for one of the approval engine's Machines: by the role its
// Application cast it in, never by its id (2026-09-28, the slice after development-history.md's Stage A).
//
// `machines[action.StepMachineID]` was the old form, and it is wrong for a reason no test could see
// until the binding existed: it asks for a *name*. A Workspace that installs Document Approval
// alongside an Application already using `mch_document` has to rename one of them on install, and
// every one of those lookups would then ask for an id that Workspace no longer has -- the copy would
// load, bind, engage, and fail at the first read.
//
// Returns nil for a Workspace whose Application casts nobody in this role, which is a real answer
// (see domain.MachineInWorkflowRole for the precedence and for when several Applications make the
// question ambiguous). An optional role -- a signature store, a saved flow template -- is nil
// whenever that feature was simply not installed, so every caller of those has to say what it does
// without one rather than dereferencing and panicking.
//
// This is the one place in internal/web that names the engine. A second engine means passing it in,
// not copying this.
func approvalMachine(ctx context.Context, role string) *domain.Machine {
	app, _ := rendering.CurrentApplication(ctx)
	return rendering.CurrentWorkspace(ctx).MachineInWorkflowRole(domain.WorkflowEngineDocumentApproval, role, app.ID)
}

// approvalMachineID is approvalMachine for the callers that need the id itself -- a record query, or
// a /machines/{id}/... URL. Empty string when the role is uncast, which those callers must check:
// querying an empty Machine id would silently read nothing.
func approvalMachineID(ctx context.Context, role string) string {
	if m := approvalMachine(ctx, role); m != nil {
		return m.ID
	}
	return ""
}

// machineForDocument and machineForStep are the two roles asked for often enough to name. Named
// this way round rather than documentMachine/stepMachine because internal/web is full of locals and
// parameters called exactly that, and a helper that silently disappears behind a local is worse than
// a slightly longer name.
func machineForDocument(ctx context.Context) *domain.Machine {
	return approvalMachine(ctx, domain.WorkflowRoleDocument)
}

func machineForStep(ctx context.Context) *domain.Machine {
	return approvalMachine(ctx, domain.WorkflowRoleStep)
}

// requireMachineForDocument is machineForDocument for the handlers that cannot continue without it,
// answering 404 itself -- the same shape resolveMachine already has, and for the same reason: the
// alternative is the identical eight-line guard at every such call site.
//
// It exists for the Workspace-level routes in particular (POST /documents, /documents/new): they are
// named by no navigation item, so nothing resolves an Application for them, and "this Workspace has
// no approval Application" or "it has two" is genuinely a missing page rather than an error.
func requireMachineForDocument(w http.ResponseWriter, ctx context.Context) (*domain.Machine, bool) {
	m := machineForDocument(ctx)
	if m == nil {
		http.Error(w, "this workspace has no document approval application", http.StatusNotFound)
		return nil, false
	}
	return m, true
}

func showMachinePage(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machines := machinesFor(req.Context())
		machine, ok := resolveMachine(w, req)
		if !ok {
			return
		}

		v, ok := resolveView(w, machine, req)
		if !ok {
			return
		}
		actor := currentActor(req, store, cfg)
		p, err := readMachineView(req, machines, machine, store, actor, v)
		if err != nil {
			serverError(w, err)
			return
		}
		p.boardColumns = columnAbilities(req.Context(), store, p.boardColumns, actor)
		workspaceName, viewer, switchHref, err := pageChrome(req.Context(), req, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		render(req.Context(), w, rendering.MachinePage(machine, v, p.records, p.relations, p.groups, p.boardColumns, p.cards, actor, workspaceName, viewer, switchHref, time.Now(), p.header))
	}
}

func renderMachineBody(w http.ResponseWriter, req *http.Request, machines map[string]*domain.Machine, machine *domain.Machine, store *data.Store, actor domain.Actor) {
	v, ok := resolveView(w, machine, req)
	if !ok {
		return
	}
	p, err := readMachineView(req, machines, machine, store, actor, v)
	if err != nil {
		serverError(w, err)
		return
	}
	p.boardColumns = columnAbilities(req.Context(), store, p.boardColumns, actor)
	render(req.Context(), w, rendering.MachineBody(machine, v, p.records, p.relations, p.groups, p.boardColumns, p.cards, actor, time.Now(), p.header))
}

// columnAbilities asks, for each column that is a record of another Machine (a List), what this viewer may do to it -- the question the routes
// answer when the menu's buttons are pressed (`allowsActionResolvingParents`), so a menu never offers what the route would refuse, nor hides what
// it would allow. A Permission that reads a parent record is the case a renderer cannot decide; everywhere else this is `AllowsAction` again.
func columnAbilities(ctx context.Context, store *data.Store, columns []experience.Column, actor domain.Actor) []experience.Column {
	for i, col := range columns {
		if col.Machine == nil {
			continue
		}
		columns[i].Can = map[string]bool{}
		for _, action := range []string{domain.ActionEdit, domain.ActionDelete} {
			columns[i].Can[action] = allowsActionResolvingParents(ctx, store, col.Machine, action, col.Values, actor)
		}
	}
	return columns
}

// machineViewReads is everything one arrangement of a Machine's records needs to render. Both the
// full page and the HTMX fragment need exactly the same set, which is why it is resolved once
// here rather than twice in two handlers that would drift apart the next time a View kind is
// added (the drift that made boardColumns easy to forget at the second call site).
type machineViewReads struct {
	records      []*data.Record
	relations    rendering.RelationOptions
	groups       rendering.GroupOptions
	boardColumns []experience.Column
	cards        []rendering.RecordCard
	// header is the view group's header when this Machine's Board is one of its screens, nil otherwise (the generic board header).
	header *rendering.ViewHeader
}

func readMachineView(req *http.Request, machines map[string]*domain.Machine, machine *domain.Machine, store *data.Store, actor domain.Actor, v domain.View) (machineViewReads, error) {
	ld := composition.NewLoader(store, machines)
	records, err := ld.ListRecords(req.Context(), machine.ID)
	if err != nil {
		return machineViewReads{}, err
	}
	// A Board that is one screen of a view group draws the group's header and shows the records of the scope the request chose
	// (a fragment after a write asks the same, from the page it came from).
	var header *rendering.ViewHeader
	if app, ok := rendering.CurrentApplication(req.Context()); ok {
		if item, group, ok := composition.ViewGroupItem(app, machine.ID, ""); ok {
			var params map[string]string
			if header, params, err = composition.ViewHeaderFor(req.Context(), ld, app, item, group, actor, scopeParams(req, group), time.Now()); err != nil {
				return machineViewReads{}, err
			}
			records = composition.ScopeRecords(group, params, records)
		}
	}
	relations, err := ld.RelationOptions(req.Context(), machine)
	if err != nil {
		return machineViewReads{}, err
	}
	groups, err := ld.GroupOptions(req.Context(), machine)
	if err != nil {
		return machineViewReads{}, err
	}
	boardColumns, err := ld.BoardColumns(req.Context(), machine, v)
	if err != nil {
		return machineViewReads{}, err
	}
	tags, err := ld.CardTags(req.Context(), machine, v, records)
	if err != nil {
		return machineViewReads{}, err
	}
	return machineViewReads{
		records:      records,
		relations:    relations,
		groups:       groups,
		boardColumns: boardColumns,
		cards:        composition.RecordCards(machine, v, records, relations, tags, time.Now(), composition.NumberFormatOf(req.Context())),
		header:       header,
	}, nil
}

// scopeParams is the request's value for a view group's scope parameter, as a parameter map: from the address, or -- for a fragment
// a write swaps in -- from the page the viewer was looking at (HX-Current-URL, as resolveView reads the View), so a created task
// lands in the project that was showing.
func scopeParams(req *http.Request, group domain.ViewGroup) map[string]string {
	out := map[string]string{}
	if group.Scope == nil {
		return out
	}
	name := group.Scope.Parameter
	if v := req.URL.Query().Get(name); v != "" {
		out[name] = v
	} else if current, err := url.Parse(req.Header.Get("HX-Current-URL")); err == nil && current.Query().Get(name) != "" {
		out[name] = current.Query().Get(name)
	}
	return out
}

// viewParam is the query parameter a Machine page uses to select among its declared Views. A
// parameter, deliberately not a path segment: a View decides what renders inside the page, never
// which shell or which route, and a per-View route would put a path in the URL space that no
// navigation item declares (exactly what TestRenderingLinksOnlyToDeclaredRoutes exists to catch).
const viewParam = "view"

// resolveView picks which of machine's declared Views this request is asking for, 404ing on an id
// the Machine does not declare -- a wrong id should say so rather than silently rendering a
// different arrangement, the same reasoning rendering.routeByID applies to an unknown nav id.
//
// A mutating fragment request POSTs to a URL of its own, which carries no selection, so the
// fallback reads the page the viewer is actually looking at: HTMX sends that as HX-Current-URL on
// every request it makes. Without it, a create or an edit would swap the body back to the default
// arrangement under someone who was looking at another one.
func resolveView(w http.ResponseWriter, machine *domain.Machine, req *http.Request) (domain.View, bool) {
	id := req.URL.Query().Get(viewParam)
	if id == "" {
		if current, err := url.Parse(req.Header.Get("HX-Current-URL")); err == nil {
			id = current.Query().Get(viewParam)
		}
	}
	if id == "" {
		return machine.DefaultView(), true
	}
	v, ok := machine.ViewByID(id)
	if !ok {
		http.Error(w, "unknown view", http.StatusNotFound)
		return domain.View{}, false
	}
	return v, true
}

// resolveMachine answers "which Machine does this URL's machineID mean, here" -- resolved against
// this request's own Workspace, so a Workspace that never installed it gets "unknown machine"
// rather than one of somebody else's.
//
// It took the process-wide map and paired it with a separate ws.HasMachine gate until 2026-09-27
// (the map alone said only that *some* Workspace had declared the id). The Workspace's own
// Machines answer both halves at once, so the gate is the lookup now.
func resolveMachine(w http.ResponseWriter, req *http.Request) (*domain.Machine, bool) {
	id := chi.URLParam(req, "machineID")
	m, ok := machinesFor(req.Context())[id]
	if !ok {
		http.Error(w, "unknown machine", http.StatusNotFound)
		return nil, false
	}
	return m, true
}
