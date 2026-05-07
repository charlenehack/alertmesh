// KafkaTestMessageModal calls the dry-run endpoint with the in-flight
// (unsaved) filter / mapping so the operator can iterate without
// round-tripping the whole row through save → kcat → log-tail.  Pulls
// the current values out of the parent form via Form.useFormInstance.

import { useState } from 'react'
import { Alert, App, Button, Card, Form, Input, Modal, Typography } from 'antd'
import { PlayCircleOutlined } from '@ant-design/icons'

import { testKafkaMessage, type KafkaTestMessageResult } from '../../../api/datasources'
import type { DataSourceFormShape } from './types'
import { attachKafkaError, kafkaMappingFromForm, NGINX_GATEWAY_SAMPLE } from './kafkaMapping'
import { MappingHitsTable } from './MappingHitsTable'

const { Text } = Typography

export interface KafkaTestMessageModalProps {
  open: boolean
  onClose: () => void
  editingId: string
}

export function KafkaTestMessageModal({ open, onClose, editingId }: KafkaTestMessageModalProps) {
  const form = Form.useFormInstance<DataSourceFormShape>()
  const { message } = App.useApp()
  const [sample, setSample] = useState<string>(NGINX_GATEWAY_SAMPLE)
  const [result, setResult] = useState<KafkaTestMessageResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  const run = async () => {
    setLoading(true)
    setError(null)
    setResult(null)
    try {
      const v = form.getFieldsValue()
      const mapping = kafkaMappingFromForm(v)
      const res = await testKafkaMessage(editingId || 'new', {
        sample,
        config: { filter: v.kafka_filter || '', mapping },
      })
      setResult(res)
    } catch (e: unknown) {
      const msg = (e as { message?: string })?.message || String(e)
      setError(msg)
      // Try to surface mapping/filter compile errors directly on the
      // offending Form.Item too (the modal shares the parent form).
      if (!attachKafkaError(form, msg)) message.error('测试失败：' + msg)
    } finally {
      setLoading(false)
    }
  }

  return (
    <Modal
      title="Kafka 消息测试（不会写入数据库）"
      open={open}
      onCancel={onClose}
      width={760}
      footer={[
        <Button key="close" onClick={onClose}>关闭</Button>,
        <Button key="run" type="primary" loading={loading} onClick={run} icon={<PlayCircleOutlined />}>
          运行测试
        </Button>,
      ]}
    >
      <Text type="secondary">粘贴一段 JSON 样例消息（与 Kafka 实际推送的格式一致）：</Text>
      <Input.TextArea
        rows={8}
        value={sample}
        onChange={(e) => setSample(e.target.value)}
        style={{ fontFamily: 'monospace', fontSize: 12, marginTop: 8 }}
      />

      {error && <Alert type="error" showIcon style={{ marginTop: 16 }} message={error} />}

      {result && (
        <div style={{ marginTop: 16 }}>
          <Alert
            type={result.kept ? 'success' : 'warning'}
            showIcon
            message={result.kept ? '保留 — 该消息会进入告警引擎' : `丢弃 — 原因：${result.drop_reason || '未知'}`}
            description={
              <>
                <div>filter_eval: <Text code>{String(result.debug.filter_eval)}</Text></div>
                <div>resolved: <Text code>{String(result.debug.resolved)}</Text></div>
              </>
            }
          />
          {result.kept && result.raw_alert && (
            <Card size="small" title="生成的 RawAlert" style={{ marginTop: 12 }}>
              <pre style={{ margin: 0, maxHeight: 280, overflow: 'auto', fontSize: 12 }}>
                {JSON.stringify(result.raw_alert, null, 2)}
              </pre>
            </Card>
          )}
          <Card size="small" title="字段抽取明细（gjson 路径 / expr 表达式 → 解析值）" style={{ marginTop: 12 }}>
            <MappingHitsTable hits={result.debug.mapping_resolved as Record<string, string> | null | undefined} />
          </Card>
        </div>
      )}
    </Modal>
  )
}
