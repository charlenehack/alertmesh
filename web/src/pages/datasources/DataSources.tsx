// Data sources management page (admin-only).
//
// Single registry for every upstream the platform talks to:
//  - Prometheus      (queried by AI tools + the Explore graph page; never
//                     used to push alerts — alertmesh stays a passive
//                     receiver per the README).
//  - Kubernetes /
//    OpenSearch /
//    Elasticsearch / (Phase-3+ connectors will subscribe and map raw
//    Kafka              events into RawAlert via the §4.1.4 selector +
//                       mapper pipeline; today this page persists the
//                       registry rows + smoke-tests credentials).
//
// This file is the orchestrator: page chrome + table + drawer + mutations.
// Per-kind form bodies live in `./forms/*Form.tsx`; shared helpers
// (mapping helpers, constants, serialization) live under `./shared/`.
//
// Secret handling matches LLMProviders.tsx:
//   - All values entered into "敏感字段" inputs are RSA-encrypted with
//     the system public key before they leave the browser ("ENC:" prefix).
//   - Edits send "******" for any secret the user did not touch — the
//     server then keeps the existing ciphertext untouched.

import { useMemo, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Alert, App, Button, Divider, Drawer, Form, Input, Popconfirm, Segmented,
  Select, Space, Switch, Table, Tabs, Tag, Tooltip, Typography,
} from 'antd'
import {
  CheckCircleOutlined, DatabaseOutlined, DeleteOutlined, EditOutlined,
  ExperimentOutlined, LineChartOutlined, PlusOutlined, ThunderboltOutlined,
} from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'

import {
  createDataSource, deleteDataSource, getDataSources, setDefaultDataSource,
  testDataSource, updateDataSource,
} from '../../api/datasources'
import { PageHeader } from '../../components/PageHeader'
import { SurfaceCard } from '../../components/SurfaceCard'
import type { DataSource, DataSourceKind } from '../../types'

import { isAIEligibleKind, KIND_LABEL, KIND_TABS } from './shared/constants'
import { kindDefaults } from './shared/defaults'
import { attachKafkaError } from './shared/kafkaMapping'
import { formToPayload, rowToForm } from './shared/serialize'
import type { DataSourceFormShape } from './shared/types'

import { ElasticForm } from './forms/ElasticForm'
import { KafkaBasicSection, KafkaMappingSection } from './forms/KafkaForm'
import { KubernetesForm } from './forms/KubernetesForm'
import { OpenSearchForm } from './forms/OpenSearchForm'
import { PrometheusForm } from './forms/PrometheusForm'

const { Text } = Typography

