import type { Severity } from '../types'

const config: Record<Severity, { bg: string; color: string; label: string }> = {
  P0: { bg: '#2a0000', color: '#ff4d4f', label: 'P0' },
  P1: { bg: '#2a1200', color: '#fa8c16', label: 'P1' },
  P2: { bg: '#2a2000', color: '#fadb14', label: 'P2' },
  P3: { bg: '#0d2a00', color: '#73d13d', label: 'P3' },
}

export default function SeverityBadge({ severity }: { severity: Severity }) {
  const { bg, color, label } = config[severity] ?? { bg: '#1a1a1a', color: '#888888', label: severity }
  return (
    <span
      style={{
        display: 'inline-block',
        padding: '2px 8px',
        borderRadius: 4,
        background: bg,
        color,
        fontSize: 11,
        fontWeight: 700,
        letterSpacing: '0.5px',
        border: `1px solid ${color}22`,
      }}
    >
      {label}
    </span>
  )
}
