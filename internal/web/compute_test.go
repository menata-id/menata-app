package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/authorization"
	"menata.app/internal/data"
	"menata.app/internal/mail"
	"menata.app/internal/metadata"
	"menata.app/internal/rendering"
)

// TestGenericWriteRoutes_fillAComputedTotal: the three generic routes that write a Machine allowed to
// declare a computed Field each compute it, overwriting whatever was submitted. internal/metadata
// refuses a computed Field anywhere else, so these routes are the whole population.
func TestGenericWriteRoutes_fillAComputedTotal(t *testing.T) {
	s := newEventTestSetup(t, "compute_water_total")
	water, err := metadata.Parse([]byte(`id: mch_water
name: Water
fields:
  - id: fld_pdam
    name: PDAM
    type: number
  - id: fld_well
    name: Well
    type: number
  - id: fld_total
    name: Total
    type: number
    compute:
      op: sum
      fields: [fld_pdam, fld_well]
`))
	if err != nil {
		t.Fatal(err)
	}
	if err := metadata.Validate(water); err != nil {
		t.Fatal(err)
	}
	s.machines["mch_water"] = water

	serve := func(method, path, contentType, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", contentType)
		req = req.WithContext(rendering.WithCurrentWorkspace(data.WithWorkspaceScope(req.Context(), s.workspaceID), testWorkspaceFor(s.machines), "Test Workspace", false))
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, s.cfg, s.actor, 0)})
		r := chi.NewRouter()
		r.Post("/machines/{machineID}/records", createRecordForm(s.store, nil, mail.LogMailer{}, s.cfg))
		r.Put("/machines/{machineID}/records/{id}", updateRecordForm(s.store, nil, mail.LogMailer{}, s.cfg))
		r.Post("/api/machines/{machineID}/records", createRecord(s.store, nil, mail.LogMailer{}, s.cfg))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code >= 300 {
			t.Fatalf("%s %s = %d; body=%s", method, path, rec.Code, rec.Body.String())
		}
		return rec
	}
	recordWith := func(pdam float64) *data.Record {
		rows, err := s.store.ListRecords(s.ctx, "mch_water")
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rows {
			if r.Values["fld_pdam"] == pdam {
				return r
			}
		}
		t.Fatalf("no record with fld_pdam = %v", pdam)
		return nil
	}
	totalOf := func(pdam float64) any { return recordWith(pdam).Values["fld_total"] }
	form := "application/x-www-form-urlencoded"

	serve(http.MethodPost, "/machines/mch_water/records", form, "fld_pdam=2&fld_well=3&fld_total=999")
	if got := totalOf(2); got != 5.0 {
		t.Errorf("form create: total = %v, want 5", got)
	}

	serve(http.MethodPut, "/machines/mch_water/records/"+recordWith(2).ID, form, "fld_pdam=10&fld_well=3")
	if got := totalOf(10); got != 13.0 {
		t.Errorf("form edit: total = %v, want 13", got)
	}

	serve(http.MethodPost, "/api/machines/mch_water/records", "application/json", `{"fld_pdam": 7, "fld_well": 1, "fld_total": 0}`)
	if got := totalOf(7); got != 8.0 {
		t.Errorf("api create: total = %v, want 8", got)
	}
}
