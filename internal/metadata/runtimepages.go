package metadata

import (
	"bytes"
	"embed"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"

	"menata.app/internal/domain"
)

// runtimePageFiles holds the pages the runtime declares for the screens it owns (domain.RuntimeScreens). They ship with
// the binary, so every Workspace has them and no manifest lists them: a screen that exists identically in a Workspace
// with ten Applications or none is the runtime's, and its body being metadata is a statement about how it is written,
// not about who owns it (001 #2 and #3 both hold).
//
//go:embed runtimepages/*.yaml
var runtimePageFiles embed.FS

// runtimePageDoc is one file. It names the screen and declares only what the screen's identity does not already say:
// who may open it and its body. Its route, label and heading are the RuntimeScreens entry's, so none of them is written
// twice (001 #8).
type runtimePageDoc struct {
	ID     string       `yaml:"id"`
	OpenTo string       `yaml:"open_to"`
	Page   *pageNodeDoc `yaml:"page"`
}

// loadRuntimePages reads every embedded file into navigation items: the RuntimeScreens entry each names, with the page
// and the open_to it declares. A file naming no runtime screen, naming one twice, or declaring no page is an error here
// and not a screen that quietly stays bespoke.
func loadRuntimePages() ([]domain.NavigationItem, error) {
	entries, err := runtimePageFiles.ReadDir("runtimepages")
	if err != nil {
		return nil, err
	}
	var items []domain.NavigationItem
	seen := map[string]bool{}
	for _, e := range entries {
		raw, err := runtimePageFiles.ReadFile("runtimepages/" + e.Name())
		if err != nil {
			return nil, err
		}
		var doc runtimePageDoc
		dec := yaml.NewDecoder(bytes.NewReader(raw))
		dec.KnownFields(true)
		if err := dec.Decode(&doc); err != nil {
			return nil, fmt.Errorf("runtime page %s: %w", e.Name(), err)
		}
		var screen *domain.NavigationItem
		for i := range domain.RuntimeScreens {
			if domain.RuntimeScreens[i].ID == doc.ID {
				screen = &domain.RuntimeScreens[i]
			}
		}
		switch {
		case screen == nil:
			return nil, fmt.Errorf("runtime page %s: %q is not one of domain.RuntimeScreens", e.Name(), doc.ID)
		case seen[doc.ID]:
			return nil, fmt.Errorf("runtime page %s: %q is declared twice", e.Name(), doc.ID)
		case doc.Page == nil:
			return nil, fmt.Errorf("runtime page %s: declares no page", e.Name())
		}
		seen[doc.ID] = true
		item := *screen
		page := doc.Page.toDomain()
		item.Page, item.OpenTo = &page, doc.OpenTo
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}

// runtimePagesThisWorkspaceCanDraw keeps the runtime pages whose every bound Dataset this Workspace declares. A page
// naming a Dataset the Workspace lacks is not an error in the Workspace (a minimal manifest has no notifications to
// list), and it is not drawn half-empty either: the screen is absent there, and its route answers 404.
func runtimePagesThisWorkspaceCanDraw(pages []domain.NavigationItem, machines []*domain.Machine) []domain.NavigationItem {
	have := map[string]bool{}
	for _, m := range machines {
		for _, ds := range m.Datasets {
			have[ds.ID] = true
		}
	}
	var out []domain.NavigationItem
	for _, p := range pages {
		ok := true
		for _, id := range boundDatasets(*p.Page) {
			ok = ok && have[id]
		}
		if ok {
			out = append(out, p)
		}
	}
	return out
}

// boundDatasets is every Dataset id a page's Bindings name.
func boundDatasets(n domain.PageNode) []string {
	var ids []string
	if n.Binding != nil && n.Binding.Dataset != "" {
		ids = append(ids, n.Binding.Dataset)
	}
	for _, c := range n.Children {
		ids = append(ids, boundDatasets(c)...)
	}
	return ids
}
