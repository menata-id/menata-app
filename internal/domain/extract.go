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
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	switch in.Kind {
	case InputNumber:
		if n, ok := parseNumber(raw); ok {
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

// groupedNumber is a number written with thousands separators ("180,000,000", "1,250.5"): a reader told not to may still do it.
var plainNumber = regexp.MustCompile(`^-?\d+(\.\d+)?$`)

var groupedNumber = regexp.MustCompile(`^-?\d{1,3}(,\d{3})+(\.\d+)?$`)

func parseNumber(s string) (float64, bool) {
	s = strings.TrimSpace(strings.TrimPrefix(s, "Rp"))
	if groupedNumber.MatchString(s) {
		s = strings.ReplaceAll(s, ",", "")
	}
	// Plain decimal digits only: ParseFloat would also read NaN, Inf, hex floats and digit separators.
	if !plainNumber.MatchString(s) {
		return 0, false
	}
	n, err := strconv.ParseFloat(s, 64)
	return n, err == nil && !math.IsInf(n, 0)
}
