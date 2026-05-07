/**
 * Unified fetch wrapper for AlertMesh API.
 *
 * Features:
 *  - Auto-prepend /api/v1 base path
 *  - Inject Bearer token from auth store
 *  - Unified response parsing and error normalisation
 *  - 401 → auto logout + redirect to /login
 *  - Permission guard helpers
 */

import { useAuthStore } from '../store/auth'

const BASE = '/api/v1'

// ─── Types ────────────────────────────────────────────────────────────────────

// Wire envelope returned by every backend handler. Consumers of the
// `http.*` helpers only ever see the inner `data` field — the envelope
// is stripped by the `request<T>` core below.
interface ApiEnvelope<T = unknown> {
  code: number
  message: string
  data: T
}

// Re-exported for the rare call site that wants the full envelope shape.
export type ApiResponse<T = unknown> = ApiEnvelope<T>

/** Thrown when the server returns a non-2xx status or code !== 0. */
export class ApiError extends Error {
  status: number
  code: number

  constructor(status: number, code: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

// ─── Core fetch ───────────────────────────────────────────────────────────────

type Method = 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH'

interface RequestOptions {
  /** Skip attaching the Authorization header (e.g. login, public-key). */
  public?: boolean
  /** Additional headers to merge. */
  headers?: Record<string, string>
  /** Query-string parameters */
  params?: Record<string, string | number | boolean | undefined>
  /** Request body – will be JSON-serialised automatically. */
  body?: unknown
  signal?: AbortSignal
}

async function request<T>(
  method: Method,
  path: string,
  opts: RequestOptions = {},
): Promise<T> {
  let url = BASE + path
  if (opts.params) {
    const qs = new URLSearchParams()
    for (const [k, v] of Object.entries(opts.params)) {
      if (v !== undefined && v !== null) qs.set(k, String(v))
    }
    const str = qs.toString()
    if (str) url += '?' + str
  }

  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    Accept: 'application/json',
    ...opts.headers,
  }

  if (!opts.public) {
    const token = useAuthStore.getState().token
    if (token) headers['Authorization'] = `Bearer ${token}`
  }

  let res: Response
  try {
    res = await fetch(url, {
      method,
      headers,
      body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
      signal: opts.signal,
    })
  } catch (err) {
    throw new ApiError(0, -1, (err as Error).message || 'Network error')
  }

  if (res.status === 401) {
    useAuthStore.getState().logout()
    window.location.href = '/login'
    throw new ApiError(401, 401, 'Unauthorized')
  }

  let json: ApiEnvelope<T>
  try {
    json = await res.json()
  } catch {
    throw new ApiError(res.status, -1, `Non-JSON response (${res.status})`)
  }

  if (!res.ok) {
    throw new ApiError(res.status, json.code ?? res.status, json.message || res.statusText)
  }

  if (json.code !== 0) {
    throw new ApiError(res.status, json.code, json.message || 'API error')
  }

  return json.data
}

// ─── Convenience methods ──────────────────────────────────────────────────────
//
// The envelope is stripped here, so every `http.*` returns `Promise<T>`
// directly — call sites never write `.data.data` again.

export const http = {
  get: <T>(path: string, opts?: Omit<RequestOptions, 'body'>) =>
    request<T>('GET', path, opts),

  post: <T>(path: string, body?: unknown, opts?: Omit<RequestOptions, 'body'>) =>
    request<T>('POST', path, { ...opts, body }),

  put: <T>(path: string, body?: unknown, opts?: Omit<RequestOptions, 'body'>) =>
    request<T>('PUT', path, { ...opts, body }),

  delete: <T>(path: string, opts?: Omit<RequestOptions, 'body'>) =>
    request<T>('DELETE', path, opts),
}

// ─── Permission helpers ───────────────────────────────────────────────────────
//
// Re-export from the auth store so existing imports from `api/request`
// keep working. New code should import from `store/auth` directly.

export { hasPermission, hasRole } from '../store/auth'
