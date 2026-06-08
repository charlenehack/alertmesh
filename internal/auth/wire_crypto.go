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

// HYBPrefix marks request payload values that the browser encrypted using
// the chunked RSA scheme (for values exceeding the RSA plaintext limit of
// 245 bytes).  Wire format:
//
//	HYB:<rsa_chunk1_b64>.<rsa_chunk2_b64>.<rsa_chunk3_b64>
//
// Each chunk is a separate RSA-PKCS1v15 ciphertext. The Go backend
// splits by ".", RSA-decrypts each chunk, and concatenates the results.
const HYBPrefix = "HYB:"

// HasENCPrefix reports whether v is a wire-encrypted secret (RSA-only).
func HasENCPrefix(v string) bool {
	return strings.HasPrefix(v, ENCPrefix)
}

// HasHYBPrefix reports whether v is a hybrid-encrypted secret (AES-GCM + RSA).
func HasHYBPrefix(v string) bool {
	return strings.HasPrefix(v, HYBPrefix)
}

// DecodeClientCipher returns the plaintext if v is prefixed with ENCPrefix
// or HYBPrefix and successfully decrypts; otherwise v is returned unchanged.
//
// This is the canonical entry point for "decrypt-this-field-if-the-client-
// encrypted-it" across all HTTP handlers.
func DecodeClientCipher(v string) string {
	if HasHYBPrefix(v) {
		plain, err := decodeHybrid(v)
		if err != nil {
			log.Warn().Err(err).Msg("auth: hybrid decrypt failed, treating value as plaintext")
			return v
		}
		return plain
	}
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

// decodeHybrid reverses the chunked RSA encryption performed by the
// browser's crypto.ts::chunkedRsaEncrypt().
//
// Wire format: "HYB:<rsa_chunk1_b64>.<rsa_chunk2_b64>.…"
func decodeHybrid(v string) (string, error) {
	payload := strings.TrimPrefix(v, HYBPrefix)
	if payload == "" {
		return "", errMalformedHybrid
	}

	chunks := strings.Split(payload, ".")
	if len(chunks) == 0 {
		return "", errMalformedHybrid
	}

	var plain strings.Builder
	for i, chunk := range chunks {
		plainChunk, err := DecryptCipher(chunk)
		if err != nil {
			return "", err
		}
		plain.WriteString(plainChunk)
		// Sanity guard: a single secret should never exceed 64 KiB.
		if plain.Len() > 64*1024 {
			return "", errMalformedHybrid
		}
		_ = i
	}

	return plain.String(), nil
}

var errMalformedHybrid = &malformedHybridError{}

type malformedHybridError struct{}

func (e *malformedHybridError) Error() string { return "auth: malformed hybrid ciphertext" }

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
