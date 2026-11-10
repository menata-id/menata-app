package authorization

import (
	"strings"
	"testing"
)

func TestPasswordProblem(t *testing.T) {
	cases := []struct {
		name, plain, want string
	}{
		{"fine", "correct horse battery", ""},
		{"eight characters is enough", "tr0ub4dr", ""},
		{"seven is too short", "tr0ub4d", "must be at least 8 characters"},
		{"empty", "", "must be at least 8 characters"},
		{"length counts characters not bytes", "пароль12", ""},
		{"exactly 72 bytes", strings.Repeat("ab", 36), ""},
		{"73 bytes", strings.Repeat("ab", 36) + "c", "must be at most 72 bytes long"},
		{"multibyte past 72 bytes", strings.Repeat("é", 37), "must be at most 72 bytes long"},
		{"a common one", "password123", "is too common; choose something less guessable"},
		{"common whatever its case", "PassWord123", "is too common; choose something less guessable"},
		{"one character repeated", "zzzzzzzzzz", "is too common; choose something less guessable"},
		{"common word inside a longer one is fine", "password123-and-more", ""},
	}
	for _, c := range cases {
		if got := PasswordProblem(c.plain); got != c.want {
			t.Errorf("%s: PasswordProblem(%q) = %q, want %q", c.name, c.plain, got, c.want)
		}
	}
}

func TestTheCommonListHoldsNothingTheMinimumAlreadyRefuses(t *testing.T) {
	for p := range commonPasswords {
		if len([]rune(p)) < MinPasswordLength {
			t.Errorf("%q is shorter than the minimum, so the list entry never fires", p)
		}
		if p != strings.ToLower(p) {
			t.Errorf("%q must be lower case: the lookup lower-cases the password", p)
		}
	}
}
