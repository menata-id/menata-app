package authorization

import (
	"strings"
	"unicode/utf8"
)

// MinPasswordLength is the fewest characters a new password may have; MaxPasswordBytes is bcrypt's
// own limit (it ignores everything past 72 bytes, so a longer password would be accepted and
// silently shortened). Both apply to a password being chosen, never to one being checked at
// login: an older, shorter password stays valid until its owner changes it.
const (
	MinPasswordLength = 8
	MaxPasswordBytes  = 72
)

// PasswordProblem is the one password policy every place that sets a password asks (registration,
// accepting an invitation, a reset, a change). It returns "" when plain is acceptable, otherwise a
// phrase that completes "Password ...", e.g. "must be at least 8 characters". It judges only the
// password; whether the two typed copies match is the caller's question.
func PasswordProblem(plain string) string {
	switch {
	case utf8.RuneCountInString(plain) < MinPasswordLength:
		return "must be at least 8 characters"
	case len(plain) > MaxPasswordBytes:
		return "must be at most 72 bytes long"
	case isCommonPassword(plain):
		return "is too common; choose something less guessable"
	}
	return ""
}

// isCommonPassword reports whether plain is one of the passwords everybody tries first, or one
// character repeated. The list is small on purpose (the passwords of at least eight characters
// that lead every public breach ranking) and is not a substitute for a breach database.
func isCommonPassword(plain string) bool {
	lower := strings.ToLower(plain)
	if _, ok := commonPasswords[lower]; ok {
		return true
	}
	first, _ := utf8.DecodeRuneInString(lower)
	return strings.Count(lower, string(first)) == utf8.RuneCountInString(lower)
}

var commonPasswords = func() map[string]struct{} {
	m := map[string]struct{}{}
	for _, p := range strings.Fields(`
		password password1 password12 password123 password1234 passw0rd p@ssw0rd p@ssword
		12345678 123456789 1234567890 12345678910 11111111 00000000 87654321 123123123 12341234
		qwertyui qwerty123 qwertyuiop qwerty12345 1q2w3e4r 1q2w3e4r5t 1qaz2wsx zaq12wsx
		abc12345 abcd1234 abcdefgh abcdefg1 asdfghjk asdfghjkl zxcvbnm1 zxcvbnm123
		iloveyou iloveyou1 iloveyou2 sunshine1 princess1 football1 baseball1 superman1
		welcome1 welcome123 letmein1 letmein123 admin123 administrator changeme changeme1
		trustno1 monkey123 dragon123 master123 login123 starwars1 whatever1 michael1
		menata123 menatapassword indonesia indonesia1 indonesia123 rahasia123 sandi123
		katasandi kata_sandi bismillah bismillah1 sayangku1 sayang123
	`) {
		m[p] = struct{}{}
	}
	return m
}()
