package web

import (
	"context"
	"strings"
	"testing"

	"menata.app/internal/data"
	"menata.app/internal/domain"
)

func TestFrozenReason(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	ws, err := store.CreateWorkspace(ctx, "Frozen Test", "frozen-test-workspace")
	if err != nil {
		t.Fatal(err)
	}
	cleanupAuthTest(t, pool, ws.ID, "frozen_test@example.com")
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)
	mk := func(machine string, v map[string]any) string {
		r, err := store.CreateRecord(wsCtx, machine, v)
		if err != nil {
			t.Fatal(err)
		}
		return r.ID
	}
	open := mk("mch_period_fixture", map[string]any{"fld_status": "terbuka"})
	closed := mk("mch_period_fixture", map[string]any{"fld_status": "ditutup"})
	voucher := &domain.Machine{ID: "mch_voucher_fixture", Name: "Voucher",
		Fields: []domain.Field{{ID: "fld_period", Name: "Periode", Type: domain.FieldTypeRelation, RelatedMachine: "mch_period_fixture"}},
		Frozen: &domain.FrozenWhen{Via: "fld_period", Field: "fld_status", Equals: "ditutup"}}
	plain := &domain.Machine{ID: "mch_plain_fixture", Fields: voucher.Fields}

	for _, tc := range []struct {
		name           string
		machine        *domain.Machine
		existing, next map[string]any
		frozen         bool
	}{
		{"an edit of a record under a closed period", voucher, map[string]any{"fld_period": closed}, map[string]any{"fld_period": closed}, true},
		{"an edit of a record under an open period", voucher, map[string]any{"fld_period": open}, map[string]any{"fld_period": open}, false},
		{"a create under a closed period", voucher, nil, map[string]any{"fld_period": closed}, true},
		{"a create under an open period", voucher, nil, map[string]any{"fld_period": open}, false},
		{"moving a record into a closed period", voucher, map[string]any{"fld_period": open}, map[string]any{"fld_period": closed}, true},
		{"moving a record out of a closed period", voucher, map[string]any{"fld_period": closed}, map[string]any{"fld_period": open}, true},
		{"a delete under a closed period", voucher, map[string]any{"fld_period": closed}, nil, true},
		{"a record naming no period", voucher, map[string]any{}, map[string]any{}, false},
		{"a dangling period", voucher, map[string]any{"fld_period": "rec_gone"}, map[string]any{"fld_period": "rec_gone"}, false},
		{"a Machine declaring no freeze", plain, map[string]any{"fld_period": closed}, map[string]any{"fld_period": closed}, false},
	} {
		reason, err := frozenReason(wsCtx, store, tc.machine, tc.existing, tc.next)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if (reason != "") != tc.frozen {
			t.Errorf("%s: reason = %q, want frozen=%v", tc.name, reason, tc.frozen)
		}
		if tc.frozen && !strings.Contains(reason, "frozen") {
			t.Errorf("%s: the reason does not say so: %q", tc.name, reason)
		}
	}
}
