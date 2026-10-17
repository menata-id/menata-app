package web

import (
	"maps"
	"net/http"
	"time"

	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/mail"
	"menata.app/internal/storage"
)

// quietWriter takes the answer a per-record guard would give a client and keeps only that it was a refusal. The bulk
// route below runs the same guards as the single-record PATCH for each record, and each of them reports through a
// ResponseWriter; one record's refusal must not become the answer to the whole request.
type quietWriter struct{ refused bool }

func (q *quietWriter) Header() http.Header         { return http.Header{} }
func (q *quietWriter) Write(b []byte) (int, error) { return len(b), nil }
func (q *quietWriter) WriteHeader(code int) {
	if code >= 300 {
		q.refused = true
	}
}

// transitionAll is the route behind a `write: transition` Button with `all: true` (`POST /machines/{id}/transition-all
// ?dataset=&to=`): it moves every record of the named Dataset to the status `to`, for the records this viewer may move.
// The Dataset is selected here, for this viewer, by the same function the page used to decide whether to draw the
// Button (composition.AllTransitionRecords), so the request names no record and a stale page cannot aim at one. Each
// record then goes through the single-record write exactly as a PATCH would (the `edit` Permission, the declared
// edges, the write guards, the Events): a record any of them refuses is left as it was, and the rest still move.
func transitionAll(store *data.Store, files *storage.Store, mailer mail.Mailer, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		machines := machinesFor(ctx)
		machine, ok := resolveMachine(w, req)
		if !ok || refusesAppendOnlyWrite(w, machine) {
			return
		}
		datasetID, to := req.URL.Query().Get("dataset"), req.URL.Query().Get("to")
		loader := composition.NewLoader(store, machines)
		if ds, found := loader.Dataset(datasetID); !found || ds.Source != machine.ID {
			http.Error(w, "unknown dataset", http.StatusNotFound)
			return
		}
		actor := currentActor(req, store, cfg)
		src, target, records, err := composition.AllTransitionRecords(ctx, loader, actor, map[string]string{}, time.Now(), datasetID, to)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		field := src.CardFieldFor(domain.CardFieldRoleStatus)
		for _, r := range records {
			if composition.DisplayString(r.Values[field]) == target {
				continue
			}
			values := maps.Clone(r.Values)
			values[field] = target
			q := &quietWriter{}
			if !allowsRecordEdit(q, req, store, machine, r.ID, actor) {
				continue
			}
			commitRecordUpdate(q, req, store, files, mailer, machines, machine, r.ID, values, actor)
		}
		w.Header().Set("HX-Refresh", "true")
	}
}
