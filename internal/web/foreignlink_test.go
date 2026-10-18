package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"menata.app/internal/config"
	"menata.app/internal/domain"
)

// TestLinkTargetsForOffersOnlyScreensTheViewerMayOpen: a link into another Application (`in:`) resolves from the page's
// own targets plus the other Applications' screens the viewer may open, each named "<application>/<item>"; an Application
// the viewer has no role in contributes none -- the link is then not drawn rather than drawn to a 403 -- and a record page
// is never a target (it needs a record's id).
func TestLinkTargetsForOffersOnlyScreensTheViewerMayOpen(t *testing.T) {
	contracts := domain.Application{ID: "app_contracts", Roles: []string{"production_manager"},
		AllNavigation: []domain.NavigationItem{{ID: "nav_list", Route: "/pages/nav_list"}}}
	payments := domain.Application{ID: "app_payments", Roles: []string{"finance", "production_manager"},
		AllNavigation: []domain.NavigationItem{
			{ID: "nav_new_plan", Route: "/pages/nav_new_plan", NotInMenu: true},
			{ID: "nav_plan", Route: "/pages/nav_plan", RecordOf: "mch_plan"},
		}}
	closed := domain.Application{ID: "app_closed", Roles: []string{"admin"}, AllNavigation: []domain.NavigationItem{{ID: "nav_x", Route: "/pages/nav_x"}}}
	ws := domain.Workspace{Applications: []domain.Application{contracts, payments, closed}}
	cfg := config.Config{AdminUserID: "usr_admin"}

	ids := func(actor domain.Actor) map[string]bool {
		out := map[string]bool{}
		for _, it := range linkTargetsFor(ws, contracts, actor, cfg) {
			out[it.ID] = true
		}
		return out
	}
	member := domain.Actor{ID: "usr_silvia", Roles: map[string][]string{"app_contracts": {"production_manager"}, "app_payments": {"production_manager"}}}
	got := ids(member)
	if !got["nav_list"] || !got[domain.ForeignLinkID("app_payments", "nav_new_plan")] {
		t.Errorf("a member of both Applications is offered %v, want its own screen and the other's form", got)
	}
	if got[domain.ForeignLinkID("app_payments", "nav_plan")] || got[domain.ForeignLinkID("app_closed", "nav_x")] || got[domain.ForeignLinkID("app_contracts", "nav_list")] {
		t.Errorf("offered %v: a record page, an Application the viewer cannot open, or the page's own Application by a foreign name", got)
	}
	alone := domain.Actor{ID: "usr_bob", Roles: map[string][]string{"app_contracts": {"production_manager"}}}
	if ids(alone)[domain.ForeignLinkID("app_payments", "nav_new_plan")] {
		t.Error("a viewer with no role in the other Application was offered its screen")
	}
}

// TestPageParamsNeverCarryARecordNameFromTheQueryString: the `record.` parameters of a record page are the runtime's, set from
// the record; a query string naming one is dropped, so it cannot aim a Dataset at another record's value.
func TestPageParamsNeverCarryARecordNameFromTheQueryString(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/pages/nav_x?q=1&record.fld_contract=rec_other&application=app_other", nil)
	got := pageParams(req, "app_real")
	if _, taken := got["record.fld_contract"]; taken {
		t.Errorf("a record parameter came from the query string: %v", got)
	}
	if got["q"] != "1" || got["application"] != "app_real" {
		t.Errorf("params = %v, want q kept and the application the runtime's", got)
	}
}
