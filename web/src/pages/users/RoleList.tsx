import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Table, Card, Tag, Typography, Space, Button, Modal, Checkbox, App, Tooltip, Collapse,
} from 'antd'
import { SafetyCertificateOutlined, EditOutlined } from '@ant-design/icons'
import { getRoles, getEndpoints, updateRoleEndpoints } from '../../api/system'
import { useTheme } from '../../hooks/useTheme'
import type { Role, Endpoint } from '../../types'

const { Title, Text } = Typography

/** 权限点 identity → 中文名映射（菜单级权限收敛后每个 identity 代表一个子菜单） */
const IDENTITY_LABEL: Record<string, string> = {
  incidentAccess:     '告警事件（含 AI 分析）',
  alertRouteAccess:   '告警路由',
  aggregationAccess:  '告警聚合策略',
  silenceAccess:      '告警静默策略',
  notificationAccess: '通知策略（含联系人/联系人组/升级策略）',
  templateAccess:     '通知消息模板',
  webhookSourceAccess:'Webhook 可信源',
  opsAccess:          '运维操作（Nginx 配置）',
  k8sAccess:          'K8S 管理（集群/Pods/服务路由/Volumes/节点）',
  sysAccess:          '系统管理（管理员专属）',
  dataSourceAccess:   '数据源管理（管理员专属）',
}

