/**
 * YamlEditor – JSON/YAML 查看 & 编辑 Drawer
 *
 * 由于项目未引入 js-yaml / monaco，采用 antd TextArea 作为编辑器，
 * 以 JSON 格式展示 K8s 对象（K8s API 原生支持 JSON）。
 * readOnly=true 时只展示，不提供保存按钮。
 */
import { useState, useEffect } from 'react'
import { Drawer, Button, Space, Alert, Typography } from 'antd'
import { useTheme } from '../../hooks/useTheme'

const { Text } = Typography

interface YamlEditorProps {
  title: string
  /** K8s 资源对象（会被 JSON.stringify 格式化展示） */
  value: unknown
  open: boolean
  onClose: () => void
  /** 提交时传入编辑后的 JSON 字符串；undefined 则只读 */
  onSave?: (json: string) => void
  loading?: boolean
}

export function YamlEditor({ title, value, open, onClose, onSave, loading }: YamlEditorProps) {
  const { c, isDark } = useTheme()
  const [text, setText] = useState('')
  const [parseError, setParseError] = useState('')

  useEffect(() => {
    if (open && value !== undefined) {
      setText(JSON.stringify(value, null, 2))
      setParseError('')
    }
  }, [open, value])

  const handleSave = () => {
    try {
      JSON.parse(text) // validate
      setParseError('')
      onSave?.(text)
    } catch (e) {
      setParseError(`JSON 格式错误: ${(e as Error).message}`)
    }
  }

  const readOnly = !onSave

  return (
    <Drawer
      title={title}
      placement="right"
      width={680}
      open={open}
      onClose={onClose}
      styles={{ body: { padding: '12px 16px', background: c.bgPage, display: 'flex', flexDirection: 'column' } }}
      extra={
        !readOnly && (
          <Space>
            <Button onClick={onClose}>取消</Button>
            <Button type="primary" loading={loading} onClick={handleSave}>
              应用
            </Button>
          </Space>
        )
      }
    >
      {parseError && (
        <Alert type="error" message={parseError} style={{ marginBottom: 8 }} showIcon />
      )}
      <div style={{ fontSize: 11, color: c.textSecondary, marginBottom: 6 }}>
        {readOnly ? '只读模式' : '直接编辑 JSON，点击「应用」提交到 K8s API'}
      </div>
      <textarea
        value={text}
        onChange={e => { setText(e.target.value); setParseError('') }}
        readOnly={readOnly}
        spellCheck={false}
        style={{
          flex: 1,
          width: '100%',
          minHeight: 500,
          resize: 'vertical',
          fontFamily: '"JetBrains Mono", "Fira Code", "Cascadia Code", Consolas, monospace',
          fontSize: 12,
          lineHeight: 1.6,
          padding: '10px 12px',
          background: isDark ? '#0d0d0d' : '#fafafa',
          color: isDark ? '#d4d4d4' : '#1a1a1a',
          border: `1px solid ${c.border}`,
          borderRadius: 6,
          outline: 'none',
          boxSizing: 'border-box',
        }}
      />
    </Drawer>
  )
}
