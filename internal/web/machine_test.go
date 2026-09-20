package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"menata.app/internal/domain"
)

func viewMachine() *domain.Machine {
	return &domain.Machine{ID: "mch_document", Views: []domain.View{
		{ID: "vw_document_table", Name: "Table", Type: domain.ViewTable},
		{ID: "vw_document_cards", Name: "Cards", Type: domain.ViewCards},
	}}
}

func TestResolveView_defaultsToTheFirstDeclared(t *testing.T) {
	w := httptest.NewRecorder()
	v, ok := resolveView(w, viewMachine(), httptest.NewRequest(http.MethodGet, "/machines/mch_document", nil))
	if !ok || v.ID != "vw_document_table" {
		t.Fatalf("resolveView() = %+v, %v; want the first declared view", v, ok)
	}
}

func TestResolveView_readsTheQueryParameter(t *testing.T) {
	w := httptest.NewRecorder()
	v, ok := resolveView(w, viewMachine(), httptest.NewRequest(http.MethodGet, "/machines/mch_document?view=vw_document_cards", nil))
	if !ok || v.EffectiveType() != domain.ViewCards {
		t.Fatalf("resolveView() = %+v, %v; want the cards view", v, ok)
	}
}

// An id the Machine does not declare must say so. Rendering a different arrangement than the one
// asked for would look like the feature working, which is the failure this whole step is about.
func TestResolveView_unknownIDIs404(t *testing.T) {
	w := httptest.NewRecorder()
	if _, ok := resolveView(w, viewMachine(), httptest.NewRequest(http.MethodGet, "/machines/mch_document?view=vw_nope", nil)); ok {
		t.Fatal("resolveView() reported ok for an undeclared view id")
	}
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// A mutating fragment POSTs to a URL of its own, which carries no selection: without the
// HX-Current-URL fallback, creating a record while looking at the cards view would swap the body
// back to the table underneath the viewer.
func TestResolveView_fallsBackToTheHTMXCurrentURL(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/machines/mch_document/records", nil)
	req.Header.Set("HX-Current-URL", "http://localhost:4000/machines/mch_document?view=vw_document_cards")

	w := httptest.NewRecorder()
	v, ok := resolveView(w, viewMachine(), req)
	if !ok || v.ID != "vw_document_cards" {
		t.Fatalf("resolveView() = %+v, %v; want the view the page was on", v, ok)
	}
}

// A Machine declaring no views: resolves to one implicit table, exactly as before views existed.
func TestResolveView_machineWithNoDeclaredViews(t *testing.T) {
	w := httptest.NewRecorder()
	m := &domain.Machine{ID: "mch_task"}
	v, ok := resolveView(w, m, httptest.NewRequest(http.MethodGet, "/machines/mch_task", nil))
	if !ok || v.EffectiveType() != domain.ViewTable {
		t.Fatalf("resolveView() = %+v, %v; want an implicit table", v, ok)
	}
}
