import { useQuery } from '@tanstack/react-query'
import { Table, Card, Tag, Avatar, Typography, Space } from 'antd'
import { UserOutlined } from '@ant-design/icons'
import { getUsers } from '../../api/system'
import type { User, Role } from '../../types'

const { Title } = Typography

export default function UserList() {
  const { data, isLoading } = useQuery({
    queryKey: ['users'],
    queryFn: getUsers,
  })

  const users: User[] = data ?? []

  const columns = [
    {
      title: '用户',
      render: (_: unknown, row: User) => (
        <Space>
          <Avatar
            size={32}
            style={{ background: '#1677ff', fontSize: 13, fontWeight: 600 }}
          >
            {row.username[0].toUpperCase()}
          </Avatar>
          <div>
            <div style={{ fontWeight: 500 }}>{row.display_name || row.username}</div>
            <div style={{ fontSize: 12, color: '#8c8c8c' }}>{row.username}</div>
          </div>
        </Space>
      ),
    },
    {
      title: '邮箱',
      dataIndex: 'email',
      render: (e: string) => <span style={{ color: '#666', fontSize: 13 }}>{e || '—'}</span>,
    },
    {
      title: '来源',
      dataIndex: 'source',
      width: 100,
      render: (s: string) => <Tag color="blue">{s}</Tag>,
    },
    {
      title: '角色',
      dataIndex: 'roles',
      render: (roles: Role[]) =>
        roles?.length
          ? roles.map((r) => <Tag key={r.id} color="purple">{r.name}</Tag>)
          : <span style={{ color: '#d9d9d9' }}>无角色</span>,
    },
    {
      title: '状态',
      dataIndex: 'is_active',
      width: 90,
      render: (active: boolean) => (
        <Tag color={active ? 'success' : 'default'}>{active ? '启用' : '禁用'}</Tag>
      ),
    },
  ]

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 20 }}>
        <UserOutlined style={{ fontSize: 20, color: '#1677ff' }} />
        <Title level={4} style={{ margin: 0, color: '#1a1a2e' }}>
          用户管理
        </Title>
      </div>

      <Card style={{ borderRadius: 12, border: 'none', boxShadow: '0 2px 12px rgba(0,0,0,0.06)' }}>
        <Table
          dataSource={users}
          columns={columns}
          rowKey="id"
          loading={isLoading}
          pagination={{ pageSize: 15, showTotal: (t) => `共 ${t} 个用户` }}
        />
      </Card>
    </div>
  )
}
