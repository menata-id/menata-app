package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/builder"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/installer"
)

// maxBuilderBody bounds a request of operations; a list of edits is a few hundred bytes.
const maxBuilderBody = 64 << 10

// builderPlanView is the answer to a list of operations: what they would do (PlanUpdate's own lines), what the policy
// refuses, and the files the writer would produce (the inspector's Code tab). Nothing has been written.
type builderPlanView struct {
	Items    []builderPlanItem `json:"items"`
	Refusals []string          `json:"refusals,omitempty"`
	Files    []builderFile     `json:"files,omitempty"`
	Version  string            `json:"version,omitempty"`
	Error    string            `json:"error,omitempty"`
}

type builderPlanItem struct {
	Op      string `json:"op"`
	What    string `json:"what"`
	Confirm string `json:"confirm,omitempty"`
}

type builderFile struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	Diff   string `json:"diff"`
}

// builderRequest is the body of both builder routes: the operations, and for an apply the version the plan was made
// against and the removals the owner ticked.
type builderRequest struct {
	Ops     []builder.Op `json:"ops"`
	Version string       `json:"version,omitempty"`
	Confirm []string     `json:"confirm,omitempty"`
}

func readBuilderRequest(w http.ResponseWriter, req *http.Request) (builderRequest, bool) {
	var body builderRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, req.Body, maxBuilderBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil || len(body.Ops) == 0 {
		http.Error(w, `expected {"ops": [...]} with at least one operation`, http.StatusBadRequest)
		return body, false
	}
	return body, true
}

