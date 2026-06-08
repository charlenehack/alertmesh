/**
 * K8sAIDrawer – 流式 AI 分析侧边栏
 * 接收 Pod 日志 / K8s 事件文本，POST /k8s/ai/analyze SSE 流式返回 Markdown 报告。
 */
import { useEffect, useRef, useState } from 'react'
import { Button, Drawer, Space, Spin, Alert } from 'antd'
import { RobotOutlined, ReloadOutlined } from '@ant-design/icons'
import MarkdownView from '../../components/MarkdownView'
import { useAuthStore } from '../../store/auth'

export interface K8sAIDrawerProps {
  open: boolean
  onClose: () => void
  resourceKind: string   // Pod / Deployment / DaemonSet
  namespace: string
  name: string
  analysisKind: 'logs' | 'events' | 'describe'
  content: string        // raw text to analyse
}

export function K8sAIDrawer({
  open, onClose, resourceKind, namespace, name, analysisKind, content,
}: K8sAIDrawerProps) {
  const token = useAuthStore(s => s.token)
  const [report, setReport] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const abortRef = useRef<AbortController | null>(null)

  // 每次打开或内容变化时自动触发分析
  useEffect(() => {
    if (!open || !content) return
    startAnalysis()
    return () => abortRef.current?.abort()
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, content])

  function startAnalysis() {
    abortRef.current?.abort()
    const ctrl = new AbortController()
    abortRef.current = ctrl

    setReport('')
    setError(null)
    setLoading(true)

    const BASE = import.meta.env.DEV ? 'http://10.11.12.146:8081' : ''

    // 截断超长内容，避免超出 LLM token 限制
    const MAX_CONTENT = 12000
    const truncatedContent = content.length > MAX_CONTENT
      ? content.slice(0, MAX_CONTENT) + '\n...(内容过长，已截断)'
      : content

    fetch(`${BASE}/api/v1/k8s/ai/analyze`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      body: JSON.stringify({
        resource_kind: resourceKind,
        namespace,
        name,
        analysis_kind: analysisKind,
        content: truncatedContent,
      }),
      signal: ctrl.signal,
    })
      .then(async res => {
        if (!res.ok) {
          const txt = await res.text()
          throw new Error(`HTTP ${res.status}: ${txt}`)
        }
        const reader = res.body!.getReader()
        const decoder = new TextDecoder()
        let buf = ''

        const pump = async (): Promise<void> => {
          const { done, value } = await reader.read()
          if (done) return
          buf += decoder.decode(value, { stream: true })
          const lines = buf.split('\n')
          buf = lines.pop() ?? ''
          for (const line of lines) {
            if (!line.startsWith('data:')) continue
            // slice(6) skips 'data: ' (6 chars) to preserve leading spaces in AI tokens
            const payload = line.startsWith('data: ') ? line.slice(6) : line.slice(5)
            if (payload === '[DONE]') return
            if (payload.startsWith('[ERROR]')) {
              setError(payload.slice(7).trim())
              return
            }
            // Restore newlines escaped by server
            const text = payload.replace(/\\n/g, '\n')
            setReport(prev => prev + text)
          }
          return pump()
        }
        return pump()
      })
      .catch(err => {
        if (err.name !== 'AbortError') {
          setError(err.message ?? '分析失败')
        }
      })
      .finally(() => setLoading(false))
  }

  const title = (
    <Space size={8}>
      <RobotOutlined style={{ color: '#722ed1' }} />
      <span>AI 分析 — {resourceKind} <span style={{ color: '#722ed1' }}>{name}</span></span>
      <span style={{ fontSize: 12, color: '#8c8c8c' }}>
        ({analysisKind === 'logs' ? '日志' : analysisKind === 'describe' ? '详情' : '事件'})
      </span>
    </Space>
  )

  return (
    <Drawer
      title={title}
      open={open}
      onClose={() => { abortRef.current?.abort(); onClose() }}
      width={720}
      extra={
        <Button
          size="small"
          icon={<ReloadOutlined />}
          loading={loading}
          onClick={startAnalysis}
        >
          重新分析
        </Button>
      }
    >
      {error && (
        <Alert
          type="error"
          message={error}
          style={{ marginBottom: 16 }}
          showIcon
        />
      )}

      {loading && !report && (
        <div style={{ textAlign: 'center', padding: '60px 0' }}>
          <Spin tip="AI 正在分析中，请稍候..." size="large">
            <div style={{ padding: 20 }} />
          </Spin>
        </div>
      )}

      {report && <MarkdownView source={report} />}

      {loading && report && (
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginTop: 12, color: '#8c8c8c' }}>
          <Spin size="small" />
          <span style={{ fontSize: 12 }}>正在生成...</span>
        </div>
      )}

      {!loading && !report && !error && (
        <div style={{ textAlign: 'center', padding: '60px 0', color: '#bbb' }}>
          暂无分析结果
        </div>
      )}
    </Drawer>
  )
}
