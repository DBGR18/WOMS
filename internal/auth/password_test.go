package auth

import "testing"

func TestPasswordHashVerifiesAndRejectsWrongPassword(t *testing.T) {
	hash, err := HashPassword("temporary-secret")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if hash == "temporary-secret" {
		t.Fatal("expected password hash to differ from password")
	}
	if !VerifyPassword(hash, "temporary-secret") {
		t.Fatal("expected hash to verify correct password")
	}
	if VerifyPassword(hash, "wrong") {
		t.Fatal("expected hash to reject wrong password")
	}
}

func TestVerifyPasswordSupportsLegacyDemoPasswords(t *testing.T) {
	if !VerifyPassword("demo", "demo") {
		t.Fatal("expected legacy demo password to verify")
	}
	if VerifyPassword("demo", "wrong") {
		t.Fatal("expected legacy demo password to reject wrong password")
	}
}