export default function DataSources() {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const { message } = App.useApp()
  const [tab, setTab] = useState<'all' | DataSourceKind>('all')
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<DataSource | null>(null)
  const [form] = Form.useForm<DataSourceFormShape>()
  const [testing, setTesting] = useState<string | null>(null)
  // The form's `kind` is what drives which sub-section renders below.  We
  // keep a mirror in component state so the JSX can react to it without
  // calling form.getFieldValue on every render.
  const [activeKind, setActiveKind] = useState<DataSourceKind>('prometheus')
  // Drawer tab index — reset whenever the drawer (re)opens so a previous
  // editing session doesn't land on a hidden tab (e.g. mapping tab when
  // the new row is non-kafka).
  const [drawerTab, setDrawerTab] = useState<'basic' | 'mapping'>('basic')

  const { data, isLoading } = useQuery({
    queryKey: ['data-sources'],
    queryFn: () => getDataSources(),
  })
  const allRows = useMemo<DataSource[]>(() => data ?? [], [data])
  const rows = useMemo(
    () => (tab === 'all' ? allRows : allRows.filter((r) => r.kind === tab)),
    [allRows, tab],
  )

  const saveMut = useMutation({
    mutationFn: async (values: DataSourceFormShape) => {
      const payload = await formToPayload(values)
      if (editing) return updateDataSource(editing.id, payload)
      return createDataSource(payload)
    },
    onSuccess: () => {
      message.success(editing ? '已更新' : '已创建')
      qc.invalidateQueries({ queryKey: ['data-sources'] })
      setOpen(false)
      setEditing(null)
      form.resetFields()
    },
    onError: (err: Error) => {
      const msg = err.message || '保存失败'
      // For Kafka mapping/filter compile errors, attach the message inline
      // to the offending field instead of (or in addition to, when nothing
      // matches) the global toast.  Mirrors backend wrapping in
      // internal/ingestion/kafka_filter.go::CompileKafkaProgram.
      if (attachKafkaError(form, msg)) {
        // Make sure the operator can actually see the inline error: jump
        // to the mapping tab when the failure originated there.
        if (/^kafka\s+(mapping|filter)/.test(msg)) setDrawerTab('mapping')
      } else {
        message.error(msg)
      }
    },
  })

  const deleteMut = useMutation({
    mutationFn: deleteDataSource,
    onSuccess: () => {
      message.success('已删除')
      qc.invalidateQueries({ queryKey: ['data-sources'] })
    },
    onError: (err: Error) => message.error(err.message || '删除失败'),
  })

  const setDefaultMut = useMutation({
    mutationFn: setDefaultDataSource,
    onSuccess: () => {
      message.success('默认数据源已切换')
      qc.invalidateQueries({ queryKey: ['data-sources'] })
    },
    onError: (err: Error) => message.error(err.message || '设置默认失败'),
  })

  const runTest = async (row: DataSource) => {
    setTesting(row.id)
    try {
      // Saved row → backend re-uses the stored ciphertext; only the id matters.
      const r = await testDataSource(row.id, { kind: row.kind })
      if (r.ok) message.success(`连接成功：${r.message || 'ok'}`)
      else message.error(`连接失败：${r.message || 'unknown'}`)
      qc.invalidateQueries({ queryKey: ['data-sources'] })
    } catch (err) {
      message.error((err as Error).message || '连接测试失败')
    } finally {
      setTesting(null)
    }
  }

  const runFormTest = async () => {
    const v = await form.validateFields().catch(() => null)
    if (!v) return
    setTesting('form')
    try {
      const payload = await formToPayload(v)
      const r = await testDataSource(editing?.id || 'new', payload)
      if (r.ok) message.success(`连接成功：${r.message || 'ok'}`)
      else message.error(`连接失败：${r.message || 'unknown'}`)
    } catch (err) {
      message.error((err as Error).message || '连接测试失败')
    } finally {
      setTesting(null)
    }
  }

  const openCreate = () => {
    setEditing(null)
    form.resetFields()
    const initialKind: DataSourceKind = tab === 'all' ? 'prometheus' : tab
    setActiveKind(initialKind)
    setDrawerTab('basic')
    form.setFieldsValue({
      kind: initialKind,
      is_enabled: true,
      is_default: false,
      ai_enabled: false,
      ai_auto_trigger: false,
      ...kindDefaults(initialKind),
    } as DataSourceFormShape)
    setOpen(true)
  }

  const openEdit = (row: DataSource) => {
    setEditing(row)
    setActiveKind(row.kind)
    setDrawerTab('basic')
    form.resetFields()
    form.setFieldsValue(rowToForm(row))
    setOpen(true)
  }

  const columns = [
    {
      title: '名称',
      dataIndex: 'name',
      render: (name: string, row: DataSource) => (
        <Space>
          <Text style={{ fontWeight: 500 }}>{name}</Text>
          {row.is_default && (
            <Tag color="blue" icon={<CheckCircleOutlined />} style={{ fontSize: 11 }}>
              默认
            </Tag>
          )}
        </Space>
      ),
    },
    {
      title: '类型',
      dataIndex: 'kind',
      width: 130,
      render: (k: DataSourceKind) => <Tag style={{ fontSize: 11 }}>{KIND_LABEL[k]}</Tag>,
    },
    {
      title: 'Endpoint',
      dataIndex: 'endpoint',
      ellipsis: true,
      render: (e: string) =>
        e ? <Text type="secondary" style={{ fontSize: 11 }}>{e}</Text>
          : <Text type="secondary" style={{ fontSize: 11 }}>—</Text>,
    },
    {
      title: '凭据',
      dataIndex: 'secret_keys',
      width: 160,
      render: (keys: string[]) =>
        keys.length === 0
          ? <Text type="secondary" style={{ fontSize: 11 }}>无</Text>
          : <Space size={4} wrap>
              {keys.map((k) => <Tag key={k} color="purple" style={{ fontSize: 10 }}>{k}</Tag>)}
            </Space>,
    },
    {
      title: 'AI',
      dataIndex: 'ai_enabled',
      width: 90,
      render: (v: boolean, row: DataSource) => {
        if (!isAIEligibleKind(row.kind)) {
          return (
            <Tooltip title="仅 Kafka / OpenSearch / Elasticsearch 数据源支持 AI 分析">
              <Tag style={{ fontSize: 11 }}>不支持</Tag>
            </Tooltip>
          )
        }
        if (!v) {
          return <Tag style={{ fontSize: 11 }}>未启用</Tag>
        }
        return (
          <Space size={4}>
            <Tag color="green" style={{ fontSize: 11 }}>已启用</Tag>
            {row.ai_auto_trigger && (
              <Tag color="purple" style={{ fontSize: 10 }}>创建自动</Tag>
            )}
          </Space>
        )
      },
    },
    {
      title: '上次测试',
      dataIndex: 'last_test_at',
      width: 130,
      render: (t: string | null, row: DataSource) => {
        if (!t) return <Text type="secondary" style={{ fontSize: 11 }}>—</Text>
        const ok = row.last_test_ok ?? false
        return (
          <Tooltip title={row.last_error || (ok ? 'ok' : 'failed')}>
            <Tag color={ok ? 'green' : 'red'} style={{ fontSize: 11 }}>
              {ok ? '通过' : '失败'} · {new Date(t).toLocaleTimeString()}
            </Tag>
          </Tooltip>
        )
      },
    },
    {
      title: '启用',
      dataIndex: 'is_enabled',
      width: 70,
      render: (v: boolean) => <Switch checked={v} size="small" disabled />,
    },
    {
      title: '操作',
      width: 240,
      render: (_: unknown, row: DataSource) => (
        <Space size={4}>
          {row.kind === 'prometheus' && (
            <Tooltip title="PromQL Explore">
              <Button
                size="small" type="text"
                icon={<LineChartOutlined />}
                onClick={() => navigate(`/datasources/${row.id}/prom-explore`)}
              />
            </Tooltip>
          )}
          <Tooltip title="测试连接">
            <Button
              size="small" type="text"
              icon={<ExperimentOutlined />}
              loading={testing === row.id}
              onClick={() => runTest(row)}
            />
          </Tooltip>
          {!row.is_default && (
            <Tooltip title="设为该 kind 的默认数据源">
              <Button
                size="small" type="text"
                icon={<ThunderboltOutlined />}
                loading={setDefaultMut.isPending}
                onClick={() => setDefaultMut.mutate(row.id)}
              />
            </Tooltip>
          )}
          <Tooltip title="编辑">
            <Button size="small" type="text" icon={<EditOutlined />} onClick={() => openEdit(row)} />
          </Tooltip>
          <Popconfirm
            title="确认删除？"
            description={row.is_default ? '此数据源为该 kind 的默认，删除后相关连接器将无法运行。' : undefined}
            okButtonProps={{ danger: true }}
            onConfirm={() => deleteMut.mutate(row.id)}
          >
            <Button size="small" type="text" danger icon={<DeleteOutlined />} />
          </Popconfirm>
        </Space>
      ),
    },
  ]

  // Drawer tab definitions: "基础配置" always present; "字段映射 / 过滤"
  // only renders for Kafka (the other kinds have no per-row mapping).
  // Computed inside render so changing `activeKind` re-derives the tab
  // list without an effect.
  const editingId = editing?.id || ''
  // forceRender on every tab item is mandatory here: antd's default is
  // to lazy-mount inactive tab panels, which means Form.Items inside an
  // unvisited tab never register against the Form.useForm() store.  If
  // the operator only touches the "基础配置" tab and hits 保存, the
  // mapping fields (kafka_map_alertname / kafka_labels / ...) come back
  // as `undefined` from form.getFieldsValue(), kafkaMappingFromForm
  // collapses them to empty strings, and the PUT payload silently wipes
  // the entire mapping/filter/labels/annotations server-side.  Backend
  // bounces with "alertname 路径必填" only because alertname has a
  // required validator — for any source that *could* save with empty
  // mapping (e.g. if defaults change later) the data loss would be
  // invisible.  Forcing both tabs to mount keeps every Form.Item
  // registered and guarantees getFieldsValue() returns a full snapshot.
  const tabItems = [
    {
      key: 'basic',
      label: '基础配置',
      forceRender: true,
      children: <BasicTab activeKind={activeKind} editing={!!editing} />,
    },
    ...(activeKind === 'kafka' ? [{
      key: 'mapping',
      label: '字段映射 / 过滤',
      forceRender: true,
      children: <KafkaMappingSection editingId={editingId} />,
    }] : []),
  ]

  return (
    <div>
      <PageHeader
        title="数据源"
        icon={<DatabaseOutlined />}
        description="统一管理 Prometheus / Kubernetes / OpenSearch / Elasticsearch / Kafka 接入"
        extra={
          <>
            <Segmented
              value={tab}
              options={KIND_TABS.map((t) => ({ label: t.label, value: t.value }))}
              onChange={(v) => setTab(v as 'all' | DataSourceKind)}
            />
            <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>新建</Button>
          </>
        }
      />

      {allRows.length === 0 && !isLoading && (
        <Alert
          type="info" showIcon style={{ marginBottom: 16 }}
          message="尚未配置任何数据源"
          description={
            <span>
              添加 Prometheus 后，事件详情页的 AI 分析会自动调用 <Text code style={{ fontSize: 11 }}>metrics_query</Text> 工具检索指标；
              添加 Kubernetes / OpenSearch / Elasticsearch / Kafka 后，Phase-3 连接器会按 §4.1.4 selector + mapper 配置订阅消息并写入 alerts。
            </span>
          }
        />
      )}

      <SurfaceCard flush>
        <Table
          dataSource={rows} columns={columns} rowKey="id" loading={isLoading}
          size="small" pagination={{ pageSize: 15 }}
        />
      </SurfaceCard>

      <Drawer
        title={editing ? `编辑数据源 — ${KIND_LABEL[editing.kind]}` : '新建数据源'}
        open={open}
        // Responsive width: ~960px on wide displays, never wider than
        // 80% of the viewport so the drawer leaves room for the page
        // chrome / dimming overlay on smaller laptops.  Replaces the
        // previous fixed `size={620}` that clipped the kafka mapping
        // rows.
        width="min(960px, 80vw)"
        onClose={() => { setOpen(false); setEditing(null) }}
        styles={{
          header: { background: '#111111', borderBottom: '1px solid #1e1e1e', color: '#e8e8e8' },
          body:   { background: '#111111', padding: '20px 24px' },
          footer: { background: '#111111', borderTop: '1px solid #1e1e1e' },
        }}
        footer={
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <Button icon={<ExperimentOutlined />} loading={testing === 'form'} onClick={runFormTest}>
              测试连接
            </Button>
            <Space>
              <Button onClick={() => { setOpen(false); setEditing(null) }}>取消</Button>
              <Button type="primary" loading={saveMut.isPending} onClick={() => form.submit()}>
                {editing ? '保存' : '创建'}
              </Button>
            </Space>
          </div>
        }
      >
        <Alert
          type="warning" showIcon style={{ marginBottom: 16 }}
          message="敏感字段端到端加密"
          description="所有 token / password 字段在浏览器侧使用系统 RSA 公钥加密后以 ENC: 前缀提交；服务端落库前再做 AES-256-GCM 加密。列表 / 详情接口仅返回字段名，编辑时保留 ****** 即可不修改。"
        />

        <Form
          form={form}
          layout="vertical"
          onFinish={(values) => saveMut.mutate(values)}
          onValuesChange={(changed) => {
            if (changed.kind && changed.kind !== activeKind) {
              setActiveKind(changed.kind)
              // Switching to a non-Kafka kind hides the mapping tab; if
              // the operator was on it, snap back to basic so they're
              // never staring at an empty drawer body.
              setDrawerTab('basic')
              // Reset kind-specific fields so we don't smuggle stale state
              // (e.g. kafka topic) into a prometheus row.
              form.setFieldsValue(kindDefaults(changed.kind))
            }
            if (changed.ai_enabled === false) {
              form.setFieldsValue({ ai_auto_trigger: false })
            }
          }}
        >
          <Tabs
            activeKey={drawerTab}
            onChange={(k) => setDrawerTab(k as 'basic' | 'mapping')}
            items={tabItems}
          />
        </Form>
      </Drawer>
    </div>
  )
}

