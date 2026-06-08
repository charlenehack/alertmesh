import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Table, Button, Tag, Switch, Drawer, Form, Input,
  InputNumber, Space, Popconfirm, App, Typography, Select, Collapse,
} from 'antd'
import { PlusOutlined, EditOutlined, DeleteOutlined, NodeIndexOutlined } from '@ant-design/icons'
import {
  getAlertRoutes, createAlertRoute, updateAlertRoute, deleteAlertRoute,
  getPolicies,
} from '../../api/alert'
import { ApiError } from '../../api/request'
import { PageHeader } from '../../components/PageHeader'
import { SurfaceCard } from '../../components/SurfaceCard'
import { useTheme } from '../../hooks/useTheme'
import type { AlertRoute, LabelMatcher, NotificationPolicy } from '../../types'

const ALLOWED_MATCHER_OPS: LabelMatcher['op'][] = ['=', '!=', '=~', '!~']

// Engine-normalised severities the matcher engine actually compares against;
// see internal/engine/aggregation.go mapSeverity.  Anything outside this set
// gets folded into "P3" before routing, so an operator typing
// severity="critical" will never see the route fire.  We surface that as a
// non-blocking warning rather than an error so power users can still do it.
const NORMALISED_SEVERITIES = new Set(['P0', 'P1', 'P2', 'P3'])

// parseMatchers normalises the textarea contents into LabelMatcher[].  An
// empty or whitespace-only input returns [] which the backend interprets as a
// catch-all (兜底) route — combined with priority=0 this is the canonical
// shape for "match anything not handled above".  Both UI-style ({key,op})
// and engine-native ({name,isRegex}) shapes are accepted; we emit the UI
// shape so the backend serialises a stable on-disk representation.
function parseMatchers(raw: string): LabelMatcher[] {
  const trimmed = (raw ?? '').trim()
  if (!trimmed) return []
  let parsed: unknown
  try {
    parsed = JSON.parse(trimmed)
  } catch (err) {
    throw new Error(`JSON 解析失败：${(err as Error).message}`)
  }
  if (!Array.isArray(parsed)) {
    throw new Error('匹配条件必须是 JSON 数组，例如 [{"name":"severity","op":"=","value":"P1"}]')
  }
  return parsed.map((item, idx) => {
    if (!item || typeof item !== 'object') {
      throw new Error(`第 ${idx + 1} 项必须是对象`)
    }
    const m = item as Record<string, unknown>
    const rawKey = (typeof m.key === 'string' && m.key) ? m.key
      : (typeof m.name === 'string' && m.name) ? m.name : ''
    if (!rawKey) {
      throw new Error(`第 ${idx + 1} 项缺少字符串字段 "key"（或 "name"）`)
    }
    const op = (typeof m.op === 'string' && m.op) ? m.op : '='
    if (!ALLOWED_MATCHER_OPS.includes(op as LabelMatcher['op'])) {
      throw new Error(`第 ${idx + 1} 项的 "op" 必须是 ${ALLOWED_MATCHER_OPS.join(' / ')} 之一`)
    }
    if (m.value === undefined || m.value === null) {
      throw new Error(`第 ${idx + 1} 项缺少字段 "value"`)
    }
    return { key: rawKey, op: op as LabelMatcher['op'], value: String(m.value) }
  })
}

// severityWarnings returns human-readable hints when an operator types a
// severity matcher value that the engine will silently rewrite to "P3".
// These are warnings, not errors: returning an empty array means everything
// looks normal.
function severityWarnings(matchers: LabelMatcher[]): string[] {
  return matchers
    .filter((m) => m.key === 'severity' && !NORMALISED_SEVERITIES.has(m.value))
    .map((m) => `severity="${m.value}" 不在 P0/P1/P2/P3 内，引擎会把它归一化为 P3，当前 matcher 永远不会命中。`)
}

const { Text } = Typography
const { TextArea } = Input

