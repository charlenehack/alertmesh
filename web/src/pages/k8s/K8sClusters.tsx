/**
 * K8sClusters – 集群管理
 */
import { useNavigate } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Button, Empty, Space, Typography, Input, Spin, Drawer, Tag } from 'antd'
import {
  PlusOutlined, ReloadOutlined, CheckCircleOutlined,
  CloseCircleOutlined, SearchOutlined,
} from '@ant-design/icons'
import { useState } from 'react'
import type { ReactNode } from 'react'
import dayjs from 'dayjs'
import { PageHeader } from '../../components/PageHeader'
import { SurfaceCard } from '../../components/SurfaceCard'
import { useTheme } from '../../hooks/useTheme'
import { http } from '../../api/request'
import { useClusters, useSelectedCluster, type ClusterRow } from './useCluster'

const { Text } = Typography

// ─── types ────────────────────────────────────────────────────────────────────

interface ClusterSummary {
  total_nodes: number
  ready_nodes: number
  cpu_usage_rate: number
  mem_usage_rate: number
  cpu_request_rate: number
  mem_request_rate: number
  metrics_available: boolean
}

interface ClusterDetail {
  name: string
  description: string
  created_at: string
  is_enabled: boolean
  total_nodes: number
  ready_nodes: number
  total_pods: number
  pod_capacity: number
  cap_cpu_cores: number
  cap_mem_gi: number
  alloc_cpu_cores: number
  alloc_mem_gi: number
  metrics_available: boolean
  usage_cpu_cores: number
  usage_mem_gi: number
  cpu_usage_rate: number
  mem_usage_rate: number
  req_cpu_cores: number
  req_mem_gi: number
  lim_cpu_cores: number
  lim_mem_gi: number
  cpu_req_rate: number
  mem_req_rate: number
  cpu_lim_rate: number
  mem_lim_rate: number
}

// ─── helpers ──────────────────────────────────────────────────────────────────

const fmtCores     = (v: number) => `${v.toFixed(2)}核`
const fmtGi        = (v: number) => `${v.toFixed(2)}G`
const fmtRate      = (v: number) => v < 0 ? '—' : `${v.toFixed(2)}%`
const fmtCoresRate = (c: number, r: number) => r < 0 ? fmtCores(c) : `${fmtCores(c)}(${fmtRate(r)})`
const fmtGiRate    = (g: number, r: number) => r < 0 ? fmtGi(g)   : `${fmtGi(g)}(${fmtRate(r)})`

// ─── 单指标展示块 ─────────────────────────────────────────────────────────────

function Metric({ value, label }: { value: number; label: string }) {
  const { c } = useTheme()
  const na    = value < 0
  const color = na ? c.textTertiary
    : value > 85 ? c.danger
    : value > 70 ? c.warning
    : c.primary
  return (
    <div style={{ flex: 1, textAlign: 'center', padding: '12px 0' }}>
      <div style={{
        fontSize: 26,
        fontWeight: 700,
        color,
        lineHeight: 1,
        letterSpacing: '-0.5px',
        marginBottom: 8,
      }}>
        {na ? '—' : `${value.toFixed(2)}%`}
      </div>
      <div style={{ fontSize: 13, color: c.textSecondary }}>{label}</div>
    </div>
  )
}

// ─── 抽屉 row / section ───────────────────────────────────────────────────────

function DR({ label, value }: { label: string; value: ReactNode }) {
  const { c } = useTheme()
  return (
    <div style={{
      display: 'flex',
      alignItems: 'center',
      padding: '9px 0',
      borderBottom: `1px solid ${c.borderSubtle}`,
    }}>
      <span style={{ width: 130, flexShrink: 0, fontSize: 13, color: c.textSecondary }}>{label}</span>
      <span style={{ fontSize: 13, color: c.textBody }}>{value}</span>
    </div>
  )
}

function DS({ title, children }: { title: string; children: ReactNode }) {
  const { c } = useTheme()
  return (
    <div style={{ marginBottom: 24 }}>
      <div style={{
        fontSize: 12,
        fontWeight: 600,
        color: c.textSecondary,
        padding: '0 0 8px',
        borderBottom: `2px solid ${c.border}`,
        marginBottom: 0,
        letterSpacing: '0.3px',
      }}>
        {title}
      </div>
      {children}
    </div>
  )
}

// ─── ClusterDetailDrawer ──────────────────────────────────────────────────────

