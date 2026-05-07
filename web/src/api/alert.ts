import { http } from './request'
import type {
  NotificationTemplate,
  AlertRoute, AggregationPolicy, SilencePolicy,
} from '../types'

// ─── Notification Templates ───────────────────────────────────────────────────
export const getTemplates = () => http.get<NotificationTemplate[]>('/alert/templates')
export const createTemplate = (data: Partial<NotificationTemplate>) => http.post<NotificationTemplate>('/alert/templates', data)
export const updateTemplate = (id: string, data: Partial<NotificationTemplate>) => http.put<NotificationTemplate>(`/alert/templates/${id}`, data)
export const deleteTemplate = (id: string) => http.delete<null>(`/alert/templates/${id}`)

// ─── Alert Routes ─────────────────────────────────────────────────────────────
export const getAlertRoutes = () => http.get<AlertRoute[]>('/alert/routes')
export const createAlertRoute = (data: Partial<AlertRoute>) => http.post<AlertRoute>('/alert/routes', data)
export const updateAlertRoute = (id: string, data: Partial<AlertRoute>) => http.put<AlertRoute>(`/alert/routes/${id}`, data)
export const deleteAlertRoute = (id: string) => http.delete<null>(`/alert/routes/${id}`)

// ─── Aggregation Policies ─────────────────────────────────────────────────────
export const getAggregations = () => http.get<AggregationPolicy[]>('/alert/aggregations')
export const createAggregation = (data: Partial<AggregationPolicy>) => http.post<AggregationPolicy>('/alert/aggregations', data)
export const updateAggregation = (id: string, data: Partial<AggregationPolicy>) => http.put<AggregationPolicy>(`/alert/aggregations/${id}`, data)
export const deleteAggregation = (id: string) => http.delete<null>(`/alert/aggregations/${id}`)

// ─── Silence Policies ─────────────────────────────────────────────────────────
export const getSilences = () => http.get<SilencePolicy[]>('/alert/silences')
export const createSilence = (data: Partial<SilencePolicy>) => http.post<SilencePolicy>('/alert/silences', data)
export const deleteSilence = (id: string) => http.delete<null>(`/alert/silences/${id}`)

// ─── Notification Policies (通知策略) ─────────────────────────────────────────
import type { NotificationPolicy, NotificationContact, NotificationContactGroup } from '../types'

export const getPolicies = () => http.get<NotificationPolicy[]>('/alert/policies')
export const createPolicy = (data: Partial<NotificationPolicy>) => http.post<NotificationPolicy>('/alert/policies', data)
export const updatePolicy = (id: string, data: Partial<NotificationPolicy>) => http.put<NotificationPolicy>(`/alert/policies/${id}`, data)
export const deletePolicy = (id: string) => http.delete<null>(`/alert/policies/${id}`)

// ─── Notification Contacts (联系人) ───────────────────────────────────────────
export const getContacts = () => http.get<NotificationContact[]>('/alert/contacts')
export const createContact = (data: Partial<NotificationContact>) => http.post<NotificationContact>('/alert/contacts', data)
export const updateContact = (id: string, data: Partial<NotificationContact>) => http.put<NotificationContact>(`/alert/contacts/${id}`, data)
export const deleteContact = (id: string) => http.delete<null>(`/alert/contacts/${id}`)

// ─── Notification Contact Groups (联系人组) ───────────────────────────────────
export const getContactGroups = () => http.get<NotificationContactGroup[]>('/alert/contact-groups')
export const createContactGroup = (data: Partial<NotificationContactGroup>) => http.post<NotificationContactGroup>('/alert/contact-groups', data)
export const updateContactGroup = (id: string, data: Partial<NotificationContactGroup>) => http.put<NotificationContactGroup>(`/alert/contact-groups/${id}`, data)
export const deleteContactGroup = (id: string) => http.delete<null>(`/alert/contact-groups/${id}`)

// ─── Webhook Sources (RFC 9421 trusted alert sources) ────────────────────────
import type { WebhookSource, WebhookSourceCreated } from '../types'

export const getWebhookSources = () => http.get<WebhookSource[]>('/alert/webhook-sources')
export const createWebhookSource = (data: Partial<WebhookSource>) =>
  http.post<WebhookSourceCreated>('/alert/webhook-sources', data)
export const updateWebhookSource = (id: string, data: Partial<WebhookSource>) =>
  http.put<WebhookSource>(`/alert/webhook-sources/${id}`, data)
export const rotateWebhookSourceKey = (id: string) =>
  http.post<WebhookSourceCreated>(`/alert/webhook-sources/${id}/rotate`, {})
export const deleteWebhookSource = (id: string) => http.delete<null>(`/alert/webhook-sources/${id}`)
