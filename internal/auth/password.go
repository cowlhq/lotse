// Package auth checks who someone is: passwords, session tokens, and a
// brake on guessing.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id with the parameters OWASP recommends: 19 MiB, two passes, one
// thread. A login costs a few dozen milliseconds and those 19 MiB, which a
// small machine can afford for the few logins it sees.
const (
	argonMemory  = 19 * 1024
	argonTime    = 2
	argonThreads = 1
	argonKeyLen  = 32
	saltLen      = 16
)

// MinPasswordLength is the shortest password accepted.
const MinPasswordLength = 10

// HashPassword returns password hashed with Argon2id, in the PHC string
// format: $argon2id$v=19$m=19456,t=2,p=1$<salt>$<hash>.
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads, b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// CheckPassword says whether password matches hash. A hash made with other
// parameters is checked with its own.
func CheckPassword(hash, password string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var version int
	var memory, time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return false
	}
	b64 := base64.RawStdEncoding
	salt, err := b64.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := b64.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// CheckNewPassword says what is wrong with a password someone chose.
func CheckNewPassword(password string) error {
	if len([]rune(password)) < MinPasswordLength {
		return fmt.Errorf("a password needs at least %d characters", MinPasswordLength)
	}
	return nil
}

// NewToken returns a random token with a prefix that says what it is for,
// such as lotse_ses_, and the hash under which it is stored.
func NewToken(prefix string) (token, hash string) {
	b := make([]byte, 32)
	rand.Read(b)
	token = prefix + base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token)
}

// HashToken is how a token is stored: random tokens need no slow hash.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

var ErrTooManyAttempts = errors.New("too many failed attempts; try again later")
