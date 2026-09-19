package authorization

import "testing"

func TestHashPassword_verifyRoundTrip(t *testing.T) {
	hash, err := HashPassword("hunter2")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "" || hash == "hunter2" {
		t.Fatalf("HashPassword() = %q, want a non-empty hash distinct from the plain password", hash)
	}
	if !VerifyPassword("hunter2", hash) {
		t.Error("VerifyPassword(correct password) = false, want true")
	}
}

func TestVerifyPassword_wrongPassword(t *testing.T) {
	hash, err := HashPassword("hunter2")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if VerifyPassword("wrong", hash) {
		t.Error("VerifyPassword(wrong password) = true, want false")
	}
}

func TestVerifyPassword_malformedHash(t *testing.T) {
	if VerifyPassword("hunter2", "not-a-bcrypt-hash") {
		t.Error("VerifyPassword(malformed hash) = true, want false")
	}
}
