// Package auth – RSA key management for login password encryption.
//
// The key pair is generated once during system bootstrap (sysconfig.Bootstrap)
// and stored in system_configs:
//   - "system.rsa_public_key"   – PEM, plain text, readable by anyone
//   - "security.rsa_private_key" – PEM, AES-256-GCM encrypted at rest
//
// At startup, providers.go calls InitRSAFromPEM with the decrypted private PEM,
// which derives the public key automatically.  All subsequent calls to
// GetPublicKeyPEM / DecryptPassword use the in-memory key pair.
package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"sync/atomic"
	"unsafe"
)

// rsaState holds the loaded key pair.
type rsaState struct {
	priv   *rsa.PrivateKey
	pubPEM string
}

// rsaPtr is an atomic pointer to the current rsaState.
// Using atomic.Pointer[rsaState] (Go 1.19+) for lock-free reads.
var rsaPtr atomic.Pointer[rsaState]

// GenerateRSAKeyPair creates a new RSA-2048 key pair and returns the
// PEM-encoded private key (PKCS#8) and public key (PKIX).
// Called by sysconfig.Bootstrap on first run.
func GenerateRSAKeyPair() (privatePEM, publicPEM string, err error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}

	// Private key → PKCS#8 DER → PEM
	privDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", "", err
	}
	privBlock := &pem.Block{Type: "PRIVATE KEY", Bytes: privDER}
	privatePEM = string(pem.EncodeToMemory(privBlock))

	// Public key → PKIX DER → PEM
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", "", err
	}
	pubBlock := &pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}
	publicPEM = string(pem.EncodeToMemory(pubBlock))

	return privatePEM, publicPEM, nil
}

// InitRSAFromPEM loads the RSA key pair from the given PKCS#8 PEM-encoded
// private key and makes it available for GetPublicKeyPEM / DecryptPassword.
// Called once at startup by providers.go after sysconfig.Bootstrap.
func InitRSAFromPEM(privatePEM string) error {
	block, _ := pem.Decode([]byte(privatePEM))
	if block == nil {
		return errors.New("auth: failed to decode RSA private key PEM block")
	}

	keyAny, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return errors.New("auth: failed to parse RSA PKCS#8 private key: " + err.Error())
	}
	rsaKey, ok := keyAny.(*rsa.PrivateKey)
	if !ok {
		return errors.New("auth: PEM key is not an RSA private key")
	}

	// Derive public key PEM from the private key.
	pubDER, err := x509.MarshalPKIXPublicKey(&rsaKey.PublicKey)
	if err != nil {
		return errors.New("auth: failed to marshal RSA public key")
	}
	pubPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))

	rsaPtr.Store(&rsaState{priv: rsaKey, pubPEM: pubPEM})
	return nil
}

// GetPublicKeyPEM returns the PEM-encoded RSA-2048 public key that was loaded
// at startup.  Returns an empty string if InitRSAFromPEM has not been called.
func GetPublicKeyPEM() string {
	s := rsaPtr.Load()
	if s == nil {
		return ""
	}
	return s.pubPEM
}

// DecryptCipher decrypts a PKCS1v15-encrypted, base64-standard-encoded
// ciphertext produced by the browser with GetPublicKeyPEM.
//
// Used both for login passwords and for sensitive form fields (e.g. contact
// webhook tokens / Slack bot tokens) that are encrypted client-side before
// being sent over the wire.
func DecryptCipher(cipherBase64 string) (string, error) {
	s := rsaPtr.Load()
	if s == nil {
		return "", errors.New("auth: RSA key pair not initialised")
	}

	cipherBytes, err := base64.StdEncoding.DecodeString(cipherBase64)
	if err != nil {
		return "", errors.New("auth: invalid base64 encoding")
	}

	plain, err := rsa.DecryptPKCS1v15(rand.Reader, s.priv, cipherBytes)
	if err != nil {
		return "", errors.New("auth: failed to decrypt cipher")
	}

	return string(plain), nil
}

// DecryptPassword is an alias kept for the login flow.
//
// Deprecated: use DecryptCipher.
func DecryptPassword(cipherBase64 string) (string, error) {
	return DecryptCipher(cipherBase64)
}

// Ensure rsaState is pointer-sized for atomic.Pointer (compile-time guard).
var _ = (*unsafe.Pointer)(nil)
