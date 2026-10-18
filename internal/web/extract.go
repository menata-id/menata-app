package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/aiassist"
	"menata.app/internal/authorization"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/execution"
	"menata.app/internal/mail"
	"menata.app/internal/rendering"
	"menata.app/internal/storage"
)

// A page's Form that declares `extract:` is filled from a document in two requests of its own page, and checked by a person
// in between: POST /pages/<nav>/extract sends the document, has it read and stores what was proposed; the page then opens with
// ?proposal=<id> and draws the form filled from it; POST /pages/<nav>/extract/save submits that form, which writes the record
// and its proposed rows as one. Both routes belong to the page: who may use them is who may open it, and what they may write is
// what the Machines' own create rules allow.

// extractTimeout bounds one reading: the model is asked for a few thousand tokens over a document.
const extractTimeout = 100 * time.Second

// readableDocuments are what a reading accepts, by what the bytes are (never by the name the client gave).
var readableDocuments = map[string]bool{"application/pdf": true, "image/png": true, "image/jpeg": true, "image/webp": true}

// declaredExtract finds, in the Application the request is in, the Form of page navID that declares `extract:` and the
// Machine its Dataset creates. ok is false for a page that has none -- the routes are not there for it.
func declaredExtract(req *http.Request, store *data.Store, navID string) (x domain.FormExtract, source *domain.Machine, loader *composition.Loader, ok bool) {
	ctx := req.Context()
	app, found := rendering.CurrentApplication(ctx)
	if !found {
		return x, nil, nil, false
	}
	for _, item := range app.AllNavigation {
		if item.ID != navID || item.Page == nil {
			continue
		}
		node := findExtractForm(*item.Page)
		if node == nil || node.Binding == nil {
			return x, nil, nil, false
		}
		loader = composition.NewLoader(store, machinesFor(ctx))
		ds, has := loader.Dataset(node.Binding.Dataset)
		if !has {
			return x, nil, nil, false
		}
		if source = loader.Machine(ds.Source); source == nil {
			return x, nil, nil, false
		}
		return *node.Extract, source, loader, true
	}
	return x, nil, nil, false
}

// findExtractForm is the Form of a page that declares `extract:`; the loader allows one.
func findExtractForm(n domain.PageNode) *domain.PageNode {
	if n.Extract != nil {
		return &n
	}
	for _, c := range n.Children {
		if f := findExtractForm(c); f != nil {
			return f
		}
	}
	return nil
}

// extractDocument reads the document a person sent for a page's form and stores what it proposed.
func extractDocument(store *data.Store, cfg config.Config, files *storage.Store, reader aiassist.Extractor) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		navID := chi.URLParam(req, "navID")
		x, source, loader, ok := declaredExtract(req, store, navID)
		if !ok || !aiassist.Available(reader) {
			http.NotFound(w, req)
			return
		}
		actor := currentActor(req, store, cfg)
		if !authorization.AllowsAction(source, domain.ActionCreate, nil, actor) {
			http.Error(w, "you may not create "+source.Name, http.StatusForbidden)
			return
		}
		content, filename, mime, ok := sentDocument(w, req, cfg)
		if !ok {
			return
		}
		workspaceID, _ := data.WorkspaceScope(ctx)
		if n, err := store.CountExtractions(ctx, workspaceID, time.Now().Add(-time.Hour)); err != nil {
			serverError(w, err)
			return
		} else if n >= cfg.ReadingsPerHour() {
			w.Header().Set("Retry-After", "600")
			http.Error(w, "too many documents have been read this hour -- fill the form by hand, or try again later", http.StatusTooManyRequests)
			return
		}
		key, err := files.Save(source.ID, x.File, filename, bytes.NewReader(content))
		if err != nil {
			serverError(w, err)
			return
		}
		attempt, err := store.StartExtraction(ctx, data.Extraction{WorkspaceID: workspaceID, UserID: actor.ID, NavID: navID, MachineID: source.ID, FileKey: key, FileName: filename})
		if err != nil {
			_ = files.Remove(key)
			serverError(w, err)
			return
		}
		readCtx, cancel := context.WithTimeout(ctx, extractTimeout)
		defer cancel()
		started := time.Now()
		proposal, err := composition.ExtractProposal(readCtx, loader, reader, x, source, content, mime)
		if err != nil {
			_ = files.Remove(key)
			_ = store.FailExtraction(ctx, workspaceID, attempt.ID)
			log.Printf("AI-EXTRACT-FAIL nav=%s machine=%s bytes=%d after=%s: %q", navID, source.ID, len(content), time.Since(started).Round(time.Millisecond), err.Error())
			http.Error(w, "the document could not be read -- try again, or fill the form by hand", http.StatusBadGateway)
			return
		}
		raw, err := json.Marshal(proposal)
		if err == nil {
			err = store.FinishExtraction(ctx, workspaceID, attempt.ID, raw)
		}
		if err != nil {
			_ = files.Remove(key)
			_ = store.FailExtraction(ctx, workspaceID, attempt.ID)
			serverError(w, err)
			return
		}
		saved := attempt
		log.Printf("AI-EXTRACT nav=%s machine=%s extraction=%s bytes=%d values=%d rows=%d took=%s", navID, source.ID, saved.ID, len(content),
			len(proposal.Values), proposedRowCount(proposal), time.Since(started).Round(time.Millisecond))
		w.Header().Set("HX-Redirect", "/pages/"+navID+"?"+url.Values{domain.ExtractParam: {saved.ID}}.Encode())
	}
}

