import type { ReactNode } from 'react'
import { Space, Typography } from 'antd'
import { useTheme } from '../hooks/useTheme'

const { Title, Text } = Typography

interface PageHeaderProps {
  title: ReactNode
  icon?: ReactNode
  description?: ReactNode
  extra?: ReactNode
}

// Standard page chrome — adapts to dark / light theme automatically.
export function PageHeader({ title, icon, description, extra }: PageHeaderProps) {
  const { c } = useTheme()
  return (
    <div className="am-page-header">
      <Space size={10} align="center">
        {icon && <span style={{ fontSize: 18, color: c.primary, lineHeight: 0 }}>{icon}</span>}
        <Title level={5} style={{ margin: 0, color: c.textBody }}>{title}</Title>
        {description && (
          <Text type="secondary" style={{ fontSize: 12 }}>{description}</Text>
        )}
      </Space>
      {extra && <Space size={8}>{extra}</Space>}
    </div>
  )
}