// planBuilderOperations reviews a list of Apps Builder operations against one installed Application of the current
// Workspace (Fase 1 of menata-app-document audits/2026-10-27-kajian-apps-builder.md). It is a POST because the list is a
// body, not because it writes: it never does. Applying a plan is the one write path, not this route.
func planBuilderOperations(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		body, ok := readBuilderRequest(w, req)
		if !ok {
			return
		}
		manifestPath, _, err := workspaceInstallation(req.Context(), store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		plan, err := builder.Review(manifestPath, chi.URLParam(req, "app"), body.Ops)
		view := builderPlanViewOf(plan)
		var rejected *installer.RejectedError
		switch {
		case errors.As(err, &rejected):
			view.Error = rejected.Error()
			w.WriteHeader(http.StatusUnprocessableEntity)
		case err != nil:
			serverError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		writeJSON(w, view)
	}
}

func builderPlanViewOf(plan builder.Plan) builderPlanView {
	view := builderPlanView{Items: []builderPlanItem{}, Refusals: plan.Plan.Refusals, Version: plan.Version}
	for _, it := range plan.Plan.Items {
		view.Items = append(view.Items, builderPlanItem{Op: it.Op, What: it.What, Confirm: it.Confirm})
	}
	for _, f := range plan.Files {
		view.Files = append(view.Files, builderFile{Path: f.Path, Status: f.Status, Diff: f.Diff})
	}
	return view
}

// applyBuilderOperations writes what planBuilderOperations showed -- the one write: builder.Commit, i.e. the assistant's own
// writer -- after a snapshot, and reloads the Workspace. It refuses a plan made against an installation that has since changed
// (409) and a removal the owner did not tick (422), exactly as publishing a conversation does.
func applyBuilderOperations(store *data.Store, cfg config.Config, reload func(slug string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		body, ok := readBuilderRequest(w, req)
		if !ok {
			return
		}
		if body.Version == "" {
			http.Error(w, "version is required: apply the plan you reviewed", http.StatusBadRequest)
			return
		}
		ctx, appID := req.Context(), chi.URLParam(req, "app")
		manifestPath, _, err := workspaceInstallation(ctx, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		snapshotBeforeWrite(cfg, manifestPath)
		err = builder.Commit(manifestPath, appID, body.Ops, body.Version, body.Confirm)
		var rejected *installer.RejectedError
		switch {
		case errors.Is(err, builder.ErrStale):
			http.Error(w, err.Error(), http.StatusConflict)
			return
		case errors.As(err, &rejected):
			http.Error(w, rejected.Error(), http.StatusUnprocessableEntity)
			return
		case err != nil:
			serverError(w, err)
			return
		}
		workspaceID, _ := data.WorkspaceScope(ctx)
		log.Printf("BUILDER-APPLY app=%s workspace=%s ops=%d", appID, workspaceID, len(body.Ops))
		if reload == nil {
			serverError(w, errors.New("builder: written, but this process has no reload hook -- a restart will pick it up"))
			return
		}
		if err := reloadCurrentWorkspace(ctx, store, reload); err != nil {
			serverError(w, fmt.Errorf("builder: written, but the live reload failed -- a restart will pick it up: %w", err))
			return
		}
		version, _ := builder.Version(manifestPath)
		w.Header().Set("Content-Type", "application/json")
		writeJSON(w, map[string]any{"applied": true, "version": version})
	}
}

// runEditField is the Edit tab of the builder for a Field: the record is the part the tree names ("<app>/machines/<machine>/fields/<field>"),
// and the form says the Field's new name and whether it is required. It makes the same two operations a conversation would
// (rename_field, set_required) and writes them through builder.Commit -- the one write path, which load-verifies and rolls back -- after a
// snapshot, then reloads the Workspace and returns to the part, so the person sees it renamed in the tree and in the preview.
func runEditField(w http.ResponseWriter, req *http.Request, env actionEnv, record string) (string, bool) {
	ctx := req.Context()
	appID, rest, _ := strings.Cut(record, "/")
	parts := strings.Split(rest, "/")
	if len(parts) != 4 || parts[0] != "machines" || parts[2] != "fields" || appID == "" {
		http.Error(w, "this part is not a Field", http.StatusBadRequest)
		return "", false
	}
	name := strings.TrimSpace(req.PostForm.Get("name"))
	if name == "" {
		http.Error(w, "a Field needs a name", http.StatusBadRequest)
		return "", false
	}
	current := fieldOf(machinesFor(ctx)[parts[1]], parts[3])
	if current == nil {
		http.Error(w, "this Field is not in this Workspace", http.StatusNotFound)
		return "", false
	}
	// Only what differs is an operation: saving a form nobody changed writes nothing (the writer refuses an identical Application).
	var ops []builder.Op
	if name != current.Name {
		ops = append(ops, builder.Op{Kind: builder.OpRenameField, Machine: parts[1], Field: parts[3], Value: name})
	}
	if required := req.PostForm.Get("required") != ""; required != current.Required {
		ops = append(ops, builder.Op{Kind: builder.OpSetRequired, Machine: parts[1], Field: parts[3], Flag: required})
	}
	want, err := parseChoices(req.PostForm.Get("options"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return "", false
	}
	if len(current.Options) > 0 {
		m := machinesFor(ctx)[parts[1]]
		haveCategory := func(o string) string {
			if m.StatusCategories != nil && m.StatusCategories.Field == current.ID {
				return string(m.StatusCategories.Values[o])
			}
			return ""
		}
		more, err := optionOps(parts[1], parts[3], current.Options, haveCategory, want)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return "", false
		}
		ops = append(ops, more...)
	} else if len(want) > 0 {
		http.Error(w, "this Field offers no choices to edit", http.StatusBadRequest)
		return "", false
	}
	back := "/builder/" + url.PathEscape(appID) + "?" + url.Values{"node": {record}, "tab": {"edit"}}.Encode()
	if len(ops) == 0 {
		return back, true
	}
	return commitBuilderOps(w, req, env, appID, ops, back)
}

func fieldOf(m *domain.Machine, id string) *domain.Field {
	if m == nil {
		return nil
	}
	for i := range m.Fields {
		if m.Fields[i].ID == id {
			return &m.Fields[i]
		}
	}
	return nil
}

// choice is one line of a choices box: a choice and, after a bar, the category it is drawn in ("Approved | done").
type choice struct{ Name, Category string }

// parseChoices reads a choices box: one choice a line, a blank or repeated line says nothing, and a category after "|" must be one of the closed five.
func parseChoices(typed string) ([]choice, error) {
	var out []choice
	seen := map[string]bool{}
	for _, line := range strings.Split(typed, "\n") {
		name, cat, _ := strings.Cut(line, "|")
		name, cat = strings.TrimSpace(name), strings.TrimSpace(cat)
		if name == "" || seen[name] {
			continue
		}
		if cat != "" && !domain.KnownStatusCategories[domain.StatusCategory(cat)] {
			return nil, fmt.Errorf("%q is not a colour a status can have (start, active, attention, done or stopped)", cat)
		}
		seen[name] = true
		out = append(out, choice{name, cat})
	}
	return out, nil
}

// optionOps are the operations that turn a Field's choices into the lines typed: a line not there yet is added (at the end, in the order typed, with its
// category) and a choice no line names is removed. A colour typed against a choice that already exists, or one that differs from the one it has, is an
// error: a status someone has learned to read does not change colour. Typing no line at all would remove every choice, which is not an edit but a
// different Field, so it makes no operation.
func optionOps(machine, field string, have []string, haveCategory func(string) string, want []choice) ([]builder.Op, error) {
	if len(want) == 0 {
		return nil, nil
	}
	names := make([]string, len(want))
	for i, c := range want {
		names[i] = c.Name
	}
	var ops []builder.Op
	for _, o := range have {
		if !slices.Contains(names, o) {
			ops = append(ops, builder.Op{Kind: builder.OpRemoveOption, Machine: machine, Field: field, Value: o})
		}
	}
	for _, c := range want {
		if slices.Contains(have, c.Name) {
			if c.Category != haveCategory(c.Name) {
				return nil, fmt.Errorf("%q keeps the colour it has: a status someone has learned to read does not change colour", c.Name)
			}
			continue
		}
		ops = append(ops, builder.Op{Kind: builder.OpAddOption, Machine: machine, Field: field, Value: c.Name, Category: c.Category})
	}
	return ops, nil
}

// runAddField is the Edit tab of a Machine: the record is the Machine's part ("<app>/machines/<machine>") and the form says the new Field's name, type,
// choices (a status needs them) and whether it is required. It is the `add_field` operation a conversation would make, written through builder.Commit
// like every other builder write. The Field's id is made from its name ("fld_" and the name in lower case with words joined by "_"), made unique within
// the Machine, so a person never types an id; a Machine's form, lists and boards offer the new Field as they do any other.
func runAddField(w http.ResponseWriter, req *http.Request, env actionEnv, record string) (string, bool) {
	ctx := req.Context()
	appID, rest, _ := strings.Cut(record, "/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[0] != "machines" || appID == "" {
		http.Error(w, "this part is not a Machine", http.StatusBadRequest)
		return "", false
	}
	m := machinesFor(ctx)[parts[1]]
	if m == nil {
		http.Error(w, "this Machine is not in this Workspace", http.StatusNotFound)
		return "", false
	}
	name, typ := strings.TrimSpace(req.PostForm.Get("name")), domain.FieldType(req.PostForm.Get("type"))
	if name == "" || !slices.Contains(domain.AddableFieldTypes, typ) {
		http.Error(w, "a new Field needs a name and one of the types offered", http.StatusBadRequest)
		return "", false
	}
	choices, err := parseChoices(req.PostForm.Get("options"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return "", false
	}
	var options []string
	categories := map[string]string{}
	for _, c := range choices {
		options = append(options, c.Name)
		if c.Category != "" {
			categories[c.Name] = c.Category
		}
	}
	if typ == domain.FieldTypeStatus && len(options) == 0 {
		http.Error(w, "a status needs at least one choice", http.StatusBadRequest)
		return "", false
	}
	if typ != domain.FieldTypeStatus {
		options, categories = nil, nil
	}
	op := builder.Op{Kind: builder.OpAddField, Machine: m.ID, Field: newFieldID(m, name), Value: name, Type: string(typ), Flag: req.PostForm.Get("required") != "", Options: options, Categories: categories}
	field := record + "/fields/" + op.Field
	return commitBuilderOps(w, req, env, appID, []builder.Op{op}, "/builder/"+url.PathEscape(appID)+"?"+url.Values{"node": {field}, "tab": {"edit"}}.Encode())
}

// newFieldID makes a Field's id from its name and keeps it unique within the Machine ("fld_amount", then "fld_amount_2").
func newFieldID(m *domain.Machine, name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "_"):
			b.WriteByte('_')
		}
	}
	base := "fld_" + strings.Trim(b.String(), "_")
	if base == "fld_" {
		base = "fld_field"
	}
	id := base
	for n := 2; fieldOf(m, id) != nil; n++ {
		id = base + "_" + strconv.Itoa(n)
	}
	return id
}

