package data

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestStore_Extraction: a proposal is read back only in its own Workspace, and a proposal is saved once.
func TestStore_Extraction(t *testing.T) {
	store := NewStore(storePool(t))
	ctx := context.Background()
	mine := newAISessionTestWorkspace(t, store)
	ws2, err := store.CreateWorkspace(ctx, "Extraction Other", "extraction-other-workspace")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = store.pool.Exec(context.Background(), `DELETE FROM workspaces WHERE id = $1`, ws2.ID) })

	e, err := store.StartExtraction(ctx, Extraction{WorkspaceID: mine, UserID: "usr_a", NavID: "nav_new", MachineID: "mch_c",
		FileKey: "k/doc.pdf", FileName: "doc.pdf", Proposal: []byte(`{"values":{"fld_x":{"value":"1"}}}`)})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.FinishExtraction(ctx, mine, e.ID, []byte(`{"values":{"fld_x":{"value":"1"}}}`)); err != nil {
		t.Fatal(err)
	}
	failed, err := store.StartExtraction(ctx, Extraction{WorkspaceID: mine, UserID: "usr_a", NavID: "nav_new", MachineID: "mch_c", FileKey: "k/b.pdf", FileName: "b.pdf"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.FailExtraction(ctx, mine, failed.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.GetExtraction(ctx, mine, failed.ID); got.Status != ExtractionFailed {
		t.Errorf("a failed reading is %q", got.Status)
	}
	got, err := store.GetExtraction(ctx, mine, e.ID)
	if err != nil || got.UserID != "usr_a" || got.NavID != "nav_new" || got.Status != ExtractionProposed || string(got.Proposal) != `{"values": {"fld_x": {"value": "1"}}}` {
		t.Fatalf("read back %+v (%v)", got, err)
	}
	if _, err := store.GetExtraction(ctx, ws2.ID, e.ID); !errors.Is(err, ErrRecordNotFound) {
		t.Errorf("another Workspace read the proposal: %v", err)
	}
	if ok, err := store.MarkExtractionSaved(ctx, ws2.ID, e.ID, "rec_1"); err != nil || ok {
		t.Errorf("another Workspace saved the proposal: %v %v", ok, err)
	}
	if ok, err := store.MarkExtractionSaved(ctx, mine, e.ID, "rec_1"); err != nil || !ok {
		t.Fatalf("first save: %v %v", ok, err)
	}
	if ok, err := store.MarkExtractionSaved(ctx, mine, e.ID, "rec_2"); err != nil || ok {
		t.Errorf("a proposal was saved twice: %v %v", ok, err)
	}
	if n, err := store.CountExtractions(ctx, mine, time.Now().Add(-time.Hour)); err != nil || n != 2 {
		t.Errorf("count = %d (%v), want 2 (the failed attempt counts)", n, err)
	}
}

// TestStore_ExpireExtractions: a reading nobody saved is deleted once it is old enough, and its document's key is handed back;
// a saved one -- whose document is now on a record -- and a recent one are left alone.
func TestStore_ExpireExtractions(t *testing.T) {
	store := NewStore(storePool(t))
	ctx := context.Background()
	ws := newAISessionTestWorkspace(t, store)
	start := func(name string) *Extraction {
		t.Helper()
		e, err := store.StartExtraction(ctx, Extraction{WorkspaceID: ws, UserID: "usr_a", NavID: "nav_new", MachineID: "mch_c", FileKey: "k/" + name, FileName: name})
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	old := func(e *Extraction) {
		t.Helper()
		if _, err := store.pool.Exec(ctx, `UPDATE document_extractions SET created_at = NOW() - interval '2 days' WHERE id = $1`, e.ID); err != nil {
			t.Fatal(err)
		}
	}
	abandoned, saved, recent, reading := start("abandoned.pdf"), start("saved.pdf"), start("recent.pdf"), start("reading.pdf")
	for _, e := range []*Extraction{abandoned, saved, recent} {
		if err := store.FinishExtraction(ctx, ws, e.ID, []byte(`{}`)); err != nil {
			t.Fatal(err)
		}
	}
	if ok, err := store.MarkExtractionSaved(ctx, ws, saved.ID, "rec_1"); err != nil || !ok {
		t.Fatalf("save: %v %v", ok, err)
	}
	old(abandoned)
	old(saved)
	old(reading)

	keys, err := store.ExpireExtractions(ctx, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, k := range keys {
		got[k] = true
	}
	if !got["k/abandoned.pdf"] || !got["k/reading.pdf"] || got["k/saved.pdf"] || got["k/recent.pdf"] {
		t.Errorf("expired keys = %v", keys)
	}
	if _, err := store.GetExtraction(ctx, ws, abandoned.ID); !errors.Is(err, ErrRecordNotFound) {
		t.Errorf("the abandoned reading is still there: %v", err)
	}
	for _, e := range []*Extraction{saved, recent} {
		if _, err := store.GetExtraction(ctx, ws, e.ID); err != nil {
			t.Errorf("%s was deleted: %v", e.FileName, err)
		}
	}
}
