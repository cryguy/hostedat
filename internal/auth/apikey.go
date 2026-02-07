package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const apiKeyPrefix = "hd_"

func GenerateAPIKey() (rawKey, hash string, err error) {
	b := make([]byte, 30)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generating random bytes: %w", err)
	}
	rawKey = apiKeyPrefix + hex.EncodeToString(b)
	hash = HashAPIKey(rawKey)
	return rawKey, hash, nil
}

func HashAPIKey(rawKey string) string {
	h := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(h[:])
}
