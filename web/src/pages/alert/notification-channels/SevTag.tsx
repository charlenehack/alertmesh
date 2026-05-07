import { Tag } from 'antd'
import type { Severity } from '../../../types'
import { SEV_COLOR } from './sevColor'

export function SevTag({ s }: { s: Severity }) {
  return (
    <Tag
      style={{
        background: 'transparent',
        border: `1px solid ${SEV_COLOR[s]}`,
        color: SEV_COLOR[s],
        fontSize: 11,
        fontWeight: 700,
        padding: '0 5px',
        lineHeight: '18px',
        borderRadius: 3,
      }}
    >
      {s}
    </Tag>
  )
}
