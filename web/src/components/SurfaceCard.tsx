import type { CSSProperties, ReactNode } from 'react'
import { Card } from 'antd'
import { colors } from '../theme/tokens'

interface SurfaceCardProps {
  children?: ReactNode
  title?: ReactNode
  extra?: ReactNode
  /** When true, the body has zero padding — useful for flush <Table /> cards. */
  flush?: boolean
  style?: CSSProperties
  className?: string
}

// Antd Card with the standard dark surface + border + radius the project
// uses everywhere. Replaces the 15+ inline
// `style={{ background: '#111111', border: '1px solid #1e1e1e', borderRadius: 8 }}`
// chunks across pages.
export function SurfaceCard({ children, title, extra, flush, style, className }: SurfaceCardProps) {
  return (
    <Card
      title={title}
      extra={extra}
      className={className}
      style={{
        background: colors.bgSurface,
        border: `1px solid ${colors.borderSubtle}`,
        borderRadius: 8,
        ...style,
      }}
      styles={flush ? { body: { padding: 0 } } : undefined}
    >
      {children}
    </Card>
  )
}
