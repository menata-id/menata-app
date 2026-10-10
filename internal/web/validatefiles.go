package web

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/installer"
)

// maxValidateBytes bounds one uploaded YAML document; a Machine or Application file is a few kilobytes.
const maxValidateBytes = 1 << 20

// submitValidateFiles is the workspace-admin dry run for a hand-written YAML file (D2, option G's emergency door):
// `path` is where the file would live inside this Workspace's own directory and `yaml` its content. The answer is
// the loader's own -- 200 "valid" when the Workspace would still load with it, 422 with the loader's error when it
// would not -- and nothing live is written, reloaded or snapshotted (installer.ValidateFiles works on a staged
// copy). Plain text on purpose: the screen that asks is an Experience-Plane change, and the answer is a sentence.
func submitValidateFiles(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
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
		unreferenced, err := installer.ValidateFiles(manifestPath, map[string][]byte{strings.TrimSpace(req.PostFormValue("path")): []byte(req.PostFormValue("yaml"))})
		var rejected *installer.RejectedError
		switch {
		case errors.As(err, &rejected):
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		case err != nil:
			serverError(w, err)
		case len(unreferenced) > 0:
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			fmt.Fprintf(w, "the file parses, but nothing in this workspace loads it, so it was not checked: %s\n", strings.Join(unreferenced, ", "))
		default:
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			fmt.Fprintln(w, "valid")
		}
	}
}
