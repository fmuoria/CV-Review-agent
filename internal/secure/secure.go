// Package secure holds small cryptographic helpers: encrypting stored OAuth
// refresh tokens and signing short-lived values such as OAuth state.
package secure

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"
)

// Box encrypts and signs values with keys derived from the app secret.
type Box struct{ secret []byte }

// New returns a Box for the given app secret.
func New(secret string) *Box { return &Box{secret: []byte(secret)} }

func (b *Box) key(purpose string) []byte {
	m := hmac.New(sha256.New, b.secret)
	m.Write([]byte(purpose))
	return m.Sum(nil)
}

// Encrypt seals plaintext with AES-256-GCM.
func (b *Box) Encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(b.key("encrypt"))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.RawURLEncoding.EncodeToString(out), nil
}

// Decrypt opens a value produced by Encrypt.
func (b *Box) Decrypt(sealed string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(sealed)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(b.key("encrypt"))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	pt, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", errors.New("could not decrypt (was APP_SECRET changed?)")
	}
	return string(pt), nil
}

// Sign returns value|expiry|mac, valid for ttl.
func (b *Box) Sign(value string, ttl time.Duration) string {
	exp := strconv.FormatInt(time.Now().Add(ttl).Unix(), 10)
	payload := value + "|" + exp
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + b.mac(payload)
}

// Verify checks a signed value and returns it if valid and unexpired.
func (b *Box) Verify(signed string) (string, error) {
	enc, mac, ok := strings.Cut(signed, ".")
	if !ok {
		return "", errors.New("malformed signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return "", err
	}
	payload := string(raw)
	if !hmac.Equal([]byte(mac), []byte(b.mac(payload))) {
		return "", errors.New("bad signature")
	}
	i := strings.LastIndex(payload, "|")
	if i < 0 {
		return "", errors.New("malformed payload")
	}
	exp, err := strconv.ParseInt(payload[i+1:], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return "", errors.New("expired")
	}
	return payload[:i], nil
}

func (b *Box) mac(payload string) string {
	m := hmac.New(sha256.New, b.key("sign"))
	m.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// RandomToken returns n random bytes, base64url-encoded.
func RandomToken(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}
