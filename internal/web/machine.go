package web

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"menata.app/internal/authorization"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

func showMachineList(machines []*domain.Machine) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		render(req.Context(), w, rendering.MachineList(machines))
	}
}

func showMachinePage(machines map[string]*domain.Machine, store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machine, ok := resolveMachine(w, machines, req)
		if !ok {
			return
		}

		ld := composition.NewLoader(store, machines)
		records, err := ld.ListRecords(req.Context(), machine.ID)
		if err != nil {
			serverError(w, err)
			return
		}
		relations, err := ld.RelationOptions(req.Context(), machine)
		if err != nil {
			serverError(w, err)
			return
		}
		boardColumns, err := ld.BoardColumns(req.Context(), machine)
		if err != nil {
			serverError(w, err)
			return
		}
		actor, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
		render(req.Context(), w, rendering.MachinePage(machine, records, relations, boardColumns, actor))
	}
}

func renderMachineBody(w http.ResponseWriter, req *http.Request, machines map[string]*domain.Machine, machine *domain.Machine, store *data.Store, actor string) {
	ld := composition.NewLoader(store, machines)
	records, err := ld.ListRecords(req.Context(), machine.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	relations, err := ld.RelationOptions(req.Context(), machine)
	if err != nil {
		serverError(w, err)
		return
	}
	boardColumns, err := ld.BoardColumns(req.Context(), machine)
	if err != nil {
		serverError(w, err)
		return
	}
	render(req.Context(), w, rendering.MachineBody(machine, records, relations, boardColumns, actor))
}

func resolveMachine(w http.ResponseWriter, machines map[string]*domain.Machine, req *http.Request) (*domain.Machine, bool) {
	m, ok := machines[chi.URLParam(req, "machineID")]
	if !ok {
		http.Error(w, "unknown machine", http.StatusNotFound)
		return nil, false
	}
	return m, true
}