export default function RoleList() {
  const { data: rolesData, isLoading } = useQuery({ queryKey: ['roles'], queryFn: getRoles })
  const { data: endpointsData } = useQuery({ queryKey: ['endpoints'], queryFn: getEndpoints })
  const { c } = useTheme()
  const { message } = App.useApp()
  const qc = useQueryClient()

  const roles: Role[] = rolesData ?? []
  const endpoints: Endpoint[] = endpointsData ?? []

  const [editingRole, setEditingRole] = useState<Role | null>(null)
  const [checkedIds, setCheckedIds] = useState<string[]>([])

  // 按模块分组 endpoints
  const grouped = endpoints.reduce<Record<string, Endpoint[]>>((acc, ep) => {
    const mod = ep.module || '其他'
    if (!acc[mod]) acc[mod] = []
    acc[mod].push(ep)
    return acc
  }, {})

  const updateMut = useMutation({
    mutationFn: ({ id, identities }: { id: number; identities: string[] }) =>
      updateRoleEndpoints(id, identities),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['roles'] })
      setEditingRole(null)
      message.success('权限已保存')
    },
    onError: (e: Error) => message.error('保存失败：' + e.message),
  })

  const openEdit = (role: Role) => {
    setEditingRole(role)
    setCheckedIds(role.endpoints?.map((ep) => ep.identity) ?? [])
  }

  // 模块全选 / 反选
  const toggleModule = (eps: Endpoint[], checked: boolean) => {
    const ids = eps.map((ep) => ep.identity)
    if (checked) {
      setCheckedIds((prev) => Array.from(new Set([...prev, ...ids])))
    } else {
      setCheckedIds((prev) => prev.filter((id) => !ids.includes(id)))
    }
  }

  // 全选所有权限
  const allIds = endpoints.map((ep) => ep.identity)
  const allChecked = allIds.length > 0 && allIds.every((id) => checkedIds.includes(id))
  const allIndeterminate = !allChecked && allIds.some((id) => checkedIds.includes(id))

  const columns = [
    {
      title: '角色',
      render: (_: unknown, row: Role) => (
        <Space>
          <Tag color="purple" style={{ fontWeight: 600 }}>{row.name}</Tag>
          {row.description && (
            <Text style={{ fontSize: 12, color: c.textSecondary }}>{row.description}</Text>
          )}
        </Space>
      ),
    },
    {
      title: '继承自',
      render: (_: unknown, row: Role) => (
        <Space size={4}>
          {(row as Role & { parents?: string[] }).parents?.length
            ? (row as Role & { parents?: string[] }).parents!.map((p) => (
                <Tag key={p} color="blue">{p}</Tag>
              ))
            : <Text style={{ color: c.textTertiary, fontSize: 12 }}>—</Text>}
        </Space>
      ),
    },
    {
      title: '已绑定权限',
      render: (_: unknown, row: Role) => (
        <Space size={4} wrap>
          {row.endpoints?.length
            ? row.endpoints.map((ep) => (
                <Tag key={ep.identity} color="geekblue" style={{ fontSize: 11 }}>
                  {ep.identity}
                </Tag>
              ))
            : <Text style={{ color: c.textTertiary, fontSize: 12 }}>无权限</Text>}
        </Space>
      ),
    },
    {
      title: '操作',
      width: 80,
      render: (_: unknown, row: Role) => (
        <Tooltip title="编辑权限">
          <Button size="small" icon={<EditOutlined />} onClick={() => openEdit(row)} />
        </Tooltip>
      ),
    },
  ]

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 20 }}>
        <SafetyCertificateOutlined style={{ fontSize: 20, color: c.primary }} />
        <Title level={4} style={{ margin: 0, color: c.textBody }}>角色管理</Title>
      </div>

      <Card style={{ borderRadius: 12, border: `1px solid ${c.border}`, boxShadow: 'none', background: c.bgSurface }}>
        <Table
          dataSource={roles}
          columns={columns}
          rowKey="id"
          loading={isLoading}
          pagination={false}
        />
      </Card>

      {/* 编辑权限弹窗 */}
      <Modal
        title={`编辑权限 — ${editingRole?.name}`}
        open={!!editingRole}
        onCancel={() => setEditingRole(null)}
        onOk={() => editingRole && updateMut.mutate({ id: editingRole.id, identities: checkedIds })}
        okText="保存"
        confirmLoading={updateMut.isPending}
        destroyOnClose
        width={600}
      >
        {/* 全选栏 */}
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '8px 0', marginBottom: 8, borderBottom: `1px solid ${c.border}` }}>
          <Checkbox
            indeterminate={allIndeterminate}
            checked={allChecked}
            onChange={(e) => {
              if (e.target.checked) {
                setCheckedIds(allIds)
              } else {
                setCheckedIds([])
              }
            }}
          >
            <span style={{ fontWeight: 600 }}>全选</span>
          </Checkbox>
          <Text style={{ fontSize: 12, color: c.textSecondary }}>
            已选 {checkedIds.length} / {allIds.length} 个权限
          </Text>
        </div>
        <Collapse
          size="small"
          defaultActiveKey={Object.keys(grouped)}
          items={Object.entries(grouped).map(([mod, eps]) => {
            const modIds = eps.map((ep) => ep.identity)
            const modChecked = modIds.every((id) => checkedIds.includes(id))
            const modIndeterminate = !modChecked && modIds.some((id) => checkedIds.includes(id))
            return {
              key: mod,
              label: (
                <Space onClick={(e) => e.stopPropagation()}>
                  <Checkbox
                    indeterminate={modIndeterminate}
                    checked={modChecked}
                    onChange={(e) => toggleModule(eps, e.target.checked)}
                  />
                  <span style={{ fontWeight: 600 }}>{mod}</span>
                  <Tag>{eps.filter((ep) => checkedIds.includes(ep.identity)).length}/{eps.length}</Tag>
                </Space>
              ),
            children: (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                {eps.map((ep) => (
                  <Checkbox
                    key={ep.identity}
                    checked={checkedIds.includes(ep.identity)}
                    onChange={(e) => {
                      setCheckedIds((prev) =>
                        e.target.checked
                          ? [...prev, ep.identity]
                          : prev.filter((id) => id !== ep.identity),
                      )
                    }}
                  >
                    <Space size={6}>
                      <Text code style={{ fontSize: 12 }}>{ep.identity}</Text>
                      <Text style={{ fontSize: 12, color: c.textSecondary }}>
                        {IDENTITY_LABEL[ep.identity] || ep.remark}
                      </Text>
                    </Space>
                  </Checkbox>
                ))}
              </div>
            ),
          }
          })}
        />
      </Modal>
    </div>
  )
}
