package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"menata.app/internal/domain"
)

func roleTestApplications() []domain.Application {
	return []domain.Application{
		{ID: "app_document_approval", Name: "Document Approval", Roles: []string{"approver", "submitter"}},
		{ID: "app_project_management", Name: "Project Management"}, // declares none
	}
}

func postForm(t *testing.T, values url.Values) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

// TestSubmittedAppRoles_rejectsUndeclaredRole is the point of declaring a vocabulary at all.
// Before Fase 3b the write paths stored whatever string arrived in app_role -- workspace_role has
// always been checked ("admin"/"member"), the Application role never was, because there was
// nothing to check it against. A crafted POST could store a role no screen could render.
func TestSubmittedAppRoles_rejectsUndeclaredRole(t *testing.T) {
	req := postForm(t, url.Values{"app_role[app_document_approval]": {"emperor"}})
	if _, err := submittedAppRoles(req, roleTestApplications()); err == nil {
		t.Fatal("submittedAppRoles() error = nil, want rejection of a role the Application does not declare")
	}
}

func TestSubmittedAppRoles_acceptsDeclaredRole(t *testing.T) {
	req := postForm(t, url.Values{"app_role[app_document_approval]": {"approver"}})
	roles, err := submittedAppRoles(req, roleTestApplications())
	if err != nil {
		t.Fatalf("submittedAppRoles() error = %v", err)
	}
	if roles["app_document_approval"] != "approver" {
		t.Errorf("roles = %+v, want app_document_approval=approver", roles)
	}
}

// An empty value is legitimate: it means "no role here", which SetMemberAppRole stores as the
// absence of a row rather than a blank one.
func TestSubmittedAppRoles_emptyMeansNoRole(t *testing.T) {
	req := postForm(t, url.Values{"app_role[app_document_approval]": {""}})
	roles, err := submittedAppRoles(req, roleTestApplications())
	if err != nil {
		t.Fatalf("submittedAppRoles() error = %v", err)
	}
	if got, ok := roles["app_document_approval"]; !ok || got != "" {
		t.Errorf("roles = %+v, want an explicit empty role for app_document_approval", roles)
	}
}

// An Application declaring no roles offers no select, so nothing it receives came from a form this
// app rendered -- it is skipped rather than validated or stored.
func TestSubmittedAppRoles_skipsApplicationsWithoutRoles(t *testing.T) {
	req := postForm(t, url.Values{"app_role[app_project_management]": {"anything"}})
	roles, err := submittedAppRoles(req, roleTestApplications())
	if err != nil {
		t.Fatalf("submittedAppRoles() error = %v", err)
	}
	if _, ok := roles["app_project_management"]; ok {
		t.Errorf("roles = %+v, want no entry for an Application that declares no roles", roles)
	}
}
