package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/installer"
)

// The statuses of a file's review, as the page's badge words. Only Loads (a file that loads and would change something) can be
// applied: a file nothing loads was not checked, and an unchanged one has nothing to write.
const (
	reviewLoads      = "Loads"
	reviewUnchanged  = "Unchanged"
	reviewNotChecked = "Not checked"
	reviewRejected   = "Rejected"
)

// filesRoute is the files screen's address; `?path=` names the file being edited.
func filesRoute(path string) string {
	route := domain.RuntimeScreenRoute("nav_workspace_files")
	if path == "" {
		return route
	}
	return route + "?" + url.Values{"path": {path}}.Encode()
}

// ownedFile reports whether path names one of the YAML files this Workspace owns. A path is looked up by membership in the listing
// and never joined from the request, so a crafted value reads nothing and writes nothing.
func ownedFile(ctx context.Context, store *data.Store, cfg config.Config, path string) (manifestPath string, ok bool, err error) {
	if cfg.MetadataPath == "" {
		return "", false, errors.New("this process has no metadata directory configured")
	}
	manifestPath, _, err = workspaceInstallation(ctx, store, cfg)
	if err != nil {
		return "", false, err
	}
	files, err := ownedYAMLFiles(strings.TrimSuffix(manifestPath, filepath.Ext(manifestPath)))
	if err != nil {
		return "", false, err
	}
	return manifestPath, slices.Contains(files, path), nil
}

func draftFor(ctx context.Context, userID, path string) (string, bool) {
	workspaceID, _ := data.WorkspaceScope(ctx)
	return fileDrafts.get(draftKey{workspaceID, userID, path})
}

// workspaceFilesProvider is the reader of the `workspace_files` source: the YAML files this Workspace owns, each with where to
// edit it, and -- for the one file the request names (`?path=`) -- the content an editor starts as: the viewer's own unsent draft
// when they have one, else the file's current content. Only the named file's content is read.
func workspaceFilesProvider(store *data.Store, cfg config.Config) composition.RuntimeProvider {
	return func(ctx context.Context, l *composition.Loader, viewer domain.Actor) ([]*data.Record, error) {
		if cfg.MetadataPath == "" {
			return nil, nil
		}
		manifestPath, _, err := workspaceInstallation(ctx, store, cfg)
		if err != nil {
			return nil, err
		}
		ownDir := strings.TrimSuffix(manifestPath, filepath.Ext(manifestPath))
		files, err := ownedYAMLFiles(ownDir)
		if err != nil {
			return nil, err
		}
		selected := l.SourceParam("workspace_files", "path")
		workspaceID, _ := data.WorkspaceScope(ctx)
		out := make([]*data.Record, 0, len(files))
		for _, f := range files {
			state := "Saved"
			content := ""
			if draft, has := draftFor(ctx, viewer.ID, f); has {
				state = "Draft"
				if f == selected {
					content = draft
				}
			}
			if f == selected && content == "" {
				current, err := os.ReadFile(filepath.Join(ownDir, filepath.FromSlash(f)))
				if err != nil {
					return nil, err
				}
				content = string(current)
			}
			out = append(out, &data.Record{ID: f, MachineID: "mch_workspace_file", WorkspaceID: workspaceID, Values: map[string]any{
				"fld_id": f, "fld_path": f, "fld_state": state, "fld_link": filesRoute(f), "fld_yaml": content,
			}})
		}
		return out, nil
	}
}

// filePreviews computes, once for a request, what the loader says about the viewer's draft of the file the request names. The two
// sources that show it (the verdict and the diffs) read the same answer, so the workspace is staged and loaded once and not twice.
type filePreviews struct {
	once    sync.Once
	preview installer.FilesPreview
	path    string
	ok      bool
	err     error
}

func (p *filePreviews) get(ctx context.Context, store *data.Store, cfg config.Config, l *composition.Loader, viewer domain.Actor) (installer.FilesPreview, string, bool, error) {
	p.once.Do(func() {
		p.path = l.SourceParam("file_reviews", "path")
		if p.path == "" {
			return
		}
		draft, has := draftFor(ctx, viewer.ID, p.path)
		if !has {
			return
		}
		manifestPath, owned, err := ownedFile(ctx, store, cfg, p.path)
		if err != nil || !owned {
			p.err = err
			return
		}
		p.preview, p.err = installer.PreviewFiles(manifestPath, map[string][]byte{p.path: []byte(draft)})
		p.ok = p.err == nil
	})
	return p.preview, p.path, p.ok, p.err
}

// fileReviewsProvider is the reader of the `file_reviews` source: the loader's verdict on the viewer's own draft of the file the
// request names, as one record, or none when they have no draft of it.
func fileReviewsProvider(store *data.Store, cfg config.Config, previews *filePreviews) composition.RuntimeProvider {
	return func(ctx context.Context, l *composition.Loader, viewer domain.Actor) ([]*data.Record, error) {
		preview, path, ok, err := previews.get(ctx, store, cfg, l, viewer)
		if err != nil || !ok {
			return nil, err
		}
		status, reason := reviewRejected, preview.Reason
		changed := false
		for _, f := range preview.Files {
			changed = changed || f.Status == "changed"
		}
		switch preview.Verdict {
		case installer.PreviewValid:
			status = reviewUnchanged
			if changed {
				status = reviewLoads
			}
		case installer.PreviewUnreferenced:
			status = reviewNotChecked
			reason = "The file parses, but nothing in this workspace loads it, so it was not checked: " + preview.Reason
		}
		workspaceID, _ := data.WorkspaceScope(ctx)
		return []*data.Record{{ID: path, MachineID: "mch_file_review", WorkspaceID: workspaceID, Values: map[string]any{
			"fld_id": path, "fld_path": path, "fld_status": status, "fld_reason": reason,
		}}}, nil
	}
}

