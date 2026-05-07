import { http } from './request'
import type { User, UserInfo, OncallSchedule, SystemConfig, AuthConfig } from '../types'

export const getPublicKey = () =>
  http.get<{ public_key: string }>('/auth/public-key', { public: true })

export const login = (username: string, encryptedPassword: string) =>
  http.post<{ token: string }>('/auth/login', { username, password: encryptedPassword }, { public: true })

export const getUserInfo = () =>
  http.get<UserInfo>('/user/info')

export const getUsers = () =>
  http.get<User[]>('/users')

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
