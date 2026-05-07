import JSEncrypt from 'jsencrypt'
import { getPublicKey } from './system'

// Prefix that the server uses to detect wire-encrypted values.
// MUST match `auth.ENCPrefix` in internal/auth/wire_crypto.go.
const ENC_PREFIX = 'ENC:'

// Sentinel value returned by the server for masked secret fields in list /
// detail responses.  Forms re-submit it unchanged when the user did not
// touch the field — must NOT be re-encrypted, the server treats it as
// "keep existing".
const MASK = '******'

let cachedKeyPromise: Promise<string> | null = null

async function loadPublicKey(): Promise<string> {
  if (cachedKeyPromise) return cachedKeyPromise
  cachedKeyPromise = getPublicKey()
    .then((res) => res.public_key)
    .catch((err) => {
      cachedKeyPromise = null
      throw err
    })
  return cachedKeyPromise
}

// encryptSecret RSA-encrypts a single sensitive field for transit.
// Returns the value unchanged when:
//   - it is empty / undefined
//   - it is the masked placeholder (server keeps the existing DB value)
//   - it is already prefixed with ENC: (already encrypted)
// Throws if the public key cannot be loaded or jsencrypt fails.
export async function encryptSecret(
  value: string | undefined | null,
): Promise<string> {
  if (!value) return ''
  if (value === MASK) return MASK
  if (value.startsWith(ENC_PREFIX)) return value

  const publicKey = await loadPublicKey()
  const encryptor = new JSEncrypt()
  encryptor.setPublicKey(publicKey)
  const cipher = encryptor.encrypt(value)
  if (!cipher) {
    throw new Error('RSA encryption failed (public key not loaded?)')
  }
  return ENC_PREFIX + cipher
}

// encryptSecrets is a convenience that walks the given keys on the input
// object and replaces each one with its encryptSecret() result.  Returns
// a shallow copy of the input.
export async function encryptSecrets<T extends Record<string, unknown>>(
  values: T,
  keys: Array<keyof T>,
): Promise<T> {
  const out: Record<string, unknown> = { ...values }
  for (const k of keys) {
    const v = values[k]
    if (typeof v === 'string') {
      out[k as string] = await encryptSecret(v)
    }
  }
  return out as T
}

// encryptJSONSecrets parses a JSON config blob, RSA-encrypts the listed
// top-level string fields, and returns the re-serialised JSON.
//
// Useful for handlers that accept a single JSON payload containing several
// nested secrets (e.g. PUT /configs/auth with {bind_password, client_secret, …}).
// On parse failure the original blob is returned unchanged so the caller can
// surface a "invalid JSON" error from the server.
export async function encryptJSONSecrets(
  jsonBlob: string,
  keys: string[],
): Promise<string> {
  if (!jsonBlob) return jsonBlob
  let parsed: Record<string, unknown>
  try {
    parsed = JSON.parse(jsonBlob)
  } catch {
    return jsonBlob
  }
  for (const k of keys) {
    const v = parsed[k]
    if (typeof v === 'string') {
      parsed[k] = await encryptSecret(v)
    }
  }
  return JSON.stringify(parsed)
}
