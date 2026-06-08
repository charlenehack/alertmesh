import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { App, Button, Drawer, Form, Input, Popconfirm, Select, Space, Table, Tag, Typography } from 'antd'
import dayjs from 'dayjs'
import {
  createPolicy, deletePolicy, getContactGroups, getContacts, getPolicies, updatePolicy,
} from '../../../api/alert'
import type { NotificationPolicy, Severity } from '../../../types'
import { ContactGroupPicker } from './ContactGroupPicker'
import { SEV_COLOR } from './sevColor'
import { SevTag } from './SevTag'
import { Toolbar } from './Toolbar'
import { useTheme } from '../../../hooks/useTheme'

const { Text } = Typography
const { Option } = Select

export function PolicyList() {
  const qc = useQueryClient()
  const { message } = App.useApp()
  const { c } = useTheme()
  const [search, setSearch] = useState('')
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<NotificationPolicy | null>(null)
  const [form] = Form.useForm<Partial<NotificationPolicy>>()

  const { data, isLoading, refetch } = useQuery({ queryKey: ['policies'], queryFn: getPolicies })
  const { data: groupData } = useQuery({ queryKey: ['contact-groups'], queryFn: getContactGroups })
  const { data: contactData } = useQuery({ queryKey: ['contacts'], queryFn: getContacts })
  const allGroups = groupData ?? []
  const allContacts = contactData ?? []

  const rows = (data ?? []).filter(
    (r) => !search || r.name.toLowerCase().includes(search.toLowerCase()),
  )

  const saveMut = useMutation({
    mutationFn: (v: Partial<NotificationPolicy>) =>
      editing ? updatePolicy(editing.id, v) : createPolicy(v),
    onSuccess: () => {
      message.success(editing ? '更新成功' : '创建成功')
      qc.invalidateQueries({ queryKey: ['policies'] })
      setOpen(false); form.resetFields(); setEditing(null)
    },
  })

  const delMut = useMutation({
    mutationFn: deletePolicy,
    onSuccess: () => {
      message.success('已删除')
      qc.invalidateQueries({ queryKey: ['policies'] })
    },
  })

  const openEdit = (row: NotificationPolicy) => {
    setEditing(row)
    form.setFieldsValue({ ...row, group_ids: row.group_ids ?? [] })
    setOpen(true)
  }

  const columns = [
    {
      title: '策略名称',
      dataIndex: 'name',
      render: (n: string, row: NotificationPolicy) => (
        <div>
          <span style={{ color: c.textBody, fontWeight: 500, fontSize: 13 }}>{n}</span>
          <div style={{ color: c.textTertiary, fontSize: 11, marginTop: 2, fontFamily: 'monospace' }}>{row.id}</div>
        </div>
      ),
    },
    {
      title: '告警等级',
      dataIndex: 'severities',
      width: 160,
      render: (sevs: Severity[]) => (
        <Space size={4}>{(sevs || []).map((s) => <SevTag key={s} s={s} />)}</Space>
      ),
    },
    {
      title: '描述',
      dataIndex: 'description',
      render: (d: string) => <Text style={{ color: c.textHint, fontSize: 12 }}>{d || '—'}</Text>,
    },
    {
      title: '已关联规则数',
      dataIndex: 'linked_rules',
      width: 120,
      render: (n: number) => (
        <span style={{ color: n > 0 ? c.textBody : c.textTertiary, fontWeight: n > 0 ? 600 : 400 }}>{n}</span>
      ),
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      width: 160,
      render: (t: string) => (
        <span style={{ color: c.textHint, fontSize: 12 }}>{dayjs(t).format('YYYY-MM-DD HH:mm:ss')}</span>
      ),
    },
    {
      title: '操作',
      width: 100,
      render: (_: unknown, row: NotificationPolicy) => (
        <Space size={0}>
          <Button type="link" size="small" style={{ color: '#1677ff', padding: '0 4px' }}
            onClick={() => openEdit(row)}>编辑</Button>
          <Popconfirm title="确认删除该策略？" onConfirm={() => delMut.mutate(row.id)}>
            <Button type="link" size="small" danger style={{ padding: '0 4px' }}>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <>
      <Toolbar
        createLabel="创建通知策略"
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
        title={editing ? '编辑告警通知策略' : '创建告警通知策略'}
        open={open}
        size={700}
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
          <SectionLabel>基础信息</SectionLabel>
          <Form.Item name="name" label="通知策略名称" rules={[{ required: true, message: '请输入策略名称' }]}>
            <Input placeholder="请输入策略名称" />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea rows={2} placeholder="策略描述（可选）" />
          </Form.Item>

          <SectionLabel marginTop>告警通知策略</SectionLabel>
          <Form.Item name="severities" label="告警等级" rules={[{ required: true, message: '请选择告警等级' }]}>
            <Select
              mode="multiple"
              placeholder="请选择告警等级"
              tagRender={({ value, onClose }) => (
                <Tag
                  closable
                  onClose={onClose}
                  style={{
                    background: 'transparent',
                    border: `1px solid ${SEV_COLOR[value as Severity] || '#555'}`,
                    color: SEV_COLOR[value as Severity] || '#ccc',
                    fontWeight: 700,
                    fontSize: 11,
                  }}
                >
                  {value}
                </Tag>
              )}
            >
              {(['P0', 'P1', 'P2', 'P3'] as Severity[]).map((s) => (
                <Option key={s} value={s}><SevTag s={s} /></Option>
              ))}
            </Select>
          </Form.Item>

          <Form.Item name="group_ids" label="通知人" rules={[{ required: true, message: '请选择至少一个联系人组' }]}>
            <ContactGroupPicker allGroups={allGroups} allContacts={allContacts} />
          </Form.Item>
        </Form>
      </Drawer>
    </>
  )
}

function SectionLabel({ children, marginTop }: { children: React.ReactNode; marginTop?: boolean }) {
  return (
    <div
      style={{
        color: '#1677ff',
        fontSize: 13,
        fontWeight: 500,
        margin: marginTop ? '16px 0 12px' : '0 0 12px',
        borderLeft: '3px solid #1677ff',
        paddingLeft: 8,
      }}
    >
      {children}
    </div>
  )
}
