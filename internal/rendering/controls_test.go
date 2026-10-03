package rendering

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"
)

// TestSearchBox_hiddenInputsRenderInAStableOrder holds 007 §4.6 Determinism ("identical input MUST produce
// identical output") for the one place this repository was measurably breaking it.
//
// `searchBox` took a `map[string]string` and ranged over it, and Go randomises map iteration. The Approval
// Inbox's two filter tabs pass two keys each, so the form emitted its hidden inputs in a different order
// between requests. Found on 2026-10-03 while diffing that screen against a worktree baseline: the *baseline
// alone* flipped on the sixth of six fetches, which is also why one fetch would not have found it.
//
// Nothing visibly broke -- both inputs are submitted either way -- and that is the point. A nondeterministic
// renderer makes every byte-level comparison of the page unreliable: a render diff, an ETag, a cache key. The
// defect reports itself as noise in whatever tool is checking something else.
//
// Rendered repeatedly here rather than once, because one render of a randomised map passes about half the
// time with two keys and far more often with three.
func TestSearchBox_hiddenInputsRenderInAStableOrder(t *testing.T) {
	hidden := map[string]string{"tab": "mine", "status": "all", "sort": "due"}
	name := regexp.MustCompile(`<input type="hidden" name="([^"]+)"`)

	var first []string
	for i := 0; i < 50; i++ {
		var buf bytes.Buffer
		if err := searchBox("/approval-inbox", "", "Search…", hidden).Render(context.Background(), &buf); err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		var got []string
		for _, m := range name.FindAllStringSubmatch(buf.String(), -1) {
			got = append(got, m[1])
		}
		if i == 0 {
			first = got
			if want := []string{"sort", "status", "tab"}; strings.Join(got, ",") != strings.Join(want, ",") {
				t.Fatalf("hidden inputs rendered %v, want sorted order %v", got, want)
			}
			continue
		}
		if strings.Join(got, ",") != strings.Join(first, ",") {
			t.Fatalf("render %d gave %v, render 0 gave %v -- the order is not stable, so a map is being ranged over directly (007 §4.6)", i, got, first)
		}
	}
}
