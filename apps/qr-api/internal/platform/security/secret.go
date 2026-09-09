package security

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
)

// ClientCredentials verifies a pair against the secret's SHA-256; no plaintext is stored.
type ClientCredentials struct {
	clientID     string
	secretSHA256 string
}

// NewClientCredentials builds the verifier; the 64-hex digest is checked at config load.
func NewClientCredentials(clientID, secretSHA256 string) *ClientCredentials {
	return &ClientCredentials{clientID: clientID, secretSHA256: strings.ToLower(secretSHA256)}
}

// Verify reports whether the pair is valid. Id and secret are compared in constant time
// and both comparisons always run, so timing cannot enumerate client ids.
func (c *ClientCredentials) Verify(clientID, clientSecret string) bool {
	idOK := subtle.ConstantTimeCompare([]byte(clientID), []byte(c.clientID)) == 1
	secretOK := VerifyClientSecret(clientSecret, c.secretSHA256)
	return idOK && secretOK
}

// VerifyClientSecret compares sha256(secret) with the stored hex digest in constant time.
func VerifyClientSecret(secret, storedHex string) bool {
	sum := sha256.Sum256([]byte(secret))
	stored, err := hex.DecodeString(storedHex)
	if err != nil {
		// Burn a comparison anyway: a malformed config must not become a timing oracle.
		subtle.ConstantTimeCompare(sum[:], sum[:])
		return false
	}
	return subtle.ConstantTimeCompare(sum[:], stored) == 1
}

// HashSecret returns the lowercase hex SHA-256 of a secret, as scripts/gen-secrets.sh does.
func HashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}
