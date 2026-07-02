package accounts

import (
	"crypto/rand"
	"math/big"
)

const (
	apiKeyPrefix = "sk-"
	apiKeyLength = 32
	charset      = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
)

// GenerateAPIKey generates a cryptographically random API key with the format
// "sk-" followed by 32 base62 characters (~190 bits of entropy).
func GenerateAPIKey() string {
	b := make([]byte, apiKeyLength)
	max := big.NewInt(int64(len(charset)))
	for i := range b {
		n, _ := rand.Int(rand.Reader, max)
		b[i] = charset[n.Int64()]
	}
	return apiKeyPrefix + string(b)
}
