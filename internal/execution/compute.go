package execution

import (
	"context"
	"fmt"
	"log"
	"maps"
	"slices"

	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// recordEnv is domain.ComputeEnv over the store, for one record about to be written: the record its relation Fields
// point at (`parent:`), and the records pointing at it (`sum_children`, `count_children`). A new record (no id) has no
// children. A read that fails reads as nothing and is logged: a computed value is then empty, never invented.
type recordEnv struct {
	ctx     context.Context
	store   *data.Store
	machine *domain.Machine
	id      string
	values  map[string]any
}

func (e recordEnv) Parent(via string) map[string]any {
	f, ok := e.machine.FieldByID(via)
	id := fmt.Sprint(e.values[via])
	if !ok || id == "" || id == "<nil>" {
		return nil
	}
	parent, err := e.store.GetRecord(e.ctx, f.RelatedMachine, id)
	if err != nil {
		log.Printf("compute %s %s: reading parent %s: %v", e.machine.ID, e.id, id, err)
		return nil
	}
	return parent.Values
}

func (e recordEnv) Children(agg domain.ChildAggregate) []map[string]any {
	if e.id == "" {
		return nil
	}
	children, err := e.store.ListRecordsBy(e.ctx, agg.Machine, agg.Via, e.id)
	if err != nil {
		log.Printf("compute %s %s: reading %s: %v", e.machine.ID, e.id, agg.Machine, err)
		return nil
	}
	out := make([]map[string]any, 0, len(children))
	for _, c := range children {
		out = append(out, c.Values)
	}
	return out
}

// readsBeyond reports whether any computed Field of m reads another record -- the only case that costs a read.
func readsBeyond(m *domain.Machine) bool {
	beyond := false
	for _, f := range m.Fields {
		if f.Compute != nil {
			f.Compute.Tree().Leaves(func(l domain.ComputeExpr) {
				beyond = beyond || l.Parent != nil || l.Children != nil
			})
		}
	}
	return beyond
}

// ApplyComputed is data.ApplyComputed for a record about to be written through a route: what its computed Fields read
// on other records is read from the store (id is "" for a record not yet created).
func ApplyComputed(ctx context.Context, store *data.Store, m *domain.Machine, id string, values map[string]any) {
	if !readsBeyond(m) {
		data.ApplyComputed(m, values)
		return
	}
	data.ApplyComputedWith(m, values, recordEnv{ctx: ctx, store: store, machine: m, id: id, values: values})
}

// RecomputeParents recomputes the records whose computed Fields aggregate records of child (`sum_children`,
// `count_children`) and that the written record pointed at in any of valueSets -- before and after a move, both. Called
// in the same transaction as the child's write, so a contract's paid figure never disagrees with its instalments, and
// after a write the runtime makes itself (a rollup, a scheduled set_field). A parent whose values do not change is not
// written; one that does is recomputed upward in turn (a grandparent), at most a few levels deep.
func RecomputeParents(ctx context.Context, store *data.Store, machines map[string]*domain.Machine, child *domain.Machine, valueSets ...map[string]any) error {
	return recomputeParents(ctx, store, machines, child, valueSets, 0)
}

func recomputeParents(ctx context.Context, store *data.Store, machines map[string]*domain.Machine, child *domain.Machine, valueSets []map[string]any, depth int) error {
	if depth > len(machines) {
		return nil // the loader refuses a Machine aggregating itself, so a chain of parents ends within the Machines there are
	}
	for _, pid := range slices.Sorted(maps.Keys(machines)) {
		parent := machines[pid]
		vias := map[string]bool{}
		for _, f := range parent.Fields {
			if f.Compute == nil {
				continue
			}
			f.Compute.Tree().Leaves(func(l domain.ComputeExpr) {
				if l.Children != nil && l.Children.Machine == child.ID {
					vias[l.Children.Via] = true
				}
			})
		}
		ids := map[string]bool{}
		for via := range vias {
			for _, values := range valueSets {
				if id := fmt.Sprint(values[via]); values != nil && id != "" && id != "<nil>" {
					ids[id] = true
				}
			}
		}
		for _, id := range slices.Sorted(maps.Keys(ids)) {
			before, after, err := recomputeRecord(ctx, store, parent, id)
			if err != nil {
				return err
			}
			if after == nil {
				continue
			}
			if err := recomputeParents(ctx, store, machines, parent, []map[string]any{before, after}, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

// recomputeRecord recomputes one record's computed Fields in a transaction (the caller's, when it has one) holding the
// record's row (FOR UPDATE), so a person's edit of it and this write queue instead of one writing over the other. It
// answers the values before and after when they changed, and nil after when they did not or the record is gone.
func recomputeRecord(ctx context.Context, store *data.Store, m *domain.Machine, id string) (before, after map[string]any, err error) {
	err = store.InTx(ctx, func(ctx context.Context) error {
		rec, err := store.GetRecordForUpdate(ctx, m.ID, id)
		if err != nil {
			return nil // a record that is gone has nothing to recompute
		}
		next := maps.Clone(rec.Values)
		data.ApplyComputedWith(m, next, recordEnv{ctx: ctx, store: store, machine: m, id: id, values: next})
		if sameValues(rec.Values, next) {
			return nil
		}
		if _, err := store.UpdateRecord(ctx, m.ID, id, next); err != nil {
			return fmt.Errorf("recompute %s %s: %w", m.ID, id, err)
		}
		before, after = rec.Values, next
		return nil
	})
	return before, after, err
}

// RecomputeChildren recomputes the records whose computed Fields read a Field of this one through `parent:`, after it is
// written, so a value taken from a contract follows the contract. What they add up to on their own parents follows them.
func RecomputeChildren(ctx context.Context, store *data.Store, machines map[string]*domain.Machine, parent *domain.Machine, id string) error {
	return recomputeChildren(ctx, store, machines, parent, id, 0)
}

// recomputeChildren follows a changed value down as well: an instalment's category follows its contract's, and a payment
// plan's follows the instalment's, so a record that changed recomputes the records reading it in turn, at most as deep
// as there are Machines.
func recomputeChildren(ctx context.Context, store *data.Store, machines map[string]*domain.Machine, parent *domain.Machine, id string, depth int) error {
	if depth > len(machines) {
		return nil
	}
	for _, cid := range slices.Sorted(maps.Keys(machines)) {
		child := machines[cid]
		vias := map[string]bool{}
		for _, f := range child.Fields {
			if f.Compute == nil {
				continue
			}
			f.Compute.Tree().Leaves(func(l domain.ComputeExpr) {
				if l.Parent != nil {
					if via, ok := child.FieldByID(l.Parent.Via); ok && via.RelatedMachine == parent.ID {
						vias[l.Parent.Via] = true
					}
				}
			})
		}
		for _, via := range slices.Sorted(maps.Keys(vias)) {
			children, err := store.ListRecordsBy(ctx, child.ID, via, id)
			if err != nil {
				return err
			}
			for _, c := range children {
				before, after, err := recomputeRecord(ctx, store, child, c.ID)
				if err != nil {
					return err
				}
				if after != nil {
					if err := RecomputeParents(ctx, store, machines, child, before, after); err != nil {
						return err
					}
					if err := recomputeChildren(ctx, store, machines, child, c.ID, depth+1); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// Aggregated reports whether some Machine of machines adds up records of m (`sum_children`, `count_children`), so a
// write of m recomputes a parent and belongs in a transaction.
func Aggregated(machines map[string]*domain.Machine, m *domain.Machine) bool {
	found := false
	for _, p := range machines {
		for _, f := range p.Fields {
			if f.Compute != nil {
				f.Compute.Tree().Leaves(func(l domain.ComputeExpr) { found = found || (l.Children != nil && l.Children.Machine == m.ID) })
			}
		}
	}
	return found
}

// RecomputeAll recomputes every record of every Machine in machines that declares a computed Field, writing only the
// ones whose values change, and answers how many were written: owner decision K3 -- when a formula changes, the records
// already stored follow it. Run when a Workspace's metadata is loaded (start, reload), so a formula published to the
// metadata is in every record before anyone reads it.
func RecomputeAll(ctx context.Context, store *data.Store, machines []*domain.Machine) (int, error) {
	written := 0
	// A Machine that aggregates others is recomputed after them, so it adds up what they now hold; and the pass repeats
	// while it still writes (a grandparent adding up parents that themselves add up), at most once per Machine.
	ordered := slices.Clone(machines)
	slices.SortStableFunc(ordered, func(a, b *domain.Machine) int { return boolInt(aggregates(a)) - boolInt(aggregates(b)) })
	for pass := 0; pass <= len(ordered); pass++ {
		n, err := recomputePass(ctx, store, ordered)
		written += n
		if err != nil || n == 0 {
			return written, err
		}
	}
	return written, nil
}

func recomputePass(ctx context.Context, store *data.Store, ordered []*domain.Machine) (int, error) {
	written := 0
	for _, m := range ordered {
		if len(m.ComputeOrder()) == 0 || !m.GenericallyWritten() {
			continue
		}
		err := store.EachRecordPage(ctx, m.ID, 200, func(records []*data.Record) error {
			for _, r := range records {
				next := maps.Clone(r.Values)
				data.ApplyComputedWith(m, next, recordEnv{ctx: ctx, store: store, machine: m, id: r.ID, values: next})
				if sameValues(r.Values, next) {
					continue
				}
				// Written under the record's row lock, re-read: a person's edit since the page was read is not undone.
				_, after, err := recomputeRecord(ctx, store, m, r.ID)
				if err != nil {
					log.Printf("recompute %s %s: %v", m.ID, r.ID, err) // one record that cannot be written does not stop the rest
					continue
				}
				if after != nil {
					written++
				}
			}
			return nil
		})
		if err != nil {
			return written, err
		}
	}
	return written, nil
}

func sameValues(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if fmt.Sprint(v) != fmt.Sprint(b[k]) {
			return false
		}
	}
	return true
}

func aggregates(m *domain.Machine) bool {
	found := false
	for _, f := range m.Fields {
		if f.Compute != nil {
			f.Compute.Tree().Leaves(func(l domain.ComputeExpr) { found = found || l.Children != nil })
		}
	}
	return found
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
