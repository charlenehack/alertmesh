// Renders the test-message endpoint's `mapping_resolved` map as a
// 3-column table (类型 / 表达式或路径 / 解析值).  Backend stamps expr
// cells with the `expr:` prefix so we can tag them visibly — that lets
// the operator immediately see which rows were computed by an
// expression vs which were a plain gjson lookup, without having to
// scroll back to the form.
//
// Empty / falsy values render as a muted `(空)` so it's clear the cell
// produced "" rather than "the request failed".  Sorted by key so two
// runs with the same config show rows in the same order.

import { useMemo } from 'react'
import { Tag, Typography } from 'antd'

import { EXPR_PREFIX } from './mappingValue'

const { Text } = Typography

export interface MappingHitsTableProps {
  hits: Record<string, string> | null | undefined
}

export function MappingHitsTable({ hits }: MappingHitsTableProps) {
  const rows = useMemo(() => {
    const entries = Object.entries(hits || {})
    entries.sort(([a], [b]) => {
      const ax = a.startsWith(EXPR_PREFIX) ? 1 : 0
      const bx = b.startsWith(EXPR_PREFIX) ? 1 : 0
      if (ax !== bx) return ax - bx
      return a.localeCompare(b)
    })
    return entries.map(([rawKey, value]) => {
      const isExpr = rawKey.startsWith(EXPR_PREFIX)
      const cleanKey = isExpr
        ? rawKey.slice(EXPR_PREFIX.length).replace(/^\s*expr:\s*/i, '').trimStart()
        : rawKey
      return { rawKey, isExpr, cleanKey, value }
    })
  }, [hits])

  if (rows.length === 0) {
    return <Text type="secondary" style={{ fontSize: 12 }}>无字段被解析（mapping 全空）。</Text>
  }
  return (
    <table style={{ width: '100%', fontSize: 12, borderCollapse: 'collapse' }}>
      <thead>
        <tr style={{ color: '#888' }}>
          <th style={{ textAlign: 'left', padding: '4px 8px', width: 64 }}>类型</th>
          <th style={{ textAlign: 'left', padding: '4px 8px' }}>路径 / 表达式</th>
          <th style={{ textAlign: 'left', padding: '4px 8px' }}>解析值</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((r) => (
          <tr key={r.rawKey} style={{ borderTop: '1px solid #1e1e1e' }}>
            <td style={{ padding: '4px 8px', verticalAlign: 'top' }}>
              {r.isExpr
                ? <Tag color="purple" style={{ marginRight: 0 }}>expr</Tag>
                : <Tag color="blue"   style={{ marginRight: 0 }}>gjson</Tag>}
            </td>
            <td style={{ padding: '4px 8px', fontFamily: 'monospace', wordBreak: 'break-all' }}>
              {r.cleanKey}
            </td>
            <td style={{ padding: '4px 8px', fontFamily: 'monospace', wordBreak: 'break-all' }}>
              {r.value === '' || r.value == null
                ? <Text type="secondary">(空)</Text>
                : <Text style={{ color: r.isExpr ? '#a371f7' : undefined }}>{r.value}</Text>}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
