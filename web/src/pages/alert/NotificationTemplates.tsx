import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Table, Button, Tag, Drawer, Form, Input, Select,
  Space, Popconfirm, App, Typography, Switch
} from 'antd'
import { PlusOutlined, EditOutlined, DeleteOutlined, MessageOutlined } from '@ant-design/icons'
import { getTemplates, createTemplate, updateTemplate, deleteTemplate } from '../../api/alert'
import { PageHeader } from '../../components/PageHeader'
import { SurfaceCard } from '../../components/SurfaceCard'
import { useTheme } from '../../hooks/useTheme'
import type { NotificationTemplate, ChannelType } from '../../types'

const { Text } = Typography
const { Option } = Select
const { TextArea } = Input

const CHANNEL_TYPES: { value: ChannelType; label: string }[] = [
  { value: 'dingtalk', label: '钉钉' },
  { value: 'feishu', label: '飞书' },
  { value: 'slack', label: 'Slack' },
  { value: 'email', label: '邮件' },
  { value: 'webhook', label: 'Webhook' },
]

export default function NotificationTemplates() {
  const qc = useQueryClient()
  const { message } = App.useApp()
  const { c } = useTheme()
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<NotificationTemplate | null>(null)
  const [form] = Form.useForm()
  const channelType = Form.useWatch('channel_type', form)

  const { data, isLoading } = useQuery({ queryKey: ['templates'], queryFn: getTemplates })
  const rows: NotificationTemplate[] = data ?? []

  const saveMut = useMutation({
    mutationFn: (values: Partial<NotificationTemplate>) =>
      editing ? updateTemplate(editing.id, values) : createTemplate(values),
    onSuccess: () => {
      message.success(editing ? '更新成功' : '创建成功')
      qc.invalidateQueries({ queryKey: ['templates'] })
      setOpen(false); form.resetFields(); setEditing(null)
    },
  })

  const deleteMut = useMutation({
    mutationFn: deleteTemplate,
    onSuccess: () => { message.success('已删除'); qc.invalidateQueries({ queryKey: ['templates'] }) },
  })

  const openEdit = (row: NotificationTemplate) => {
    setEditing(row); form.setFieldsValue(row); setOpen(true)
  }

  const columns = [
    {
      title: '名称',
      dataIndex: 'name',
      render: (n: string) => <Text style={{ fontWeight: 500 }}>{n}</Text>,
    },
    {
      title: '渠道类型',
      dataIndex: 'channel_type',
      width: 100,
      render: (t: string) => <Tag>{CHANNEL_TYPES.find(x => x.value === t)?.label || t}</Tag>,
    },
    {
      title: '默认模板',
      dataIndex: 'is_default',
      width: 100,
      render: (v: boolean) => v ? <Tag color="blue">默认</Tag> : null,
    },
    {
      title: '描述',
      dataIndex: 'description',
      render: (d: string) => <Text type="secondary" style={{ fontSize: 12 }}>{d || '—'}</Text>,
    },
    {
      title: '操作',
      width: 120,
      render: (_: unknown, row: NotificationTemplate) => (
        <Space>
          <Button size="small" icon={<EditOutlined />} type="text" onClick={() => openEdit(row)} />
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
        title="通知消息模板"
        icon={<MessageOutlined />}
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
        title={editing ? '编辑模板' : '新建模板'}
        open={open}
        size={640}
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
          <Form.Item name="name" label="模板名称" rules={[{ required: true }]}>
            <Input placeholder="唯一标识名称" />
          </Form.Item>
          <Form.Item name="channel_type" label="渠道类型" rules={[{ required: true }]}>
            <Select placeholder="选择渠道类型">
              {CHANNEL_TYPES.map(t => <Option key={t.value} value={t.value}>{t.label}</Option>)}
            </Select>
          </Form.Item>
          {channelType === 'email' && (
            <Form.Item name="subject" label="邮件主题">
              <Input placeholder="告警通知：{{.Title}}" />
            </Form.Item>
          )}
          <Form.Item name="body" label="消息内容" rules={[{ required: true }]}>
            <TextArea rows={8} placeholder="支持 Go template 语法，可用变量：{{.Title}} {{.Severity}} {{.Status}} 等"
              style={{ fontFamily: 'monospace', fontSize: 12 }} />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input placeholder="可选描述" />
          </Form.Item>
          <Form.Item name="is_default" label="设为默认" valuePropName="checked">
            <Switch />
          </Form.Item>
        </Form>
      </Drawer>
    </div>
  )
}
