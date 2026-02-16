package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
)

const apiKeyPrefix = "fora_ak_"

func GenerateAPIKey() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	return apiKeyPrefix + hex.EncodeToString(raw), nil
}

func HashAPIKey(apiKey string) string {
	sum := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(sum[:])
}

func VerifyAPIKey(rawAPIKey, expectedHash string) bool {
	actual := HashAPIKey(rawAPIKey)
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expectedHash)) == 1
}

func BearerToken(authHeader string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(authHeader, prefix) {
		return ""
	}
	token := strings.TrimSpace(strings.TrimPrefix(authHeader, prefix))
	if token == "" {
		return ""
	}
	return token
}
