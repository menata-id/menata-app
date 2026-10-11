package web

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/installer"
	"menata.app/internal/rendering"
)

// showWorkspaceFiles is the screen's GET: the Workspace's own files as a picker, or -- with ?path= naming one of
// them -- that file's current content in the editor. A path is looked up by membership in the listing and never
// joined from the query string, so a crafted value reads nothing.
func showWorkspaceFiles(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if noMetadataDirectory(w, cfg) {
			return
		}
		manifestPath, _, err := workspaceInstallation(req.Context(), store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		view, err := workspaceFilesView(manifestPath, req.URL.Query().Get("path"), "")
		if err != nil {
			serverError(w, err)
			return
		}
		renderWorkspaceFiles(w, req, store, cfg, view)
	}
}

// reviewWorkspaceFiles is the screen's POST: the same dry run as /workspace-settings/validate, answered as a page
// with the loader's verdict and the diff against the live file (installer.PreviewFiles). It writes nothing; the
// Apply button on the page posts to /workspace-settings/apply, which re-validates before writing.
func reviewWorkspaceFiles(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if noMetadataDirectory(w, cfg) {
			return
		}
		req.Body = http.MaxBytesReader(w, req.Body, maxValidateBytes)
		if err := req.ParseForm(); err != nil {
			http.Error(w, "the upload is missing or larger than the limit", http.StatusRequestEntityTooLarge)
			return
		}
		manifestPath, _, err := workspaceInstallation(req.Context(), store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		path, body := strings.TrimSpace(req.PostFormValue("path")), req.PostFormValue("yaml")
		view, err := workspaceFilesView(manifestPath, path, body)
		if err != nil {
			serverError(w, err)
			return
		}
		preview, err := installer.PreviewFiles(manifestPath, map[string][]byte{path: []byte(body)})
		if err != nil {
			serverError(w, err)
			return
		}
		view.Review = fileReview(preview)
		renderWorkspaceFiles(w, req, store, cfg, view)
	}
}

// noMetadataDirectory answers 503 when this process was started without a metadata directory: there are no files
// to list, an environment fact rather than a fault in the request.
func noMetadataDirectory(w http.ResponseWriter, cfg config.Config) bool {
	if cfg.MetadataPath != "" {
		return false
	}
	http.Error(w, "this process has no metadata directory configured", http.StatusServiceUnavailable)
	return true
}

func renderWorkspaceFiles(w http.ResponseWriter, req *http.Request, store *data.Store, cfg config.Config, view rendering.WorkspaceFilesView) {
	ctx := req.Context()
	chrome, err := resolveChrome(ctx, req, store, cfg)
	if err != nil {
		serverError(w, err)
		return
	}
	userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
	_, switchHref := viewerWorkspaceContext(ctx, store, userID)
	render(ctx, w, rendering.WorkspaceFilesPage(view, chrome.WorkspaceName, chrome.Viewer(), switchHref))
}

// workspaceFilesView resolves the picker and, when path names a file the Workspace owns, the editor. yaml is the
// text to show in the editor; when empty the file's current content is shown instead. A path that names no owned
// file shows the picker, never an error: the listing is the only authority on what exists.
func workspaceFilesView(manifestPath, path, yaml string) (rendering.WorkspaceFilesView, error) {
	ownDir := strings.TrimSuffix(manifestPath, filepath.Ext(manifestPath))
	files, err := ownedYAMLFiles(ownDir)
	if err != nil {
		return rendering.WorkspaceFilesView{}, err
	}
	view := rendering.WorkspaceFilesView{Files: files}
	if !slices.Contains(files, path) {
		return view, nil
	}
	view.Path, view.YAML = path, yaml
	if yaml == "" {
		current, err := os.ReadFile(filepath.Join(ownDir, filepath.FromSlash(path)))
		if err != nil {
			return rendering.WorkspaceFilesView{}, err
		}
		view.YAML = string(current)
	}
	return view, nil
}

// ownedYAMLFiles lists, as slash paths relative to the Workspace's own directory, every .yaml file in it -- the
// set installer.ApplyFiles will agree to replace. A Workspace that has no directory yet owns none.
func ownedYAMLFiles(ownDir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(ownDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() && strings.HasSuffix(d.Name(), ".yaml") {
			rel, relErr := filepath.Rel(ownDir, p)
			if relErr != nil {
				return relErr
			}
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	if os.IsNotExist(err) {
		return nil, nil
	}
	slices.Sort(out)
	return out, err
}

// fileReview turns the installer's verdict into what the page shows. Only "valid" with something changed can be
// applied: a file nothing loads was not checked, and an unchanged one has nothing to write.
func fileReview(p installer.FilesPreview) *rendering.FileReview {
	r := &rendering.FileReview{Reason: p.Reason}
	switch p.Verdict {
	case installer.PreviewValid:
		r.VerdictLabel, r.Tone = "Loads", domain.ToneGood
	case installer.PreviewUnreferenced:
		r.VerdictLabel, r.Tone = "Not checked", domain.ToneWarn
		r.Reason = "The file parses, but nothing in this workspace loads it, so it was not checked: " + p.Reason
	default:
		r.VerdictLabel, r.Tone = "Rejected", domain.ToneBad
	}
	changed := false
	for _, f := range p.Files {
		r.Diffs = append(r.Diffs, rendering.FileDiffView{Path: f.Path, Changed: f.Status == "changed", Diff: f.Diff})
		changed = changed || f.Status == "changed"
	}
	r.CanApply = p.Verdict == installer.PreviewValid && changed
	return r
}
