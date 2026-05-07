import { useEffect, useMemo, useState } from 'react'
import { useParams, useNavigate, useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import {
  Card, Input, Button, Select, Space, Typography, Alert, Tag,
  Spin, Empty, Tooltip, Segmented, Table,
} from 'antd'
import type { ColumnsType } from 'antd/es/table'
import {
  PlayCircleOutlined, ReloadOutlined, ArrowLeftOutlined,
  LineChartOutlined, TableOutlined, AreaChartOutlined,
} from '@ant-design/icons'
import {
  LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip as RTip,
  Legend, ResponsiveContainer,
} from 'recharts'
import { promQueryRange, getDataSources } from '../../api/datasources'
import type { DataSource } from '../../types'

const { Title, Text } = Typography

// Quick-pick time ranges (label → seconds back from "now").  Mirrors what
// Prometheus's own /graph page offers + a couple of ops-centric windows.
const RANGES: { label: string; sec: number }[] = [
  { label: '过去 5 分钟',  sec: 5 * 60       },
  { label: '过去 15 分钟', sec: 15 * 60      },
  { label: '过去 30 分钟', sec: 30 * 60      },
  { label: '过去 1 小时',  sec: 60 * 60      },
  { label: '过去 3 小时',  sec: 3 * 60 * 60  },
  { label: '过去 6 小时',  sec: 6 * 60 * 60  },
  { label: '过去 12 小时', sec: 12 * 60 * 60 },
  { label: '过去 24 小时', sec: 24 * 60 * 60 },
  { label: '过去 7 天',    sec: 7 * 86400    },
]

// Auto-refresh interval options (label → seconds, 0 = off).  The selected
// value drives react-query's `refetchInterval` so the active view (Table /
// Graph) re-pulls data without the user clicking 查询 again, exactly
// matching what we labelled "step" in the toolbar.
const REFRESH_OPTIONS: { label: string; sec: number }[] = [
  { label: '关闭', sec: 0   },
  { label: '5s',  sec: 5   },
  { label: '10s', sec: 10  },
  { label: '30s', sec: 30  },
  { label: '1m',  sec: 60  },
  { label: '5m',  sec: 300 },
  { label: '15m', sec: 900 },
]

// autoStep mirrors Prometheus's /graph resolution heuristic: aim for ~250
// samples across the visible range so the line stays smooth without
// blowing up the response payload.  Returned as a Prometheus duration
// string ("15s", "1m", …) because the proxy forwards it verbatim.
function autoStep(rangeSec: number): string {
  const sec = Math.max(1, Math.floor(rangeSec / 250))
  if (sec >= 3600) return `${Math.round(sec / 3600)}h`
  if (sec >= 60)   return `${Math.round(sec / 60)}m`
  return `${sec}s`
}

// ─── Prometheus result reshape ────────────────────────────────────────────────
//
// Both matrix (range query) and vector (instant query) get normalised into a
// single SeriesInfo[] so the Table / Graph renderers below don't have to care
// which API path produced them.

type Matrix = { metric: Record<string, string>; values: [number, string][] }
type Vector = { metric: Record<string, string>; value:  [number, string]   }

interface SeriesInfo {
  label:   string                          // legend-friendly metric{a="x", …}
  metric:  Record<string, string>          // raw labels for table columns
  samples: { t: number; v: number }[]      // ms timestamps, sorted ascending
  last:    number
  min:     number
  max:     number
}

function buildSeriesFromMatrix(matrix: Matrix[]): SeriesInfo[] {
  return matrix.map((m) => {
    const samples = m.values
      .map(([t, vstr]) => ({ t: Math.round(t * 1000), v: parseFloat(vstr) }))
      .filter((p) => Number.isFinite(p.v))
      .sort((a, b) => a.t - b.t)
    return statsFor(m.metric, samples)
  })
}

function buildSeriesFromVector(vec: Vector[]): SeriesInfo[] {
  return vec.map((v) => {
    const num = parseFloat(v.value[1])
    const samples = Number.isFinite(num)
      ? [{ t: Math.round(v.value[0] * 1000), v: num }]
      : []
    return statsFor(v.metric, samples)
  })
}

function statsFor(metric: Record<string, string>, samples: { t: number; v: number }[]): SeriesInfo {
  // Plain reduce loop — avoids `Math.min(...arr)` blowing the call stack
  // when a query returns thousands of samples per series.
  let min = Number.POSITIVE_INFINITY
  let max = Number.NEGATIVE_INFINITY
  for (const p of samples) {
    if (p.v < min) min = p.v
    if (p.v > max) max = p.v
  }
  const last = samples.length ? samples[samples.length - 1].v : NaN
  return {
    label:  seriesLabel(metric),
    metric,
    samples,
    last,
    min: samples.length ? min : NaN,
    max: samples.length ? max : NaN,
  }
}

function seriesLabel(labels: Record<string, string>): string {
  const name = labels.__name__ || ''
  const rest = Object.entries(labels)
    .filter(([k]) => k !== '__name__')
    .map(([k, v]) => `${k}="${v}"`)
    .join(', ')
  return rest ? `${name}{${rest}}` : (name || '{}')
}

// ─── Recharts helpers ────────────────────────────────────────────────────────

interface ChartShape {
  rows:   Array<Record<string, number>>
  series: { key: string; label: string }[]
}

function reshapeForGraph(seriesList: SeriesInfo[]): ChartShape {
  const series = seriesList.map((s, i) => ({ key: `s${i}`, label: s.label }))
  const buckets = new Map<number, Record<string, number>>()
  seriesList.forEach((s, i) => {
    const k = `s${i}`
    for (const { t, v } of s.samples) {
      const row = buckets.get(t) ?? { time: t }
      row[k] = Number.isFinite(v) ? v : 0
      buckets.set(t, row)
    }
  })
  const rows = Array.from(buckets.values()).sort((a, b) => a.time - b.time)
  return { rows, series }
}

const PALETTE = [
  '#5b8def', '#ff7a45', '#52c41a', '#faad14', '#722ed1',
  '#13c2c2', '#eb2f96', '#a0d911', '#fa541c', '#2f54eb',
]
function colorFor(name: string, idx: number): string {
  if (idx < PALETTE.length) return PALETTE[idx]
  let h = 0
  for (let i = 0; i < name.length; i++) h = (h * 31 + name.charCodeAt(i)) | 0
  return `hsl(${Math.abs(h) % 360}, 60%, 60%)`
}

// ─── Table row shape (typed properly so antd v6 strict columns are happy) ─────

interface TableRow {
  key:       number
  __name__:  string
  __value__: number
  __min__:   number
  __max__:   number
  __n__:     number
  // Per-label columns are keyed by the label name (instance, job, …).  Their
  // values come from `s.metric` and are always strings.
  [label: string]: string | number
}

/**
 * PromExplore — interactive PromQL workbench for a single Prometheus data
 * source.  Mirrors Prometheus's native /graph page:
 *
 *   Table  — one row per series with its latest value (always usable, the
 *            only sensible view for constant series like an SSL-expiry
 *            timestamp that doesn't move between scrapes).
 *   Graph  — Recharts LineChart with explicit y-padding so a flat-line
 *            series doesn't collapse onto an invisible 0-height stroke.
 *
 * "刷新" controls the auto-poll cadence (react-query refetchInterval).  "0"
 * = manual only.  Switching Table ↔ Graph also triggers a fresh fetch
 * because `view` is part of the queryKey, so the user explicitly sees a
 * round-trip when they click Graph (matches Prometheus's own behaviour).
 *
 * AlertMesh deliberately does NOT let the user create alert rules from
 * here — alerts continue to come from upstream Prometheus / Alertmanager
 * via the trusted-webhook integration; this page is for ops triage and
 * AI-driven root-cause exploration only.
 */
export default function PromExplore() {
  const { id = '' } = useParams<{ id: string }>()
  const [searchParams, setSearchParams] = useSearchParams()
  const navigate = useNavigate()

  const [query, setQuery]         = useState(searchParams.get('q') ?? '')
  const [rangeIdx, setRangeIdx]   = useState(3) // 1h default
  const [refreshSec, setRefreshSec] = useState(0)
  const [submitted, setSubmitted] = useState(searchParams.get('q') ?? '')
  const [view, setView]           = useState<'table' | 'graph'>('table')

  // Pull the data source row for the breadcrumb / sanity-check the kind.
  const { data: list } = useQuery({
    queryKey: ['data-sources'],
    queryFn:  () => getDataSources(),
    staleTime: 30_000,
  })
  const ds: DataSource | undefined = list?.find((r) => r.id === id)

  // Keep the URL in sync so a query is shareable (Cmd-L → paste).
  useEffect(() => {
    if (submitted) setSearchParams({ q: submitted }, { replace: true })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [submitted])

  const range = RANGES[rangeIdx]
  const step  = useMemo(() => autoStep(range.sec), [range.sec])

  // pinnedNow is bumped on every (re)fetch so the time window slides
  // forward to the latest data without the user having to resubmit, but
  // stays frozen between explicit refreshes so the X-axis doesn't drift
  // mid-render.  The setInterval below ticks at the same cadence as
  // refetchInterval so the X-axis end always equals the request's `end`.
  const [pinnedNow, setPinnedNow] = useState(() => Math.floor(Date.now() / 1000))
  useEffect(() => {
    if (refreshSec <= 0 || !submitted) return
    const t = setInterval(() => {
      setPinnedNow(Math.floor(Date.now() / 1000))
    }, refreshSec * 1000)
    return () => clearInterval(t)
  }, [refreshSec, submitted])

  const startSec = pinnedNow - range.sec
  const endSec   = pinnedNow

  // queryKey includes `view` so toggling Table ↔ Graph fires a fresh fetch
  // (the user wanted "click Graph → call the graph API → render").
  // refetchInterval drives the auto-poll loop; we disable background
  // polling so a hidden tab doesn't burn CPU + Prom request budget.
  const { data, isFetching, error, refetch } = useQuery({
    queryKey: ['prom-query', id, submitted, view, startSec, endSec, step],
    enabled:  !!id && !!submitted,
    queryFn:  () => promQueryRange(id, submitted, startSec, endSec, step),
    refetchInterval: refreshSec > 0 ? refreshSec * 1000 : false,
    refetchIntervalInBackground: false,
  })

  const seriesList: SeriesInfo[] = useMemo(() => {
    if (!data?.data) return []
    if (data.data.resultType === 'matrix') return buildSeriesFromMatrix(data.data.result as Matrix[])
    if (data.data.resultType === 'vector') return buildSeriesFromVector(data.data.result as Vector[])
    return []
  }, [data])

  const chart = useMemo(() => reshapeForGraph(seriesList), [seriesList])

  // Compute the y-axis domain ourselves so a constant series (every sample
  // identical, e.g. an SSL-expiry timestamp) still draws a visible line
  // instead of collapsing onto the axis edge.
  const yDomain = useMemo<[number | 'auto', number | 'auto']>(() => {
    let min = Number.POSITIVE_INFINITY
    let max = Number.NEGATIVE_INFINITY
    let count = 0
    for (const s of seriesList) {
      for (const p of s.samples) {
        if (p.v < min) min = p.v
        if (p.v > max) max = p.v
        count++
      }
    }
    if (count === 0) return ['auto', 'auto']
    if (min === max) {
      const pad = Math.abs(min) > 0 ? Math.abs(min) * 0.05 : 1
      return [min - pad, max + pad]
    }
    const pad = (max - min) * 0.05
    return [min - pad, max + pad]
  }, [seriesList])

  const onSubmit = () => {
    const q = query.trim()
    if (!q) return
    setSubmitted(q)
    setPinnedNow(Math.floor(Date.now() / 1000))
  }

  // Build dynamic Table columns: one column per label key actually present
  // in the result, plus a final Value column.  Matches Prometheus's
  // /graph Table tab, lets the user spot "which instance / job is this?"
  // without parsing a long `metric{a="x", …}` string.
  const tableCols: ColumnsType<TableRow> = useMemo(() => {
    const labelKeys = new Set<string>()
    for (const s of seriesList) {
      for (const k of Object.keys(s.metric)) {
        if (k !== '__name__') labelKeys.add(k)
      }
    }
    const cols: ColumnsType<TableRow> = [{
      title:     'Metric',
      dataIndex: '__name__',
      key:       '__name__',
      width:     220,
      ellipsis:  true,
      render:    (v: string) => <Text code style={{ fontSize: 12 }}>{v || '-'}</Text>,
    }]
    for (const k of Array.from(labelKeys).sort()) {
      cols.push({
        title:     k,
        dataIndex: k,
        key:       k,
        ellipsis:  true,
        render:    (v: string) =>
          v ? <Text style={{ fontSize: 12 }}>{v}</Text>
            : <Text type="secondary">-</Text>,
      })
    }
    cols.push({
      title:     'Value',
      dataIndex: '__value__',
      key:       '__value__',
      width:     200,
      align:     'right',
      render:    (v: number, row: TableRow) => (
        <Tooltip title={`min ${formatNumber(row.__min__)} · max ${formatNumber(row.__max__)} · n=${row.__n__}`}>
          <Text strong style={{ fontFamily: 'ui-monospace, monospace' }}>{formatNumber(v)}</Text>
        </Tooltip>
      ),
    })
    return cols
  }, [seriesList])

  const tableRows: TableRow[] = useMemo(
    () => seriesList.map((s, i): TableRow => ({
      key:       i,
      __name__:  s.metric.__name__ || '',
      __value__: s.last,
      __min__:   s.min,
      __max__:   s.max,
      __n__:     s.samples.length,
      ...s.metric,
    })),
    [seriesList],
  )

  const totalPoints = seriesList.reduce((n, s) => n + s.samples.length, 0)

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 16 }}>
        <Button type="text" icon={<ArrowLeftOutlined />} onClick={() => navigate('/datasources')} />
        <LineChartOutlined style={{ fontSize: 18, color: '#ffffff' }} />
        <Title level={5} style={{ margin: 0, color: '#ffffff' }}>Explore</Title>
        {ds && (
          <Space>
            <Tag color="blue">{ds.name}</Tag>
            <Text type="secondary" style={{ fontSize: 12 }}>{ds.endpoint}</Text>
          </Space>
        )}
      </div>

      {ds && ds.kind !== 'prometheus' && (
        <Alert
          type="error" showIcon style={{ marginBottom: 12 }}
          message="该数据源不是 Prometheus，无法在此页查询"
        />
      )}

      {/* ── Query bar ─────────────────────────────────────────────── */}
      <Card
        style={{ background: '#111111', border: '1px solid #1e1e1e', borderRadius: 8, marginBottom: 16 }}
        styles={{ body: { padding: 12 } }}
      >
        <Space.Compact style={{ width: '100%' }}>
          <Input
            placeholder="请选择或输入查询语句（PromQL，例如 sum(rate(http_requests_total[5m])) by (status)）"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onPressEnter={onSubmit}
            style={{ flex: 1 }}
          />
          <Tooltip title="自动刷新间隔（每隔该时间向 Prometheus 重新拉取一次数据）">
            <Select
              value={refreshSec}
              style={{ width: 130 }}
              onChange={(v) => setRefreshSec(Number(v))}
              options={REFRESH_OPTIONS.map((r) => ({ value: r.sec, label: `刷新：${r.label}` }))}
            />
          </Tooltip>
          <Select
            value={rangeIdx}
            style={{ width: 150 }}
            onChange={(v) => setRangeIdx(Number(v))}
            options={RANGES.map((r, i) => ({ value: i, label: r.label }))}
          />
          <Tooltip title="重新拉取（重置时间窗末端为现在）">
            <Button
              icon={<ReloadOutlined />}
              onClick={() => { setPinnedNow(Math.floor(Date.now() / 1000)); refetch() }}
            />
          </Tooltip>
          <Button type="primary" icon={<PlayCircleOutlined />} onClick={onSubmit}>查询</Button>
        </Space.Compact>
      </Card>

      {/* ── Result panel ──────────────────────────────────────────── */}
      <Card
        style={{ background: '#111111', border: '1px solid #1e1e1e', borderRadius: 8 }}
        styles={{ body: { padding: 16 } }}
      >
        {/* Tabs + summary, always present once the user has issued a query. */}
        {submitted && (
          <div style={{
            display: 'flex', alignItems: 'center', justifyContent: 'space-between',
            marginBottom: 12, flexWrap: 'wrap', gap: 8,
          }}>
            <Segmented
              value={view}
              onChange={(v) => setView(v as 'table' | 'graph')}
              options={[
                { label: <span><TableOutlined /> Table</span>,    value: 'table' },
                { label: <span><AreaChartOutlined /> Graph</span>, value: 'graph' },
              ]}
            />
            <Space size={12} wrap>
              {data?.data?.resultType && (
                <Text type="secondary" style={{ fontSize: 12 }}>
                  resultType=<Text code style={{ fontSize: 12 }}>{data.data.resultType}</Text>
                </Text>
              )}
              <Text type="secondary" style={{ fontSize: 12 }}>
                step=<Text code style={{ fontSize: 12 }}>{step}</Text>
              </Text>
              {refreshSec > 0 && (
                <Text type="secondary" style={{ fontSize: 12 }}>
                  自动刷新 <Text code style={{ fontSize: 12 }}>{refreshSec}s</Text>
                  {isFetching && <Spin size="small" style={{ marginLeft: 6 }} />}
                </Text>
              )}
              <Text type="secondary" style={{ fontSize: 12 }}>
                {seriesList.length} series · {totalPoints} points
              </Text>
            </Space>
          </div>
        )}

        {/* Body — exclusive branches so we never render two states at once. */}
        {!submitted && !error && (
          <Empty description="输入 PromQL 后点击「查询」" image={Empty.PRESENTED_IMAGE_SIMPLE} />
        )}

        {error && (
          <Alert type="error" showIcon message="查询失败" description={(error as Error).message} />
        )}

        {submitted && !error && isFetching && seriesList.length === 0 && (
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 48 }}>
            <Space>
              <Spin />
              <Text type="secondary">加载中…</Text>
            </Space>
          </div>
        )}

        {submitted && !error && !isFetching && seriesList.length === 0 && (
          <Empty description={
            <span>
              该时间范围内没有数据
              {data?.data?.resultType && <Text type="secondary"> · resultType={data.data.resultType}</Text>}
            </span>
          } />
        )}

        {/* TABLE VIEW */}
        {seriesList.length > 0 && view === 'table' && (
          <Table<TableRow>
            size="small"
            rowKey="key"
            columns={tableCols}
            dataSource={tableRows}
            pagination={tableRows.length > 50 ? { pageSize: 50, showSizeChanger: false } : false}
            scroll={{ x: 'max-content' }}
          />
        )}

        {/* GRAPH VIEW (≥ 2 distinct timestamps) */}
        {seriesList.length > 0 && view === 'graph' && chart.rows.length > 1 && (
          <div style={{ width: '100%', height: 420, minWidth: 0 }}>
            <ResponsiveContainer
              width="100%"
              height="100%"
              minWidth={0}
              debounce={50}
              initialDimension={{ width: 600, height: 420 }}
            >
              <LineChart data={chart.rows} margin={{ top: 12, right: 24, bottom: 8, left: 8 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="#1f1f1f" />
                <XAxis
                  dataKey="time"
                  type="number"
                  scale="time"
                  domain={['dataMin', 'dataMax']}
                  tickFormatter={(v: number) => formatAxisTime(v, range.sec)}
                  stroke="#666"
                  tick={{ fontSize: 11 }}
                />
                <YAxis
                  stroke="#666"
                  tick={{ fontSize: 11 }}
                  domain={yDomain}
                  tickFormatter={(v: number) => formatNumber(v)}
                  width={72}
                />
                <RTip
                  contentStyle={{ background: '#1a1a1a', border: '1px solid #333', fontSize: 12 }}
                  labelFormatter={(v) => new Date(Number(v)).toLocaleString()}
                  formatter={(v, name) => {
                    const label = chart.series.find((s) => s.key === name)?.label ?? String(name)
                    return [formatNumber(Number(v)), label]
                  }}
                />
                <Legend
                  wrapperStyle={{ fontSize: 11, paddingTop: 8 }}
                  formatter={(value: string) =>
                    chart.series.find((s) => s.key === value)?.label ?? String(value)
                  }
                />
                {chart.series.map((s, i) => (
                  <Line
                    key={s.key}
                    type="monotone"
                    dataKey={s.key}
                    name={s.key}
                    stroke={colorFor(s.label, i)}
                    dot={false}
                    strokeWidth={1.6}
                    isAnimationActive={false}
                    connectNulls
                  />
                ))}
              </LineChart>
            </ResponsiveContainer>
          </div>
        )}

        {/* Graph view but only one timestamp → can't draw a line, point user
            back to Table.  Common with vector / constant queries. */}
        {seriesList.length > 0 && view === 'graph' && chart.rows.length <= 1 && (
          <Alert
            type="info" showIcon
            message="该结果只有一个采样点，无法绘制折线（vector 或常量序列），请切换到 Table 视图查看。"
          />
        )}
      </Card>
    </div>
  )
}

