import type { CSSProperties, ReactNode } from 'react'
import { Card } from 'antd'
import { useTheme } from '../hooks/useTheme'

interface SurfaceCardProps {
  children?: ReactNode
  title?: ReactNode
  extra?: ReactNode
  /** When true, the body has zero padding — useful for flush <Table /> cards. */
  flush?: boolean
  style?: CSSProperties
  className?: string
}

// Antd Card with the standard surface + border + radius.
// Automatically adapts to dark / light theme via useTheme().
export function SurfaceCard({ children, title, extra, flush, style, className }: SurfaceCardProps) {
  const { c } = useTheme()
  return (
    <Card
      title={title}
      extra={extra}
      className={className}
      style={{
        background: c.bgSurface,
        border: `1px solid ${c.borderSubtle}`,
        borderRadius: 8,
        ...style,
      }}
      styles={flush ? { body: { padding: 0 } } : undefined}
    >
      {children}
    </Card>
  )
}
