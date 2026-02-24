package utils

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestHashPasswordBcrypt(t *testing.T) {
	password := "TestPassword123!"

	hash, err := HashPasswordBcrypt(password)
	if err != nil {
		t.Fatalf("HashPasswordBcrypt() failed: %v", err)
	}

	// Verify hash format (bcrypt hashes start with $2a$, $2b$, or $2y$)
	if !strings.HasPrefix(hash, "$2") {
		t.Errorf("Invalid bcrypt hash format: %s", hash)
	}

	// Verify password matches hash
	err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		t.Errorf("Password doesn't match hash: %v", err)
	}

	// Verify wrong password doesn't match
	err = bcrypt.CompareHashAndPassword([]byte(hash), []byte("WrongPassword"))
	if err == nil {
		t.Error("Wrong password should not match hash")
	}
}

func TestHashPasswordBcryptDifferentHashes(t *testing.T) {
	password := "SamePassword"

	hash1, err := HashPasswordBcrypt(password)
	if err != nil {
		t.Fatalf("HashPasswordBcrypt() failed: %v", err)
	}

	hash2, err := HashPasswordBcrypt(password)
	if err != nil {
		t.Fatalf("HashPasswordBcrypt() failed: %v", err)
	}

	// bcrypt includes random salt, so hashes should be different
	if hash1 == hash2 {
		t.Error("Two hashes of same password should be different (random salt)")
	}

	// But both should validate the password
	if err := bcrypt.CompareHashAndPassword([]byte(hash1), []byte(password)); err != nil {
		t.Errorf("hash1 should validate password: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash2), []byte(password)); err != nil {
		t.Errorf("hash2 should validate password: %v", err)
	}
}

func TestHashPasswordSHA512(t *testing.T) {
	_, err := HashPasswordSHA512("password")
	if err == nil {
		t.Fatal("expected error for SHA-512 hash not implemented")
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"Valid password", "SecurePass123!", false},
		{"Minimum length", "12345678", false},
		{"Too short", "short", true},
		{"Empty", "", true},
		{"7 chars", "1234567", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePassword() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Benchmark tests
func BenchmarkHashPasswordBcrypt(b *testing.B) {
	password := "BenchmarkPassword123!"
	for i := 0; i < b.N; i++ {
		if _, err := HashPasswordBcrypt(password); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkValidatePassword(b *testing.B) {
	password := "BenchmarkPassword123!"
	for i := 0; i < b.N; i++ {
		if err := ValidatePassword(password); err != nil {
			b.Fatal(err)
		}
	}
}
