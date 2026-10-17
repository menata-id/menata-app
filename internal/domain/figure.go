package domain

import (
	"math"
	"strconv"
	"strings"
)

// FormatFigure writes a stored figure the way a person reads a Field of type t. A number is written in full, never in
// the exponent form Go's %v switches to at a million ("1.8e+08"). Money follows the international convention: thousands
// grouped with ",", at most two decimals after ".", trailing zeros dropped -- 180000000 reads "180,000,000" and
// 1234567.5 reads "1,234,567.5". The decimal point is the one a number input accepts, so the two never disagree on
// what "." means. No Workspace can declare another convention yet, and no currency symbol is written because no
// metadata declares one (writing-guide.md, "What comes free vs. what's hardcoded today").
func FormatFigure(t FieldType, v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	if t != FieldTypeMoney {
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	// Rounded to cents as text, so a figure past float64's integer precision keeps the digits it has instead of
	// overflowing an int64 of cents.
	s := strconv.FormatFloat(math.Abs(v), 'f', 2, 64)
	whole, frac, _ := strings.Cut(s, ".")
	frac = strings.TrimRight(frac, "0")
	var b strings.Builder
	if v < 0 && strings.Trim(whole+frac, "0") != "" {
		b.WriteByte('-') // -0.001 rounds to zero, and zero has no sign
	}
	for i, d := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(d)
	}
	if frac != "" {
		b.WriteString("." + frac)
	}
	return b.String()
}

// Figure is a stored value of f written as FormatFigure writes it, and false when f is not numeric or the value is not
// a stored figure (an unset value, or text written before the type held a number) -- the caller then shows it as it is.
func (f Field) Figure(v any) (string, bool) {
	if !f.Type.IsNumeric() {
		return "", false
	}
	switch n := v.(type) {
	case float64:
		return FormatFigure(f.Type, n), true
	case float32:
		return FormatFigure(f.Type, float64(n)), true
	case int:
		return FormatFigure(f.Type, float64(n)), true
	case int64:
		return FormatFigure(f.Type, float64(n)), true
	}
	return "", false
}
