import type { ReactNode } from 'react'
import { Space, Typography } from 'antd'
import { colors } from '../theme/tokens'

const { Title, Text } = Typography

interface PageHeaderProps {
  title: ReactNode
  icon?: ReactNode
  description?: ReactNode
  extra?: ReactNode
}

// Standard page chrome — replaces the ~20 instances of an identical
// `<div style={{display:'flex',justifyContent:'space-between',marginBottom:20}}>`
// block scattered across the admin pages (DataSources, AlertRoutes,
// WebhookSources, NotificationTemplates, LLMProviders, ...).
export function PageHeader({ title, icon, description, extra }: PageHeaderProps) {
  return (
    <div className="am-page-header">
      <Space size={10} align="center">
        {icon && <span style={{ fontSize: 18, color: colors.textStrong, lineHeight: 0 }}>{icon}</span>}
        <Title level={5} style={{ margin: 0, color: colors.textStrong }}>{title}</Title>
        {description && (
          <Text type="secondary" style={{ fontSize: 12 }}>{description}</Text>
        )}
      </Space>
      {extra && <Space size={8}>{extra}</Space>}
    </div>
  )
}
