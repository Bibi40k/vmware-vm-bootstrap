// Package utils provides internal utility functions.
package utils

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// HashPasswordBcrypt hashes a password using bcrypt (default, recommended).
func HashPasswordBcrypt(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("bcrypt hash failed: %w", err)
	}
	return string(hash), nil
}

// HashPasswordSHA512 hashes a password using SHA-512 (legacy support).
// Format: $6$rounds=5000$SALT$HASH (crypt(3) compatible)
// Note: This uses a simplified implementation. For production, consider using
// a proper crypt(3) library or preferably bcrypt (HashPasswordBcrypt).
func HashPasswordSHA512(password string) (string, error) {
	// NOTE: SHA-512 crypt implementation is complex.
	// For production use, recommend bcrypt instead.
	// This is a placeholder for legacy systems that require SHA-512.
	//
	// Proper implementation would need:
	// - github.com/tredoe/osutil/user/crypt for full crypt(3) compatibility
	//
	// For now, return error directing users to bcrypt
	return "", fmt.Errorf("SHA-512 crypt not implemented - use HashPasswordBcrypt() instead (recommended)")
}

// ValidatePassword validates password strength.
func ValidatePassword(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}
	// Minimal validation; callers can enforce stricter rules if needed.
	return nil
}
