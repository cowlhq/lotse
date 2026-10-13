// Package secret seals values the hub keeps but must read again, such as
// enrollment tokens, so that a copy of the database alone does not give
// them away.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Box seals with AES-256-GCM.
type Box struct{ aead cipher.AEAD }

// NewBox makes a box from a key of any length; it is hashed to 256 bits.
func NewBox(key string) (*Box, error) {
	if len(key) < 16 {
		return nil, errors.New("the secret key must have at least 16 characters")
	}
	k := sha256.Sum256([]byte("lotse secret v1\x00" + key))
	block, err := aes.NewCipher(k[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// LoadKey returns the key in env if set, or else the one in
// dir/secret.key, which it makes on first use.
func LoadKey(env, dir string) (string, error) {
	if env != "" {
		return env, nil
	}
	path := filepath.Join(dir, "secret.key")
	b, err := os.ReadFile(path)
	if err == nil {
		return strings.TrimSpace(string(b)), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	raw := make([]byte, 32)
	rand.Read(raw)
	key := base64.RawURLEncoding.EncodeToString(raw)
	// O_EXCL: if two start at once, one wins and the other reads its key.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return LoadKey("", dir)
	}
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.WriteString(key + "\n"); err != nil {
		return "", err
	}
	return key, nil
}

// Seal returns plain sealed, as text.
func (b *Box) Seal(plain string) string {
	nonce := make([]byte, b.aead.NonceSize())
	rand.Read(nonce)
	return base64.RawStdEncoding.EncodeToString(b.aead.Seal(nonce, nonce, []byte(plain), nil))
}

// Open returns what Seal sealed.
func (b *Box) Open(sealed string) (string, error) {
	raw, err := base64.RawStdEncoding.DecodeString(sealed)
	if err != nil || len(raw) < b.aead.NonceSize() {
		return "", errors.New("not a sealed value")
	}
	n := b.aead.NonceSize()
	plain, err := b.aead.Open(nil, raw[:n], raw[n:], nil)
	if err != nil {
		return "", errors.New("cannot open a sealed value: is it the hub's secret key?")
	}
	return string(plain), nil
}
