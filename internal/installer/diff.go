package installer

import (
	"fmt"
	"strings"
)

// UnifiedDiff is a line diff of two texts with three lines of context. It is a longest-common-subsequence diff,
// quadratic in line count, which is fine for a Machine or Application file; past maxDiffLines either side it says
// so instead of spending the time.
func UnifiedDiff(a, b string) string {
	const maxDiffLines = 4000
	al, bl := splitLines(a), splitLines(b)
	if len(al) > maxDiffLines || len(bl) > maxDiffLines {
		return fmt.Sprintf("(file too large to diff: %d -> %d lines)\n", len(al), len(bl))
	}
	// lcs[i][j] is the common-subsequence length of al[i:] and bl[j:].
	lcs := make([][]int, len(al)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(bl)+1)
	}
	for i := len(al) - 1; i >= 0; i-- {
		for j := len(bl) - 1; j >= 0; j-- {
			if al[i] == bl[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	type line struct {
		mark byte
		text string
	}
	var all []line
	i, j := 0, 0
	for i < len(al) || j < len(bl) {
		switch {
		case i < len(al) && j < len(bl) && al[i] == bl[j]:
			all = append(all, line{' ', al[i]})
			i++
			j++
		case i < len(al) && (j == len(bl) || lcs[i+1][j] >= lcs[i][j+1]):
			all = append(all, line{'-', al[i]})
			i++
		default:
			all = append(all, line{'+', bl[j]})
			j++
		}
	}
	const context = 3
	keep := make([]bool, len(all))
	for k, l := range all {
		if l.mark == ' ' {
			continue
		}
		for c := max(0, k-context); c <= min(len(all)-1, k+context); c++ {
			keep[c] = true
		}
	}
	var sb strings.Builder
	gap := false
	for k, l := range all {
		if !keep[k] {
			gap = true
			continue
		}
		if gap && sb.Len() > 0 {
			sb.WriteString("@@\n")
		}
		gap = false
		sb.WriteByte(l.mark)
		sb.WriteString(l.text)
		sb.WriteByte('\n')
	}
	return sb.String()
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}
