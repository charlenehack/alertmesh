import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Table, Card, Tag, Avatar, Typography, Space, Button, Modal, Form,
  Input, Select, Switch, Popconfirm, App, Tooltip,
} from 'antd'
import {
  UserOutlined, PlusOutlined, EditOutlined, DeleteOutlined,
  LockOutlined, UnlockOutlined,
} from '@ant-design/icons'
import { getUsers, createUser, updateUser, deleteUser, getRoles } from '../../api/system'
import { useTheme } from '../../hooks/useTheme'
import type { User, Role } from '../../types'

const { Title } = Typography

type ModalMode = 'create' | 'edit'

export default function UserList() {
  const { data, isLoading } = useQuery({ queryKey: ['users'], queryFn: getUsers })
  const { data: rolesData } = useQuery({ queryKey: ['roles'], queryFn: getRoles })
  const { c } = useTheme()
  const { message } = App.useApp()
  const qc = useQueryClient()
  const users: User[] = data ?? []
  const roles: Role[] = rolesData ?? []

  const [modalOpen, setModalOpen] = useState(false)
  const [modalMode, setModalMode] = useState<ModalMode>('create')
  const [editingUser, setEditingUser] = useState<User | null>(null)
  const [form] = Form.useForm()

  // ─── mutations ──────────────────────────────────────────────────────────────
  const createMut = useMutation({
    mutationFn: createUser,
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['users'] }); closeModal(); message.success('用户已创建') },
    onError: (e: Error) => message.error('创建失败：' + e.message),
  })

  const updateMut = useMutation({
    mutationFn: ({ id, body }: { id: string; body: Parameters<typeof updateUser>[1] }) =>
      updateUser(id, body),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['users'] }); closeModal(); message.success('已保存') },
    onError: (e: Error) => message.error('保存失败：' + e.message),
  })

  const deleteMut = useMutation({
    mutationFn: deleteUser,
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['users'] }); message.success('用户已删除') },
    onError: (e: Error) => message.error('删除失败：' + e.message),
  })

  // ─── helpers ────────────────────────────────────────────────────────────────
  const openCreate = () => {
    setModalMode('create')
    setEditingUser(null)
    form.resetFields()
    setModalOpen(true)
  }

  const openEdit = (user: User) => {
    setModalMode('edit')
    setEditingUser(user)
    form.setFieldsValue({
      display_name: user.display_name,
      email: user.email,
      is_active: user.is_active,
      role_ids: user.roles?.map((r) => r.id) ?? [],
    })
    setModalOpen(true)
  }

  const closeModal = () => {
    setModalOpen(false)
    setEditingUser(null)
    form.resetFields()
  }

  const onFinish = (values: {
    username?: string
    password?: string
    display_name?: string
    email?: string
    is_active?: boolean
    role_ids?: number[]
  }) => {
    if (modalMode === 'create') {
      createMut.mutate({
        username: values.username!,
        password: values.password!,
        display_name: values.display_name,
        email: values.email,
        role_ids: values.role_ids,
      })
    } else if (editingUser) {
      updateMut.mutate({
        id: editingUser.id,
        body: {
          display_name: values.display_name,
          email: values.email,
          is_active: values.is_active,
          role_ids: values.role_ids,
          password: values.password || undefined,
        },
      })
    }
  }

  // ─── table columns ──────────────────────────────────────────────────────────
  const columns = [
    {
      title: '用户',
      render: (_: unknown, row: User) => (
        <Space>
          <Avatar size={32} style={{ background: '#1677ff', fontSize: 13, fontWeight: 600 }}>
            {row.username[0].toUpperCase()}
          </Avatar>
          <div>
            <div style={{ fontWeight: 500 }}>{row.display_name || row.username}</div>
            <div style={{ fontSize: 12, color: c.textSecondary }}>{row.username}</div>
          </div>
        </Space>
      ),
    },
    {
      title: '邮箱',
      dataIndex: 'email',
      render: (e: string) => <span style={{ color: c.textSecondary, fontSize: 13 }}>{e || '—'}</span>,
    },
    {
      title: '来源',
      dataIndex: 'source',
      width: 90,
      render: (s: string) => <Tag color={s === 'local' ? 'blue' : 'orange'}>{s}</Tag>,
    },
    {
      title: '角色',
      dataIndex: 'roles',
      render: (r: Role[]) =>
        r?.length
          ? r.map((role) => <Tag key={role.id} color="purple">{role.name}</Tag>)
          : <span style={{ color: c.textTertiary }}>无角色</span>,
    },
    {
      title: '状态',
      dataIndex: 'is_active',
      width: 80,
      render: (active: boolean) => (
        <Tag color={active ? 'success' : 'default'}>{active ? '启用' : '禁用'}</Tag>
      ),
    },
    {
      title: '操作',
      width: 160,
      render: (_: unknown, row: User) => (
        <Space size={4}>
          <Tooltip title="编辑用户">
            <Button
              size="small"
              icon={<EditOutlined />}
              onClick={() => openEdit(row)}
            />
          </Tooltip>
          <Tooltip title={row.is_active ? '禁用账户' : '启用账户'}>
            <Button
              size="small"
              icon={row.is_active ? <LockOutlined /> : <UnlockOutlined />}
              onClick={() => updateMut.mutate({ id: row.id, body: { is_active: !row.is_active } })}
            />
          </Tooltip>
          <Popconfirm
            title="确认删除该用户？"
            okText="删除"
            okButtonProps={{ danger: true }}
            cancelText="取消"
            disabled={row.username === 'admin'}
            onConfirm={() => deleteMut.mutate(row.id)}
          >
            <Tooltip title={row.username === 'admin' ? '内置账户不可删除' : '删除用户'}>
              <Button
                size="small"
                danger
                icon={<DeleteOutlined />}
                disabled={row.username === 'admin'}
              />
            </Tooltip>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 20 }}>
        <Space>
          <UserOutlined style={{ fontSize: 20, color: c.primary }} />
          <Title level={4} style={{ margin: 0, color: c.textBody }}>用户管理</Title>
        </Space>
        <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
          新建用户
        </Button>
      </div>

      <Card style={{ borderRadius: 12, border: `1px solid ${c.border}`, boxShadow: 'none', background: c.bgSurface }}>
        <Table
          dataSource={users}
          columns={columns}
          rowKey="id"
          loading={isLoading}
          pagination={{ pageSize: 15, showTotal: (t) => `共 ${t} 个用户` }}
        />
      </Card>

      {/* 创建 / 编辑用户弹窗 */}
      <Modal
        title={modalMode === 'create' ? '新建用户' : '编辑用户'}
        open={modalOpen}
        onCancel={closeModal}
        onOk={() => form.submit()}
        okText={modalMode === 'create' ? '创建' : '保存'}
        confirmLoading={createMut.isPending || updateMut.isPending}
        destroyOnClose
        width={480}
      >
        <Form
          form={form}
          layout="vertical"
          onFinish={onFinish}
          style={{ marginTop: 16 }}
          initialValues={{ is_active: true }}
        >
          {modalMode === 'create' && (
            <>
              <Form.Item
                label="用户名"
                name="username"
                rules={[{ required: true, message: '请输入用户名' }]}
              >
                <Input placeholder="登录名（唯一）" autoComplete="off" />
              </Form.Item>
              <Form.Item
                label="初始密码"
                name="password"
                rules={[{ required: true, message: '请输入初始密码' }]}
              >
                <Input.Password placeholder="至少6位" autoComplete="new-password" />
              </Form.Item>
            </>
          )}

          {modalMode === 'edit' && (
            <Form.Item label="重置密码（留空不修改）" name="password">
              <Input.Password placeholder="留空表示不修改" autoComplete="new-password" />
            </Form.Item>
          )}

          <Form.Item label="显示名称" name="display_name">
            <Input placeholder="可选，用于界面展示" />
          </Form.Item>

          <Form.Item label="邮箱" name="email">
            <Input placeholder="可选" />
          </Form.Item>

          <Form.Item label="分配角色" name="role_ids">
            <Select
              mode="multiple"
              placeholder="选择角色"
              options={roles.map((r) => ({
                value: r.id,
                label: (
                  <Space size={4}>
                    <span>{r.name}</span>
                    {r.description && (
                      <span style={{ color: c.textTertiary, fontSize: 12 }}>— {r.description}</span>
                    )}
                  </Space>
                ),
              }))}
              optionFilterProp="label"
            />
          </Form.Item>

          {modalMode === 'edit' && (
            <Form.Item label="账户状态" name="is_active" valuePropName="checked">
              <Switch checkedChildren="启用" unCheckedChildren="禁用" />
            </Form.Item>
          )}
        </Form>
      </Modal>
    </div>
  )
}
