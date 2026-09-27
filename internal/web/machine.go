package web

import (
	"context"
	"net/http"
	"net/url"

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
		p, err := readMachineView(req, machines, machine, store, v)
		if err != nil {
			serverError(w, err)
			return
		}
		actor := currentActor(req, store, cfg)
		workspaceName, viewer, switchHref, err := pageChrome(req.Context(), req, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		render(req.Context(), w, rendering.MachinePage(machine, v, p.records, p.relations, p.groups, p.boardColumns, p.cards, actor, workspaceName, viewer, switchHref))
	}
}

func renderMachineBody(w http.ResponseWriter, req *http.Request, machines map[string]*domain.Machine, machine *domain.Machine, store *data.Store, actor domain.Actor) {
	v, ok := resolveView(w, machine, req)
	if !ok {
		return
	}
	p, err := readMachineView(req, machines, machine, store, v)
	if err != nil {
		serverError(w, err)
		return
	}
	render(req.Context(), w, rendering.MachineBody(machine, v, p.records, p.relations, p.groups, p.boardColumns, p.cards, actor))
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
}

func readMachineView(req *http.Request, machines map[string]*domain.Machine, machine *domain.Machine, store *data.Store, v domain.View) (machineViewReads, error) {
	ld := composition.NewLoader(store, machines)
	records, err := ld.ListRecords(req.Context(), machine.ID)
	if err != nil {
		return machineViewReads{}, err
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
	return machineViewReads{
		records:      records,
		relations:    relations,
		groups:       groups,
		boardColumns: boardColumns,
		cards:        composition.RecordCards(machine, v, records, relations),
	}, nil
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
