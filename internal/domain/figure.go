package domain

import (
	"strconv"
	"strings"
	"time"
)

// FormatFigure writes a stored figure the way a person reads a Field of type t, in the international format: see FormatFigureIn.
func FormatFigure(t FieldType, v float64) string {
	return FormatFigureIn(NumberFormatInternational, t, v)
}

// FormatFigureIn writes a stored figure the way a person reads a Field of type t in a Workspace's number format. A number is
// written in full, never in the exponent form Go's %v switches to at a million ("1.8e+08"). Money is grouped in thousands
// and always has two decimals (FormatMoney): 180000000 reads "180,000,000.00". A money Field's `currency:` puts its symbol
// before the figure (Field.FigureIn, WithCurrency).
func FormatFigureIn(nf NumberFormat, t FieldType, v float64) string {
	if t != FieldTypeMoney {
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return FormatMoney(v, nf)
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
	return f.FigureIn(NumberFormatInternational, v)
}

// FigureIn is Figure in a Workspace's number format.
func (f Field) FigureIn(nf NumberFormat, v any) (string, bool) {
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
	return WithCurrency(f.Currency, FormatFigureIn(nf, f.Type, n)), true
}