// ─── BasicTab: name / kind / description + per-kind connection fields ────────
//
// Pulled out into its own component so the orchestrator's `tabItems`
// definition reads as a flat list, and the per-kind switch lives close
// to the form fields it controls.
interface BasicTabProps {
  activeKind: DataSourceKind
  editing: boolean
}

function BasicTab({ activeKind, editing }: BasicTabProps) {
  const form = Form.useFormInstance()
  return (
    <>
      <Form.Item name="name" label="名称" rules={[{ required: true, message: '名称必填' }]}>
        <Input placeholder="例如：prom-prod / k8s-uat / opensearch-app-logs / elastic-search / kafka-higress" />
      </Form.Item>

      <Form.Item name="kind" label="类型" rules={[{ required: true }]} initialValue="prometheus">
        <Select disabled={editing} options={KIND_TABS.filter((t) => t.value !== 'all').map((t) => ({
          value: t.value, label: t.label,
        }))} />
      </Form.Item>

      <Form.Item name="description" label="描述">
        <Input.TextArea rows={2} placeholder="可选，用于团队识别" />
      </Form.Item>

      {activeKind === 'prometheus' && <PrometheusForm editing={editing} />}
      {activeKind === 'k8s'        && <KubernetesForm editing={editing} />}
      {activeKind === 'opensearch' && <OpenSearchForm editing={editing} />}
      {activeKind === 'elastic'    && <ElasticForm    editing={editing} />}
      {activeKind === 'kafka'      && <KafkaBasicSection editing={editing} />}

      <Divider style={{ borderColor: '#1e1e1e', margin: '8px 0 16px' }} />

      {isAIEligibleKind(activeKind) && (
        <>
          <Form.Item
            name="ai_enabled"
            label="AI 分析"
            valuePropName="checked"
            extra={
              <span style={{ color: '#666', fontSize: 11 }}>
                仅日志类数据源支持。开启后事件详情可手动「触发 AI 分析」；关闭则隐藏 AI 入口且不消耗 token。
              </span>
            }
          >
            <Switch />
          </Form.Item>
          <Form.Item
            noStyle
            shouldUpdate={(prev, cur) => prev.ai_enabled !== cur.ai_enabled}
          >
            {() => (
              <Form.Item
                name="ai_auto_trigger"
                label="创建事件后自动触发 AI"
                valuePropName="checked"
                hidden={!form.getFieldValue('ai_enabled')}
                extra={
                  <span style={{ color: '#666', fontSize: 11 }}>
                    默认关闭：聚合并通知后由人工决定是否分析，最省 token。开启后新事件会立即入队 AI 任务。
                  </span>
                }
              >
                <Switch disabled={!form.getFieldValue('ai_enabled')} />
              </Form.Item>
            )}
          </Form.Item>
        </>
      )}

      <Form.Item name="is_default" label="设为该 kind 的默认" valuePropName="checked" extra={
        <span style={{ color: '#666', fontSize: 11 }}>
          每个 kind 仅一个默认；AI 分析 / 连接器在没有显式 source_id 时使用该默认行。
        </span>
      }>
        <Switch />
      </Form.Item>

      <Form.Item name="is_enabled" label="启用" valuePropName="checked" initialValue={true}>
        <Switch />
      </Form.Item>
    </>
  )
}

