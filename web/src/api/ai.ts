import { http } from './request'
import type { LLMProvider, LLMProviderTestResult } from '../types'

// ─── LLM Providers (admin-only) ───────────────────────────────────────────────
// The api_key field is masked as "******" in every list/get response.
// Send "******" or empty on update to keep the existing ciphertext.

export const getLLMProviders = () => http.get<LLMProvider[]>('/llm-providers')

export const createLLMProvider = (data: Partial<LLMProvider>) =>
  http.post<LLMProvider>('/llm-providers', data)

export const updateLLMProvider = (id: string, data: Partial<LLMProvider>) =>
  http.put<LLMProvider>(`/llm-providers/${id}`, data)

export const deleteLLMProvider = (id: string) =>
  http.delete<null>(`/llm-providers/${id}`)

export const setDefaultLLMProvider = (id: string) =>
  http.post<{ id: string; status: string }>(`/llm-providers/${id}/set-default`)

// `id` may be the literal "new" when the operator wants to validate an
// unsaved row; the backend then uses the inline body fields instead of the
// stored ciphertext.
export const testLLMProvider = (
  id: string,
  body?: { base_url?: string; model?: string; api_key?: string; provider?: string },
) => http.post<LLMProviderTestResult>(`/llm-providers/${id || 'new'}/test`, body ?? {})
