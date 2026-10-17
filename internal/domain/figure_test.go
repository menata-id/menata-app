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
		{FieldTypeMoney, 0, "0"},
		{FieldTypeMoney, 999, "999"},
		{FieldTypeMoney, 1000, "1,000"},
		{FieldTypeMoney, 180000000, "180,000,000"},
		{FieldTypeMoney, 1234567.5, "1,234,567.5"},
		{FieldTypeMoney, 1234567.05, "1,234,567.05"},
		{FieldTypeMoney, 0.999, "1"},
		{FieldTypeMoney, -2700000, "-2,700,000"},
		{FieldTypeMoney, -0.001, "0"},                             // rounds to zero, which has no sign
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
	if got, ok := money.Figure(float64(54000000)); !ok || got != "54,000,000" {
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
