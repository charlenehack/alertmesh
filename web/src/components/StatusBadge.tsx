import type { IncidentStatus } from '../types'

const config: Record<IncidentStatus, { dot: string; label: string }> = {
  open:        { dot: '#ff4d4f', label: '待处理' },
  ack:         { dot: '#faad14', label: '已确认' },
  in_progress: { dot: '#1677ff', label: '处理中' },
  resolved:    { dot: '#52c41a', label: '已解决' },
  closed:      { dot: '#444444', label: '已关闭' },
}

export default function StatusBadge({ status }: { status: IncidentStatus }) {
  const { dot, label } = config[status] ?? { dot: '#444444', label: status }
  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
      <span
        style={{
          width: 6,
          height: 6,
          borderRadius: '50%',
          background: dot,
          display: 'inline-block',
          flexShrink: 0,
        }}
      />
      <span style={{ color: '#999999', fontSize: 12 }}>{label}</span>
    </span>
  )
}
