package domain

import "testing"

func TestFitHoldsAnAnswerToItsControl(t *testing.T) {
	status := FormInput{Kind: InputSelect, Options: []string{"Cast", "Crew"}}
	ref := FormInput{Kind: InputSelect, Reference: "mch_category", Choices: []FormChoice{{Value: "rec_1", Label: "Lead Cast"}}}
	for name, tc := range map[string]struct {
		in        FormInput
		raw       string
		want      string
		wantUnfit bool
	}{
		"text is kept":                 {FormInput{Kind: InputText}, "  Reza Anggara ", "Reza Anggara", false},
		"nothing is not unfit":         {FormInput{Kind: InputNumber}, "  ", "", false},
		"a plain number":               {FormInput{Kind: InputNumber}, "180000000", "180000000", false},
		"a decimal":                    {FormInput{Kind: InputNumber}, "2700000.50", "2700000.5", false},
		"grouped digits are read":      {FormInput{Kind: InputNumber}, "180,000,000", "180000000", false},
		"a currency sign is read":      {FormInput{Kind: InputNumber}, "Rp 54000000", "54000000", false},
		"words are not a number":       {FormInput{Kind: InputNumber}, "one hundred", "", true},
		"NaN is not":                   {FormInput{Kind: InputNumber}, "NaN", "", true},
		"infinity is not":              {FormInput{Kind: InputNumber}, "+Inf", "", true},
		"hex is not":                   {FormInput{Kind: InputNumber}, "0x1p4", "", true},
		"separators are not":           {FormInput{Kind: InputNumber}, "1_000", "", true},
		"two records of one label":     {FormInput{Kind: InputSelect, Reference: "m", Choices: []FormChoice{{Value: "a", Label: "Twin"}, {Value: "b", Label: "twin"}}}, "Twin", "", true},
		"an ISO date":                  {FormInput{Kind: InputDate}, "2026-11-02", "2026-11-02", false},
		"a written date":               {FormInput{Kind: InputDate}, "2 Nov 2026", "2026-11-02", false},
		"not a date":                   {FormInput{Kind: InputDate}, "next autumn", "", true},
		"an option, any case":          {status, "cast", "Cast", false},
		"not an option":                {status, "Producer", "", true},
		"a record by its label":        {ref, "lead cast", "rec_1", false},
		"a record that does not exist": {ref, "Extras", "", true},
	} {
		got, unfit := Fit(tc.in, tc.raw)
		if got != tc.want || unfit != tc.wantUnfit {
			t.Errorf("%s: Fit(%q) = %q, %v; want %q, %v", name, tc.raw, got, unfit, tc.want, tc.wantUnfit)
		}
	}
}