function ClusterDetailDrawer({ cluster, open, onClose }: {
  cluster: ClusterRow | null
  open: boolean
  onClose: () => void
}) {
  const { c } = useTheme()
  const { data, isLoading } = useQuery<ClusterDetail>({
    queryKey: ['k8s-cluster-detail', cluster?.id],
    queryFn: () => http.get<ClusterDetail>(`/k8s/cluster-detail?ds=${cluster!.id}`),
    enabled: open && !!cluster,
    refetchInterval: 30_000,
    retry: 1,
  })

  return (
    <Drawer
      title={cluster?.name ?? '集群详情'}
      placement="right"
      width={520}
      open={open}
      onClose={onClose}
      styles={{ body: { padding: '20px 24px', background: c.bgPage } }}
    >
      {isLoading || !data ? (
        <div style={{ textAlign: 'center', marginTop: 80 }}><Spin /></div>
      ) : (
        <>
          <DS title="基本信息">
            <DR label="集群名称" value={<Text strong>{data.name}</Text>} />
            <DR label="集群状态" value={
              data.is_enabled
                ? <Tag color="success" style={{ margin: 0 }}>正常</Tag>
                : <Tag color="default" style={{ margin: 0 }}>已禁用</Tag>
            } />
            <DR label="创建时间" value={dayjs(data.created_at).format('YYYY-MM-DD HH:mm:ss')} />
            {data.description && <DR label="描述" value={data.description} />}
          </DS>

          <DS title="节点信息">
            <DR label="可用节点 / 总数" value={`${data.ready_nodes} / ${data.total_nodes}`} />
          </DS>

          <DS title="Pod 信息">
            <DR label="Pod 数量"        value={data.total_pods} />
            <DR label="可容纳 Pod 总数"  value={data.pod_capacity} />
          </DS>

          <DS title="CPU 与内存信息">
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '0 32px' }}>
              <div>
                <DR label="CPU 规格"    value={fmtCores(data.cap_cpu_cores)} />
                <DR label="CPU 可分配"  value={fmtCores(data.alloc_cpu_cores)} />
                <DR label="CPU 使用量"  value={data.metrics_available ? fmtCoresRate(data.usage_cpu_cores, data.cpu_usage_rate) : '—'} />
                <DR label="CPU Request" value={fmtCoresRate(data.req_cpu_cores, data.cpu_req_rate)} />
                <DR label="CPU Limit"   value={fmtCoresRate(data.lim_cpu_cores, data.cpu_lim_rate)} />
              </div>
              <div>
                <DR label="Memory 规格"    value={fmtGi(data.cap_mem_gi)} />
                <DR label="Memory 可分配"  value={fmtGi(data.alloc_mem_gi)} />
                <DR label="Memory 使用量"  value={data.metrics_available ? fmtGiRate(data.usage_mem_gi, data.mem_usage_rate) : '—'} />
                <DR label="Memory Request" value={fmtGiRate(data.req_mem_gi, data.mem_req_rate)} />
                <DR label="Memory Limit"   value={fmtGiRate(data.lim_mem_gi, data.mem_lim_rate)} />
              </div>
            </div>
          </DS>
        </>
      )}
    </Drawer>
  )
}

// ─── ClusterCard ──────────────────────────────────────────────────────────────

function ClusterCard({ cluster, onNavigate, onDetail }: {
  cluster: ClusterRow
  onNavigate: (page: string, id: string) => void
  onDetail: () => void
}) {
  const { c } = useTheme()
  const { data: s, isLoading } = useQuery<ClusterSummary>({
    queryKey: ['k8s-cluster-summary', cluster.id],
    queryFn: () => http.get<ClusterSummary>(`/k8s/cluster-summary?ds=${cluster.id}`),
    enabled: cluster.is_enabled,
    refetchInterval: 30_000,
    retry: 1,
  })

  const isReady = cluster.last_test_ok === true
  const isError = cluster.last_test_ok === false
  const nodeOk  = s && s.ready_nodes === s.total_nodes

  return (
    <div style={{
      background: c.bgSurface,
      border: `1px solid ${c.border}`,
      borderRadius: 10,
      padding: '20px 24px',
      minWidth: 0,
      overflow: 'hidden',
      boxSizing: 'border-box',
    }}>
      {/* ── 标题行 ── */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 4 }}>
        <Text style={{ fontSize: 15, fontWeight: 600, color: c.textBody }}>
          {cluster.name}
        </Text>
        <Space size={8}>
          {isReady && (
            <Space size={4}>
              <CheckCircleOutlined style={{ color: c.success, fontSize: 13 }} />
              <span style={{ fontSize: 13, color: c.success }}>正常</span>
            </Space>
          )}
          {isError && (
            <Space size={4}>
              <CloseCircleOutlined style={{ color: c.danger, fontSize: 13 }} />
              <span style={{ fontSize: 13, color: c.danger }}>异常</span>
            </Space>
          )}
          {!cluster.is_enabled && <Tag color="default" style={{ margin: 0 }}>已禁用</Tag>}
        </Space>
      </div>

      {/* 描述 */}
      <div style={{ fontSize: 13, color: c.textSecondary, minHeight: 20, marginBottom: 14 }}>
        {cluster.description || ' '}
      </div>

      {/* ── 节点状态行 ── */}
      <div style={{
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        padding: '10px 0',
        borderTop: `1px dashed ${c.borderSubtle}`,
        borderBottom: `1px dashed ${c.borderSubtle}`,
        marginBottom: 0,
      }}>
        <span style={{ fontSize: 13, color: c.textSecondary }}>集群状态</span>
        {s ? (
          <Space size={20}>
            <span style={{ fontSize: 13, fontWeight: 500, color: nodeOk ? c.success : c.warning }}>
              {nodeOk ? '正常' : '部分异常'}
            </span>
            <span style={{ fontSize: 13, color: c.textSecondary }}>
              可用节点/总数&nbsp;&nbsp;{s.ready_nodes}/{s.total_nodes} 台
            </span>
          </Space>
        ) : (
          <span style={{ fontSize: 13, color: c.textSecondary }}>
            {isLoading ? <Spin size="small" /> : '—'}
          </span>
        )}
      </div>

      {/* ── 四指标 ── */}
      {isLoading ? (
        <div style={{ textAlign: 'center', height: 72, lineHeight: '72px' }}>
          <Spin size="small" />
        </div>
      ) : s ? (
        <div style={{ display: 'flex', justifyContent: 'space-around', marginBottom: 0 }}>
          <Metric value={s.cpu_usage_rate}   label="CPU使用率" />
          <Metric value={s.mem_usage_rate}   label="内存使用率" />
          <Metric value={s.cpu_request_rate} label="CPU申请率" />
          <Metric value={s.mem_request_rate} label="内存申请率" />
        </div>
      ) : (
        <div style={{ height: 72 }} />
      )}

      {/* ── 操作 ── */}
      <div style={{
        display: 'flex',
        justifyContent: 'flex-end',
        gap: 8,
        paddingTop: 12,
        borderTop: `1px solid ${c.borderSubtle}`,
      }}>
        <Button size="small" onClick={onDetail}>集群详情</Button>
        <Button size="small" type="primary" ghost onClick={() => onNavigate('resources', cluster.id)}>资源管理</Button>
      </div>
    </div>
  )
}

