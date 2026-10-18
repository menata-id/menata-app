package domain

import (
	"math"
	"strconv"
	"strings"
	"time"
)

// FormatFigure writes a stored figure the way a person reads a Field of type t. A number is written in full, never in
// the exponent form Go's %v switches to at a million ("1.8e+08"). Money follows the international convention: thousands
// grouped with ",", at most two decimals after ".", trailing zeros dropped -- 180000000 reads "180,000,000" and
// 1234567.5 reads "1,234,567.5". The decimal point is the one a number input accepts, so the two never disagree on
// what "." means. No Workspace can declare another convention yet (owner decision K4: the international one); a money
// Field's `currency:` puts its symbol before the figure (Field.Figure, WithCurrency).
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

// KnownCurrencies are the currencies a money Field may declare (`currency: IDR`), each with the symbol written before
// its figures. A closed set, added to when a Workspace needs another.
var KnownCurrencies = map[string]string{
	"IDR": "Rp", "USD": "$", "EUR": "€", "GBP": "£", "SGD": "S$", "MYR": "RM", "AUD": "A$", "JPY": "¥",
}

// WithCurrency is a written figure with the symbol of currency before it ("Rp 180,000,000"; "-Rp 2,700,000" for a
// negative one); the figure unchanged when currency is "" or unknown.
func WithCurrency(currency, figure string) string {
	symbol, ok := KnownCurrencies[currency]
	if !ok || figure == "" {
		return figure
	}
	if rest, negative := strings.CutPrefix(figure, "-"); negative {
		return "-" + symbol + " " + rest
	}
	return symbol + " " + figure
}

// DayLayout is how a date reads on every screen: "02 Nov 2026" (mockups K01d, K02a).
const DayLayout = "02 Jan 2006"

// FormatDay is a stored date (DateLayout) as a screen reads it, or the stored text when it is not one.
func FormatDay(stored string) string {
	if d, err := time.Parse(DateLayout, stored); err == nil {
		return d.Format(DayLayout)
	}
	return stored
}

// Figure is a stored value of f written as FormatFigure writes it, and false when f is not numeric or the value is not
// a stored figure (an unset value, or text written before the type held a number) -- the caller then shows it as it is.
func (f Field) Figure(v any) (string, bool) {
	if !f.Type.IsNumeric() {
		return "", false
	}
	var n float64
	switch x := v.(type) {
	case float64:
		n = x
	case float32:
		n = float64(x)
	case int:
		n = float64(x)
	case int64:
		n = float64(x)
	default:
		return "", false
	}
	return WithCurrency(f.Currency, FormatFigure(f.Type, n)), true
}
