import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { App, Button, Form, Popconfirm, Space, Table, Tooltip, Typography } from 'antd'
import {
  createContact, deleteContact, getContactGroups, getContacts, updateContact,
} from '../../../api/alert'
import { encryptSecrets } from '../../../api/crypto'
import type { NotificationContact } from '../../../types'
import { ContactDrawer } from './ContactDrawer'
import { Toolbar } from './Toolbar'

const { Text } = Typography

// Names of NotificationContact fields that hold sensitive material and
// MUST be RSA-encrypted before being sent over the wire. Mirrors
// model.NotificationContact.SecretFields() on the server side.
const CONTACT_SECRET_FIELDS: Array<keyof NotificationContact> = [
  'webhook_token',
  'slack_bot_token',
  'feishu_secret',
  'dingtalk_secret',
]

export function ContactList() {
  const qc = useQueryClient()
  const { message } = App.useApp()
  const [search, setSearch] = useState('')
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<NotificationContact | null>(null)
  const [form] = Form.useForm<Partial<NotificationContact>>()

  const { data, isLoading, refetch } = useQuery({ queryKey: ['contacts'], queryFn: getContacts })
  const { data: groupData } = useQuery({ queryKey: ['contact-groups'], queryFn: getContactGroups })
  const allContacts = data ?? []
  const allGroups = groupData ?? []

  const rows = allContacts.filter(
    (r) => !search || r.name.toLowerCase().includes(search.toLowerCase()),
  )

  const saveMut = useMutation({
    mutationFn: async (v: Partial<NotificationContact>) => {
      // RSA-encrypt sensitive fields before they leave the browser.
      const payload = (await encryptSecrets(
        v as Record<string, unknown>,
        CONTACT_SECRET_FIELDS as unknown as string[],
      )) as Partial<NotificationContact>
      return editing
        ? updateContact(editing.id, payload)
        : createContact(payload)
    },
    onSuccess: () => {
      message.success(editing ? '更新成功' : '创建成功')
      qc.invalidateQueries({ queryKey: ['contacts'] })
      setOpen(false); form.resetFields(); setEditing(null)
    },
  })

  const delMut = useMutation({
    mutationFn: deleteContact,
    onSuccess: () => {
      message.success('已删除')
      qc.invalidateQueries({ queryKey: ['contacts'] })
    },
  })

  const openEdit = (row: NotificationContact) => {
    setEditing(row); form.setFieldsValue(row); setOpen(true)
  }

  // Tiny chip with a coloured dot, used to compactly indicate which
  // channels a contact has configured. Truncates long values.
  const dot = (value: string, color: string) =>
    value ? (
      <Tooltip title={value}>
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4, color: '#888', fontSize: 12 }}>
          <span style={{ width: 7, height: 7, borderRadius: '50%', background: color, display: 'inline-block' }} />
          {value.length > 22 ? value.slice(0, 22) + '…' : value}
        </span>
      </Tooltip>
    ) : <span style={{ color: '#333' }}>—</span>

  const columns = [
    { title: '名称', dataIndex: 'name', render: (n: string) => <Text style={{ fontWeight: 500 }}>{n}</Text> },
    { title: '邮箱', dataIndex: 'email', render: (v: string) => dot(v, '#52c41a') },
    { title: '手机号', dataIndex: 'phone', render: (v: string) => dot(v, '#fa8c16') },
    { title: 'Webhook', dataIndex: 'webhook_url', render: (v: string) => dot(v, '#1677ff') },
    { title: 'Slack', dataIndex: 'slack_channel_id', render: (v: string) => dot(v, '#4a154b') },
    { title: '飞书', dataIndex: 'feishu_webhook', render: (v: string) => dot(v, '#00c853') },
    { title: '钉钉', dataIndex: 'dingtalk_webhook', render: (v: string) => dot(v, '#1677ff') },
    {
      title: '操作',
      width: 100,
      render: (_: unknown, row: NotificationContact) => (
        <Space size={0}>
          <Button type="link" size="small" style={{ color: '#1677ff', padding: '0 4px' }}
            onClick={() => openEdit(row)}>编辑</Button>
          <Popconfirm title="确认删除该联系人？" onConfirm={() => delMut.mutate(row.id)}>
            <Button type="link" size="small" danger style={{ padding: '0 4px' }}>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <>
      <Toolbar
        createLabel="创建通知对象"
        onSearch={setSearch}
        onCreate={() => { setEditing(null); form.resetFields(); setOpen(true) }}
        onRefresh={refetch}
      />
      <Table
        dataSource={rows} columns={columns} rowKey="id"
        loading={isLoading} size="small"
        pagination={{ pageSize: 10, showTotal: (t) => `共 ${t} 条`, size: 'small' }}
        style={{ padding: '0 8px' }}
      />

      <ContactDrawer
        open={open}
        editing={editing}
        form={form}
        loading={saveMut.isPending}
        onClose={() => { setOpen(false); setEditing(null) }}
        onFinish={(v) => saveMut.mutate(v)}
        allGroups={allGroups}
        allContacts={allContacts}
      />
    </>
  )
}
