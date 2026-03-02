package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argon2Memory      = 64 * 1024
	argon2Iterations  = 3
	argon2Parallelism = 2
	argon2KeyLength   = 32
	argon2SaltLength  = 16
)

func HashPassword(password string) (string, error) {
	if password == "" {
		return "", errors.New("password cannot be empty")
	}

	salt := make([]byte, argon2SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	hash := argon2.IDKey(
		[]byte(password),
		salt,
		argon2Iterations,
		argon2Memory,
		argon2Parallelism,
		argon2KeyLength,
	)

	encodedSalt := base64.RawStdEncoding.EncodeToString(salt)
	encodedHash := base64.RawStdEncoding.EncodeToString(hash)

	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argon2Memory, argon2Iterations, argon2Parallelism, encodedSalt, encodedHash), nil
}

func VerifyPassword(encoded, password string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		return false, errors.New("invalid hash format")
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("decode salt: %w", err)
	}

	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("decode hash: %w", err)
	}

	computedHash := argon2.IDKey(
		[]byte(password),
		salt,
		argon2Iterations,
		argon2Memory,
		argon2Parallelism,
		uint32(len(expectedHash)),
	)

	if subtle.ConstantTimeCompare(expectedHash, computedHash) == 1 {
		return true, nil
	}

	return false, nil
}
