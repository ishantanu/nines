package model

import (
	"encoding/hex"
	"encoding/json"

	"golang.org/x/crypto/argon2"
)

// CredentialIdentity derives a stable change-detection identity, including for
// low-entropy Basic Auth passwords. It is not a password verifier or a stored
// authentication credential. The public domain salt preserves restart-stable
// identities; credential stores retain their own encryption and password salts.
// Call at connection construction rather than on each telemetry read.
func CredentialIdentity(parts ...string) string {
	data, _ := json.Marshal(parts)
	key := argon2.IDKey(data, []byte("nines-credential-identity-v1"), 2, 19*1024, 1, 32)
	return hex.EncodeToString(key)
}
