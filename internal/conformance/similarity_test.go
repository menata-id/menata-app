package conformance

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestDesignSimilarityReport is GS7 of menata-app-document's guides/menjaga-design-system.md: a **report, not a gate**.
// Run it with `-v`. It never fails except when its own scan finds nothing.
//
// It reduces every hand-written class set in `internal/rendering/*.templ` to a visual fingerprint: shape, fill, ink,
// edge, text size, weight, padding and display, with the exact values thrown away. It then groups sites that share
// one. A group of three or more sites across two or more files is a candidate: either a recipe that already has a
// reader or Component (use it), or a repeated shape with no name (a candidate for registration, rencana §5). That is
// the shape of L7 in the guide: one meaning, two values, found only by someone comparing.
//
// **Why it does not fail.** This repo has deleted two gates that were about 50% false findings. Whether two sites with
// one fingerprint are one *meaning* is a judgement a fingerprint cannot make (CLAUDE.md: "two cases" is a claim about
// sameness). The rule is to read the groups, label each real or false, record the rate, and promote the report to a
// gate only if the rate is high. The first reading is in development-history (2026-10-12).
func TestDesignSimilarityReport(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	comment := regexp.MustCompile(`//[^\n]*`)
	classAttr := regexp.MustCompile(`class="([^"]*)"`)
	type site struct {
		file    string
		line    int
		classes string
	}
	groups := map[string][]site{}
	scanned := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") || e.Name() == "layout.templ" || e.Name() == "controls.templ" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		src := joinClassExpressionsKeepingLines(comment.ReplaceAllString(string(b), ""))
		for _, m := range classAttr.FindAllStringSubmatchIndex(src, -1) {
			classes := src[m[2]:m[3]]
			scanned++
			fp := visualFingerprint(classes)
			if fp == "" {
				continue
			}
			line := strings.Count(src[:m[0]], "\n") + 1
			groups[fp] = append(groups[fp], site{e.Name(), line, classes})
		}
	}
	if scanned == 0 {
		t.Fatal("no class attribute was scanned -- the report is reading the wrong text")
	}
	type group struct {
		fp    string
		sites []site
		files int
	}
	var out []group
	for fp, ss := range groups {
		files := map[string]bool{}
		for _, s := range ss {
			files[s.file] = true
		}
		if len(ss) >= 3 && len(files) >= 2 {
			out = append(out, group{fp, ss, len(files)})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i].sites) != len(out[j].sites) {
			return len(out[i].sites) > len(out[j].sites)
		}
		return out[i].fp < out[j].fp
	})
	t.Logf("scanned %d class sets; %d fingerprint groups with >=3 sites in >=2 files", scanned, len(out))
	for i, g := range out {
		var where []string
		for _, s := range g.sites {
			where = append(where, fmt.Sprintf("%s:%d", s.file, s.line))
		}
		t.Logf("#%d %d sites / %d files  [%s]\n    e.g. %q\n    %s", i+1, len(g.sites), g.files, g.fp, g.sites[0].classes, strings.Join(where, " "))
	}
}

// visualFingerprint keeps what makes a class set look like a kind of thing and drops the exact values. A set with no
// shape, fill or edge (pure layout) has no fingerprint: layout is the layout ratchets' job.
func visualFingerprint(classes string) string {
	f := map[string]bool{}
	visual := false
	for _, tok := range strings.Fields(classes) {
		if strings.Contains(tok, ":") { // hover:, sm:, group-hover: -- state and breakpoint, not the thing itself
			continue
		}
		switch {
		case tok == "rounded-full":
			f["shape:pill"], visual = true, true
		case tok == "rounded-md" || tok == "rounded" || tok == "rounded-sm":
			f["shape:small"], visual = true, true
		case tok == "rounded-lg" || tok == "rounded-xl":
			f["shape:large"], visual = true, true
		case tok == "bg-white":
			f["fill:raised"], visual = true, true
		case regexp.MustCompile(`^bg-[a-z]+-(50|100)$`).MatchString(tok):
			f["fill:tint"], visual = true, true
		case regexp.MustCompile(`^bg-[a-z]+-[5-9]00$`).MatchString(tok):
			f["fill:solid"], visual = true, true
		case tok == "border" || tok == "border-2":
			f["edge"], visual = true, true
		case tok == "border-dashed":
			f["edge:dashed"], visual = true, true
		case regexp.MustCompile(`^text-(3xs|2xs|xs|sm|base|lg|xl|2xl)$`).MatchString(tok):
			f["size:"+strings.TrimPrefix(tok, "text-")] = true
		case regexp.MustCompile(`^text-slate-\d+$`).MatchString(tok):
			f["ink:neutral"] = true
		case regexp.MustCompile(`^text-[a-z]+-\d+$`).MatchString(tok) && tok != "text-white":
			f["ink:colour"] = true
		case tok == "font-medium" || tok == "font-semibold":
			f["weight:emphasis"] = true
		case regexp.MustCompile(`^p[xy]?-`).MatchString(tok):
			f["pad"] = true
		case tok == "flex" || tok == "inline-flex" || tok == "grid":
			f["display:"+tok] = true
		case tok == "uppercase":
			f["case:upper"] = true
		}
	}
	if !visual || len(f) < 3 {
		return ""
	}
	keys := make([]string, 0, len(f))
	for k := range f {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, " ")
}

// joinClassExpressionsKeepingLines is joinClassExpressions that pads each joined expression with the newlines it
// removed, so a report can still point at the right line.
func joinClassExpressionsKeepingLines(src string) string {
	src = radiusReadersAsLiterals(src) // a migrated corner is still part of the shape
	return classExpression.ReplaceAllStringFunc(src, func(m string) string {
		var parts []string
		for _, lit := range stringLiteral.FindAllStringSubmatch(m, -1) {
			parts = append(parts, lit[1])
		}
		return `class="` + strings.Join(parts, " ") + `"` + strings.Repeat("\n", strings.Count(m, "\n"))
	})
}
