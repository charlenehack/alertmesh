/**
 * ConfigMapEditor – ConfigMap 专用编辑器
 *
 * 两个 Tab:
 *  - 数据视图: 每个 key 单独一个 textarea，\n 还原为真实换行，可读性好
 *  - 原始 JSON: 完整 JSON 编辑，与 YamlEditor 保持一致
 */
import { useState, useEffect } from 'react'
import { Drawer, Button, Space, Alert, Tabs, Typography, Empty } from 'antd'
import { useTheme } from '../../hooks/useTheme'

const { Text } = Typography

interface ConfigMapEditorProps {
  title: string
  /** K8s ConfigMap 对象（raw） */
  value: unknown
  open: boolean
  onClose: () => void
  /** undefined = 只读 */
  onSave?: (json: string) => void
  loading?: boolean
}

/** 从 ConfigMap 对象中提取 data 字段 */
function getDataEntries(value: unknown): [string, string][] {
  if (!value || typeof value !== 'object') return []
  const cm = value as { data?: Record<string, string> }
  if (!cm.data || typeof cm.data !== 'object') return []
  return Object.entries(cm.data)
}

export function ConfigMapEditor({
  title, value, open, onClose, onSave, loading,
}: ConfigMapEditorProps) {
  const { c, isDark } = useTheme()
  const readOnly = !onSave

  // 数据视图: key → value (真实换行)
  const [dataMap, setDataMap] = useState<Record<string, string>>({})
  // 原始 JSON tab
  const [jsonText, setJsonText] = useState('')
  const [parseError, setParseError] = useState('')
  const [activeTab, setActiveTab] = useState('data')

  useEffect(() => {
    if (!open || value === undefined) return
    // 初始化原始 JSON
    setJsonText(JSON.stringify(value, null, 2))
    setParseError('')
    setActiveTab('data')
    // 初始化数据视图（\n → 真实换行）
    const entries = getDataEntries(value)
    const m: Record<string, string> = {}
    entries.forEach(([k, v]) => { m[k] = v }) // v 在 JSON.parse 后已是真实 \n
    setDataMap(m)
  }, [open, value])

  const monoStyle: React.CSSProperties = {
    fontFamily: '"JetBrains Mono", "Fira Code", Consolas, monospace',
    fontSize: 12,
    lineHeight: 1.6,
    padding: '8px 10px',
    background: isDark ? '#0d0d0d' : '#fafafa',
    color: isDark ? '#d4d4d4' : '#1a1a1a',
    border: `1px solid ${c.border}`,
    borderRadius: 6,
    outline: 'none',
    width: '100%',
    resize: 'vertical' as const,
    boxSizing: 'border-box' as const,
  }

  const handleSaveFromData = () => {
    try {
      const cm = JSON.parse(jsonText) as { data?: Record<string, string> }
      cm.data = dataMap
      const out = JSON.stringify(cm, null, 2)
      setParseError('')
      onSave?.(out)
    } catch (e) {
      setParseError(`构建 JSON 失败: ${(e as Error).message}`)
    }
  }

  const handleSaveFromJson = () => {
    try {
      JSON.parse(jsonText)
      setParseError('')
      onSave?.(jsonText)
    } catch (e) {
      setParseError(`JSON 格式错误: ${(e as Error).message}`)
    }
  }

  const handleSave = () => {
    if (activeTab === 'data') handleSaveFromData()
    else handleSaveFromJson()
  }

  const entries = Object.entries(dataMap)

  return (
    <Drawer
      title={title}
      placement="right"
      width={720}
      open={open}
      onClose={onClose}
      styles={{ body: { padding: '12px 16px', background: c.bgPage, display: 'flex', flexDirection: 'column', overflow: 'hidden' } }}
      extra={
        !readOnly && (
          <Space>
            <Button onClick={onClose}>取消</Button>
            <Button type="primary" loading={loading} onClick={handleSave}>应用</Button>
          </Space>
        )
      }
    >
      {parseError && <Alert type="error" message={parseError} style={{ marginBottom: 8 }} showIcon />}

      <Tabs
        activeKey={activeTab}
        onChange={setActiveTab}
        size="small"
        style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}
        items={[
          {
            key: 'data',
            label: '数据视图',
            children: (
              <div style={{ overflowY: 'auto', maxHeight: 'calc(100vh - 220px)', paddingRight: 4 }}>
                {entries.length === 0 ? (
                  <Empty description="无 data 字段" />
                ) : (
                  entries.map(([k, v]) => (
                    <div key={k} style={{ marginBottom: 16 }}>
                      <div style={{ marginBottom: 4, display: 'flex', alignItems: 'center', gap: 6 }}>
                        <Text code style={{ fontSize: 12 }}>{k}</Text>
                        <Text style={{ fontSize: 11, color: c.textSecondary }}>
                          {v.split('\n').length} 行
                        </Text>
                      </div>
                      <textarea
                        value={v}
                        readOnly={readOnly}
                        spellCheck={false}
                        rows={Math.min(Math.max(v.split('\n').length + 1, 4), 30)}
                        onChange={e => setDataMap(prev => ({ ...prev, [k]: e.target.value }))}
                        style={{ ...monoStyle, minHeight: 80 }}
                      />
                    </div>
                  ))
                )}
              </div>
            ),
          },
          {
            key: 'json',
            label: '原始 JSON',
            children: (
              <textarea
                value={jsonText}
                onChange={e => { setJsonText(e.target.value); setParseError('') }}
                readOnly={readOnly}
                spellCheck={false}
                style={{ ...monoStyle, minHeight: 500, height: 'calc(100vh - 220px)' }}
              />
            ),
          },
        ]}
      />
    </Drawer>
  )
}
