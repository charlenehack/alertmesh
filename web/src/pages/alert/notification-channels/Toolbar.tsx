import { Button, Input } from 'antd'
import { PlusOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons'
import type { ReactNode } from 'react'
import { useTheme } from '../../../hooks/useTheme'

interface ToolbarProps {
  createLabel: string
  onSearch: (v: string) => void
  onCreate: () => void
  onRefresh: () => void
  extra?: ReactNode
}

export function Toolbar({ createLabel, onSearch, onCreate, onRefresh, extra }: ToolbarProps) {
  const { c } = useTheme()
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 8,
        padding: '12px 16px',
        borderBottom: `1px solid ${c.border}`,
      }}
    >
      <Button
        type="primary"
        icon={<PlusOutlined />}
        onClick={onCreate}
        size="small"
      >
        {createLabel}
      </Button>
      {extra}
      <div style={{ flex: 1 }} />
      <Input
        placeholder="请输入名称搜索"
        prefix={<SearchOutlined style={{ color: c.textHint }} />}
        size="small"
        style={{ width: 200 }}
        allowClear
        onChange={(e) => onSearch(e.target.value)}
      />
      <Button
        icon={<ReloadOutlined />}
        size="small"
        type="text"
        onClick={onRefresh}
        style={{ color: c.textHint }}
      />
    </div>
  )
}

export function PageCard({ children }: { children: ReactNode }) {
  const { c } = useTheme()
  return (
    <div
      style={{
        background: c.bgSurface,
        border: `1px solid ${c.borderSubtle}`,
        borderRadius: 8,
        overflow: 'hidden',
      }}
    >
      {children}
    </div>
  )
}
