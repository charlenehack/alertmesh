import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Table, Button, Tag, Drawer, Form, Input, DatePicker,
  Popconfirm, App, Typography
} from 'antd'
import { PlusOutlined, DeleteOutlined, StopOutlined } from '@ant-design/icons'
import dayjs from 'dayjs'
import { getSilences, createSilence, deleteSilence } from '../../api/alert'
import { PageHeader } from '../../components/PageHeader'
import { SurfaceCard } from '../../components/SurfaceCard'
import type { SilencePolicy } from '../../types'

const { Text } = Typography
const { TextArea } = Input
const { RangePicker } = DatePicker

export default function SilencePolicies() {
  const qc = useQueryClient()
  const { message } = App.useApp()
  const [open, setOpen] = useState(false)
  const [form] = Form.useForm()
  const now = dayjs()

  const { data, isLoading } = useQuery({ queryKey: ['silences'], queryFn: getSilences })
  const rows: SilencePolicy[] = data ?? []

  const saveMut = useMutation({
    mutationFn: (values: Record<string, unknown>) => {
      const [start, end] = values.time_range as [dayjs.Dayjs, dayjs.Dayjs]
      return createSilence({
        name: values.name as string,
        comment: values.comment as string,
        matchers: JSON.parse((values.matchers as string) || '[]'),
        starts_at: start.toISOString(),
        ends_at: end.toISOString(),
        is_active: true,
      })
    },
    onSuccess: () => {
      message.success('静默策略已创建')
      qc.invalidateQueries({ queryKey: ['silences'] })
      setOpen(false); form.resetFields()
    },
  })

  const deleteMut = useMutation({
    mutationFn: deleteSilence,
    onSuccess: () => { message.success('已解除静默'); qc.invalidateQueries({ queryKey: ['silences'] }) },
  })

  const getStatus = (row: SilencePolicy) => {
    if (!row.is_active) return <Tag color="default">已解除</Tag>
    if (now.isAfter(dayjs(row.ends_at))) return <Tag color="default">已过期</Tag>
    if (now.isBefore(dayjs(row.starts_at))) return <Tag color="warning">未开始</Tag>
    return <Tag color="error">静默中</Tag>
  }

  const columns = [
    {
      title: '名称',
      dataIndex: 'name',
      render: (n: string) => <Text style={{ fontWeight: 500 }}>{n}</Text>,
    },
    {
      title: '状态',
      width: 90,
      render: (_: unknown, row: SilencePolicy) => getStatus(row),
    },
    {
      title: '匹配条件',
      dataIndex: 'matchers',
      render: (m: unknown[]) => (
        <span style={{ fontSize: 11, color: '#888', fontFamily: 'monospace' }}>
          {Array.isArray(m) ? m.map((x: unknown) => {
            const matcher = x as { key?: string; op?: string; value?: string }
            return `${matcher.key}${matcher.op}${matcher.value}`
          }).join(' & ') : '—'}
        </span>
      ),
    },
    {
      title: '开始时间',
      dataIndex: 'starts_at',
      width: 140,
      render: (t: string) => <span style={{ fontSize: 12, color: '#888' }}>{dayjs(t).format('MM-DD HH:mm')}</span>,
    },
    {
      title: '结束时间',
      dataIndex: 'ends_at',
      width: 140,
      render: (t: string) => <span style={{ fontSize: 12, color: '#888' }}>{dayjs(t).format('MM-DD HH:mm')}</span>,
    },
    {
      title: '创建人',
      dataIndex: 'created_by',
      width: 110,
      render: (u: string) => <Text type="secondary" style={{ fontSize: 12 }}>{u || '—'}</Text>,
    },
    {
      title: '操作',
      width: 80,
      render: (_: unknown, row: SilencePolicy) =>
        row.is_active ? (
          <Popconfirm title="解除该静默策略？" onConfirm={() => deleteMut.mutate(row.id)}>
            <Button size="small" icon={<DeleteOutlined />} type="text" danger />
          </Popconfirm>
        ) : null,
    },
  ]

  return (
    <div>
      <PageHeader
        title="告警静默策略"
        icon={<StopOutlined />}
        extra={
          <Button type="primary" icon={<PlusOutlined />} onClick={() => { form.resetFields(); setOpen(true) }}>
            新建
          </Button>
        }
      />

      <SurfaceCard flush>
        <Table dataSource={rows} columns={columns} rowKey="id" loading={isLoading} size="small" pagination={{ pageSize: 15 }} />
      </SurfaceCard>

      <Drawer
        title="新建静默策略"
        open={open}
        size={560}
        onClose={() => setOpen(false)}
        styles={{
          header: { background: '#111111', borderBottom: '1px solid #1e1e1e', color: '#e8e8e8' },
          body: { background: '#111111', padding: '20px 24px' },
          footer: { background: '#111111', borderTop: '1px solid #1e1e1e' },
        }}
        footer={
          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
            <Button onClick={() => setOpen(false)}>取消</Button>
            <Button type="primary" loading={saveMut.isPending} onClick={() => form.submit()}>
              确定
            </Button>
          </div>
        }
      >
        <Form form={form} layout="vertical" onFinish={saveMut.mutate}>
          <Form.Item name="name" label="名称" rules={[{ required: true }]}>
            <Input placeholder="静默策略名称" />
          </Form.Item>
          <Form.Item name="matchers" label="匹配条件（JSON）" rules={[{ required: true }]}>
            <TextArea rows={3} placeholder='[{"key":"alertname","op":"=","value":"HighCPU"}]'
              style={{ fontFamily: 'monospace', fontSize: 12 }} />
          </Form.Item>
          <Form.Item name="time_range" label="静默时间段" rules={[{ required: true }]}>
            <RangePicker showTime format="YYYY-MM-DD HH:mm" style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="comment" label="备注">
            <TextArea rows={2} placeholder="静默原因" />
          </Form.Item>
        </Form>
      </Drawer>
    </div>
  )
}