// commitBuilderOps writes operations the way every builder form does -- the installation's version read just before, a snapshot, builder.Commit (which
// load-verifies and rolls back), the Workspace reloaded -- and answers a refusal to w. It returns where the person goes next.
func commitBuilderOps(w http.ResponseWriter, req *http.Request, env actionEnv, appID string, ops []builder.Op, back string) (string, bool) {
	ctx := req.Context()
	if env.reload == nil {
		serverError(w, errors.New("builder: this process has no reload hook configured"))
		return "", false
	}
	manifestPath, _, err := workspaceInstallation(ctx, env.store, env.cfg)
	if err != nil {
		serverError(w, err)
		return "", false
	}
	version, err := builder.Version(manifestPath)
	if err != nil {
		serverError(w, err)
		return "", false
	}
	snapshotBeforeWrite(env.cfg, manifestPath)
	err = builder.Commit(manifestPath, appID, ops, version, nil)
	var rejected *installer.RejectedError
	switch {
	case errors.Is(err, builder.ErrStale):
		http.Error(w, err.Error(), http.StatusConflict)
		return "", false
	case errors.As(err, &rejected):
		http.Error(w, rejected.Error(), http.StatusUnprocessableEntity)
		return "", false
	case err != nil:
		serverError(w, err)
		return "", false
	}
	workspaceID, _ := data.WorkspaceScope(ctx)
	log.Printf("BUILDER-APPLY app=%s workspace=%s ops=%d", appID, workspaceID, len(ops))
	if err := reloadCurrentWorkspace(ctx, env.store, env.reload); err != nil {
		serverError(w, fmt.Errorf("builder: written, but the live reload failed -- a restart will pick it up: %w", err))
		return "", false
	}
	return back, true
}

