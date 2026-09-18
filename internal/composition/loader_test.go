package composition

import (
	"testing"

	"menata.app/internal/domain"
)

func machineMap(ids ...string) map[string]*domain.Machine {
	m := make(map[string]*domain.Machine, len(ids))
	for _, id := range ids {
		m[id] = &domain.Machine{ID: id}
	}
	return m
}

// TestMachineSliceIsStable locks the defect Phase 18 Step 2's before/after HTML comparison found:
// Go randomizes map iteration, so an unsorted slice made a record's child sections render in a
// different order on every reload. Ten runs is enough -- with 5 entries, a random order repeats
// the same sequence by chance only rarely, and a regression here shows up immediately.
func TestMachineSliceIsStable(t *testing.T) {
	l := NewLoader(nil, machineMap("mch_task", "mch_project", "mch_activity", "mch_user", "mch_list"))

	want := l.machineSlice()
	if len(want) != 5 {
		t.Fatalf("got %d machines, want 5", len(want))
	}
	for run := 0; run < 10; run++ {
		got := l.machineSlice()
		for i := range got {
			if got[i].ID != want[i].ID {
				t.Fatalf("run %d: order changed at %d: got %s, want %s", run, i, got[i].ID, want[i].ID)
			}
		}
	}
}

func TestMachineSliceIsSortedByID(t *testing.T) {
	l := NewLoader(nil, machineMap("mch_user", "mch_activity", "mch_task"))

	got := l.machineSlice()
	want := []string{"mch_activity", "mch_task", "mch_user"}
	for i, id := range want {
		if got[i].ID != id {
			t.Errorf("position %d: got %s, want %s", i, got[i].ID, id)
		}
	}
}

func TestDisplayString(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  string
	}{
		{"nil is empty", nil, ""},
		{"string passes through", "in_review", "in_review"},
		{"number is rendered", float64(3), "3"},
		{"bool is rendered", true, "true"},
		{"slice is rendered as JSON", []any{"a", "b"}, `["a","b"]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DisplayString(tc.value); got != tc.want {
				t.Errorf("DisplayString(%v) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}
