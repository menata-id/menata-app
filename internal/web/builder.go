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

	"menata.app/internal/aiassist"
	"menata.app/internal/builder"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/installer"
	"menata.app/internal/rendering"
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

// runEditField is the Edit tab of a Field: the record is the part the tree names ("<app>/machines/<machine>/fields/<field>"), and the form says the Field's
// new name, whether it is required, who may change it and, for a Field that offers choices, which choices. The form is compared with the Field **as the
// admin's draft leaves it** (stagedApplication), not as installed, so undoing a staged change stages the undoing. The operations are the ones a conversation
// would make (rename_field, set_required, set_editable_by, add_option/remove_option) and are staged (stageBuilderOps); Publish writes them.
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
	app, err := stagedApplication(ctx, env.store, env.cfg, appID, currentActorID(req, env))
	var rejected *installer.RejectedError
	if errors.As(err, &rejected) {
		http.Error(w, rejected.Error(), http.StatusNotFound)
		return "", false
	} else if err != nil {
		serverError(w, err)
		return "", false
	}
	machine := stagedMachine(app, parts[1])
	var current *aiassist.GeneratedField
	if machine != nil {
		for i := range machine.Fields {
			if machine.Fields[i].ID == parts[3] {
				current = &machine.Fields[i]
			}
		}
	}
	if current == nil {
		http.Error(w, "this Field is not in this Application", http.StatusNotFound)
		return "", false
	}
	// Only what differs is an operation: saving a form nobody changed stages nothing (the writer refuses an identical Application).
	var ops []builder.Op
	if name != current.Name {
		ops = append(ops, builder.Op{Kind: builder.OpRenameField, Machine: parts[1], Field: parts[3], Value: name})
	}
	if required := req.PostForm.Get("required") != ""; required != current.Required {
		ops = append(ops, builder.Op{Kind: builder.OpSetRequired, Machine: parts[1], Field: parts[3], Flag: required})
	}
	var editors []string
	for _, r := range req.PostForm["editable_by"] {
		if r = strings.TrimSpace(r); r != "" && !slices.Contains(editors, r) {
			editors = append(editors, r)
		}
	}
	if !slices.Equal(slices.Sorted(slices.Values(editors)), slices.Sorted(slices.Values(current.EditableBy))) {
		ops = append(ops, builder.Op{Kind: builder.OpSetEditableBy, Machine: parts[1], Field: parts[3], Roles: editors})
	}
	watch, watchValue, _ := strings.Cut(req.PostForm.Get("show_when"), ":")
	haveWatch, haveValue := "", ""
	if current.ShowWhen != nil {
		haveWatch, haveValue = current.ShowWhen.Field, current.ShowWhen.Equals
	}
	if watch != haveWatch || watchValue != haveValue {
		ops = append(ops, builder.Op{Kind: builder.OpSetShowWhen, Machine: parts[1], Field: parts[3], Watch: watch, Value: watchValue})
	}
	want, err := parseChoices(req.PostForm.Get("options"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return "", false
	}
	if len(current.Options) > 0 {
		haveCategory := func(o string) string {
			if machine.StatusCategories != nil && machine.StatusCategories.Field == current.ID {
				for _, v := range machine.StatusCategories.Values {
					if v.Option == o {
						return v.Category
					}
				}
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
	return stageBuilderOps(w, req, env, appID, ops, back)
}

// choice is one line of a choices box: a choice and, after a bar, the category it is drawn in ("Approved | done").
type choice struct{ Name, Category string }

// parseChoices reads a choices box: one choice a line, a blank or repeated line says nothing, and a category after the **last** "|" must be one of the closed
// five when it is the last thing on the line. A line whose tail after a bar is not a category is a choice whose name holds a bar ("a|b"), so such a
// name round-trips through the box.
func parseChoices(typed string) ([]choice, error) {
	var out []choice
	seen := map[string]bool{}
	for _, line := range strings.Split(typed, "\n") {
		name, cat := strings.TrimSpace(line), ""
		if i := strings.LastIndex(line, "|"); i >= 0 {
			tail := strings.TrimSpace(line[i+1:])
			switch {
			case domain.KnownStatusCategories[domain.StatusCategory(tail)]:
				name, cat = strings.TrimSpace(line[:i]), tail
			case strings.Contains(line, " | "):
				return nil, fmt.Errorf("%q is not a colour a status can have (start, active, attention, done or stopped)", tail)
			}
		}
		if name == "" || seen[name] {
			continue
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
	app, err := stagedApplication(ctx, env.store, env.cfg, appID, currentActorID(req, env))
	var rejected *installer.RejectedError
	if errors.As(err, &rejected) {
		http.Error(w, rejected.Error(), http.StatusNotFound)
		return "", false
	} else if err != nil {
		serverError(w, err)
		return "", false
	}
	m := stagedMachine(app, parts[1])
	if m == nil {
		http.Error(w, "this Machine is not in this Application", http.StatusNotFound)
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
	// Back to the Machine: the new Field is staged, not yet a part of the tree, so there is nothing of it to select until Publish.
	return stageBuilderOps(w, req, env, appID, []builder.Op{op}, "/builder/"+url.PathEscape(appID)+"?"+url.Values{"node": {record}, "tab": {"edit"}}.Encode())
}

// newFieldID makes a Field's id from its name ("fld_" and the name in lower case with words joined by "_") and keeps it unique within the Machine as the draft
// would leave it, ignoring case ("fld_amount", then "fld_amount_2"). A name with no letter or digit in it ("é", "!!!") is "fld_field" and counts up the same way.
func newFieldID(m *aiassist.GeneratedMachine, name string) string {
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
	taken := func(id string) bool {
		return slices.ContainsFunc(m.Fields, func(f aiassist.GeneratedField) bool { return strings.EqualFold(f.ID, id) })
	}
	id := base
	for n := 2; taken(id); n++ {
		id = base + "_" + strconv.Itoa(n)
	}
	return id
}

// commitBuilderOpsAt writes operations the way every builder write does -- the installation's version read just before (or the one a draft was made against,
// refused with 409 when the installation is no longer it), a snapshot, builder.Commit (which load-verifies and rolls back), then afterWrite, then the Workspace
// reloaded -- and answers a refusal to w. It returns where the person goes next.
func commitBuilderOpsAt(w http.ResponseWriter, req *http.Request, env actionEnv, appID string, ops []builder.Op, base string, afterWrite func() error, back string) (string, bool) {
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
	version := base
	if version == "" {
		if version, err = builder.Version(manifestPath); err != nil {
			serverError(w, err)
			return "", false
		}
	}
	snapshotBeforeWrite(env.cfg, manifestPath)
	err = builder.Commit(manifestPath, appID, ops, version, nil)
	var rejected *installer.RejectedError
	switch {
	case errors.Is(err, builder.ErrStale):
		http.Error(w, "the application changed since this draft began: discard it and make the changes again", http.StatusConflict)
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
	if afterWrite != nil {
		if err := afterWrite(); err != nil {
			serverError(w, err)
			return "", false
		}
	}
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
	app, err := stagedApplication(req.Context(), env.store, env.cfg, appID, currentActorID(req, env))
	var rejected *installer.RejectedError
	if errors.As(err, &rejected) {
		http.Error(w, rejected.Error(), http.StatusNotFound)
		return "", false
	} else if err != nil {
		serverError(w, err)
		return "", false
	}
	var current *aiassist.GeneratedPermission
	if m := stagedMachine(app, parts[1]); m != nil {
		for i := range m.Permissions {
			if m.Permissions[i].ID == parts[3] {
				current = &m.Permissions[i]
			}
		}
	}
	if current == nil {
		http.Error(w, "this Permission is not in this Application", http.StatusNotFound)
		return "", false
	}
	back := "/builder/" + url.PathEscape(appID) + "?" + url.Values{"node": {record}, "tab": {"edit"}}.Encode()
	if slices.Equal(slices.Sorted(slices.Values(roles)), slices.Sorted(slices.Values(current.Roles))) {
		return back, true
	}
	return stageBuilderOps(w, req, env, appID, []builder.Op{{Kind: builder.OpSetPermissionRoles, Machine: parts[1], Permission: parts[3], Roles: roles}}, back)
}

// runAskAssistantAbout is the Ask AI tab of any part: it opens the assistant's conversation with what the person wants, in words that name the part -- its
// kind, its place in the tree and its metadata path, and the Application's name -- so the assistant is asked about this Field and not "a field". Only
// structure goes: names and ids the builder's tree already shows, never a record's contents. It writes nothing; whatever the assistant proposes is
// reviewed and applied by the person in the conversation, through the same writer as every other change.
func runAskAssistantAbout(w http.ResponseWriter, req *http.Request, env actionEnv, record string) (string, bool) {
	wish := strings.TrimSpace(req.PostForm.Get("wish"))
	if wish == "" || len(wish) > 1000 {
		http.Error(w, "say in a few words (up to 1000 characters) what should change", http.StatusBadRequest)
		return "", false
	}
	appID, path, _ := strings.Cut(record, "/")
	appName, known := "", false
	for _, app := range rendering.CurrentWorkspace(req.Context()).Applications {
		if app.ID == appID {
			appName, known = app.Name, true
		}
	}
	// The record must be a part the tree lists: an application of this Workspace and a path of the shapes it gives its parts, so nothing else is ever put in a
	// sentence addressed to the assistant.
	if !known || path == "" || len(path) > 200 || strings.ContainsAny(path, "\"\n\r\\") || strings.Contains(path, "..") {
		http.Error(w, "this is not a part of an application of this Workspace", http.StatusBadRequest)
		return "", false
	}
	idea := "In the application \"" + appName + "\" (" + appID + "), about " + path + ": " + wish
	return domain.RuntimeScreenRoute("nav_new_application") + "?" + url.Values{"idea": {idea}}.Encode(), true
}

// runEditMenuItem is the Edit tab of a screen the menu lists: the record is "<app>/navigation/<item>", and the form says its name in the menu, its place among
// the items and whether to take it out. Compared with the menu as the admin's draft leaves it; the operations are the ones a conversation makes
// (rename_menu_item, move_menu_item, remove_menu_item) and are staged. Taking an item out wins over the rest; the plan refuses taking out one that opens
// something other than a Machine's records, for the reason it gives in a conversation.
func runEditMenuItem(w http.ResponseWriter, req *http.Request, env actionEnv, record string) (string, bool) {
	appID, rest, _ := strings.Cut(record, "/")
	item, ok := strings.CutPrefix(rest, "navigation/")
	if !ok || item == "" || strings.Contains(item, "/") || appID == "" {
		http.Error(w, "this part is not a screen of the menu", http.StatusBadRequest)
		return "", false
	}
	label := strings.TrimSpace(req.PostForm.Get("label"))
	if label == "" {
		http.Error(w, "a menu item needs a name", http.StatusBadRequest)
		return "", false
	}
	app, err := stagedApplication(req.Context(), env.store, env.cfg, appID, currentActorID(req, env))
	var rejected *installer.RejectedError
	if errors.As(err, &rejected) {
		http.Error(w, rejected.Error(), http.StatusNotFound)
		return "", false
	} else if err != nil {
		serverError(w, err)
		return "", false
	}
	at := slices.IndexFunc(app.Navigation, func(n aiassist.GeneratedMenuItem) bool { return n.ID == item })
	if at < 0 {
		http.Error(w, "this screen is not in the menu of this Application", http.StatusNotFound)
		return "", false
	}
	back := "/builder/" + url.PathEscape(appID) + "?" + url.Values{"node": {record}, "tab": {"edit"}}.Encode()
	var ops []builder.Op
	switch {
	case req.PostForm.Get("remove") != "":
		for _, app := range rendering.CurrentWorkspace(req.Context()).Applications {
			for _, other := range app.AllNavigation {
				if app.ID == appID && other.Parent == item {
					http.Error(w, "other menu items hang under this one ("+other.Label+"): move or take them out first", http.StatusUnprocessableEntity)
					return "", false
				}
			}
		}
		ops = append(ops, builder.Op{Kind: builder.OpRemoveMenuItem, Item: item})
		back = "/builder/" + url.PathEscape(appID)
	default:
		if label != app.Navigation[at].Label {
			ops = append(ops, builder.Op{Kind: builder.OpRenameMenuItem, Item: item, Value: label})
		}
		if place, err := strconv.Atoi(req.PostForm.Get("position")); err == nil && place != at+1 {
			ops = append(ops, builder.Op{Kind: builder.OpMoveMenuItem, Item: item, Position: place})
		}
		if parent := req.PostForm.Get("parent"); parent != app.Navigation[at].Parent {
			ops = append(ops, builder.Op{Kind: builder.OpSetMenuParent, Item: item, Value: parent})
		}
		var offered []string
		for _, r := range req.PostForm["visible_to"] {
			if r = strings.TrimSpace(r); r != "" && !slices.Contains(offered, r) {
				offered = append(offered, r)
			}
		}
		if !slices.Equal(slices.Sorted(slices.Values(offered)), slices.Sorted(slices.Values(app.Navigation[at].VisibleTo))) {
			ops = append(ops, builder.Op{Kind: builder.OpSetMenuVisibleTo, Item: item, Roles: offered})
		}
		if badge := req.PostForm.Get("badge"); badge != app.Navigation[at].Badge {
			ops = append(ops, builder.Op{Kind: builder.OpSetMenuBadge, Item: item, Value: badge})
		}
	}
	if len(ops) == 0 {
		return back, true
	}
	return stageBuilderOps(w, req, env, appID, ops, back)
}

// runAddMenuItem is the Edit tab of the menu itself: a new item with a name, opening the records of a Machine of the Application (add_menu_item). The Machine
// must be one the draft has.
func runAddMenuItem(w http.ResponseWriter, req *http.Request, env actionEnv, record string) (string, bool) {
	appID, rest, _ := strings.Cut(record, "/")
	if rest != "navigation" || appID == "" {
		http.Error(w, "this part is not the menu", http.StatusBadRequest)
		return "", false
	}
	label, machine := strings.TrimSpace(req.PostForm.Get("label")), req.PostForm.Get("machine")
	if label == "" || machine == "" {
		http.Error(w, "a menu item needs a name and the Machine it opens", http.StatusBadRequest)
		return "", false
	}
	back := "/builder/" + url.PathEscape(appID) + "?" + url.Values{"node": {record}, "tab": {"edit"}}.Encode()
	return stageBuilderOps(w, req, env, appID, []builder.Op{{Kind: builder.OpAddMenuItem, Machine: machine, Value: label}}, back)
}

// runEditList is the View tab of a list the assistant wrote: the record is "<app>/navigation/<item>", and the form says its columns (Field names, one a line, in
// order), how it is ordered, the status its chips count by and the text Fields its search looks in. Compared with the list as the admin's draft leaves it; the
// operations are the ones a conversation makes (set_list_columns, set_list_sort, set_list_filter, set_list_search) and are staged, then Publish writes the page and its
// Datasets again from the list. A column named by something that is not a Field of the Machine is refused in words, not guessed.
func runEditList(w http.ResponseWriter, req *http.Request, env actionEnv, record string) (string, bool) {
	appID, rest, _ := strings.Cut(record, "/")
	item, ok := strings.CutPrefix(rest, "navigation/")
	if !ok || item == "" || strings.Contains(item, "/") || appID == "" {
		http.Error(w, "this part is not a screen of the menu", http.StatusBadRequest)
		return "", false
	}
	app, err := stagedApplication(req.Context(), env.store, env.cfg, appID, currentActorID(req, env))
	var rejected *installer.RejectedError
	if errors.As(err, &rejected) {
		http.Error(w, rejected.Error(), http.StatusNotFound)
		return "", false
	} else if err != nil {
		serverError(w, err)
		return "", false
	}
	at := slices.IndexFunc(app.Navigation, func(n aiassist.GeneratedMenuItem) bool { return n.ID == item })
	if at < 0 || app.Navigation[at].List == nil {
		http.Error(w, "this screen is not a list this editor can change", http.StatusNotFound)
		return "", false
	}
	list := app.Navigation[at].List
	machine := stagedMachine(app, app.Navigation[at].MachineID)
	if machine == nil {
		http.Error(w, "this list's Machine is not in this Application", http.StatusNotFound)
		return "", false
	}
	idOf := func(name string) (string, bool) {
		for _, f := range machine.Fields {
			if strings.EqualFold(strings.TrimSpace(name), f.Name) || name == f.ID {
				return f.ID, true
			}
		}
		return "", false
	}
	var columns []string
	for _, line := range strings.Split(req.PostForm.Get("columns"), "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		id, found := idOf(line)
		if !found {
			http.Error(w, fmt.Sprintf("%q is not a field of %s", line, machine.Name), http.StatusUnprocessableEntity)
			return "", false
		}
		if !slices.Contains(columns, id) {
			columns = append(columns, id)
		}
	}
	if len(columns) == 0 {
		http.Error(w, "a list needs at least one column", http.StatusBadRequest)
		return "", false
	}
	back := "/builder/" + url.PathEscape(appID) + "?" + url.Values{"node": {record}, "tab": {"view"}}.Encode()
	var ops []builder.Op
	if !slices.Equal(columns, list.Columns) {
		ops = append(ops, builder.Op{Kind: builder.OpSetListColumns, Item: item, Fields: columns})
	}
	sortField, dir := req.PostForm.Get("sort"), req.PostForm.Get("direction")
	haveSort, haveDir := list.SortField, list.Direction
	if haveSort == "" {
		haveSort = "created_at"
	}
	if haveDir == "" {
		haveDir = "desc"
	}
	if sortField == "" {
		sortField = haveSort
	}
	if dir == "" {
		dir = haveDir
	}
	if sortField != haveSort || dir != haveDir {
		ops = append(ops, builder.Op{Kind: builder.OpSetListSort, Item: item, Field: sortField, Value: dir})
	}
	if filter := req.PostForm.Get("filter"); filter != list.FilterField {
		ops = append(ops, builder.Op{Kind: builder.OpSetListFilter, Item: item, Field: filter})
	}
	var search []string
	for _, id := range req.PostForm["search"] {
		if id = strings.TrimSpace(id); id != "" && !slices.Contains(search, id) {
			search = append(search, id)
		}
	}
	if !slices.Equal(search, list.SearchFields) {
		ops = append(ops, builder.Op{Kind: builder.OpSetListSearch, Item: item, Fields: search})
	}
	if len(ops) == 0 {
		return back, true
	}
	return stageBuilderOps(w, req, env, appID, ops, back)
}

// runEditWorkflow is the Edit tab of a Machine's Workflow part: the record is "<app>/machines/<machine>/workflow", and the form adds a status to the lifecycle
// (with what it means) and/or a move between two statuses (with who may take it) -- the add_option and add_transition operations a conversation makes. A status or a move
// already there cannot be changed or taken away (the policy of an update), so this form only adds. A move may name the status being added in the same form
// ("+new"). The loader's own rule stays: a status a move reaches must have a meaning, which is why a new status carries one.
func runEditWorkflow(w http.ResponseWriter, req *http.Request, env actionEnv, record string) (string, bool) {
	appID, rest, _ := strings.Cut(record, "/")
	parts := strings.Split(rest, "/")
	if len(parts) != 3 || parts[0] != "machines" || parts[2] != "workflow" || appID == "" {
		http.Error(w, "this part is not a workflow", http.StatusBadRequest)
		return "", false
	}
	app, err := stagedApplication(req.Context(), env.store, env.cfg, appID, currentActorID(req, env))
	var rejected *installer.RejectedError
	if errors.As(err, &rejected) {
		http.Error(w, rejected.Error(), http.StatusNotFound)
		return "", false
	} else if err != nil {
		serverError(w, err)
		return "", false
	}
	machine := stagedMachine(app, parts[1])
	if machine == nil {
		http.Error(w, "this Machine is not in this Application", http.StatusNotFound)
		return "", false
	}
	var field *aiassist.GeneratedField
	for i := range machine.Fields {
		if machine.Fields[i].Type == "status" && (len(machine.Transitions) == 0 || machine.Transitions[0].Field == machine.Fields[i].ID) {
			field = &machine.Fields[i]
			break
		}
	}
	if field == nil {
		http.Error(w, "this Machine has no status to build a workflow on", http.StatusUnprocessableEntity)
		return "", false
	}
	status, step := strings.TrimSpace(req.PostForm.Get("status")), strings.TrimSpace(req.PostForm.Get("step"))
	if status == "" && step == "" {
		http.Error(w, "name a new status, a new move, or both", http.StatusBadRequest)
		return "", false
	}
	var ops []builder.Op
	if status != "" {
		meaning := req.PostForm.Get("meaning")
		if meaning == "" {
			meaning = "active"
		}
		if !domain.KnownStatusCategories[domain.StatusCategory(meaning)] {
			http.Error(w, fmt.Sprintf("%q is not a meaning a status can have", meaning), http.StatusBadRequest)
			return "", false
		}
		ops = append(ops, builder.Op{Kind: builder.OpAddOption, Machine: machine.ID, Field: field.ID, Value: status, Category: meaning})
	}
	if step != "" {
		resolve := func(v string) string {
			if v == composition.NewStatusChoice {
				return status
			}
			return v
		}
		from, to := resolve(req.PostForm.Get("from")), resolve(req.PostForm.Get("to"))
		if from == "" || to == "" {
			http.Error(w, "a move needs a status to start from and one to go to (the new status needs its name above)", http.StatusBadRequest)
			return "", false
		}
		var roles []string
		for _, r := range req.PostForm["roles"] {
			if r = strings.TrimSpace(r); r != "" && !slices.Contains(roles, r) {
				roles = append(roles, r)
			}
		}
		ops = append(ops, builder.Op{Kind: builder.OpAddTransition, Machine: machine.ID, Field: field.ID, Value: step, From: from, To: to, Roles: roles})
	}
	back := "/builder/" + url.PathEscape(appID) + "?" + url.Values{"node": {record}, "tab": {"edit"}}.Encode()
	return stageBuilderOps(w, req, env, appID, ops, back)
}
