package auth

import (
	"encoding/json"
	"strings"

	"github.com/rs/zerolog/log"
)

// ENCPrefix marks request payload values that the browser RSA-encrypted with
// the system public key (see GetPublicKeyPEM / web/src/api/crypto.ts).
//
// Server-side helpers strip the prefix, RSA-decrypt the remainder, and pass
// the plaintext to the business handler.  Values that do NOT carry the
// prefix are returned unchanged so the wire format remains backwards-
// compatible with plain-text clients during migration.
const ENCPrefix = "ENC:"

// HasENCPrefix reports whether v is a wire-encrypted secret.
func HasENCPrefix(v string) bool {
	return strings.HasPrefix(v, ENCPrefix)
}

// DecodeClientCipher returns the plaintext if v is prefixed with ENCPrefix
// and successfully RSA-decrypts; otherwise v is returned unchanged.
//
// This is the canonical entry point for "decrypt-this-field-if-the-client-
// encrypted-it" across all HTTP handlers.
func DecodeClientCipher(v string) string {
	if !HasENCPrefix(v) {
		return v
	}
	cipherB64 := strings.TrimPrefix(v, ENCPrefix)
	plain, err := DecryptCipher(cipherB64)
	if err != nil {
		log.Warn().Err(err).Msg("auth: RSA decrypt failed, treating value as plaintext")
		return v
	}
	return plain
}

// DecodeJSONClientCiphers walks an arbitrary JSON document and replaces every
// string value carrying ENCPrefix with its RSA-decrypted plaintext.
// Non-string values, plain strings, and ENCPrefix-strings whose ciphertext
// does not decrypt are left unchanged.
//
// Use this for handlers that accept a JSON blob containing one or more
// sensitive sub-fields (e.g. auth.ldap config: { bind_password: "ENC:…" }).
//
// Returns the original blob unchanged if it isn't valid JSON.
func DecodeJSONClientCiphers(jsonBlob string) string {
	if jsonBlob == "" {
		return jsonBlob
	}
	var parsed interface{}
	if err := json.Unmarshal([]byte(jsonBlob), &parsed); err != nil {
		// Not valid JSON – return as-is so handlers can surface a proper error.
		return jsonBlob
	}
	walked := walkDecodeENC(parsed)
	out, err := json.Marshal(walked)
	if err != nil {
		return jsonBlob
	}
	return string(out)
}

// walkDecodeENC recursively decodes ENC-prefixed strings in a parsed JSON value.
func walkDecodeENC(v interface{}) interface{} {
	switch x := v.(type) {
	case string:
		return DecodeClientCipher(x)
	case map[string]interface{}:
		for k, vv := range x {
			x[k] = walkDecodeENC(vv)
		}
		return x
	case []interface{}:
		for i, vv := range x {
			x[i] = walkDecodeENC(vv)
		}
		return x
	default:
		return v
	}
}
