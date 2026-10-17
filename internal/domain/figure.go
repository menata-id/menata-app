package domain

import (
	"math"
	"strconv"
	"strings"
)

// FormatFigure writes a stored figure the way a person reads a Field of type t. A number is written in full, never in
// the exponent form Go's %v switches to at a million ("1.8e+08"). Money is grouped in thousands with "." and takes at
// most two decimals after "," -- 180000000 reads "180.000.000". That grouping is Indonesian usage and the one every
// mockup of the design target writes; no Workspace can declare another yet (writing-guide.md, "What comes free vs.
// what's hardcoded today"). No currency symbol is written: no metadata declares one.
func FormatFigure(t FieldType, v float64) string {
	if t != FieldTypeMoney {
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	sign := ""
	if v < 0 {
		sign, v = "-", -v
	}
	cents := math.Round(v * 100)
	whole, frac := strconv.FormatFloat(math.Floor(cents/100), 'f', 0, 64), int64(math.Mod(cents, 100))
	var b strings.Builder
	for i, d := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(d)
	}
	if frac != 0 {
		b.WriteString("," + strings.TrimRight(strconv.FormatInt(100+frac, 10)[1:], "0"))
	}
	return sign + b.String()
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
