package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"menata.app/internal/domain"
	"menata.app/internal/metadata"
)

// SharedMachineIDs are the runtime-level Machines a Workspace references rather than owning a copy
// of -- the same closed set internal/conformance's own isolation gate allows outside a Workspace's
// directory, and for the same reasons: mch_user is the one identity that genuinely crosses
// Workspaces (a membership row points at one of its records), and mch_activity/mch_notification are
// runtime chrome that no Application claims.
//
// An install never copies one. It may *add a reference* to one the target Workspace is missing, which
// is not the same thing (see Template.RequiresShared).
var SharedMachineIDs = map[string]bool{
	domain.UserMachineID: true,
	"mch_activity":       true,
	"mch_notification":   true,
}

// Template is one installable Application in the library: what it declares, and which files declare
// it. Everything here is read from the library itself, so a template that is edited is a template
// that installs differently the next time -- there is no second description of it anywhere.
type Template struct {
	// Application is the loaded declaration (metadata.LoadApplicationFile), so a caller reads a
	// template's card face, roles and workflow binding from the same type a running Application has.
	Application *domain.Application
	// ApplicationFile is the Application's own file, relative to the library root.
	ApplicationFile string
	// Machines are the Machines this Application claims, in its own declared order, each with the
	// library file that declares it. All Application-owned: a template claiming a shared Machine is
	// refused when the library is read, since copying one is exactly what isolation forbids.
	Machines []TemplateMachine
	// RequiresShared are the runtime-level Machines this template's own metadata implies -- derived,
	// never listed: mch_notification when it declares a send_notification Event, mch_activity for
	// log_activity, mch_user when any Field is a person. A Workspace missing one gets the reference
	// added, because an Application whose notifications have nowhere to land is installed broken in a
	// way nothing would report.
	RequiresShared []string
}

// TemplateMachine is one Machine a template brings, and the file it comes from.
type TemplateMachine struct {
	Machine *domain.Machine
	// File is relative to the library root.
	File string
}

// Templates reads the whole library: every metadata/applications/*.yaml, resolved against the
// Machine files beside it.
//
// libraryDir is the library root (metadata/), whose own *.yaml files are Machines and whose
// applications/ subdirectory holds Applications. Sorted by name, so a listing is stable.
//
// A template that names a Machine the library does not declare is an error rather than a skipped
// entry: the library is small, hand-written, and in this repo -- a dangling claim there is a bug to
// fix, not a condition to degrade around.
func Templates(libraryDir string) ([]Template, error) {
	machines, err := libraryMachines(libraryDir)
	if err != nil {
		return nil, err
	}

	appDir := filepath.Join(libraryDir, "applications")
	entries, err := os.ReadDir(appDir)
	if err != nil {
		return nil, fmt.Errorf("read template applications %s: %w", appDir, err)
	}

	var out []Template
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		rel := filepath.ToSlash(filepath.Join("applications", e.Name()))
		app, err := metadata.LoadApplicationFile(filepath.Join(libraryDir, rel))
		if err != nil {
			return nil, fmt.Errorf("load template %s: %w", rel, err)
		}
		t := Template{Application: app, ApplicationFile: rel}
		for _, id := range app.Machines {
			if SharedMachineIDs[id] {
				return nil, fmt.Errorf("template %s claims %s, a runtime-level Machine no Application may own -- installing copies what it claims, and copying that one is what Workspace isolation forbids", rel, id)
			}
			m, ok := machines[id]
			if !ok {
				return nil, fmt.Errorf("template %s claims machine %s, which no file in %s declares", rel, id, libraryDir)
			}
			t.Machines = append(t.Machines, m)
		}
		t.RequiresShared = requiresShared(t.Machines)
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Application.Name < out[j].Application.Name })
	return out, nil
}

// TemplateByID is Templates narrowed to one Application id -- what a POST handed a template id needs,
// without a second way of reading the library.
func TemplateByID(libraryDir, applicationID string) (Template, error) {
	all, err := Templates(libraryDir)
	if err != nil {
		return Template{}, err
	}
	for _, t := range all {
		if t.Application.ID == applicationID {
			return t, nil
		}
	}
	return Template{}, fmt.Errorf("no template in %s declares application %q", libraryDir, applicationID)
}

// libraryMachines indexes the library's Machine files by the id each one declares. Machine files are
// the *.yaml directly in the library root; applications/ is the other half and is read separately.
//
// domain.Machine carries no record of the file it was loaded from (the same absence
// aiassist.FileMachineResolver works around), so this index is the only way to answer "which file
// declares mch_document" -- and an install has to copy files, not values, or every comment in them
// would be lost.
func libraryMachines(libraryDir string) (map[string]TemplateMachine, error) {
	entries, err := os.ReadDir(libraryDir)
	if err != nil {
		return nil, fmt.Errorf("read template library %s: %w", libraryDir, err)
	}
	out := map[string]TemplateMachine{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		m, err := metadata.Load(filepath.Join(libraryDir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("load template machine %s: %w", e.Name(), err)
		}
		if earlier, taken := out[m.ID]; taken {
			return nil, fmt.Errorf("template library declares %s twice (%s and %s)", m.ID, earlier.File, e.Name())
		}
		out[m.ID] = TemplateMachine{Machine: m, File: e.Name()}
	}
	return out, nil
}

// requiresShared derives which runtime-level Machines a template's own Machines imply. Derived rather
// than declared for the reason 001 Principle #8 gives: the dependency is already stated, by the Event
// that writes a notification and the Field that names a person, and a second list would be one more
// thing to forget.
func requiresShared(machines []TemplateMachine) []string {
	need := map[string]bool{}
	for _, tm := range machines {
		for _, f := range tm.Machine.Fields {
			if f.RelatedMachine == domain.UserMachineID {
				need[domain.UserMachineID] = true
			}
		}
		for _, e := range tm.Machine.Events {
			switch e.Then.Name {
			case domain.ServiceSendNotification:
				need["mch_notification"] = true
			case domain.ServiceLogActivity:
				need["mch_activity"] = true
			}
		}
	}
	out := make([]string, 0, len(need))
	for id := range need {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
