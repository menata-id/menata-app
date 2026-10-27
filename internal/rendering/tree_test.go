package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"menata.app/internal/domain"
	"menata.app/internal/ir"
)

func treeItem(text string) ir.UINode {
	return ir.UINode{Kind: ir.NodeStatic, Type: string(domain.StaticParagraph), Props: map[string]string{"text": text}}
}

func renderTree(t *testing.T, outline string, leaves ...string) string {
	t.Helper()
	n := ir.UINode{Kind: ir.NodeComponent, Type: string(domain.ComponentTree), Props: map[string]string{"label": "Files", "query": "path", "outline": outline, "empty": "No files."}}
	for _, l := range leaves {
		n.Children = append(n.Children, treeItem(l))
	}
	var buf bytes.Buffer
	if err := uiNode(n).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// Rows of one folder that arrive together share one heading, a folder inside a folder nests, a row at the top sits beside the folders, and the
// order given is the order drawn.
func TestTree_groupsConsecutiveRowsUnderOneFolderAndNestsThem(t *testing.T) {
	got := renderTree(t, "a\t0\na\t0\na/b\t0\n\t0", "one", "two", "three", "four")
	if n := strings.Count(got, "<details"); n != 2 {
		t.Errorf("want folders a and a/b, got %d <details>:\n%s", n, got)
	}
	if strings.Count(got, ">a</summary>") != 1 || strings.Count(got, ">b</summary>") != 1 {
		t.Errorf("each folder is named once:\n%s", got)
	}
	order := []string{">a</summary>", "one", "two", ">b</summary>", "three", "four"}
	at := 0
	for _, w := range order {
		i := strings.Index(got[at:], w)
		if i < 0 {
			t.Fatalf("%q not found in order after offset %d:\n%s", w, at, got)
		}
		at += i
	}
	if !strings.Contains(got[strings.Index(got, ">b</summary>"):], "three") || strings.Index(got, "four") < strings.LastIndex(got, "</details>") {
		t.Errorf("three belongs inside b and four is outside every folder:\n%s", got)
	}
}

func TestTree_marksOnlyTheSelectedRowAndSaysWhatToSelectBy(t *testing.T) {
	got := renderTree(t, "a\t0\na\t1", "one", "two")
	if strings.Count(got, `aria-current="true"`) != 1 || !strings.Contains(got, `data-selection="path"`) {
		t.Fatalf("one row marked and the parameter named:\n%s", got)
	}
	if !strings.Contains(got[strings.Index(got, `aria-current="true"`):], "two") {
		t.Errorf("the second row is the selected one:\n%s", got)
	}
}

func TestTree_withNoRowsSaysWhy(t *testing.T) {
	if got := renderTree(t, ""); !strings.Contains(got, "No files.") || strings.Contains(got, "<ul") {
		t.Errorf("an empty tree says why and draws no list:\n%s", got)
	}
}
