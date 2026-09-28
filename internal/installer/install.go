package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"menata.app/internal/metadata"
)

// Install copies a planned template into the target Workspace's own directory and adds it to that
// Workspace's manifest. Returns the installed Application's id -- the template's own, or its rename --
// so the caller can reload and redirect to it.
//
// libraryDir is the template library root (metadata/); workspaceManifestPath is
// metadata/workspaces/<slug>.yaml, and everything lands beside it under <slug>/, that Workspace's own
// namespace and nobody else's. **The library is only ever read.** That is the property the whole
// isolation model exists for, and the one a test asserts by comparing the library's bytes before and
// after (2026-09-27: a write into the shared library destroyed 266 lines of the real Document Approval
// Machine).
//
// Ordering is the contract:
//
//  1. refuse a plan with any refusal, and refuse a destination file that already exists
//     (RefuseIfExists) -- before a single byte is written;
//  2. copy each Machine file, rewriting only the ids the plan renames;
//  3. copy the Application file, rewriting the same ids plus its own if it is being renamed;
//  4. append to the manifest: each Machine, each missing shared reference, then the Application;
//  5. load the whole manifest back through metadata.LoadApplication -- the real loader, the same one
//     the caller's reload is about to run -- and roll every write back if it does not load.
//
// Step 5 is what makes step 2 safe to attempt at all. A rename that missed a cross-reference fails
// one of the loader's own validators (validateRelationTargets, validateApplicationClaims,
// validateWorkflowBinding, validateMachineIDsAreUnique), and what cannot load is removed rather than
// left for the next restart to choke on.
func Install(plan Plan, libraryDir, workspaceManifestPath string) (applicationID string, err error) {
	if !plan.OK() {
		return "", fmt.Errorf("cannot install %s here: %s", plan.Template.Application.ID, strings.Join(plan.Refusals, "; "))
	}

	written := NewWriteSet()
	defer func() {
		if err != nil {
			written.Rollback()
		}
	}()

	workspaceDir := filepath.Dir(workspaceManifestPath)
	slug := strings.TrimSuffix(filepath.Base(workspaceManifestPath), filepath.Ext(workspaceManifestPath))
	ownDir := filepath.Join(workspaceDir, slug)

	var machineRelPaths []string
	for _, tm := range plan.Template.Machines {
		src, err := os.ReadFile(filepath.Join(libraryDir, tm.File))
		if err != nil {
			return "", fmt.Errorf("read template machine %s: %w", tm.File, err)
		}
		body, hits := rewriteIDs(src, plan.Renames)
		// A Machine being renamed must have had its own id: line rewritten. Zero there would install a
		// copy still declaring the template's id, which collides with what is already in this Workspace
		// -- the load-verify below would catch it, but this says which file and why.
		if _, renamed := plan.Renames[tm.Machine.ID]; renamed && hits == 0 {
			return "", fmt.Errorf("rewrite %s: %s is being renamed to %s, but no declaration in that file was rewritten", tm.File, tm.Machine.ID, plan.RenamedMachineID(tm.Machine.ID))
		}
		body = provenance(body, tm.File, plan)

		// A renamed Machine gets a filename derived from its *new* id, following the library's own
		// convention (mch_document -> document.yaml). Keeping the template's filename would collide with
		// whatever the Workspace already has under that name -- which is precisely the case a rename
		// exists to resolve, so it would refuse itself. An unrenamed Machine keeps the template's own
		// filename, so a byte-identical copy is also an identically-named one.
		dest := filepath.Join(ownDir, machineFileName(tm, plan))
		if err := RefuseIfExists(dest, "machine "+plan.RenamedMachineID(tm.Machine.ID)); err != nil {
			return "", err
		}
		if err := written.Note(dest); err != nil {
			return "", err
		}
		if err := WriteFileStrict[FullMachineCheckDoc](dest, body); err != nil {
			return "", fmt.Errorf("write machine %s: %w", dest, err)
		}
		rel, err := filepath.Rel(workspaceDir, dest)
		if err != nil {
			return "", err
		}
		machineRelPaths = append(machineRelPaths, toSlash(rel))
	}

	appSrc, err := os.ReadFile(filepath.Join(libraryDir, plan.Template.ApplicationFile))
	if err != nil {
		return "", fmt.Errorf("read template application %s: %w", plan.Template.ApplicationFile, err)
	}
	appBody, appHits := rewriteIDs(appSrc, plan.Renames)
	// An Application declares every Machine it claims, so any rename at all must reach its file.
	if len(plan.Renames) > 0 && appHits == 0 {
		return "", fmt.Errorf("rewrite %s: ids were renamed (%s) but the application file references none of them", plan.Template.ApplicationFile, strings.Join(sortedKeys(plan.Renames), ", "))
	}
	appBody = provenance(appBody, plan.Template.ApplicationFile, plan)

	appDest := filepath.Join(ownDir, "applications", filepath.Base(plan.Template.ApplicationFile))
	if err := RefuseIfExists(appDest, "application "+plan.ApplicationID()); err != nil {
		return "", err
	}
	if err := written.Note(appDest); err != nil {
		return "", err
	}
	if err := WriteFileStrict[FullApplicationCheckDoc](appDest, appBody); err != nil {
		return "", fmt.Errorf("write application %s: %w", appDest, err)
	}
	appRel, err := filepath.Rel(workspaceDir, appDest)
	if err != nil {
		return "", err
	}

	manifest, err := os.ReadFile(workspaceManifestPath)
	if err != nil {
		return "", fmt.Errorf("read workspace manifest: %w", err)
	}
	updated := manifest
	for _, rel := range machineRelPaths {
		if updated, err = AppendBlockListItem(updated, "machines:", rel); err != nil {
			return "", fmt.Errorf("append machine to workspace manifest: %w", err)
		}
	}
	// A shared runtime Machine is *referenced* out of the library, deliberately -- the one thing a
	// Workspace does not own a copy of (see SharedMachineIDs). The path is computed from the two
	// directories rather than written as "../": that is what it comes out as for the real layout
	// (metadata/workspaces/<slug>.yaml against metadata/user.yaml), and assuming it would make this
	// function silently wrong for any other arrangement, starting with its own tests.
	for _, id := range plan.AddShared {
		file := filepath.Join(libraryDir, sharedFileFor(id))
		if _, statErr := os.Stat(file); statErr != nil {
			return "", fmt.Errorf("template needs shared machine %s, but %s is not in the library: %w", id, sharedFileFor(id), statErr)
		}
		rel, relErr := filepath.Rel(workspaceDir, file)
		if relErr != nil {
			return "", relErr
		}
		if updated, err = AppendBlockListItem(updated, "machines:", toSlash(rel)); err != nil {
			return "", fmt.Errorf("append shared machine %s to workspace manifest: %w", id, err)
		}
	}
	if updated, err = AppendBlockListItem(updated, "applications:", toSlash(appRel)); err != nil {
		return "", fmt.Errorf("append application to workspace manifest: %w", err)
	}
	if err := written.Note(workspaceManifestPath); err != nil {
		return "", err
	}
	if err := WriteFileStrict[WorkspaceManifestCheckDoc](workspaceManifestPath, updated); err != nil {
		return "", fmt.Errorf("write workspace manifest: %w", err)
	}

	if _, err = metadata.LoadApplication(workspaceManifestPath); err != nil {
		return "", fmt.Errorf("the installed copy does not load, so it has been rolled back: %w", err)
	}
	return plan.ApplicationID(), nil
}

