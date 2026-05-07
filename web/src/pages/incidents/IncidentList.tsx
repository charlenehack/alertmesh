import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Table, Card, Tag, Button, Space, Input, Select, Typography, Tooltip
} from 'antd'
import { SearchOutlined, ReloadOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import dayjs from 'dayjs'
import { getIncidents } from '../../api/incidents'
import { useRealtime } from '../../hooks/useRealtime'
import SeverityBadge from '../../components/SeverityBadge'
import StatusBadge from '../../components/StatusBadge'
import type { Incident, Severity, IncidentStatus } from '../../types'

const { Title } = Typography
const { Option } = Select

const PAGE_SIZE = 20

export default function IncidentList() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const [page, setPage] = useState(1)
  const [search, setSearch] = useState('')
  const [severityFilter, setSeverityFilter] = useState<string>('')
  const [statusFilter, setStatusFilter] = useState<string>('')

  const { data, isLoading, refetch } = useQuery({
    queryKey: ['incidents', page],
    queryFn: () => getIncidents((page - 1) * PAGE_SIZE, PAGE_SIZE),
  })

  // Push channel — every incident lifecycle event server-side fires a
  // pg_notify('incident_event') that the realtime hub fans out here.
  // Replaces the 20s refetchInterval that was hammering /api/v1/incidents
  // even when nothing changed.  We invalidate the entire `incidents`
  // query family so all paginated views stay consistent.
  useRealtime(['incidents'], () => {
    qc.invalidateQueries({ queryKey: ['incidents'] })
  })

  const allItems: Incident[] = data?.items ?? []
  const total = data?.total ?? 0

  const filtered = allItems.filter((inc) => {
    const matchSearch = !search || inc.title.toLowerCase().includes(search.toLowerCase())
    const matchSeverity = !severityFilter || inc.severity === severityFilter
    const matchStatus = !statusFilter || inc.status === statusFilter
    return matchSearch && matchSeverity && matchStatus
  })

  const columns = [
    {
      title: '严重度',
      dataIndex: 'severity',
      width: 110,
      render: (s: Severity) => <SeverityBadge severity={s} />,
    },
    {
      title: '事件标题',
      dataIndex: 'title',
      render: (t: string, row: Incident) => (
        <a
          onClick={() => navigate(`/incidents/${row.id}`)}
          style={{ fontWeight: 500, color: '#1a1a2e' }}
        >
          {t}
        </a>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 110,
      render: (s: IncidentStatus) => <StatusBadge status={s} />,
    },
    {
      title: '来源',
      dataIndex: 'source',
      width: 130,
      render: (s: string) => <Tag style={{ borderRadius: 4 }}>{s}</Tag>,
    },
    {
      title: 'AI 分析',
      dataIndex: 'ai_status',
      width: 100,
      render: (s: string) => {
        const map: Record<string, { color: string; label: string }> = {
          pending: { color: 'default', label: '未分析' },
          running: { color: 'processing', label: '分析中' },
          done: { color: 'success', label: '已完成' },
          failed: { color: 'error', label: '失败' },
        }
        const { color, label } = map[s] ?? { color: 'default', label: s }
        return <Tag color={color}>{label}</Tag>
      },
    },
    {
      title: '开始时间',
      dataIndex: 'opened_at',
      width: 160,
      render: (t: string) => (
        <Tooltip title={dayjs(t).format('YYYY-MM-DD HH:mm:ss')}>
          <span style={{ fontSize: 12, color: '#666' }}>{dayjs(t).fromNow()}</span>
        </Tooltip>
      ),
    },
    {
      title: '操作',
      width: 80,
      render: (_: unknown, row: Incident) => (
        <Button size="small" type="link" onClick={() => navigate(`/incidents/${row.id}`)}>
          详情
        </Button>
      ),
    },
  ]

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
        <Title level={4} style={{ margin: 0, color: '#1a1a2e' }}>
          事件管理
        </Title>
        <Button icon={<ReloadOutlined />} onClick={() => refetch()}>
          刷新
        </Button>
      </div>

      <Card
        style={{ borderRadius: 12, border: 'none', boxShadow: '0 2px 12px rgba(0,0,0,0.06)' }}
      >
        <Space style={{ marginBottom: 16 }} wrap>
          <Input
            placeholder="搜索事件标题"
            prefix={<SearchOutlined />}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            style={{ width: 220 }}
            allowClear
          />
          <Select
            placeholder="严重度"
            value={severityFilter || undefined}
            onChange={(v) => setSeverityFilter(v ?? '')}
            allowClear
            style={{ width: 120 }}
          >
            {['P0', 'P1', 'P2', 'P3'].map((s) => (
              <Option key={s} value={s}>{s}</Option>
            ))}
          </Select>
          <Select
            placeholder="状态"
            value={statusFilter || undefined}
            onChange={(v) => setStatusFilter(v ?? '')}
            allowClear
            style={{ width: 130 }}
          >
            {[
              { value: 'open', label: '待处理' },
              { value: 'ack', label: '已确认' },
              { value: 'in_progress', label: '处理中' },
              { value: 'resolved', label: '已解决' },
              { value: 'closed', label: '已关闭' },
            ].map((s) => (
              <Option key={s.value} value={s.value}>{s.label}</Option>
            ))}
          </Select>
        </Space>

        <Table
          dataSource={filtered}
          columns={columns}
          rowKey="id"
          loading={isLoading}
          pagination={{
            current: page,
            pageSize: PAGE_SIZE,
            total,
            onChange: (p) => setPage(p),
            showTotal: (t) => `共 ${t} 条`,
            showSizeChanger: false,
          }}
          onRow={(record) => ({
            onClick: () => navigate(`/incidents/${record.id}`),
            style: { cursor: 'pointer' },
          })}
          rowClassName={(_, idx) => (idx % 2 === 0 ? '' : 'ant-table-row-striped')}
        />
      </Card>
    </div>
  )
}