function formatAxisTime(ms: number, rangeSec: number): string {
  const d = new Date(ms)
  if (rangeSec >= 86400) return `${d.getMonth() + 1}/${d.getDate()} ${pad(d.getHours())}:${pad(d.getMinutes())}`
  return `${pad(d.getHours())}:${pad(d.getMinutes())}`
}
function pad(n: number) { return n < 10 ? '0' + n : '' + n }

// Compact number formatter so axes stay readable for both QPS (12.3) and
// byte-counter (3.4M) style metrics without per-metric configuration.
// Large round integers (Prometheus often exposes unix-seconds in metrics
// like `probe_ssl_earliest_cert_expiry`) keep their full precision so the
// table value still matches what `date -d @<value>` would print.
function formatNumber(v: number): string {
  if (!Number.isFinite(v)) return String(v)
  const abs = Math.abs(v)
  if (abs >= 1e12) return v.toExponential(3)
  if (Number.isInteger(v) && abs >= 1e6) return v.toString()
  if (abs >= 1e9) return (v / 1e9).toFixed(2) + 'G'
  if (abs >= 1e6) return (v / 1e6).toFixed(2) + 'M'
  if (abs >= 1e3) return (v / 1e3).toFixed(2) + 'k'
  if (abs < 1 && abs > 0) return v.toPrecision(3)
  return v.toFixed(2)
}