func toSlash(p string) string { return filepath.ToSlash(p) }

func machineFileName(tm TemplateMachine, plan Plan) string {
	renamed := plan.RenamedMachineID(tm.Machine.ID)
	if renamed == tm.Machine.ID {
		return filepath.Base(tm.File)
	}
	return strings.TrimPrefix(renamed, "mch_") + ".yaml"
}

// sharedFileFor is the library filename for a runtime-level Machine id. A three-entry convention
// rather than a lookup, because SharedMachineIDs is itself a closed set of three and both lists would
// have to change together anyway.
func sharedFileFor(machineID string) string {
	return strings.TrimPrefix(machineID, "mch_") + ".yaml"
}

// idReference matches one line that *references* an id by value: "key: mch_x", "- mch_x", with an
// optional trailing comment. Value-based rather than key-based on purpose -- ids in this metadata are
// distinctive (mch_*, app_*), so a line whose value is exactly one of them is a reference to it, and
// enumerating which keys may hold one (id, machine, summary_machine, a workflow role name, a
// machines: entry) would be a list to keep in step with every future key that can hold an id.
//
// Comment lines are left alone: the prose in a copied template still explains the original, and
// rewriting English is not this function's business. The provenance header says so out loud.
var idReference = regexp.MustCompile(`^(\s*(?:-\s+|[a-z_]+:\s+))([a-z_]+)(\s*(?:#.*)?)$`)

// rewriteIDs applies a plan's renames to one file's bytes, touching only reference lines, and reports
// how many it changed.
//
// The count matters because zero is sometimes right and sometimes a bug, and only the caller knows
// which: a Machine file that mentions none of the renamed ids is ordinary (signature.yaml when only
// mch_document was renamed), while the renamed Machine's *own* file must have had its id: line
// rewritten, and so must the Application file that claims it. Install checks both rather than having
// this function guess -- the same "refuse rather than guess" posture AppendBlockListItem takes, moved
// to where the expectation actually lives.
func rewriteIDs(src []byte, renames map[string]string) ([]byte, int) {
	if len(renames) == 0 {
		return src, 0 // byte-identical copy, the normal case
	}
	lines := strings.Split(string(src), "\n")
	hits := 0
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		m := idReference.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if renamed, ok := renames[m[2]]; ok {
			lines[i] = m[1] + renamed + m[3]
			hits++
		}
	}
	return []byte(strings.Join(lines, "\n")), hits
}

// provenance prepends the header every installed copy carries: where it came from, when, and what was
// renamed on the way in.
//
// It exists because the rewrite is deliberately narrow. Comments below this header still name the
// template's original ids -- rewriting prose would be guessing at English -- so a reader who finds
// "mch_document" in a comment above a Machine whose id is mch_document_approval needs one line
// explaining why. It also makes the copy's origin greppable, which nothing else records: a Workspace's
// own file carries no field naming the template it came from.
func provenance(body []byte, templateFile string, plan Plan) []byte {
	var b strings.Builder
	b.WriteString("# Installed from metadata/" + toSlash(templateFile) + " on " + time.Now().Format("2006-01-02") + ".\n")
	b.WriteString("# This is this Workspace's own copy: editing it changes nothing anywhere else, and the\n")
	b.WriteString("# template it came from is unchanged (internal/installer).\n")
	if len(plan.Renames) > 0 {
		b.WriteString("#\n# Ids renamed on install, because this Workspace already used them:\n")
		for _, from := range sortedKeys(plan.Renames) {
			b.WriteString("#   " + from + " -> " + plan.Renames[from] + "\n")
		}
		b.WriteString("# Comments below may still name the original ids -- only declarations were rewritten.\n")
	}
	b.WriteString("\n")
	return append([]byte(b.String()), body...)
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