// ─── K8sClusters page ─────────────────────────────────────────────────────────

export default function K8sClusters() {
  const navigate = useNavigate()
  const qc       = useQueryClient()
  const { c }    = useTheme()
  const [search, setSearch]               = useState('')
  const [detailCluster, setDetailCluster] = useState<ClusterRow | null>(null)
  const { data, isLoading }               = useClusters()
  const { select }                        = useSelectedCluster(data)

  const clusters = (data ?? []).filter(cl =>
    !search || cl.name.toLowerCase().includes(search.toLowerCase())
  )

  const handleNavigate = (id: string, page: string) => { select(id); navigate(`/k8s/${page}`) }

  const handleRefresh = () => {
    qc.invalidateQueries({ queryKey: ['k8s-clusters'] })
    qc.invalidateQueries({ queryKey: ['k8s-cluster-summary'] })
    qc.invalidateQueries({ queryKey: ['k8s-cluster-detail'] })
  }

  return (
    <>
      <PageHeader
        title="集群管理"
        extra={
          <Space>
            <Button icon={<ReloadOutlined />} onClick={handleRefresh}>刷新</Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={() => navigate('/datasources')}>
              新建集群
            </Button>
          </Space>
        }
      />

      <div style={{ margin: '0 24px 24px' }}>
        {/* 搜索栏 */}
        <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: 20 }}>
          <Input
            placeholder="请输入集群名称"
            prefix={<SearchOutlined style={{ color: c.textSecondary }} />}
            allowClear
            style={{ width: 220 }}
            value={search}
            onChange={e => setSearch(e.target.value)}
          />
        </div>

        {isLoading ? (
          <Spin style={{ display: 'block', margin: '80px auto' }} />
        ) : clusters.length === 0 ? (
          <SurfaceCard>
            <Empty
              description={search ? `没有匹配「${search}」的集群` : '暂无 K8s 集群配置'}
              style={{ padding: '60px 0' }}
            >
              {!search && (
                <Button type="primary" icon={<PlusOutlined />} onClick={() => navigate('/datasources')}>
                  前往数据源管理添加
                </Button>
              )}
            </Empty>
          </SurfaceCard>
        ) : (
          <div style={{
            display: 'grid',
            gridTemplateColumns: `repeat(${Math.min(clusters.length, 3)}, 1fr)`,
            gap: 20,
            alignItems: 'start',
          }}>
            {clusters.map(cl => (
              <ClusterCard
                key={cl.id}
                cluster={cl}
                onNavigate={(page, id) => handleNavigate(id, page)}
                onDetail={() => setDetailCluster(cl)}
              />
            ))}
          </div>
        )}

        {clusters.length > 0 && (
          <div style={{ textAlign: 'right', marginTop: 16, fontSize: 12, color: c.textSecondary }}>
            共 {clusters.length} 条
          </div>
        )}
      </div>

      <ClusterDetailDrawer
        cluster={detailCluster}
        open={!!detailCluster}
        onClose={() => setDetailCluster(null)}
      />
    </>
  )
}
