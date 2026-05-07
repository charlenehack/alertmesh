import { useMemo } from 'react'
import { Col, Divider, Empty, Row, Typography } from 'antd'
import type { Alert as AlertRow } from '../../../types'
import { decodeRawPayload } from './helpers'

const { Text } = Typography

interface KeyValueListProps {
  data: Record<string, string> | null | undefined
  emptyText?: string
}

// Renders a Record<string,string> as stacked key→value rows. Sorting keys
// alphabetically keeps the rendering stable across reloads (so operators
// can train muscle memory on where each field sits) and across repeated
// alerts in the same incident.
export function KeyValueList({ data, emptyText }: KeyValueListProps) {
  const entries = useMemo(() => {
    if (!data) return []
    return Object.entries(data).sort(([a], [b]) => a.localeCompare(b))
  }, [data])

  if (entries.length === 0) {
    return (
      <Empty
        image={Empty.PRESENTED_IMAGE_SIMPLE}
        description={<Text type="secondary" style={{ fontSize: 12 }}>{emptyText ?? '无'}</Text>}
        style={{ margin: '8px 0' }}
      />
    )
  }

  return (
    <div>
      {entries.map(([k, v]) => (
        <Row key={k} gutter={8} style={{ marginBottom: 6 }} wrap={false}>
          <Col flex="160px" style={{ minWidth: 0 }}>
            <Text type="secondary" style={{ fontSize: 12 }} ellipsis={{ tooltip: k }}>
              {k}
            </Text>
          </Col>
          <Col flex="auto" style={{ minWidth: 0 }}>
            <ValueCell value={v} />
          </Col>
        </Row>
      ))}
    </div>
  )
}

// ValueCell decides whether a single key→value can fit on one line as a
// <Text code> chip or needs the antd collapsible ellipsis treatment.
//
// Heuristic: anything with a newline OR longer than 80 chars goes into the
// collapsible Paragraph (which renders multi-line JSON nicely once expanded
// thanks to white-space: pre-wrap).
function ValueCell({ value }: { value: string }) {
  const v = value ?? ''
  const isLong = v.length > 80 || /\r|\n/.test(v)

  if (!isLong) {
    return (
      <Text code style={{ fontSize: 12, wordBreak: 'break-all' }}>
        {v || '—'}
      </Text>
    )
  }

  return (
    <Typography.Paragraph
      code
      style={{ marginBottom: 0, fontSize: 12, whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}
      ellipsis={{
        rows: 1,
        expandable: 'collapsible',
        symbol: (expanded: boolean) => (expanded ? '收起' : '展开'),
      }}
    >
      {v}
    </Typography.Paragraph>
  )
}

// AlertExpandedPanel is the per-row expansion shown in the 告警列表 tab.
// Layout: two columns (labels | annotations) + an optional raw_payload
// section below them.
export function AlertExpandedPanel({ alert }: { alert: AlertRow }) {
  const rawPretty = useMemo(() => decodeRawPayload(alert.raw_payload), [alert.raw_payload])

  return (
    <div style={{ padding: '4px 8px 8px' }}>
      <Row gutter={16}>
        <Col xs={24} lg={12}>
          <Text type="secondary" style={{ fontSize: 12 }}>labels</Text>
          <div style={{ marginTop: 6 }}>
            <KeyValueList data={alert.labels} emptyText="无 labels" />
          </div>
        </Col>
        <Col xs={24} lg={12}>
          <Text type="secondary" style={{ fontSize: 12 }}>annotations</Text>
          <div style={{ marginTop: 6 }}>
            <KeyValueList data={alert.annotations} emptyText="无 annotations" />
          </div>
        </Col>
      </Row>
      {rawPretty && (
        <>
          <Divider style={{ margin: '12px 0 8px' }} />
          <Text type="secondary" style={{ fontSize: 12 }}>raw_payload</Text>
          <div style={{ marginTop: 6 }}>
            <Typography.Paragraph
              code
              style={{ marginBottom: 0, fontSize: 12, whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}
              ellipsis={{
                rows: 2,
                expandable: 'collapsible',
                symbol: (expanded: boolean) => (expanded ? '收起' : '展开'),
              }}
            >
              {rawPretty}
            </Typography.Paragraph>
          </div>
        </>
      )}
    </div>
  )
}
