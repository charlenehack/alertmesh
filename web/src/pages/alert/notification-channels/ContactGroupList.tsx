import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  App, Button, Drawer, Form, Input, Popconfirm, Select, Space, Table, Tag, Typography,
} from 'antd'
import {
  createContactGroup, deleteContactGroup, getContactGroups, getContacts, updateContactGroup,
} from '../../../api/alert'
import type { NotificationContactGroup } from '../../../types'
import { Toolbar } from './Toolbar'
import { useTheme } from '../../../hooks/useTheme'

const { Text } = Typography
const { Option } = Select

export function ContactGroupList() {
  const qc = useQueryClient()
  const { message } = App.useApp()
  const { c } = useTheme()
  const [search, setSearch] = useState('')
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<NotificationContactGroup | null>(null)
  const [form] = Form.useForm<Partial<NotificationContactGroup>>()

  const { data: cData } = useQuery({ queryKey: ['contacts'], queryFn: getContacts })
  const allContacts = cData ?? []

  const { data, isLoading, refetch } = useQuery({ queryKey: ['contact-groups'], queryFn: getContactGroups })
  const rows = (data ?? []).filter(
    (r) => !search || r.name.toLowerCase().includes(search.toLowerCase()),
  )

  const saveMut = useMutation({
    mutationFn: (v: Partial<NotificationContactGroup>) =>
      editing ? updateContactGroup(editing.id, v) : createContactGroup(v),
    onSuccess: () => {
      message.success(editing ? '更新成功' : '创建成功')
      qc.invalidateQueries({ queryKey: ['contact-groups'] })
      setOpen(false); form.resetFields(); setEditing(null)
    },
  })

  const delMut = useMutation({
    mutationFn: deleteContactGroup,
    onSuccess: () => {
      message.success('已删除')
      qc.invalidateQueries({ queryKey: ['contact-groups'] })
    },
  })

  const openEdit = (row: NotificationContactGroup) => {
    setEditing(row); form.setFieldsValue(row); setOpen(true)
  }

  const nameMap = Object.fromEntries(allContacts.map((c) => [c.id, c.name]))

  const columns = [
    { title: '分组名称', dataIndex: 'name', render: (n: string) => <Text style={{ fontWeight: 500 }}>{n}</Text> },
    {
      title: '描述', dataIndex: 'description',
      render: (d: string) => <Text style={{ color: c.textHint, fontSize: 12 }}>{d || '—'}</Text>,
    },
    {
      title: '联系人',
      dataIndex: 'contact_ids',
      render: (ids: string[]) => (
        <Space size={4} wrap>
          {(ids || []).map((id) => (
            <Tag key={id} style={{ background: c.bgElevated, border: `1px solid ${c.border}`, color: c.textSecondary, fontSize: 11 }}>
              {nameMap[id] || id.slice(0, 8)}
            </Tag>
          ))}
          {(!ids || ids.length === 0) && <span style={{ color: c.textTertiary }}>—</span>}
        </Space>
      ),
    },
    {
      title: '操作',
      width: 100,
      render: (_: unknown, row: NotificationContactGroup) => (
        <Space size={0}>
          <Button type="link" size="small" style={{ color: '#1677ff', padding: '0 4px' }}
            onClick={() => openEdit(row)}>编辑</Button>
          <Popconfirm title="确认删除该联系人组？" onConfirm={() => delMut.mutate(row.id)}>
            <Button type="link" size="small" danger style={{ padding: '0 4px' }}>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <>
      <Toolbar
        createLabel="创建联系人组"
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

      <Drawer
        title={editing ? '编辑联系人组' : '创建联系人组'}
        open={open}
        size={480}
        onClose={() => { setOpen(false); setEditing(null) }}
        styles={{
          header: { background: c.bgSurface, borderBottom: `1px solid ${c.border}`, color: c.textBody },
          body: { background: c.bgSurface, padding: '20px 24px' },
          footer: { background: c.bgSurface, borderTop: `1px solid ${c.border}` },
        }}
        footer={
          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
            <Button onClick={() => { setOpen(false); setEditing(null) }}>取消</Button>
            <Button type="primary" loading={saveMut.isPending} onClick={() => form.submit()}>确定</Button>
          </div>
        }
      >
        <Form form={form} layout="vertical" onFinish={(v) => saveMut.mutate(v)}>
          <Form.Item name="name" label="分组名称" rules={[{ required: true }]}>
            <Input placeholder="联系人组名称" />
          </Form.Item>
          <Form.Item name="contact_ids" label="选择联系人" rules={[{ required: true }]}>
            <Select mode="multiple" placeholder="选择联系人" style={{ width: '100%' }}>
              {allContacts.map((c) => (
                <Option key={c.id} value={c.id}>{c.name}</Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input placeholder="可选描述" />
          </Form.Item>
        </Form>
      </Drawer>
    </>
  )
}
