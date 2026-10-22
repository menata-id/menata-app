package ir

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// trackedRow is a row of three children, the middle one shown only for a record whose status is "Pending": the shape Workspace Home's
// rows have (a name, a value that is sometimes absent, a badge).
func trackedRow(tracks string) domain.PageNode {
	return domain.PageNode{Kind: "layout", Type: "row", Props: map[string]string{"tracks": tracks}, Children: []domain.PageNode{
		{Kind: "static", Type: "paragraph", From: map[string]string{"text": "title"}},
		{Kind: "static", Type: "caption", From: map[string]string{"text": "detail"}, When: &domain.PageCondition{Role: "status", Is: "Pending"}},
		{Kind: "static", Type: "caption", From: map[string]string{"text": "summary"}},
	}}
}

func recordWithStatus(status string) Resolver {
	return fixedRecords(map[string]string{"title": "T", "detail": "D", "summary": "S", "status": status})
}

func lowerTracked(t *testing.T, tracks, status string) []UINode {
	t.Helper()
	root := domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{listBound(trackedRow(tracks))}}
	got, err := Lower(root, recordWithStatus(status))
	if err != nil {
		t.Fatalf("Lower: %v", err)
	}
	list := got.Children[0]
	if len(list.Children) != 1 {
		t.Fatalf("want one item, got %+v", list.Children)
	}
	return list.Children[0].Children
}

func TestLower_aRowsChildrenKeepTheirDeclaredColumns(t *testing.T) {
	kids := lowerTracked(t, "fill field badge", "Pending")
	want := []domain.RowTrack{domain.TrackFill, domain.TrackField, domain.TrackBadge}
	if len(kids) != 3 {
		t.Fatalf("children = %+v", kids)
	}
	for i, w := range want {
		if kids[i].Track != w {
			t.Errorf("child %d is in column %q, want %q", i, kids[i].Track, w)
		}
	}
	for _, k := range kids {
		if _, still := k.Props["tracks"]; still {
			t.Errorf("a child carries the row's tracks prop: %+v", k.Props)
		}
	}
}

// The reason `tracks:` is positional rather than a width on each child: a child a `when:` hides must still hold its column, or the
// badge slides into the field's place in exactly the rows that have no field.
func TestLower_aChildTheWhenHidesStillHoldsItsColumn(t *testing.T) {
	kids := lowerTracked(t, "fill field badge", "Done")
	if len(kids) != 3 {
		t.Fatalf("the hidden child's column vanished: %+v", kids)
	}
	if kids[1].Track != domain.TrackField || kids[1].Props["text"] != "" {
		t.Errorf("the hidden child is not an empty node in the field column: %+v", kids[1])
	}
	if kids[2].Track != domain.TrackBadge {
		t.Errorf("the badge slid out of its column: %+v", kids[2])
	}
	if issues := Validate(UINode{Kind: NodeLayout, Type: "row", Children: kids}); len(issues) > 0 {
		t.Errorf("the placeholder is not a valid node: %v", issues)
	}
}

func TestLower_refusesTracksThatCannotBeDrawn(t *testing.T) {
	for name, tc := range map[string]struct{ tracks, want string }{
		"too few":      {"fill field", "names 2 column(s) and the row holds 3"},
		"too many":     {"fill field badge fit", "names 4 column(s) and the row holds 3"},
		"unknown word": {"fill field wide", `"wide" is not a column`},
	} {
		root := domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{listBound(trackedRow(tc.tracks))}}
		_, err := Lower(root, recordWithStatus("Pending"))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want it to say %q", name, err, tc.want)
		}
	}
	notARow := domain.PageNode{Kind: "layout", Type: "stack", Props: map[string]string{"tracks": "fill"}, Children: []domain.PageNode{
		{Kind: "static", Type: "paragraph", Props: map[string]string{"text": "x"}},
	}}
	if _, err := Lower(notARow, Resolver{}); err == nil || !strings.Contains(err.Error(), "only on a layout: row") {
		t.Errorf("tracks on a stack: err = %v", err)
	}
}

func TestValidate_refusesAColumnOutsideTheClosedSet(t *testing.T) {
	n := UINode{Kind: NodeLayout, Type: "row", Children: []UINode{{Kind: NodeStatic, Type: "paragraph", Props: map[string]string{"text": "x"}, Track: "huge"}}}
	if issues := Validate(n); len(issues) == 0 || !strings.Contains(issues[0], `"huge"`) {
		t.Errorf("issues = %v", issues)
	}
}

func TestValidate_aRowTakesAMarkFromTheClosedSetOnly(t *testing.T) {
	row := func(mark string) UINode {
		return UINode{Kind: NodeLayout, Type: "row", Props: map[string]string{"mark": mark}, Children: []UINode{{Kind: NodeStatic, Type: "paragraph", Props: map[string]string{"text": "x"}}}}
	}
	if issues := Validate(row("chevron")); len(issues) > 0 {
		t.Errorf("chevron refused: %v", issues)
	}
	if issues := Validate(row("arrow")); len(issues) == 0 {
		t.Error("an unknown mark was accepted")
	}
}

// A child that draws once for each of something would put several boxes in one column, and a different number of them in each row: refused
// when the page is declared, not when a record happens to have three tags.
func TestLower_refusesAnEachOrBoundChildUnderTracks(t *testing.T) {
	row := domain.PageNode{Kind: "layout", Type: "row", Props: map[string]string{"tracks": "fill badge"}, Children: []domain.PageNode{
		{Kind: "static", Type: "paragraph", From: map[string]string{"text": "title"}},
		{Kind: "component", Type: "StatusBadge", Each: domain.PageEachTags, Props: map[string]string{"tone": "info"}, From: map[string]string{"label": "title"}},
	}}
	root := domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{listBound(row)}}
	if _, err := Lower(root, recordWithStatus("Pending")); err == nil || !strings.Contains(err.Error(), "draws once for each of something") {
		t.Errorf("err = %v", err)
	}
}
