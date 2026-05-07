import { lazy } from 'react'
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import {
  MutationCache,
  QueryCache,
  QueryClient,
  QueryClientProvider,
} from '@tanstack/react-query'
import { ConfigProvider, App as AntApp, Spin } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'
import 'dayjs/locale/zh-cn'

import AppLayout from './components/AppLayout'
import Login from './pages/Login'
import { surfaceApiError } from './api/notify'
import { useUserInfoHydration } from './hooks/useUserInfoHydration'
import { antdTheme } from './theme/antdTheme'
import { colors } from './theme/tokens'

const Dashboard = lazy(() => import('./pages/Dashboard'))
const IncidentList = lazy(() => import('./pages/incidents/IncidentList'))
const IncidentDetail = lazy(() => import('./pages/incidents/incident-detail'))
const NotificationChannels = lazy(() => import('./pages/alert/notification-channels'))
const NotificationTemplates = lazy(() => import('./pages/alert/NotificationTemplates'))
const AlertRoutes = lazy(() => import('./pages/alert/AlertRoutes'))
const AggregationPolicies = lazy(() => import('./pages/alert/AggregationPolicies'))
const SilencePolicies = lazy(() => import('./pages/alert/SilencePolicies'))
const WebhookSources = lazy(() => import('./pages/alert/WebhookSources'))
const UserList = lazy(() => import('./pages/users/UserList'))
const OncallList = lazy(() => import('./pages/oncall/OncallList'))
const SystemSettings = lazy(() => import('./pages/settings/SystemSettings'))
const LLMProviders = lazy(() => import('./pages/settings/LLMProviders'))
const DataSources = lazy(() => import('./pages/datasources/DataSources'))
const PromExplore = lazy(() => import('./pages/datasources/PromExplore'))

dayjs.extend(relativeTime)
dayjs.locale('zh-cn')

const qc = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      staleTime: 10_000,
      gcTime: 5 * 60_000,
      refetchOnWindowFocus: false,
    },
    mutations: { retry: 0 },
  },
  queryCache: new QueryCache({ onError: surfaceApiError }),
  mutationCache: new MutationCache({ onError: surfaceApiError }),
})

function RequireAuth({ children }: { children: React.ReactNode }) {
  const { token, hydrating } = useUserInfoHydration()

  if (!token) return <Navigate to="/login" replace />
  if (hydrating) {
    return (
      <div
        style={{
          minHeight: '100vh',
          background: colors.bgPage,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
        }}
      >
        <Spin />
      </div>
    )
  }
  return <>{children}</>
}

export default function App() {
  return (
    <QueryClientProvider client={qc}>
      <ConfigProvider locale={zhCN} theme={antdTheme}>
        <AntApp>
          <BrowserRouter>
            <Routes>
              <Route path="/login" element={<Login />} />
              <Route
                path="/"
                element={
                  <RequireAuth>
                    <AppLayout />
                  </RequireAuth>
                }
              >
                <Route index element={<Dashboard />} />

                {/* Alert Center */}
                <Route path="incidents" element={<IncidentList />} />
                <Route path="incidents/:id" element={<IncidentDetail />} />
                <Route path="alert/routes" element={<AlertRoutes />} />
                <Route path="alert/aggregations" element={<AggregationPolicies />} />
                <Route path="alert/silences" element={<SilencePolicies />} />
                <Route path="alert/channels" element={<NotificationChannels />} />
                <Route path="alert/templates" element={<NotificationTemplates />} />
                <Route path="alert/webhook-sources" element={<WebhookSources />} />

                {/* System – admin only (frontend guards in AppLayout) */}
                <Route path="users" element={<UserList />} />
                <Route path="oncall" element={<OncallList />} />
                <Route path="settings" element={<SystemSettings />} />
                <Route path="settings/llm-providers" element={<LLMProviders />} />
                <Route path="datasources" element={<DataSources />} />
                <Route path="datasources/:id/prom-explore" element={<PromExplore />} />
              </Route>
              <Route path="*" element={<Navigate to="/" replace />} />
            </Routes>
          </BrowserRouter>
        </AntApp>
      </ConfigProvider>
    </QueryClientProvider>
  )
}
