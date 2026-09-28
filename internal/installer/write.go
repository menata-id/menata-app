package installer

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// WriteSet remembers what a write touched so a failure can leave the tree exactly as it found it:
// which paths it created (remove them) and what the ones it edited held before (put it back).
//
// Lived in internal/aiassist until 2026-09-28, when installing a *template* needed the identical
// discipline -- see this package's own doc comment for why it moved rather than being copied.
//
// Note must be called for every path before it is written. A path noted twice keeps its *first*
// recorded state, which is what makes a rollback correct when one file is edited more than once
// in a single change -- the manifest, which gains a line per Machine plus one per Application.
type WriteSet struct {
	created  []string
	original map[string][]byte
}

// NewWriteSet returns an empty WriteSet, ready to Note paths into.
func NewWriteSet() *WriteSet {
	return &WriteSet{original: map[string][]byte{}}
}

func (w *WriteSet) Note(path string) error {
	if _, seen := w.original[path]; seen {
		return nil
	}
	for _, p := range w.created {
		if p == path {
			return nil
		}
	}
	src, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			w.created = append(w.created, path)
			return nil
		}
		return fmt.Errorf("read %s before writing it: %w", path, err)
	}
	w.original[path] = src
	return nil
}

// Rollback is best-effort by necessity -- it runs while another error is already being returned,
// so there is nothing useful to do with a second one but say so. Each individual restore is still
// atomic (WriteFileStrict's own temp-then-rename), so a failure here leaves whichever files it
// did reach correctly restored rather than half-written.
func (w *WriteSet) Rollback() {
	for path, src := range w.original {
		if err := os.WriteFile(path, src, 0o644); err != nil {
			log.Printf("installer: rolling back %s: %v", path, err)
		}
	}
	for _, path := range w.created {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			log.Printf("installer: rolling back (removing) %s: %v", path, err)
		}
	}
}

// RefuseIfExists is the "refuse rather than corrupt" guard both write paths share, and the one thing
// standing between a colliding id and a destroyed file.
//
// A generated Machine's filename is derived from its id alone (mch_document -> document.yaml) into
// one flat metadata/ directory -- there is no per-Workspace or per-Application subdirectory, so
// "different Workspace" and "different Application" do not make a different path. On 2026-09-27 a
// generated Application for the empty "Dokter Kecil" Workspace named its own Machine mch_document
// and this function did not exist: WriteFileStrict renamed straight over metadata/document.yaml,
// the real Document Approval Machine installed in the "default" Workspace, replacing 266 lines of
// Permissions, Transitions, Events, datasets and views with the 69-line generated one. Nothing
// caught it before the write, because Validate is handed only the *current* Workspace's machine
// ids (internal/web.existingStateFor, now widened) and mch_document genuinely was not one of them.
//
// So this check is deliberately not "is this id in some list the caller gave me" -- it is the file
// system itself, asked at the last possible moment. Validate's own widened collision check is the
// friendly half (it fails the conversation early, with something the assistant can act on); this is
// the half that holds even when a future caller builds ExistingState wrong.
//
// Only the paths that *create* files go through here -- a generated Application, and a template
// install. A surgical edit to a file that is supposed to already exist calls WriteFileStrict
// directly (internal/aiassist's own extend path).
func RefuseIfExists(path, what string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("refusing to write %s: %s already exists and this change would overwrite it -- generated metadata may only ever create new files", what, path)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check %s before writing %s: %w", path, what, err)
	}
	return nil
}