export default function AlertRoutes() {
  const qc = useQueryClient()
  const { message } = App.useApp()
  const { c } = useTheme()
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<AlertRoute | null>(null)
  const [form] = Form.useForm()

  const { data, isLoading } = useQuery({ queryKey: ['alert-routes'], queryFn: getAlertRoutes })
  const rows: AlertRoute[] = data ?? []

  const { data: policyData } = useQuery({ queryKey: ['policies'], queryFn: getPolicies })
  const allPolicies: NotificationPolicy[] = policyData ?? []
  const policyNameMap = Object.fromEntries(allPolicies.map((p) => [p.id, p.name]))

  const toFormValues = (row: AlertRoute) => ({
    ...row,
    matchers: JSON.stringify(row.matchers, null, 2),
    group_by: Array.isArray(row.group_by) ? row.group_by.join(', ') : '',
    channel_ids: Array.isArray(row.channel_ids) ? row.channel_ids : [],
  })

  const fromFormValues = (values: Record<string, unknown>) => ({
    ...values,
    matchers: parseMatchers((values.matchers as string) || '[]'),
    group_by: String(values.group_by || '').split(',').map((s: string) => s.trim()).filter(Boolean),
    channel_ids: Array.isArray(values.channel_ids) ? values.channel_ids : [],
  })

  const saveMut = useMutation({
    mutationFn: (values: Record<string, unknown>) => {
      let payload: ReturnType<typeof fromFormValues>
      try {
        payload = fromFormValues(values)
      } catch (err) {
        return Promise.reject(err)
      }
      // Non-blocking severity hints: surface to the operator but still save.
      severityWarnings(payload.matchers).forEach((w) => message.warning(w))
      return editing ? updateAlertRoute(editing.id, payload) : createAlertRoute(payload)
    },
    onSuccess: () => {
      message.success(editing ? '更新成功' : '创建成功')
      qc.invalidateQueries({ queryKey: ['alert-routes'] })
      setOpen(false); form.resetFields(); setEditing(null)
    },
    onError: (err: unknown) => {
      const msg = err instanceof ApiError
        ? `${err.message}${err.status ? ` (HTTP ${err.status})` : ''}`
        : err instanceof Error ? err.message : '操作失败'
      message.error(msg)
    },
  })

  const deleteMut = useMutation({
    mutationFn: deleteAlertRoute,
    onSuccess: () => { message.success('已删除'); qc.invalidateQueries({ queryKey: ['alert-routes'] }) },
  })

  const columns = [
    {
      title: '优先级',
      dataIndex: 'priority',
      width: 80,
      render: (p: number) => <Tag style={{ background: c.bgElevated, borderColor: c.border, color: c.textHint }}>{p}</Tag>,
    },
    {
      title: '名称',
      dataIndex: 'name',
      render: (n: string) => <Text style={{ fontWeight: 500 }}>{n}</Text>,
    },
    {
      title: '匹配条件',
      dataIndex: 'matchers',
      render: (m: unknown[]) => {
        if (!Array.isArray(m) || m.length === 0) {
          return (
            <Tag color="gold" style={{ fontSize: 11 }}>兜底（匹配所有）</Tag>
          )
        }
        return (
          <span style={{ fontSize: 11, color: c.textSecondary, fontFamily: 'monospace' }}>
            {m.map((x: unknown) => {
              const matcher = x as { key?: string; name?: string; op?: string; value?: string }
              const label = matcher.key || matcher.name || ''
              return `${label}${matcher.op || '='}${matcher.value}`
            }).join(' & ')}
          </span>
        )
      },
    },
    {
      title: '分组字段',
      dataIndex: 'group_by',
      render: (g: string[]) => (
        <>{(g || []).map((k: string) => <Tag key={k} style={{ fontSize: 11 }}>{k}</Tag>)}</>
      ),
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
      render: (_: unknown, row: AlertRoute) => (
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
        title="告警路由策略"
        icon={<NodeIndexOutlined />}
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
        title={editing ? '编辑路由策略' : '新建路由策略'}
        open={open}
        size={600}
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
        <Form
          form={form}
          layout="vertical"
          onFinish={saveMut.mutate}
          onFinishFailed={({ errorFields }) => {
            const first = errorFields[0]?.errors?.[0]
            message.error(first || '请检查表单填写')
          }}
        >
          <Form.Item name="name" label="策略名称" rules={[{ required: true }]}>
            <Input placeholder="路由策略名称" />
          </Form.Item>
          <Form.Item name="priority" label="优先级（数值越大越优先）" initialValue={0}>
            <InputNumber style={{ width: '100%' }} min={0} max={999} />
          </Form.Item>
          <Form.Item
            name="matchers"
            label="匹配条件（JSON 数组，留空 = 兜底）"
            validateTrigger={['onBlur', 'onSubmit']}
            rules={[
              {
                validator: async (_, value: string) => {
                  if (!value || !value.trim()) return
                  parseMatchers(value)
                },
              },
            ]}
            extra={
              <span style={{ color: c.textTertiary, fontSize: 11 }}>
                命中后该路由才会接管这条告警，并把它发往下面选择的通知策略。
                严重级请使用 <Text code style={{ fontSize: 11 }}>P0/P1/P2/P3</Text>；
                <strong style={{ color: '#faad14' }}> 留空 + 最低优先级</strong> 即兜底路由（建议指向 SRE on-call 通知策略）。
              </span>
            }
          >
            <TextArea rows={4}
              placeholder='留空表示兜底（匹配所有未匹配的告警）；示例：[{"name":"severity","op":"=","value":"P1"}]'
              style={{ fontFamily: 'monospace', fontSize: 12 }} />
          </Form.Item>

          <Form.Item
            name="channel_ids"
            label="通知策略"
            rules={[{ required: true, message: '请至少选择一个通知策略' }]}
            extra={
              <span style={{ color: c.textTertiary, fontSize: 11 }}>
                命中本路由的告警会按所选策略路由给联系人 / 联系人组。
                {allPolicies.length === 0 && (
                  <span style={{ color: '#fa8c16' }}> 当前还没有通知策略，请先到「通知策略」页面创建。</span>
                )}
              </span>
            }
          >
            <Select
              mode="multiple"
              placeholder="选择一个或多个通知策略"
              showSearch
              optionFilterProp="label"
              options={allPolicies.map((p) => ({
                value: p.id,
                label: p.name,
              }))}
              tagRender={({ value, onClose }) => (
                <Tag closable onClose={onClose}
                  style={{ background: 'rgba(22,119,255,0.1)', border: '1px solid rgba(22,119,255,0.3)', color: '#1677ff', marginRight: 4 }}>
                  {policyNameMap[value as string] || value}
                </Tag>
              )}
            />
          </Form.Item>

          <Form.Item name="description" label="描述">
            <Input placeholder="可选描述" />
          </Form.Item>
          <Form.Item name="is_enabled" label="启用" valuePropName="checked" initialValue={true}>
            <Switch />
          </Form.Item>

          <Collapse
            ghost
            items={[
              {
                key: 'advanced',
                label: <span style={{ color: c.textTertiary, fontSize: 13 }}>高级配置（可选）</span>,
                children: (
                  <>
                    <Form.Item
                      name="group_by"
                      label="分组字段（逗号分隔）"
                      extra={
                        <span style={{ color: c.textTertiary, fontSize: 11 }}>
                          仅作为兜底；建议在「告警聚合策略」里集中管理。留空时引擎按 alertname 分组。
                        </span>
                      }
                    >
                      <Input placeholder="alertname, cluster" />
                    </Form.Item>
                  </>
                ),
              },
            ]}
          />
        </Form>
      </Drawer>
    </div>
  )
}
