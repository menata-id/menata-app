package metadata

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"

	"menata.app/internal/domain"
)

var (
	workspaceIDPattern   = regexp.MustCompile(`^ws_[a-z][a-z0-9_]*$`)
	applicationIDPattern = regexp.MustCompile(`^app_[a-z][a-z0-9_]*$`)
)

// App is a loaded Application manifest: its Workspace, itself, and every Machine it references.
type App struct {
	Workspace   domain.Workspace
	Application domain.Application
	Machines    []*domain.Machine
}

type appDoc struct {
	Workspace struct {
		ID   string `yaml:"id"`
		Name string `yaml:"name"`
	} `yaml:"workspace"`
	Application struct {
		ID       string   `yaml:"id"`
		Name     string   `yaml:"name"`
		Machines []string `yaml:"machines"`
	} `yaml:"application"`
}

// LoadApplication reads an Application manifest and every Machine file it references (paths
// resolved relative to the manifest's own directory), validating each in turn
// (005-runtime-lifecycle.md Phase 3-4: invalid metadata must not enter execution).
func LoadApplication(path string) (*App, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read application manifest %s: %w", path, err)
	}

	var doc appDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse application manifest: %w", err)
	}

	var issues []string
	if !workspaceIDPattern.MatchString(doc.Workspace.ID) {
		issues = append(issues, fmt.Sprintf("workspace id %q must match %s", doc.Workspace.ID, workspaceIDPattern.String()))
	}
	if !applicationIDPattern.MatchString(doc.Application.ID) {
		issues = append(issues, fmt.Sprintf("application id %q must match %s", doc.Application.ID, applicationIDPattern.String()))
	}
	if len(doc.Application.Machines) == 0 {
		issues = append(issues, fmt.Sprintf("application %q: at least one machine is required", doc.Application.ID))
	}
	if len(issues) > 0 {
		return nil, &ValidationError{Issues: issues}
	}

	dir := filepath.Dir(path)
	app := &App{
		Workspace:   domain.Workspace{ID: doc.Workspace.ID, Name: doc.Workspace.Name},
		Application: domain.Application{ID: doc.Application.ID, Name: doc.Application.Name, WorkspaceID: doc.Workspace.ID},
	}
	for _, rel := range doc.Application.Machines {
		m, err := Load(filepath.Join(dir, rel))
		if err != nil {
			return nil, fmt.Errorf("application %q: machine %s: %w", doc.Application.ID, rel, err)
		}
		app.Machines = append(app.Machines, m)
	}
	return app, nil
}