// AppendBlockListItem finds "key:" at any indentation, then the last consecutive "- item" line
// directly under it (matching that block's own indentation), and inserts a new line with the same
// indentation and list-marker style right after it. Refuses (clear error, no partial edit) rather
// than guess if the key is missing or is not declared in block style -- exactly the "refuse rather
// than corrupt" posture this package's own doc comment promises.
func AppendBlockListItem(src []byte, key, newItem string) ([]byte, error) {
	lines := strings.Split(string(src), "\n")
	keyLine := -1
	var keyIndent string
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		if trimmed == key || strings.HasPrefix(trimmed, key+" ") {
			keyLine = i
			keyIndent = line[:len(line)-len(trimmed)]
			break
		}
	}
	if keyLine == -1 {
		return nil, fmt.Errorf("no %q key found", key)
	}

	// "key: []" (or "key: [ ]") is a real, common, documented shape -- CLAUDE.md's own words: "An
	// empty applications: [] is valid and normal... what a Workspace looks like the moment it is
	// created". Rewriting that one line into block form ("key:" plus one indented "- item" line)
	// is still a single-line replacement, not a rewrite of anything else in the file.
	emptyFlow := regexp.MustCompile(`^\[\s*\]\s*$`)
	if rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimLeft(lines[keyLine], " "), key)); emptyFlow.MatchString(rest) {
		lines[keyLine] = keyIndent + key
		itemIndent := keyIndent + "  "
		out := make([]string, 0, len(lines)+1)
		out = append(out, lines[:keyLine+1]...)
		out = append(out, itemIndent+"- "+newItem)
		out = append(out, lines[keyLine+1:]...)
		return []byte(strings.Join(out, "\n")), nil
	}

	itemPattern := regexp.MustCompile(`^(\s*)-\s`)
	lastItem := -1
	var itemIndent string
	for i := keyLine + 1; i < len(lines); i++ {
		m := itemPattern.FindStringSubmatch(lines[i])
		if m == nil {
			if strings.TrimSpace(lines[i]) == "" || strings.HasPrefix(strings.TrimSpace(lines[i]), "#") {
				continue // blank line or comment inside the block -- keep scanning
			}
			break
		}
		if lastItem == -1 {
			itemIndent = m[1]
		}
		lastItem = i
	}
	if lastItem == -1 {
		return nil, fmt.Errorf("%q has no block-style (\"- item\") entries to append after -- refusing rather than guessing a shape", key)
	}
	_ = keyIndent

	newLine := itemIndent + "- " + newItem
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:lastItem+1]...)
	out = append(out, newLine)
	out = append(out, lines[lastItem+1:]...)
	return []byte(strings.Join(out, "\n")), nil
}

// WriteYAMLStrict marshals v (of concrete type T) and writes it through WriteFileStrict[T], so the
// strict re-parse below decodes into the exact same shape it was just encoded from -- genuinely
// proving round-trip fidelity, not merely that the bytes parse as *some* YAML.
func WriteYAMLStrict[T any](path string, v T) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	return WriteFileStrict[T](path, buf.Bytes())
}

// WorkspaceManifestCheckDoc is WriteFileStrict's own strict-decode target for the workspace
// manifest specifically -- every key internal/metadata's real workspaceDoc declares, so
// KnownFields(true) only ever rejects a genuinely unrecognized key, never one of the manifest's
// own four.
type WorkspaceManifestCheckDoc struct {
	Workspace    string   `yaml:"workspace"`
	Machines     []string `yaml:"machines"`
	Navigation   []any    `yaml:"navigation"`
	Applications []string `yaml:"applications"`
	// SuggestedApplications was the *second* instance of the drift TestCheckDocsMirrorMetadatasOwnKeys
	// now gates, found by that gate the moment it was written: the key landed on metadata's own
	// workspaceDoc on 2026-09-27, and metadata/workspaces/default.yaml declares it -- so any install or
	// generated publish into that Workspace would have failed its own strict re-parse and rolled back,
	// with an error about a key the loader is perfectly happy with.
	SuggestedApplications []any `yaml:"suggested_applications"`
}

