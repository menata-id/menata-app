package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"

	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/execution"
)

// writeLockKeys names the advisory locks a write must hold while its guards read and its row is written, so that two
// writers whose rules judge the same total queue instead of both passing on what the other had not yet written. Three
// rules judge a total, and each locks the thing it totals over:
//
//   - `ensure:` locks each group the write touches, both the one the record was in and the one it moves to;
//   - `balanced:` locks the record whose lines are totalled, from the entry's side (the edge that posts) and from the
//     lines' side (a line added, changed or removed under it);
//   - `generates:` locks the record that generates, so two writers cannot each find no schedule and both write one.
//
// existing is the stored values (nil for a create) and next the values being written (nil for a delete). A Machine
// declaring none of the three answers nothing and the write takes no transaction. Keys come back sorted and unique,
// because two writers taking the same locks in different orders would deadlock.
func writeLockKeys(workspaceID string, machines map[string]*domain.Machine, machine *domain.Machine, id string, existing, next map[string]any) []string {
	set := map[string]bool{}
	of := func(v map[string]any, field string) string {
		if v == nil {
			return ""
		}
		s := fmt.Sprint(v[field])
		if s == "<nil>" {
			return ""
		}
		return s
	}
	for _, b := range machine.Bounds {
		for _, v := range []map[string]any{existing, next} {
			if g := of(v, b.Per); g != "" {
				set[fmt.Sprintf("ensure|%s|%s|%s|%s", workspaceID, machine.ID, b.Per, g)] = true
			}
		}
	}
	for _, m := range machines {
		for _, t := range m.Transitions {
			if b := t.Balanced; b != nil && m.ID == machine.ID && id != "" {
				set[fmt.Sprintf("balanced|%s|%s", workspaceID, id)] = true
			}
			if b := t.Balanced; b != nil && b.Machine == machine.ID {
				for _, v := range []map[string]any{existing, next} {
					if e := of(v, b.Via); e != "" {
						set[fmt.Sprintf("balanced|%s|%s", workspaceID, e)] = true
					}
				}
			}
		}
	}
	if id == "" {
		// A numbered Field is written under its own lock: two creates must not read the same highest number.
		for _, f := range machine.Fields {
			if f.Numbering != nil {
				set[execution.NumberingLockKey(workspaceID, machine.ID, f.ID)] = true
			}
		}
	}
	if id != "" {
		for _, t := range machine.Transitions {
			if t.Generates != nil {
				set[fmt.Sprintf("generates|%s|%s|%s", workspaceID, machine.ID, id)] = true
			}
		}
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// errRefusedWrite stops a transaction whose function already answered the request with a refusal.
var errRefusedWrite = errors.New("write refused")

// withWriteLock runs fn, the guards and the write, holding the locks writeLockKeys names inside one transaction that
// commits only if fn returns true. fn answers the request itself when it refuses, as every guard does, and gets a
// request whose context carries the transaction, so the records it reads and writes are the transaction's. A write that
// needs no lock runs fn directly, with no transaction. Events and notifications belong after the call: they must not
// run for a write that rolls back, nor hold the locks while they work.
func withWriteLock(w http.ResponseWriter, req *http.Request, store *data.Store, machines map[string]*domain.Machine, machine *domain.Machine, id string, next map[string]any, fn func(req *http.Request) bool) bool {
	ctx := req.Context()
	workspaceID, _ := data.WorkspaceScope(ctx)
	var existing map[string]any
	if id != "" && (len(machine.Bounds) > 0 || referencedByBalance(machines, machine)) {
		if rec, err := store.GetRecord(ctx, machine.ID, id); err == nil {
			existing = rec.Values
		}
	}
	keys := writeLockKeys(workspaceID, machines, machine, id, existing, next)
	// A record another Machine adds up (`sum_children`, `count_children`) is written in a transaction even with no lock
	// to take: its parent is recomputed inside it, under the parent's row lock, so the child and the parent's figures
	// commit together or not at all.
	if len(keys) == 0 && !execution.Aggregated(machines, machine) {
		return fn(req)
	}
	err := store.InTx(ctx, func(ctx context.Context) error {
		for _, k := range keys {
			if err := store.Lock(ctx, k); err != nil {
				return err
			}
		}
		if !fn(req.WithContext(ctx)) {
			return errRefusedWrite
		}
		return nil
	})
	switch {
	case err == nil:
		return true
	case errors.Is(err, errRefusedWrite):
		return false
	default:
		serverError(w, err)
		return false
	}
}

// referencedByBalance says some Machine of the Workspace has an edge that totals this Machine's records.
func referencedByBalance(machines map[string]*domain.Machine, machine *domain.Machine) bool {
	for _, m := range machines {
		for _, t := range m.Transitions {
			if t.Balanced != nil && t.Balanced.Machine == machine.ID {
				return true
			}
		}
	}
	return false
}
