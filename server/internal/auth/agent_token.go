package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"
)

const agentTokenBytes = 32

// GenerateAgentToken creates a cryptographically random agent credential.
func GenerateAgentToken() (string, error) {
	return generateAgentToken(rand.Reader)
}

// HashAgentToken returns the one-way value persisted for an agent token.
func HashAgentToken(token string) []byte {
	hash := sha256.Sum256([]byte(token))
	return hash[:]
}

func generateAgentToken(random io.Reader) (string, error) {
	value := make([]byte, agentTokenBytes)
	if _, err := io.ReadFull(random, value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