// FullMachineCheckDoc/FullApplicationCheckDoc mirror internal/metadata's own real machineDoc/
// applicationDoc *completely* (every field parse.go/application.go declare, not just the narrower
// generator-only subset above) -- the strict-decode target for a surgical edit to an *existing*
// file, which may carry Constraints/Datasets/Views/CardFields/Sequencing/AppendOnly/ShowNav/
// SummaryMachine/Navigation that this package's own generator never writes but must not corrupt or
// reject as "unknown". Using the narrower machineDoc/applicationDoc above for this check would
// reject any real file that uses one of those fields, which is not this check's job -- it exists
// only to catch a genuinely hallucinated key the surgical text edit might have introduced, never to
// second-guess a shape internal/metadata's own Validate already accepts.
type FullMachineCheckDoc struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name"`
	Fields      []any  `yaml:"fields"`
	Constraints []any  `yaml:"constraints"`
	Events      []any  `yaml:"events"`
	Permissions []any  `yaml:"permissions"`
	Transitions []any  `yaml:"transitions"`
	Datasets    []any  `yaml:"datasets"`
	Sequencing  any    `yaml:"sequencing"`
	// Actions is Stage B's own block (domain.ActionEffect, 2026-09-28) -- added here in the same change
	// that introduced it, because TestCheckDocsMirrorMetadatasOwnKeys failed the moment it did not
	// exist. That is the gate working: without it, installing Document Approval would have failed its
	// own strict re-parse on a key the loader accepts.
	Actions []any `yaml:"actions"`
	// MemberRemovalBlocks was missing until 2026-09-28, and the first template install found it: the
	// key landed on metadata's own machineDoc on 2026-09-27 (mch_approval_step's blk_step_pending) and
	// this mirror was not updated, so copying that Machine verbatim failed its own strict re-parse.
	// TestCheckDocsMirrorMetadatasOwnKeys is what turns "two lists that drift" into a build failure.
	MemberRemovalBlocks []any `yaml:"blocks_member_removal"`
	// SignaturePlacement/SignatureStore are Stage D's own blocks (domain.SignaturePlacement,
	// 2026-09-28), added here in the same change for the same reason Actions above was.
	SignaturePlacement any    `yaml:"signature_placement"`
	SignatureStore     any    `yaml:"signature_store"`
	SLAField           string `yaml:"sla_field"`
	CardFields         []any  `yaml:"card_fields"`
	Views              []any  `yaml:"views"`
	AppendOnly         bool   `yaml:"append_only"`
}

type FullApplicationCheckDoc struct {
	ID             string   `yaml:"id"`
	Name           string   `yaml:"name"`
	Machines       []string `yaml:"machines"`
	Roles          []string `yaml:"roles"`
	Description    string   `yaml:"description"`
	Icon           string   `yaml:"icon"`
	Color          string   `yaml:"color"`
	SummaryMachine string   `yaml:"summary_machine"`
	ShowNav        *bool    `yaml:"show_nav"`
	Navigation     []any    `yaml:"navigation"`
	// Workflow -- the binding an Application declares (domain.Workflow, 2026-09-28).
	Workflow any `yaml:"workflow"`
}

// WriteFileStrict re-parses data into a fresh T with strict, unknown-key-rejecting decoding before
// writing it anywhere: it catches a hallucinated or mistyped key in bytes this package itself produced,
// *before* the file is renamed into place. Writes to a temp file in the same directory and renames,
// so a crash mid-write can never leave a half-written file for the next reload to trip over.
//
// This was the only strict decoding in the repo until 2026-09-28, when internal/metadata's own loader
// became strict too (decodeStrict, ROADMAP.md's "Reject unknown metadata keys at load"). It is not
// redundant now -- it fails earlier, on this package's own output, rather than on the reload that
// follows -- and TestCheckDocsMirrorMetadatasOwnKeys keeps the mirror types below in step with the
// loader's own, which is the drift that broke the first template install.
func WriteFileStrict[T any](path string, data []byte) error {
	var check T
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&check); err != nil {
		return fmt.Errorf("internal error: freshly-written metadata does not parse strictly: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".installer-*.yaml.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	w := bufio.NewWriter(tmp)
	if _, err := w.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := w.Flush(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}