// fileDiffsProvider is the reader of the `file_diffs` source: what applying the viewer's draft would change, one record per file
// the writer would touch, each carrying the unified diff the writer produced.
func fileDiffsProvider(store *data.Store, cfg config.Config, previews *filePreviews) composition.RuntimeProvider {
	return func(ctx context.Context, l *composition.Loader, viewer domain.Actor) ([]*data.Record, error) {
		preview, path, ok, err := previews.get(ctx, store, cfg, l, viewer)
		if err != nil || !ok {
			return nil, err
		}
		workspaceID, _ := data.WorkspaceScope(ctx)
		out := make([]*data.Record, 0, len(preview.Files))
		for i, f := range preview.Files {
			state := "Unchanged"
			if f.Status == "changed" {
				state = "Changed"
			}
			out = append(out, &data.Record{ID: path + "#" + strconv.Itoa(i), MachineID: "mch_file_diff", WorkspaceID: workspaceID, Values: map[string]any{
				"fld_id": path + "#" + strconv.Itoa(i), "fld_path": path, "fld_title": f.Path, "fld_state": state, "fld_diff": f.Diff,
			}})
		}
		return out, nil
	}
}

// runReviewFile keeps the pasted content as the viewer's own draft of that file; it writes nothing to the Workspace. The file
// must be one the Workspace owns (a path is looked up, never joined) and the paste is bounded like every uploaded YAML.
func runReviewFile(w http.ResponseWriter, req *http.Request, env actionEnv, record string) (string, bool) {
	ctx := req.Context()
	if noMetadataDirectory(w, env.cfg) {
		return "", false
	}
	_, owned, err := ownedFile(ctx, env.store, env.cfg, record)
	if err != nil {
		serverError(w, err)
		return "", false
	}
	if !owned {
		http.Error(w, "this workspace has no such file", http.StatusNotFound)
		return "", false
	}
	yaml := req.PostFormValue("yaml")
	if len(yaml) > maxValidateBytes {
		http.Error(w, "the pasted content is larger than the limit", http.StatusRequestEntityTooLarge)
		return "", false
	}
	workspaceID, _ := data.WorkspaceScope(ctx)
	fileDrafts.put(draftKey{workspaceID, currentActorID(req, env), record}, yaml)
	return filesRoute(record), true
}

// runApplyFile replaces the file with the viewer's draft of it: validated on a staged copy, written through a rollback set,
// load-verified (installer.ApplyFiles), after saving a snapshot, then the Workspace is reloaded. Nothing is applied that was not
// reviewed: no draft is a 409, and a file that would not load is a 422 with the loader's error and nothing changed.
func runApplyFile(w http.ResponseWriter, req *http.Request, env actionEnv, record string) (string, bool) {
	ctx := req.Context()
	if noMetadataDirectory(w, env.cfg) {
		return "", false
	}
	if env.reload == nil {
		serverError(w, errors.New("this process has no reload hook configured"))
		return "", false
	}
	manifestPath, owned, err := ownedFile(ctx, env.store, env.cfg, record)
	if err != nil {
		serverError(w, err)
		return "", false
	}
	if !owned {
		http.Error(w, "this workspace has no such file", http.StatusNotFound)
		return "", false
	}
	workspaceID, _ := data.WorkspaceScope(ctx)
	key := draftKey{workspaceID, currentActorID(req, env), record}
	draft, has := fileDrafts.get(key)
	if !has {
		http.Error(w, "there is no reviewed draft of this file to apply: review it first", http.StatusConflict)
		return "", false
	}
	snapshotBeforeWrite(env.cfg, manifestPath)
	err = installer.ApplyFiles(manifestPath, map[string][]byte{record: []byte(draft)})
	var rejected *installer.RejectedError
	switch {
	case errors.As(err, &rejected):
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return "", false
	case err != nil:
		serverError(w, err)
		return "", false
	}
	fileDrafts.drop(key)
	if err := reloadCurrentWorkspace(ctx, env.store, env.reload); err != nil {
		serverError(w, fmt.Errorf("the file was written but the live reload failed -- a process restart will pick it up: %w", err))
		return "", false
	}
	return domain.RuntimeScreenRoute("nav_home"), true
}

// runDiscardFileDraft drops the viewer's draft of a file; the editor then starts from the file's current content again.
func runDiscardFileDraft(w http.ResponseWriter, req *http.Request, env actionEnv, record string) (string, bool) {
	workspaceID, _ := data.WorkspaceScope(req.Context())
	fileDrafts.drop(draftKey{workspaceID, currentActorID(req, env), record})
	return filesRoute(record), true
}

// currentActorID is who is acting: the viewer whose draft it is.
func currentActorID(req *http.Request, env actionEnv) string {
	return currentActor(req, env.store, env.cfg).ID
}

// noMetadataDirectory answers 503 when this process was started without a metadata directory: there are no files to list or
// replace, an environment fact rather than a fault in the request.
func noMetadataDirectory(w http.ResponseWriter, cfg config.Config) bool {
	if cfg.MetadataPath != "" {
		return false
	}
	http.Error(w, "this process has no metadata directory configured", http.StatusServiceUnavailable)
	return true
}
