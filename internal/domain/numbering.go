package domain

import (
	"fmt"
	"strconv"
	"strings"
)

// FieldNumbering (`numbering: {pattern: "CTR/{fld_contract_kind}/{seq:3}", codes: {Cast: C, Crew: K, Item: I}}`, 2026-10-18,
// plan A4) makes a text Field a **number the runtime assigns** when the record is created: "CTR/C/026", "PP-0146",
// "PO/2026/0087". Nobody types it -- no form asks for it, an edit leaves it as it is -- so two records can never claim one
// number and the pattern is the one statement of how this Workspace numbers its documents.
//
// The pattern is literal text and tokens: `{fld_x}` the record's own text or status Field (its value, or Codes[value] when
// Codes names it), `{year}` the year the record is created, and exactly one `{seq:N}`, last: the next whole number after
// the highest one already used under the same text before it, written N digits wide. So "CTR/C/" counts contracts of one
// kind, and "PP-" counts every plan. A number is never reused: the Store keeps a counter that only goes up, so deleting a record does not free its number.
type FieldNumbering struct {
	Pattern string
	// Codes maps a Field's value to the letters the number writes for it (Crew -> K); a value it does not name is written as
	// it is.
	Codes map[string]string
}

// NumberPart is one piece of a parsed numbering pattern: text, a Field's value, or the year.
type NumberPart struct {
	Literal string
	Field   string
	Year    bool
}

// ParseNumbering reads a pattern into the parts before its counter and the counter's width. An error names what is wrong,
// for the loader to say at load.
func ParseNumbering(pattern string) (prefix []NumberPart, width int, err error) {
	rest := pattern
	for rest != "" {
		open := strings.IndexByte(rest, '{')
		if open < 0 {
			prefix = append(prefix, NumberPart{Literal: rest})
			break
		}
		if open > 0 {
			prefix = append(prefix, NumberPart{Literal: rest[:open]})
		}
		closing := strings.IndexByte(rest[open:], '}')
		if closing < 0 {
			return nil, 0, fmt.Errorf("numbering %q: a { is never closed", pattern)
		}
		token := rest[open+1 : open+closing]
		rest = rest[open+closing+1:]
		switch {
		case token == "year":
			prefix = append(prefix, NumberPart{Year: true})
		case strings.HasPrefix(token, "seq:"):
			n, convErr := strconv.Atoi(strings.TrimPrefix(token, "seq:"))
			if convErr != nil || n < 1 || n > 9 {
				return nil, 0, fmt.Errorf("numbering %q: {seq:N} writes the counter N digits wide, N from 1 to 9", pattern)
			}
			if rest != "" {
				return nil, 0, fmt.Errorf("numbering %q: {seq:%d} is the counter and comes last, so numbers of one prefix sort together", pattern, n)
			}
			// The text before the counter ends in a separator: "CTR/C/" and "CTR/C1/" are two counters, but "CTRC" + digits
			// could be read as either, and one would borrow the other's numbers.
			if last := len(prefix) - 1; last < 0 || prefix[last].Literal == "" || isAlphanumeric(prefix[last].Literal[len(prefix[last].Literal)-1]) {
				return nil, 0, fmt.Errorf("numbering %q: the text before {seq:%d} ends with a separator such as / or -, so one prefix's numbers are never mistaken for another's", pattern, n)
			}
			return prefix, n, nil
		case token == "" || strings.ContainsAny(token, " :{"):
			return nil, 0, fmt.Errorf("numbering %q: {%s} is not a token (a Field id, year, or seq:N)", pattern, token)
		default:
			prefix = append(prefix, NumberPart{Field: token})
		}
	}
	return nil, 0, fmt.Errorf("numbering %q: needs one {seq:N} counter, last", pattern)
}

func isAlphanumeric(b byte) bool {
	return b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

// NumberPrefix is the text a record's number begins with: the pattern's text and tokens up to its counter, resolved against
// the record's values and the year it is created in. A Field the record holds no value for is an error -- a number cannot be
// written from nothing.
func (n FieldNumbering) NumberPrefix(values map[string]any, year int) (prefix string, width int, err error) {
	parts, width, err := ParseNumbering(n.Pattern)
	if err != nil {
		return "", 0, err
	}
	var b strings.Builder
	for _, p := range parts {
		switch {
		case p.Year:
			b.WriteString(strconv.Itoa(year))
		case p.Field != "":
			v := strings.TrimSpace(fmt.Sprint(values[p.Field]))
			if v == "" || v == "<nil>" {
				return "", 0, fmt.Errorf("the number needs %s, which has no value", p.Field)
			}
			if code, ok := n.Codes[v]; ok {
				v = code
			}
			b.WriteString(v)
		default:
			b.WriteString(p.Literal)
		}
	}
	return b.String(), width, nil
}

// Number writes the counter after the prefix, zero-padded to the pattern's width.
func Number(prefix string, width, counter int) string {
	return prefix + fmt.Sprintf("%0*d", width, counter)
}
