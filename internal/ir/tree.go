package ir

import (
	"fmt"
	"strings"

	"menata.app/internal/domain"
)

// treeOutlineProp is the Tree input lowering alone writes: one line per item, in item order, `folder<TAB>selected`. The folder is the
// directory part of the record's `title` read as a path ("" for a row at the top); selected is "1" for the one row whose id the request
// carried under the Tree's `query`. A page that writes one is refused, as it is for `selected` on a SegmentedControl: it cannot know either.
const treeOutlineProp = "outline"

// lowerTree turns `component: Tree` bound to a records Dataset into the Tree whose items are the item template cloned for each record.
// The records must already be in the order they are to be read (a path sort puts a folder's rows together), and the outline is derived from
// them: nothing about depth or selection is written on the page. Selection is the request's: the record whose id equals the parameter named
// by `query` is the current row, and none is when the request carries none.
func lowerTree(n domain.PageNode, r Resolver, path string) ([]UINode, error) {
	for _, p := range []string{treeOutlineProp} {
		if _, written := n.Props[p]; written {
			return nil, fmt.Errorf("%s: %s: is produced by lowering from the records and the request, and is never written", path, p)
		}
	}
	for _, p := range []string{"label", "query"} {
		if n.Props[p] == "" {
			return nil, fmt.Errorf("%s: a Tree names its %s:", path, p)
		}
	}
	if len(n.Children) != 1 {
		return nil, fmt.Errorf("%s: a Tree holds exactly one child, the item template cloned for each record, and has %d", path, len(n.Children))
	}
	if r.Records == nil {
		return nil, fmt.Errorf("%s: no resolver for records", path)
	}
	set, err := r.Records(*n.Binding)
	if err != nil {
		return nil, err
	}
	selected := ""
	if r.Query != nil {
		selected = r.Query(n.Props["query"])
	}
	props := make(map[string]string, len(n.Props)+1)
	for k, v := range n.Props {
		props[k] = v
	}
	out := UINode{Kind: NodeComponent, Type: n.Type, Props: props}
	var outline []string
	for i, rec := range set.Records {
		items, err := lowerRecordItem(n.Children[0], r, fmt.Sprintf("%s.children[0]", path), set, i)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			continue // a template that draws nothing for this record (a `when:` that does not hold) leaves the row out, as a Collection does
		}
		if len(items) != 1 {
			return nil, fmt.Errorf("%s: a Tree row is one node, and record %d's item template drew %d", path, i, len(items))
		}
		folder := treeFolder(rec["title"])
		current := "0"
		if selected != "" && rec[domain.PageRecordIDRole] == selected {
			current = "1"
		}
		outline = append(outline, folder+"\t"+current)
		out.Children = append(out.Children, items...)
	}
	props[treeOutlineProp] = strings.Join(outline, "\n")
	return []UINode{out}, nil
}

// treeFolder is the folder part of a title read as a path: the segments before the last slash, empty segments left out (a leading, trailing or
// doubled slash names no folder) and a tab or newline read as a space, so a title can neither add a line to the outline nor forge its marker.
func treeFolder(title string) string {
	j := strings.LastIndex(title, "/")
	if j < 0 {
		return ""
	}
	var segs []string
	for _, seg := range strings.Split(title[:j], "/") {
		if seg = strings.TrimSpace(strings.NewReplacer("\t", " ", "\n", " ", "\r", " ").Replace(seg)); seg != "" {
			segs = append(segs, seg)
		}
	}
	return strings.Join(segs, "/")
}
