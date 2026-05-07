import { App, message as staticMessage } from 'antd'
import { ApiError } from './request'

// Shared sink for query/mutation errors. Wired into React Query's
// QueryCache + MutationCache so individual pages no longer have to
// repeat `onError: (e) => message.error(e.message)`.
export function surfaceApiError(error: unknown) {
  if (error instanceof ApiError && error.code === 401) return
  const text = toUserMessage(error)
  staticMessage.error(text)
}

export function toUserMessage(error: unknown): string {
  if (error instanceof ApiError) return error.message || '请求失败'
  if (error instanceof Error) return error.message || '未知错误'
  return '未知错误'
}

// Helper for components that want to surface errors via the contextual
// App.useApp() message instance (preferred over the static one inside
// ConfigProvider scope). Returns a function with the same signature as
// `surfaceApiError`.
export function useErrorNotifier() {
  const { message } = App.useApp()
  return (error: unknown) => {
    if (error instanceof ApiError && error.code === 401) return
    message.error(toUserMessage(error))
  }
}