// sentDocument reads the document a request carries and holds it to what a reading accepts (by its bytes, never its name). It
// answers the request itself when there is none, it cannot be read or it is not a kind a reader takes.
func sentDocument(w http.ResponseWriter, req *http.Request, cfg config.Config) (content []byte, filename, mime string, ok bool) {
	if !parseRecordForm(w, req, cfg.MaxUploadBytes()) {
		return nil, "", "", false
	}
	file, header, err := req.FormFile(domain.ExtractFileParam)
	if err != nil {
		http.Error(w, "send the document to be read", http.StatusBadRequest)
		return nil, "", "", false
	}
	defer func() { _ = file.Close() }()
	if content, err = io.ReadAll(file); err != nil {
		http.Error(w, "the document could not be read", http.StatusBadRequest)
		return nil, "", "", false
	}
	mime = http.DetectContentType(content)
	if !readableDocuments[mime] {
		http.Error(w, "a PDF or an image (PNG, JPEG, WebP) can be read; this file is "+mime, http.StatusUnsupportedMediaType)
		return nil, "", "", false
	}
	return content, header.Filename, mime, true
}

func proposedRowCount(p domain.ExtractionProposal) int {
	n := 0
	for _, kind := range p.Rows {
		n += len(kind)
	}
	return n
}

// saveExtracted writes what a person checked: the record the form describes and the proposed rows they kept, all of them or
// none, then the Events each create calls for. The document is the one the proposal was read from, which the proposal names,
// never one the request does.
func saveExtracted(store *data.Store, files *storage.Store, mailer mail.Mailer, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		navID := chi.URLParam(req, "navID")
		x, source, loader, ok := declaredExtract(req, store, navID)
		if !ok {
			http.NotFound(w, req)
			return
		}
		if !parseRecordForm(w, req, cfg.MaxUploadBytes()) {
			return
		}
		actor := currentActor(req, store, cfg)
		workspaceID, _ := data.WorkspaceScope(ctx)
		proposal, err := store.GetExtraction(ctx, workspaceID, req.Form.Get(domain.ExtractParam))
		if err != nil || proposal.UserID != actor.ID || proposal.NavID != navID || proposal.MachineID != source.ID {
			http.Error(w, "this proposal is not yours to save", http.StatusNotFound)
			return
		}
		if proposal.Status != data.ExtractionProposed {
			http.Error(w, "this proposal was already saved", http.StatusConflict)
			return
		}
		values := data.ValuesFromForm(source, req.Form)
		// Only what the form asks is read from the request, as the rows are: the document's own Field is set below, from the proposal.
		inputs, _ := source.CreateFormInputs()
		for id := range values {
			if !slices.ContainsFunc(inputs, func(in domain.FormInput) bool { return in.FieldID == id }) {
				delete(values, id)
			}
		}
		values[x.File] = proposal.FileKey
		rows, err := submittedRows(x, loader, req.Form)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		parent, written, err := writeProposed(ctx, w, req, store, cfg, source, values, rows, workspaceID, proposal.ID)
		switch {
		case err == nil:
		case errors.Is(err, errRefusedWrite):
			return
		default:
			serverError(w, err)
			return
		}
		runGeneratedEvents(ctx, execution.Services{Store: store, Mailer: mailer, Files: files}, machinesFor(ctx), written, actor.ID)
		w.Header().Set("HX-Redirect", recordPageRoute(source, parent.ID))
	}
}

