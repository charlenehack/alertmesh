import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Row, Col, Card, Table, Tag, Typography, Spin, Empty } from 'antd'
import { useNavigate } from 'react-router-dom'
import dayjs from 'dayjs'
import { getIncidents } from '../api/incidents'
import { useRealtime } from '../hooks/useRealtime'
import { useTheme } from '../hooks/useTheme'
import SeverityBadge from '../components/SeverityBadge'
import StatusBadge from '../components/StatusBadge'
import type { Incident, Severity, IncidentStatus } from '../types'

const { Title, Text } = Typography

interface StatCardProps {
  label: string
  value: number
  accent?: string
}

function StatCard({ label, value, accent = '#1677ff' }: StatCardProps) {
  const { c } = useTheme()
  return (
    <Card
      style={{
        background: c.bgSurface,
        border: `1px solid ${c.borderSubtle}`,
        borderRadius: 8,
      }}
      styles={{ body: { padding: '20px 24px' } }}
    >
      <div style={{ color: c.textTertiary, fontSize: 12, marginBottom: 8, letterSpacing: '0.5px' }}>
        {label.toUpperCase()}
      </div>
      <div style={{ fontSize: 32, fontWeight: 700, color: accent, lineHeight: 1 }}>
        {value}
      </div>
    </Card>
  )
}

export default function Dashboard() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const { c } = useTheme()

  const { data, isLoading } = useQuery({
    queryKey: ['incidents-all'],
    queryFn: () => getIncidents(0, 200),
  })

  useRealtime(['incidents'], () => {
    qc.invalidateQueries({ queryKey: ['incidents-all'] })
  })

  const incidents: Incident[] = data?.items ?? []

  const counts = incidents.reduce(
    (acc, inc) => {
      acc.total++
      if (inc.status === 'open') acc.open++
      if (inc.status === 'ack' || inc.status === 'in_progress') acc.inProgress++
      if (inc.status === 'resolved') acc.resolved++
      if (inc.severity === 'P0') acc.p0++
      return acc
    },
    { total: 0, open: 0, inProgress: 0, resolved: 0, p0: 0 },
  )

  const recentOpen = incidents
    .filter((i) => i.status === 'open' || i.status === 'ack')
    .slice(0, 10)

  const columns = [
    {
      title: 'SEV',
      dataIndex: 'severity',
      width: 90,
      render: (s: Severity) => <SeverityBadge severity={s} />,
    },
    {
      title: '事件标题',
      dataIndex: 'title',
      render: (t: string, row: Incident) => (
        <span
          onClick={() => navigate(`/incidents/${row.id}`)}
          style={{ color: c.textBody, fontWeight: 500, cursor: 'pointer' }}
        >
          {t}
        </span>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 100,
      render: (s: IncidentStatus) => <StatusBadge status={s} />,
    },
    {
      title: '来源',
      dataIndex: 'source',
      width: 120,
      render: (s: string) => (
        <Tag
          style={{
            background: c.bgElevated,
            border: `1px solid ${c.border}`,
            color: c.textSecondary,
            fontSize: 11,
            borderRadius: 4,
          }}
        >
          {s}
        </Tag>
      ),
    },
    {
      title: '开始时间',
      dataIndex: 'opened_at',
      width: 140,
      render: (t: string) => (
        <Text style={{ color: c.textTertiary, fontSize: 12 }}>
          {dayjs(t).fromNow()}
        </Text>
      ),
    },
  ]

  return (
    <div>
      <div style={{ marginBottom: 24 }}>
        <Title level={5} style={{ color: c.textBody, margin: 0, fontWeight: 600 }}>
          概览
        </Title>
        <Text style={{ color: c.textTertiary, fontSize: 12 }}>
          {dayjs().format('YYYY年MM月DD日')}
        </Text>
      </div>

      <Row gutter={[12, 12]} style={{ marginBottom: 24 }}>
        <Col xs={12} lg={6}>
          <StatCard label="待处理" value={counts.open} accent="#ff4d4f" />
        </Col>
        <Col xs={12} lg={6}>
          <StatCard label="处理中" value={counts.inProgress} accent="#faad14" />
        </Col>
        <Col xs={12} lg={6}>
          <StatCard label="已解决" value={counts.resolved} accent="#52c41a" />
        </Col>
        <Col xs={12} lg={6}>
          <StatCard label="P0 紧急" value={counts.p0} accent="#ff4d4f" />
        </Col>
      </Row>

      <Card
        style={{
          background: c.bgSurface,
          border: `1px solid ${c.borderSubtle}`,
          borderRadius: 8,
        }}
        styles={{ body: { padding: 0 } }}
        title={
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <span style={{ color: c.textBody, fontSize: 13, fontWeight: 500 }}>活跃事件</span>
            <span
              onClick={() => navigate('/incidents')}
              style={{ color: c.textTertiary, fontSize: 12, cursor: 'pointer' }}
            >
              全部 →
            </span>
          </div>
        }
      >
        {isLoading ? (
          <div style={{ textAlign: 'center', padding: 48 }}>
            <Spin />
          </div>
        ) : recentOpen.length === 0 ? (
          <Empty description={<span style={{ color: c.textTertiary }}>暂无活跃事件</span>} style={{ padding: 48 }} />
        ) : (
          <Table
            dataSource={recentOpen}
            columns={columns}
            rowKey="id"
            pagination={false}
            size="small"
            onRow={(record) => ({
              onClick: () => navigate(`/incidents/${record.id}`),
              style: { cursor: 'pointer' },
            })}
          />
        )}
      </Card>
    </div>
  )
}
