import { http } from './request'
import type { Incident, PagedData } from '../types'

export const getIncidents = (offset = 0, limit = 20) =>
  http.get<PagedData<Incident>>('/incidents', { params: { offset, limit } })

export const getIncident = (id: string) =>
  http.get<Incident>(`/incidents/${id}`)

export const ackIncident = (id: string) =>
  http.post<null>(`/incidents/${id}/ack`)

export const resolveIncident = (id: string) =>
  http.post<null>(`/incidents/${id}/resolve`)

export const closeIncident = (id: string) =>
  http.post<null>(`/incidents/${id}/close`)

export const triggerAI = (id: string) =>
  http.post<{ status: string }>(`/incidents/${id}/ai/trigger`)

export const getAIReport = (id: string) =>
  http.get<unknown>(`/incidents/${id}/ai`)
