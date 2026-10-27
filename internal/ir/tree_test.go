package ir

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
)

func treeBound(props map[string]string) domain.PageNode {
	return domain.PageNode{
		Kind: "component", Type: string(domain.ComponentTree), Props: props,
		Binding:  &domain.PageBinding{Dataset: "ds_a", Rows: domain.PageRowsRecords},
		Children: []domain.PageNode{{Kind: "static", Type: "link", From: map[string]string{"text": domain.PageTitleLeafRole, "href": "link"}}},
	}
}

func treeFile(path string) map[string]string {
	leaf := path
	if i := strings.LastIndex(path, "/"); i >= 0 {
		leaf = path[i+1:]
	}
	return map[string]string{"title": path, domain.PageTitleLeafRole: leaf, domain.PageRecordIDRole: path, "link": "/f?path=" + path}
}

// A Tree is a records-bound list whose rows sit under the folders their title names, and the row the request selected is marked: the
// outline (one `folder<TAB>selected` line per item, in item order) is produced by lowering from the records and the request, so a page
// writes neither a depth nor a selection.
func TestLowerTree_outlinesTheFoldersAndMarksTheSelectedRow(t *testing.T) {
	recs := []map[string]string{treeFile("applications/a.yaml"), treeFile("applications/b.yaml"), treeFile("machines/m/x.yaml"), treeFile("root.yaml")}
	r := Resolver{
		Records: func(domain.PageBinding) (RecordSet, error) { return RecordSet{Records: recs}, nil },
		Query:   func(n string) string { return map[string]string{"path": "applications/b.yaml"}[n] },
	}
	out, err := lowerTree(treeBound(map[string]string{"label": "Files", "query": "path"}), r, "p")
	if err != nil || len(out) != 1 {
		t.Fatalf("%v %v", out, err)
	}
	tree := out[0]
	if tree.Type != "Tree" || len(tree.Children) != 4 {
		t.Fatalf("tree = %+v", tree)
	}
	if want := "applications\t0\napplications\t1\nmachines/m\t0\n\t0"; tree.Props["outline"] != want {
		t.Errorf("outline = %q, want %q", tree.Props["outline"], want)
	}
	if got := tree.Children[2].Props["text"]; got != "x.yaml" {
		t.Errorf("the item template reads the leaf: text = %q", got)
	}
}

func TestLowerTree_noRowIsSelectedWhenTheRequestNamesNone(t *testing.T) {
	r := Resolver{
		Records: func(domain.PageBinding) (RecordSet, error) {
			return RecordSet{Records: []map[string]string{treeFile("a/b.yaml")}}, nil
		},
		Query: func(string) string { return "" },
	}
	out, err := lowerTree(treeBound(map[string]string{"label": "Files", "query": "path"}), r, "p")
	if err != nil || out[0].Props["outline"] != "a\t0" {
		t.Fatalf("%v %v", out, err)
	}
}

func TestLowerTree_refusesWhatLoweringAloneKnows(t *testing.T) {
	r := Resolver{Records: func(domain.PageBinding) (RecordSet, error) { return RecordSet{}, nil }, Query: func(string) string { return "" }}
	for name, props := range map[string]map[string]string{
		"an outline written": {"label": "Files", "query": "path", "outline": "a\t1"},
		"no label":           {"query": "path"},
		"no query parameter": {"label": "Files"},
	} {
		if _, err := lowerTree(treeBound(props), r, "p"); err == nil {
			t.Errorf("%s: want a refusal", name)
		}
	}
	two := treeBound(map[string]string{"label": "Files", "query": "path"})
	two.Children = append(two.Children, two.Children[0])
	if _, err := lowerTree(two, r, "p"); err == nil {
		t.Error("two item templates: want a refusal")
	}
}
