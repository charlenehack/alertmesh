import { useQuery } from '@tanstack/react-query'
import { Table, Card, Tag, Typography } from 'antd'
import { CalendarOutlined } from '@ant-design/icons'
import dayjs from 'dayjs'
import { getOncall } from '../../api/system'
import { useTheme } from '../../hooks/useTheme'
import type { OncallSchedule } from '../../types'

const { Title } = Typography

export default function OncallList() {
  const { data, isLoading } = useQuery({
    queryKey: ['oncall'],
    queryFn: getOncall,
  })
  const { c } = useTheme()
  const schedules: OncallSchedule[] = data ?? []

  const now = dayjs()

  const columns = [
    {
      title: '值班人 ID',
      dataIndex: 'user_id',
      render: (u: string) => <Tag color="purple">{u.slice(0, 8)}…</Tag>,
    },
    {
      title: '开始时间',
      dataIndex: 'start_time',
      render: (t: string) => dayjs(t).format('YYYY-MM-DD HH:mm'),
    },
    {
      title: '结束时间',
      dataIndex: 'end_time',
      render: (t: string) => dayjs(t).format('YYYY-MM-DD HH:mm'),
    },
    {
      title: '状态',
      render: (_: unknown, row: OncallSchedule) => {
        const start = dayjs(row.start_time)
        const end = dayjs(row.end_time)
        if (now.isBefore(start)) return <Tag color="default">未开始</Tag>
        if (now.isAfter(end)) return <Tag color="default">已结束</Tag>
        return <Tag color="success">值班中</Tag>
      },
    },
  ]

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 20 }}>
        <CalendarOutlined style={{ fontSize: 20, color: c.primary }} />
        <Title level={4} style={{ margin: 0, color: c.textBody }}>
          值班管理
        </Title>
      </div>

      <Card style={{ borderRadius: 12, border: `1px solid ${c.border}`, boxShadow: 'none', background: c.bgSurface }}>
        <Table
          dataSource={schedules}
          columns={columns}
          rowKey="id"
          loading={isLoading}
          pagination={{ pageSize: 15, showTotal: (t) => `共 ${t} 条记录` }}
          locale={{ emptyText: '暂无值班安排' }}
        />
      </Card>
    </div>
  )
}
