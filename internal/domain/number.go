package domain

import (
	"math"
	"strconv"
	"strings"
)

// NumberFormat is how a Workspace writes a figure: the mark between thousands and the mark before the decimals. One
// convention for a whole Workspace -- every Application in it, every screen, every form -- so a figure reads the same wherever
// it is met. It is declared in the Workspace's manifest (`number_format:`); one that says nothing uses the international one.
type NumberFormat struct {
	// Name is the declared name ("international", "indonesian"); empty is the default.
	Name string
	// Thousands separates groups of three digits; Decimal separates the cents.
	Thousands, Decimal string
}

// Number formats a Workspace may declare. A closed set, added to when a Workspace needs another.
var (
	NumberFormatInternational = NumberFormat{Name: "international", Thousands: ",", Decimal: "."}
	NumberFormatIndonesian    = NumberFormat{Name: "indonesian", Thousands: ".", Decimal: ","}
)

// KnownNumberFormats are the declarable format names (vocabulary; NumberFormatNamed resolves one to its marks).
var KnownNumberFormats = map[string]bool{NumberFormatInternational.Name: true, NumberFormatIndonesian.Name: true}

// NumberFormatNamed is the format a declared name stands for, and false for a name that is not declarable.
func NumberFormatNamed(name string) (NumberFormat, bool) {
	switch name {
	case NumberFormatInternational.Name:
		return NumberFormatInternational, true
	case NumberFormatIndonesian.Name:
		return NumberFormatIndonesian, true
	}
	return NumberFormat{}, false
}

// Or is f, or the international format when f is the zero value (a Workspace that declares none, a screen with no Workspace).
func (f NumberFormat) Or() NumberFormat {
	if f.Thousands == "" || f.Decimal == "" {
		return NumberFormatInternational
	}
	return f
}

// FormatMoney writes an amount the way a Workspace reads money: thousands grouped, always two decimals ("1,234,567.50",
// "180,000,000.00"; "1.234.567,50" in the Indonesian format). Rounded to cents as text, so a figure past float64's integer
// precision keeps the digits it has.
func FormatMoney(v float64, nf NumberFormat) string {
	nf = nf.Or()
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	s := strconv.FormatFloat(math.Abs(v), 'f', 2, 64)
	whole, frac, _ := strings.Cut(s, ".")
	var b strings.Builder
	if v < 0 && strings.Trim(whole+frac, "0") != "" {
		b.WriteByte('-') // -0.001 rounds to zero, and zero has no sign
	}
	for i, d := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteString(nf.Thousands)
		}
		b.WriteRune(d)
	}
	b.WriteString(nf.Decimal + frac)
	return b.String()
}

// Canonical is a figure typed or read in this format as the plain decimal a number is stored and posted as ("1.234.567,50" ->
// "1234567.50"), and false when it is not a figure in this format. A figure already plain ("1234567.5") is accepted in the
// international format, whose decimal mark it is; in a format whose thousands mark is "." a plain "1234.5" is not, since
// "1.234" there is a thousand and not a decimal.
func (f NumberFormat) Canonical(s string) (string, bool) {
	f = f.Or()
	s = strings.TrimSpace(s)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	whole, frac, hasFrac := strings.Cut(s, f.Decimal)
	if strings.Contains(frac, f.Thousands) || strings.Contains(frac, f.Decimal) {
		return "", false
	}
	if strings.Contains(whole, f.Thousands) {
		groups := strings.Split(whole, f.Thousands)
		for i, g := range groups {
			if (i == 0 && (len(g) < 1 || len(g) > 3)) || (i > 0 && len(g) != 3) {
				return "", false
			}
		}
		whole = strings.Join(groups, "")
	}
	out := whole
	if hasFrac {
		out += "." + frac
	}
	if !plainNumber.MatchString(out) {
		return "", false
	}
	if neg {
		out = "-" + out
	}
	return out, true
}
