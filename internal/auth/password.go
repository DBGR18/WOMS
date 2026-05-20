package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
)

const passwordHashIterations = 120000

func HashPassword(password string) (string, error) {
	if strings.TrimSpace(password) == "" {
		return "", errors.New("password is required")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := passwordDigest([]byte(password), salt, passwordHashIterations)
	return "sha256$" + strconv.Itoa(passwordHashIterations) + "$" +
		base64.RawURLEncoding.EncodeToString(salt) + "$" +
		base64.RawURLEncoding.EncodeToString(hash), nil
}

func VerifyPassword(storedHash, password string) bool {
	if password == "" {
		return false
	}
	parts := strings.Split(storedHash, "$")
	if len(parts) != 4 || parts[0] != "sha256" {
		return subtle.ConstantTimeCompare([]byte(storedHash), []byte(password)) == 1
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations <= 0 {
		return false
	}
	salt, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	expected, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	actual := passwordDigest([]byte(password), salt, iterations)
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func passwordDigest(password, salt []byte, iterations int) []byte {
	state := append(append([]byte{}, salt...), password...)
	sum := sha256.Sum256(state)
	digest := sum[:]
	for i := 1; i < iterations; i++ {
		next := sha256.Sum256(digest)
		digest = next[:]
	}
	return digest
}
