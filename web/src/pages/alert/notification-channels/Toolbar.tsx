import { Button, Input } from 'antd'
import { PlusOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons'
import type { ReactNode } from 'react'

interface ToolbarProps {
  createLabel: string
  onSearch: (v: string) => void
  onCreate: () => void
  onRefresh: () => void
  extra?: ReactNode
}

export function Toolbar({ createLabel, onSearch, onCreate, onRefresh, extra }: ToolbarProps) {
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 8,
        padding: '12px 16px',
        borderBottom: '1px solid #1e1e1e',
      }}
    >
      <Button
        type="primary"
        icon={<PlusOutlined />}
        onClick={onCreate}
        size="small"
        style={{ background: '#1677ff', border: 'none' }}
      >
        {createLabel}
      </Button>
      {extra}
      <div style={{ flex: 1 }} />
      <Input
        placeholder="请输入名称搜索"
        prefix={<SearchOutlined style={{ color: '#555' }} />}
        size="small"
        style={{ width: 200, background: '#1a1a1a', border: '1px solid #2a2a2a', color: '#e8e8e8' }}
        allowClear
        onChange={(e) => onSearch(e.target.value)}
      />
      <Button
        icon={<ReloadOutlined />}
        size="small"
        type="text"
        onClick={onRefresh}
        style={{ color: '#666' }}
      />
    </div>
  )
}

export function PageCard({ children }: { children: ReactNode }) {
  return (
    <div
      style={{
        background: '#111111',
        border: '1px solid #1e1e1e',
        borderRadius: 8,
        overflow: 'hidden',
      }}
    >
      {children}
    </div>
  )
}
