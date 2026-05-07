import { Suspense, useState } from 'react'
import { Layout, Menu, Avatar, Dropdown, Button, Space, Typography } from 'antd'
import {
  DashboardOutlined,
  AlertOutlined,
  UserOutlined,
  SettingOutlined,
  LogoutOutlined,
  BellOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  MergeCellsOutlined,
  StopOutlined,
  MessageOutlined,
  SoundOutlined,
  NodeIndexOutlined,
  ScheduleOutlined,
  ApiOutlined,
  RobotOutlined,
  DatabaseOutlined,
} from '@ant-design/icons'
import { useNavigate, useLocation, Outlet } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { useAuthStore } from '../store/auth'
import { hasRole, http } from '../api/request'
import { ErrorBoundary } from './ErrorBoundary'
import { RouteFallback } from './RouteFallback'
import type { SystemConfig } from '../types'

const { Sider, Header, Content } = Layout
const { Text } = Typography

export default function AppLayout() {
  const navigate = useNavigate()
  const location = useLocation()
  const { userInfo, logout } = useAuthStore()
  const [collapsed, setCollapsed] = useState(false)

  const isAdmin = hasRole('admin', 'superadmin')

  // Cached for 5 minutes — system version rarely changes; React Query
  // dedupes the call across remounts (was a bare fetch in useEffect).
  const { data: configs } = useQuery({
    queryKey: ['system-configs'],
    queryFn: () => http.get<SystemConfig[]>('/configs'),
    staleTime: 5 * 60_000,
  })
  const versionConfig = configs?.find((c) => c.key === 'system.version')
  const sysVersion = versionConfig ? 'v' + versionConfig.value : ''

  const handleLogout = () => {
    logout()
    navigate('/login')
  }

  const userMenu = {
    items: [
      {
        key: 'logout',
        icon: <LogoutOutlined />,
        label: '退出登录',
        danger: true,
        onClick: handleLogout,
      },
    ],
  }

  // Build menu items based on role
  const menuItems = [
    {
      key: 'overview',
      icon: <DashboardOutlined />,
      label: '概览',
      onClick: () => navigate('/'),
    },
    {
      key: 'alert-center',
      icon: <AlertOutlined />,
      label: '告警中心',
      children: [
        {
          key: '/incidents',
          icon: <BellOutlined />,
          label: '告警事件',
          onClick: () => navigate('/incidents'),
        },
        {
          key: '/alert/routes',
          icon: <NodeIndexOutlined />,
          label: '告警路由',
          onClick: () => navigate('/alert/routes'),
        },
        {
          key: '/alert/aggregations',
          icon: <MergeCellsOutlined />,
          label: '告警聚合策略',
          onClick: () => navigate('/alert/aggregations'),
        },
        {
          key: '/alert/silences',
          icon: <StopOutlined />,
          label: '告警静默策略',
          onClick: () => navigate('/alert/silences'),
        },
        {
          key: '/alert/channels',
          icon: <SoundOutlined />,
          label: '通知策略',
          onClick: () => navigate('/alert/channels'),
        },
        {
          key: '/alert/templates',
          icon: <MessageOutlined />,
          label: '通知消息模板',
          onClick: () => navigate('/alert/templates'),
        },
        {
          key: '/alert/webhook-sources',
          icon: <ApiOutlined />,
          label: 'Webhook 可信源',
          onClick: () => navigate('/alert/webhook-sources'),
        },
      ],
    },
    // System management – admin only
    ...(isAdmin
      ? [
          {
            key: 'system',
            icon: <SettingOutlined />,
            label: '系统管理',
            children: [
              {
                key: '/users',
                icon: <UserOutlined />,
                label: '用户管理',
                onClick: () => navigate('/users'),
              },
              {
                key: '/oncall',
                icon: <ScheduleOutlined />,
                label: '值班管理',
                onClick: () => navigate('/oncall'),
              },
              {
                key: '/settings',
                icon: <SettingOutlined />,
                label: '系统配置',
                onClick: () => navigate('/settings'),
              },
              {
                key: '/settings/llm-providers',
                icon: <RobotOutlined />,
                label: 'AI 大模型配置',
                onClick: () => navigate('/settings/llm-providers'),
              },
              {
                key: '/datasources',
                icon: <DatabaseOutlined />,
                label: '数据源',
                onClick: () => navigate('/datasources'),
              },
            ],
          },
        ]
      : []),
  ]

  // Determine selected / open keys from current path
  const selectedKeys = [location.pathname === '/' ? 'overview' : location.pathname]
  const defaultOpenKeys = ['alert-center', ...(isAdmin ? ['system'] : [])]

  return (
    <Layout style={{ minHeight: '100vh', background: '#0a0a0a' }}>
      <Sider
        trigger={null}
        collapsible
        collapsed={collapsed}
        width={220}
        style={{
          background: '#0d0d0d',
          borderRight: '1px solid #1a1a1a',
          position: 'fixed',
          height: '100vh',
          left: 0,
          top: 0,
          zIndex: 100,
        }}
      >
        {/* Logo */}
        <div
          style={{
            height: 56,
            display: 'flex',
            alignItems: 'center',
            justifyContent: collapsed ? 'center' : 'flex-start',
            padding: collapsed ? 0 : '0 20px',
            borderBottom: '1px solid #1a1a1a',
            gap: 10,
          }}
        >
          <svg width="22" height="22" viewBox="0 0 28 28" fill="none">
            <rect width="28" height="28" rx="5" fill="#ffffff" />
            <path
              d="M7 14h4l3-7 4 14 3-9 2 2h2"
              stroke="#000000"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </svg>
          {!collapsed && (
            <Text style={{ color: '#ffffff', fontSize: 15, fontWeight: 700, letterSpacing: '0.5px' }}>
              AlertMesh
            </Text>
          )}
        </div>

        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={selectedKeys}
          defaultOpenKeys={defaultOpenKeys}
          items={menuItems}
          style={{
            background: 'transparent',
            borderRight: 'none',
            marginTop: 4,
            fontSize: 13,
          }}
        />

        {!collapsed && sysVersion && (
          <div
            style={{
              position: 'absolute',
              bottom: 16,
              left: 0,
              right: 0,
              textAlign: 'center',
              color: '#2a2a2a',
              fontSize: 11,
            }}
          >
            {sysVersion}
          </div>
        )}
      </Sider>

      <Layout
        style={{
          marginLeft: collapsed ? 80 : 220,
          transition: 'margin-left 0.2s',
          background: '#0a0a0a',
        }}
      >
        <Header
          style={{
            background: '#0d0d0d',
            padding: '0 20px',
            height: 56,
            lineHeight: '56px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            borderBottom: '1px solid #1a1a1a',
            position: 'sticky',
            top: 0,
            zIndex: 99,
          }}
        >
          <Button
            type="text"
            icon={
              collapsed
                ? <MenuUnfoldOutlined style={{ color: '#666666' }} />
                : <MenuFoldOutlined style={{ color: '#666666' }} />
            }
            onClick={() => setCollapsed(!collapsed)}
            style={{ background: 'transparent' }}
          />

          <Space size={4}>
            <Dropdown menu={userMenu} placement="bottomRight">
              <Space style={{ cursor: 'pointer', padding: '4px 8px', borderRadius: 6 }}>
                <Avatar
                  size={28}
                  style={{
                    background: '#1a1a1a',
                    color: '#ffffff',
                    fontSize: 12,
                    fontWeight: 600,
                    border: '1px solid #333333',
                  }}
                >
                  {userInfo?.username?.[0]?.toUpperCase() ?? 'U'}
                </Avatar>
                <Text style={{ color: '#999999', fontSize: 13 }}>
                  {userInfo?.username ?? '用户'}
                </Text>
              </Space>
            </Dropdown>
          </Space>
        </Header>

        <Content style={{ padding: 24, background: '#0a0a0a', minHeight: 'calc(100vh - 56px)' }}>
          <ErrorBoundary resetKey={location.pathname}>
            <Suspense fallback={<RouteFallback />}>
              <Outlet />
            </Suspense>
          </ErrorBoundary>
        </Content>
      </Layout>
    </Layout>
  )
}
