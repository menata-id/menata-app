package domain

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Fit says what a document reader's raw answer becomes as the start of one control: the string the control shows, and
// whether it did not fit (so a person is told to look). A reader answers in words; the control is what the Field is, so
// the answer is held to it here -- a number is a number, a date a date, a choice one of the options or one of the
// related records by its label -- and what does not fit is left empty and flagged rather than put in a control that would
// refuse it or, worse, accept it wrongly.
func Fit(in FormInput, raw string) (value string, unfit bool) {
	return FitIn(NumberFormatInternational, in, raw)
}

// FitIn is Fit for a Workspace whose figures are written in nf: "4.783.163,50" is a number in the Indonesian format.
func FitIn(nf NumberFormat, in FormInput, raw string) (value string, unfit bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	switch in.Kind {
	case InputNumber:
		if n, ok := parseNumberIn(nf, raw); ok {
			return strconv.FormatFloat(n, 'f', -1, 64), false
		}
		return "", true
	case InputDate:
		for _, layout := range dateLayouts {
			if t, err := time.Parse(layout, raw); err == nil {
				return t.Format("2006-01-02"), false
			}
		}
		return "", true
	case InputSelect:
		if in.Reference != "" {
			found, value := 0, ""
			for _, c := range in.Choices {
				if strings.EqualFold(strings.TrimSpace(c.Label), raw) {
					found, value = found+1, c.Value
				}
			}
			// Two records with one label cannot be told apart by it: a person chooses.
			if found == 1 {
				return value, false
			}
			return "", true
		}
		for _, o := range in.Options {
			if strings.EqualFold(strings.TrimSpace(o), raw) {
				return o, false
			}
		}
		return "", true
	}
	return raw, false
}

var dateLayouts = []string{"2006-01-02", "02 Jan 2006", "2 Jan 2006", "2 January 2006", "02 January 2006", "02/01/2006", "2/1/2006"}

// plainNumber is a plain decimal; a reader told to write one may still group its thousands, which NumberFormat.Canonical undoes.
var plainNumber = regexp.MustCompile(`^-?\d+(\.\d+)?$`)

func parseNumberIn(nf NumberFormat, s string) (float64, bool) {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "Rp"))
	nf = nf.Or()
	// A plain decimal ("1250.5") is always a figure in the format whose decimal mark is "."; in the other, the document's own
	// grouping is read as the Workspace writes it, and a plain "1.234" is a thousand, not a decimal.
	canonical, ok := nf.Canonical(s)
	if !ok {
		return 0, false
	}
	// Plain decimal digits only: ParseFloat would also read NaN, Inf, hex floats and digit separators.
	n, err := strconv.ParseFloat(canonical, 64)
	return n, err == nil && !math.IsInf(n, 0)
}
