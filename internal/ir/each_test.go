package ir

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
)

func tagNode() domain.PageNode {
	return domain.PageNode{Kind: "component", Type: "Tag", Each: domain.PageEachTags, From: map[string]string{"label": "title", "color": "color"}}
}

func tagsResolver(lists ...[]map[string]string) Resolver {
	recs := make([]map[string]string, len(lists))
	for i := range lists {
		recs[i] = map[string]string{"title": "t"}
	}
	return Resolver{Records: func(domain.PageBinding) (RecordSet, error) {
		return RecordSet{Records: recs, Tags: lists}, nil
	}}
}

func tag(title, color string) map[string]string {
	return map[string]string{"title": title, "color": color}
}

// TestLowerEachDrawsTheNodeOncePerTagOfEachRecord: three records with two, none and one tag draw three Tags
// in total, each in its own record's item and each reading its own tag.
func TestLowerEachDrawsTheNodeOncePerTagOfEachRecord(t *testing.T) {
	item := domain.PageNode{Kind: "layout", Type: "row", Children: []domain.PageNode{tagNode()}}
	tree, err := Lower(domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{listBound(item)}},
		tagsResolver([]map[string]string{tag("a", "blue"), tag("b", "red")}, nil, []map[string]string{tag("c", "green")}))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, row := range tree.Children[0].Children {
		var labels []string
		for _, k := range row.Children {
			labels = append(labels, k.Props["label"]+"/"+k.Props["color"])
		}
		got = append(got, strings.Join(labels, ","))
	}
	if want := "a/blue,b/red||c/green"; strings.Join(got, "|") != want {
		t.Fatalf("got %q, want %q", strings.Join(got, "|"), want)
	}
}

func TestLowerEachIsRefusedWhereItCannotMeanAThing(t *testing.T) {
	inTemplate := func(n domain.PageNode) domain.PageNode {
		return domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{listBound(domain.PageNode{Kind: "layout", Type: "row", Children: []domain.PageNode{n}})}}
	}
	res := tagsResolver([]map[string]string{tag("a", "blue")})
	noFrom := tagNode()
	noFrom.From = nil
	noFrom.Props = map[string]string{"label": "typed"}
	otherList := tagNode()
	otherList.Each = "comments"
	bound := tagNode()
	bound.Binding = &domain.PageBinding{Dataset: "ds_a", Rows: domain.PageRowsRecords}
	nested := domain.PageNode{Kind: "layout", Type: "row", Each: domain.PageEachTags, From: map[string]string{"gap": "title"}, Children: []domain.PageNode{tagNode()}}
	delInside := domain.PageNode{Kind: "layout", Type: "row", Each: domain.PageEachTags, From: map[string]string{"gap": "title"},
		Children: []domain.PageNode{{Kind: "component", Type: "Button", Binding: &domain.PageBinding{Write: domain.PageWriteDelete}}}}
	cases := map[string]struct {
		root domain.PageNode
		res  Resolver
		want string
	}{
		"outside a template":      {domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{tagNode()}}, Resolver{}, "item template"},
		"no from":                 {inTemplate(noFrom), res, "from:"},
		"a list that is not tags": {inTemplate(otherList), res, "not a list"},
		"beside a binding":        {inTemplate(bound), res, "two ways"},
		"inside another each":     {inTemplate(nested), res, "directly in the item template"},
		"a delete under a tag":    {inTemplate(delInside), res, ""},
		"a role the tag lacks":    {inTemplate(func() domain.PageNode { n := tagNode(); n.From = map[string]string{"label": "status"}; return n }()), res, "status"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Lower(c.root, c.res)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want an error mentioning %q, got %v", c.want, err)
			}
		})
	}
}
