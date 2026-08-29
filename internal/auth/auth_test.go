package auth

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestPasswordHash(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("unexpected hash format: %q", hash)
	}
	if !VerifyPassword(hash, "correct horse battery staple") {
		t.Fatal("correct password did not verify")
	}
	if VerifyPassword(hash, "wrong password") {
		t.Fatal("wrong password verified")
	}
}

func TestRandomToken(t *testing.T) {
	a, err := RandomToken()
	if err != nil {
		t.Fatal(err)
	}
	b, err := RandomToken()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("random tokens matched")
	}
	raw, err := base64.RawURLEncoding.DecodeString(a)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 32 {
		t.Fatalf("token has %d bytes, want 32", len(raw))
	}
}
