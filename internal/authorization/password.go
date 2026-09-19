package authorization

import "golang.org/x/crypto/bcrypt"

// HashPassword returns a bcrypt hash of plain, suitable for Store.CreateCredential. Never store
// or log plain anywhere else.
func HashPassword(plain string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// VerifyPassword reports whether plain matches hash (as produced by HashPassword). A malformed
// hash is treated as a non-match rather than an error -- the caller only ever wants a yes/no
// answer, the same posture as CheckCredentials.
func VerifyPassword(plain, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