// writeProposed writes the record and the rows a person kept in one transaction, and marks the proposal saved in it. A refusal
// has already answered the request (errRefusedWrite) and rolled everything back, the numbering counters included.
func writeProposed(ctx context.Context, w http.ResponseWriter, req *http.Request, store *data.Store, cfg config.Config, source *domain.Machine,
	values map[string]any, rows []submittedRow, workspaceID, proposalID string) (parent *data.Record, written []generatedRecord, err error) {
	err = store.InTx(ctx, func(ctx context.Context) error {
		inner := req.WithContext(ctx)
		rec, _, ok := writeFromValues(w, inner, store, cfg, source, values)
		if !ok {
			return errRefusedWrite
		}
		parent = rec
		written = append(written, generatedRecord{machine: source, record: rec})
		for _, row := range rows {
			row.values[row.via] = rec.ID
			child, _, ok := writeFromValues(w, inner, store, cfg, row.machine, row.values)
			if !ok {
				return errRefusedWrite
			}
			written = append(written, generatedRecord{machine: row.machine, record: child})
		}
		first, err := store.MarkExtractionSaved(ctx, workspaceID, proposalID, rec.ID)
		if err != nil {
			return err
		}
		if !first {
			http.Error(w, "this proposal was already saved", http.StatusConflict)
			return errRefusedWrite
		}
		return nil
	})
	return parent, written, err
}

// submittedRow is one proposed row a person kept, its values from the controls the page drew for it.
type submittedRow struct {
	machine *domain.Machine
	via     string
	values  map[string]any
}

// submittedRows reads the rows a form carries -- rows.<kind>.<n>.<field> -- for the kinds the page declares and the Fields it
// declares for each: a name outside the declaration is not read, whatever the request holds. A row whose include tick is
// absent was left out.
func submittedRows(x domain.FormExtract, l *composition.Loader, form url.Values) ([]submittedRow, error) {
	var out []submittedRow
	for k, r := range x.Rows {
		child := l.Machine(r.Machine)
		if child == nil {
			return nil, fmt.Errorf("%s is not installed", r.Machine)
		}
		prefix := domain.ExtractRowPrefix + strconv.Itoa(k) + "."
		var indexes []int
		for name := range form {
			if rest, ok := strings.CutPrefix(name, prefix); ok {
				if n, _, found := strings.Cut(rest, "."); found {
					if i, err := strconv.Atoi(n); err == nil && i >= 0 && i < 500 && !slices.Contains(indexes, i) {
						indexes = append(indexes, i)
					}
				}
			}
		}
		slices.Sort(indexes)
		for _, i := range indexes {
			rowPrefix := prefix + strconv.Itoa(i) + "."
			if !form.Has(rowPrefix + domain.ExtractRowInclude) {
				continue
			}
			sub := url.Values{}
			for _, f := range r.Fields {
				if v, ok := form[rowPrefix+f]; ok {
					sub[f] = v
				}
			}
			values := data.ValuesFromForm(child, sub)
			for id := range values {
				if !slices.Contains(r.Fields, id) {
					delete(values, id)
				}
			}
			out = append(out, submittedRow{machine: child, via: r.Via, values: values})
		}
	}
	return out, nil
}
