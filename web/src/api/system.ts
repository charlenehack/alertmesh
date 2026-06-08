import { http } from './request'
import type { User, UserInfo, Role, Endpoint, OncallSchedule, SystemConfig, AuthConfig } from '../types'

export const getPublicKey = () =>
  http.get<{ public_key: string }>('/auth/public-key', { public: true })

export const login = (username: string, encryptedPassword: string) =>
  http.post<{ token: string }>('/auth/login', { username, password: encryptedPassword }, { public: true })

export const getUserInfo = () =>
  http.get<UserInfo>('/user/info')

export const getUsers = () =>
  http.get<User[]>('/users')

export const createUser = (body: { username: string; password: string; display_name?: string; email?: string; role_ids?: number[] }) =>
  http.post<User>('/users', body)

export const updateUser = (id: string, body: { display_name?: string; email?: string; password?: string; is_active?: boolean; role_ids?: number[] }) =>
  http.put<User>(`/users/${id}`, body)

export const deleteUser = (id: string) =>
  http.delete<{ id: string }>(`/users/${id}`)

export const getRoles = () =>
  http.get<Role[]>('/roles')

export const updateRoleEndpoints = (id: number, identities: string[]) =>
  http.put<Role>(`/roles/${id}/endpoints`, { identities })

export const getEndpoints = () =>
  http.get<Endpoint[]>('/endpoints')

export const getOncall = () =>
  http.get<OncallSchedule[]>('/oncall')

export const getConfigs = () =>
  http.get<SystemConfig[]>('/configs')

export const updateConfig = (cfg: SystemConfig) =>
  http.put<SystemConfig>('/configs', cfg)

export const getAuthConfig = () =>
  http.get<AuthConfig>('/configs/auth')

export const setAuthConfig = (body: AuthConfig) =>
  http.put<AuthConfig>('/configs/auth', body)

export const getReportOverview = () =>
  http.get<unknown>('/reports/overview')
