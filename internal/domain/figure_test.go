package domain

import (
	"math"
	"testing"
)

func TestFormatFigure(t *testing.T) {
	for _, c := range []struct {
		t    FieldType
		v    float64
		want string
	}{
		{FieldTypeNumber, 180000000, "180000000"},
		{FieldTypeNumber, 1.5, "1.5"},
		{FieldTypeNumber, -3, "-3"},
		{FieldTypeMoney, 0, "0.00"},
		{FieldTypeMoney, 999, "999.00"},
		{FieldTypeMoney, 1000, "1,000.00"},
		{FieldTypeMoney, 180000000, "180,000,000.00"},
		{FieldTypeMoney, 1234567.5, "1,234,567.50"},
		{FieldTypeMoney, 1234567.05, "1,234,567.05"},
		{FieldTypeMoney, 0.999, "1.00"},
		{FieldTypeMoney, -2700000, "-2,700,000.00"},
		{FieldTypeMoney, -0.001, "0.00"},                          // rounds to zero, which has no sign
		{FieldTypeMoney, 1e15 + 0.25, "1,000,000,000,000,000.25"}, // past an int64 of cents' comfort
		{FieldTypeMoney, math.Inf(1), "+Inf"},
		{FieldTypeMoney, math.NaN(), "NaN"},
	} {
		if got := FormatFigure(c.t, c.v); got != c.want {
			t.Errorf("FormatFigure(%s, %v) = %q, want %q", c.t, c.v, got, c.want)
		}
	}
}

func TestFieldFigureOnlyFormatsStoredFigures(t *testing.T) {
	money := Field{ID: "fld_value", Type: FieldTypeMoney}
	if got, ok := money.Figure(float64(54000000)); !ok || got != "54,000,000.00" {
		t.Errorf("money figure = %q, %v", got, ok)
	}
	for _, v := range []any{nil, "54000000", true} {
		if _, ok := money.Figure(v); ok {
			t.Errorf("money.Figure(%#v) formatted a value that is not a stored figure", v)
		}
	}
	if _, ok := (Field{Type: FieldTypeText}).Figure(float64(1)); ok {
		t.Error("a text field was formatted as a figure")
	}
}

// TestLabelFieldPrefersTheTitleRole: a record is named by its title role, and by its first Field only when it
// declares none -- an instalment whose first Field is its contract is named by its own label, not the contract's id.
func TestLabelFieldPrefersTheTitleRole(t *testing.T) {
	m := &Machine{Fields: []Field{{ID: "fld_contract", Type: FieldTypeRelation}, {ID: "fld_term_label", Type: FieldTypeText}},
		CardFields: []CardField{{Field: "fld_term_label", Role: CardFieldRoleTitle}}}
	if got := m.LabelField(); got != "fld_term_label" {
		t.Errorf("LabelField = %q, want the title role", got)
	}
	m.CardFields = nil
	if got := m.LabelField(); got != "fld_term_label" {
		t.Errorf("LabelField with no title role = %q, want the first Field that is not a reference (an id is never a name)", got)
	}
	m.Fields = m.Fields[:1]
	if got := m.LabelField(); got != "fld_contract" {
		t.Errorf("LabelField of a Machine of references only = %q, want its first Field", got)
	}
}

// Money is written in the Workspace's marks with two decimals, and a figure typed or read in those marks is turned back into
// the plain decimal that is stored and posted.
func TestMoneyIsWrittenAndReadInTheWorkspacesMarks(t *testing.T) {
	for _, c := range []struct {
		nf   NumberFormat
		v    float64
		want string
	}{
		{NumberFormatInternational, 1234567.5, "1,234,567.50"},
		{NumberFormatIndonesian, 1234567.5, "1.234.567,50"},
		{NumberFormatIndonesian, 180000000, "180.000.000,00"},
		{NumberFormatIndonesian, -2700000, "-2.700.000,00"},
		{NumberFormat{}, 1000, "1,000.00"}, // a Workspace that declares nothing
	} {
		if got := FormatMoney(c.v, c.nf); got != c.want {
			t.Errorf("FormatMoney(%v, %s) = %q, want %q", c.v, c.nf.Name, got, c.want)
		}
	}
	for _, c := range []struct {
		nf       NumberFormat
		in, want string
		ok       bool
	}{
		{NumberFormatInternational, "1,234,567.50", "1234567.50", true},
		{NumberFormatInternational, "1234567.5", "1234567.5", true},
		{NumberFormatInternational, "1.234.567,50", "", false},
		{NumberFormatIndonesian, "1.234.567,50", "1234567.50", true},
		{NumberFormatIndonesian, "-2.700.000", "-2700000", true},
		{NumberFormatIndonesian, "1234567", "1234567", true},
		{NumberFormatIndonesian, "1.234", "1234", true},  // a thousand, not a decimal
		{NumberFormatIndonesian, "1234.5", "", false},    // "." is a thousands mark here
		{NumberFormatIndonesian, "12.34.567", "", false}, // groups of three, or not a figure
		{NumberFormatInternational, "NaN", "", false},
	} {
		got, ok := c.nf.Canonical(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("%s.Canonical(%q) = %q, %v; want %q, %v", c.nf.Name, c.in, got, ok, c.want, c.ok)
		}
	}
	in := FormInput{Kind: InputNumber, Money: true}
	if v, unfit := FitIn(NumberFormatIndonesian, in, "Rp 4.783.163,50"); v != "4783163.5" || unfit {
		t.Errorf("FitIn(indonesian) = %q, %v", v, unfit)
	}
	if v, unfit := FitIn(NumberFormatInternational, in, "4.783.163"); !unfit {
		t.Errorf("an Indonesian figure in an international Workspace was read as %q", v)
	}
}