// runSetPermissionRoles is the Edit tab of a Permission: the record is the part ("<app>/machines/<machine>/permissions/<permission>") and the form ticks
// which of the Application's roles may do what the Permission governs. It is the `set_permission_roles` operation a conversation would make. At least
// one role stays ticked: a Permission with none would open its action to everyone, which is not an edit of who may but the end of it. A role the
// Application does not declare is stopped by the plan's validation, as it is in a conversation.
func runSetPermissionRoles(w http.ResponseWriter, req *http.Request, env actionEnv, record string) (string, bool) {
	appID, rest, _ := strings.Cut(record, "/")
	parts := strings.Split(rest, "/")
	if len(parts) != 4 || parts[0] != "machines" || parts[2] != "permissions" || appID == "" {
		http.Error(w, "this part is not a Permission", http.StatusBadRequest)
		return "", false
	}
	var roles []string
	for _, r := range req.PostForm["roles"] {
		if r = strings.TrimSpace(r); r != "" && !slices.Contains(roles, r) {
			roles = append(roles, r)
		}
	}
	if len(roles) == 0 {
		http.Error(w, "at least one role must be able to do this: with none, everyone would", http.StatusBadRequest)
		return "", false
	}
	var current *domain.Permission
	if m := machinesFor(req.Context())[parts[1]]; m != nil {
		for i := range m.Permissions {
			if m.Permissions[i].ID == parts[3] {
				current = &m.Permissions[i]
			}
		}
	}
	if current == nil {
		http.Error(w, "this Permission is not in this Workspace", http.StatusNotFound)
		return "", false
	}
	back := "/builder/" + url.PathEscape(appID) + "?" + url.Values{"node": {record}, "tab": {"edit"}}.Encode()
	if slices.Equal(slices.Sorted(slices.Values(roles)), slices.Sorted(slices.Values(current.Roles))) {
		return back, true
	}
	return commitBuilderOps(w, req, env, appID, []builder.Op{{Kind: builder.OpSetPermissionRoles, Machine: parts[1], Permission: parts[3], Roles: roles}}, back)
}
