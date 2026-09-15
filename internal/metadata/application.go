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

	if err := validateRelationTargets(app.Machines); err != nil {
		return nil, err
	}
	if err := validateConstraintTargets(app.Machines); err != nil {
		return nil, err
	}
	return app, nil
}

// validateRelationTargets checks that every reference field's target Machine ID (Relation's
// explicit `machine:`, or Person's implicit mch_user) is actually among the Application's own
// Machines -- a single Machine file can't know this on its own, since it only sees its own
// declaration (006-runtime-model.md "Relation": reusable, grounded in existing Machine/reference
// semantics).
func validateRelationTargets(machines []*domain.Machine) error {
	known := make(map[string]bool, len(machines))
	for _, m := range machines {
		known[m.ID] = true
	}

	var issues []string
	for _, m := range machines {
		for _, f := range m.Fields {
			if !f.IsReference() {
				continue
			}
			if !known[f.RelatedMachine] {
				issues = append(issues, fmt.Sprintf("machine %q field %q: relation target %q is not a machine in this application", m.ID, f.ID, f.RelatedMachine))
			}
		}
	}
	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

// validateConstraintTargets checks, once every Machine is loaded, that each Constraint's
// block_if actually names a real related Machine and Field -- and that the related field is
// itself a relation pointing back to this Machine, otherwise "related_field" wouldn't identify
// which of the related Machine's records belong to the record being transitioned.
func validateConstraintTargets(machines []*domain.Machine) error {
	byID := make(map[string]*domain.Machine, len(machines))
	for _, m := range machines {
		byID[m.ID] = m
	}

	var issues []string
	for _, m := range machines {
		for _, c := range m.Constraints {
			related, ok := byID[c.BlockIf.RelatedMachine]
			if !ok {
				issues = append(issues, fmt.Sprintf("machine %q constraint %q: block_if.related_machine %q is not a machine in this application", m.ID, c.ID, c.BlockIf.RelatedMachine))
				continue
			}

			relatedField, ok := related.FieldByID(c.BlockIf.RelatedField)
			if !ok {
				issues = append(issues, fmt.Sprintf("machine %q constraint %q: block_if.related_field %q is not a field of machine %q", m.ID, c.ID, c.BlockIf.RelatedField, related.ID))
			} else if relatedField.Type != domain.FieldTypeRelation || relatedField.RelatedMachine != m.ID {
				issues = append(issues, fmt.Sprintf("machine %q constraint %q: block_if.related_field %q must be a relation field on %q pointing back to %q", m.ID, c.ID, c.BlockIf.RelatedField, related.ID, m.ID))
			}

			if _, ok := related.FieldByID(c.BlockIf.Condition.Field); !ok {
				issues = append(issues, fmt.Sprintf("machine %q constraint %q: block_if.condition.field %q is not a field of machine %q", m.ID, c.ID, c.BlockIf.Condition.Field, related.ID))
			}
		}
	}
	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}
