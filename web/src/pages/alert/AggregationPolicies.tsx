import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Table, Button, Switch, Drawer, Form, Input, InputNumber,
  Space, Popconfirm, App, Typography
} from 'antd'
import { PlusOutlined, EditOutlined, DeleteOutlined, MergeCellsOutlined } from '@ant-design/icons'
import { getAggregations, createAggregation, updateAggregation, deleteAggregation } from '../../api/alert'
import { PageHeader } from '../../components/PageHeader'
import { SurfaceCard } from '../../components/SurfaceCard'
import { useTheme } from '../../hooks/useTheme'
import type { AggregationPolicy } from '../../types'

const { Text } = Typography
const { TextArea } = Input

export default function AggregationPolicies() {
  const qc = useQueryClient()
  const { message } = App.useApp()
  const { c } = useTheme()
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<AggregationPolicy | null>(null)
  const [form] = Form.useForm()

  const { data, isLoading } = useQuery({ queryKey: ['aggregations'], queryFn: getAggregations })
  const rows: AggregationPolicy[] = data ?? []

  const toFormValues = (row: AggregationPolicy) => ({
    ...row,
    matchers: JSON.stringify(row.matchers, null, 2),
    group_by: Array.isArray(row.group_by) ? row.group_by.join(', ') : '',
  })

  const fromFormValues = (values: Record<string, unknown>) => ({
    ...values,
    matchers: JSON.parse((values.matchers as string) || '[]'),
    group_by: String(values.group_by || '').split(',').map((s: string) => s.trim()).filter(Boolean),
  })

  const saveMut = useMutation({
    mutationFn: (values: Record<string, unknown>) => {
      const payload = fromFormValues(values)
      return editing ? updateAggregation(editing.id, payload) : createAggregation(payload)
    },
    onSuccess: () => {
      message.success(editing ? '更新成功' : '创建成功')
      qc.invalidateQueries({ queryKey: ['aggregations'] })
      setOpen(false); form.resetFields(); setEditing(null)
    },
  })

  const deleteMut = useMutation({
    mutationFn: deleteAggregation,
    onSuccess: () => { message.success('已删除'); qc.invalidateQueries({ queryKey: ['aggregations'] }) },
  })

  const columns = [
    {
      title: '名称',
      dataIndex: 'name',
      render: (n: string) => <Text style={{ fontWeight: 500 }}>{n}</Text>,
    },
    {
      title: '分组字段',
      dataIndex: 'group_by',
      render: (g: string[]) => (g || []).map((k: string) => (
        <span key={k} style={{ background: c.bgElevated, border: `1px solid ${c.border}`, borderRadius: 4, padding: '1px 6px', marginRight: 4, fontSize: 11, color: c.textHint }}>{k}</span>
      )),
    },
    {
      title: '等待(s)',
      dataIndex: 'group_wait',
      width: 80,
      render: (v: number) => <span style={{ color: c.textSecondary, fontSize: 12 }}>{v}s</span>,
    },
    {
      title: '聚合间隔(s)',
      dataIndex: 'group_interval',
      width: 100,
      render: (v: number) => <span style={{ color: c.textSecondary, fontSize: 12 }}>{v}s</span>,
    },
    {
      title: '重复间隔(s)',
      dataIndex: 'repeat_interval',
      width: 100,
      render: (v: number) => <span style={{ color: c.textSecondary, fontSize: 12 }}>{v}s</span>,
    },
    {
      title: '启用',
      dataIndex: 'is_enabled',
      width: 80,
      render: (v: boolean) => <Switch checked={v} size="small" disabled />,
    },
    {
      title: '操作',
      width: 120,
      render: (_: unknown, row: AggregationPolicy) => (
        <Space>
          <Button size="small" icon={<EditOutlined />} type="text"
            onClick={() => { setEditing(row); form.setFieldsValue(toFormValues(row)); setOpen(true) }} />
          <Popconfirm title="确认删除？" onConfirm={() => deleteMut.mutate(row.id)}>
            <Button size="small" icon={<DeleteOutlined />} type="text" danger />
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <div>
      <PageHeader
        title="告警聚合策略"
        icon={<MergeCellsOutlined />}
        extra={
          <Button type="primary" icon={<PlusOutlined />}
            onClick={() => { setEditing(null); form.resetFields(); setOpen(true) }}>
            新建
          </Button>
        }
      />

      <SurfaceCard flush>
        <Table dataSource={rows} columns={columns} rowKey="id" loading={isLoading} size="small" pagination={{ pageSize: 15 }} />
      </SurfaceCard>

      <Drawer
        title={editing ? '编辑聚合策略' : '新建聚合策略'}
        open={open}
        size={580}
        onClose={() => { setOpen(false); setEditing(null) }}
        styles={{
          header: { background: c.bgSurface, borderBottom: `1px solid ${c.border}`, color: c.textBody },
          body: { background: c.bgSurface, padding: '20px 24px' },
          footer: { background: c.bgSurface, borderTop: `1px solid ${c.border}` },
        }}
        footer={
          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
            <Button onClick={() => { setOpen(false); setEditing(null) }}>取消</Button>
            <Button type="primary" loading={saveMut.isPending} onClick={() => form.submit()}>
              确定
            </Button>
          </div>
        }
      >
        <Form form={form} layout="vertical" onFinish={saveMut.mutate}>
          <Form.Item name="name" label="策略名称" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="matchers" label="匹配条件（JSON）" rules={[{ required: true }]}>
            <TextArea rows={3} placeholder='[{"key":"env","op":"=","value":"prod"}]' style={{ fontFamily: 'monospace', fontSize: 12 }} />
          </Form.Item>
          <Form.Item name="group_by" label="分组字段（逗号分隔）">
            <Input placeholder="alertname, severity" />
          </Form.Item>
          <Space style={{ width: '100%' }} size={12}>
            <Form.Item name="group_wait" label="等待时间(秒)" initialValue={30} style={{ flex: 1 }}>
              <InputNumber min={0} style={{ width: '100%' }} />
            </Form.Item>
            <Form.Item name="group_interval" label="聚合间隔(秒)" initialValue={300} style={{ flex: 1 }}>
              <InputNumber min={0} style={{ width: '100%' }} />
            </Form.Item>
            <Form.Item name="repeat_interval" label="重复间隔(秒)" initialValue={3600} style={{ flex: 1 }}>
              <InputNumber min={0} style={{ width: '100%' }} />
            </Form.Item>
          </Space>
          <Form.Item name="description" label="描述">
            <Input />
          </Form.Item>
          <Form.Item name="is_enabled" label="启用" valuePropName="checked" initialValue={true}>
            <Switch />
          </Form.Item>
        </Form>
      </Drawer>
    </div>
  )
}
