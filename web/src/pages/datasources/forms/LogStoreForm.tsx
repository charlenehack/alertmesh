// Shared connection + selector fields for the two log-store kinds
// (OpenSearch and Elasticsearch).  They share the HTTP query DSL +
// Basic-Auth credential shape, so the only thing that changes between
// the two is the labels / placeholders the operator sees.
//
// `OpenSearchForm` and `ElasticForm` are thin wrappers that mount this
// component with the appropriate `kind`.  Both are rendered via the
// orchestrator's `kind` switch.

import { Form, Input, InputNumber, Switch, Typography } from 'antd'

import { SECRET_MASK } from '../shared/constants'

const { Text } = Typography

export interface LogStoreFormProps {
  kind: 'opensearch' | 'elastic'
  editing: boolean
}

const COPY = {
  opensearch: {
    title:           'OpenSearch',
    endpointHint:    'https://opensearch.internal:9200',
    endpointMessage: 'OpenSearch URL 必填',
  },
  elastic: {
    title:           'Elasticsearch',
    endpointHint:    'https://elastic.example.com:9200',
    endpointMessage: 'Elasticsearch URL 必填',
  },
} as const

export function LogStoreForm({ kind, editing }: LogStoreFormProps) {
  const c = COPY[kind]
  return (
    <>
      <Form.Item name="endpoint" label="Endpoint" rules={[{ required: true, message: c.endpointMessage }]}>
        <Input placeholder={c.endpointHint} />
      </Form.Item>

      <Form.Item name="os_username" label="用户名">
        <Input autoComplete="off" placeholder="只读账号" />
      </Form.Item>

      <Form.Item name="os_password" label="密码 (敏感)">
        <Input.Password placeholder={editing ? SECRET_MASK : '在浏览器侧 RSA 加密后传输'} autoComplete="new-password" />
      </Form.Item>

      <Form.Item name="os_index" label="索引 / 索引模式" rules={[{ required: true, message: '索引必填' }]} extra={
        <span style={{ color: '#666', fontSize: 11 }}>支持通配符，例如 <Text code style={{ fontSize: 11 }}>prod-app-logs-*</Text></span>
      }>
        <Input placeholder="prod-app-logs-*" />
      </Form.Item>

      <Form.Item name="os_query" label={`过滤 DSL（${c.title} query 段）`} extra={
        <span style={{ color: '#666', fontSize: 11 }}>
          原生 query 段；将作为 selector 传给连接器。例：<Text code style={{ fontSize: 11 }}>{'{"match": {"level": "ERROR"}}'}</Text>
        </span>
      }>
        <Input.TextArea rows={4} placeholder='{"bool":{"must":[{"match":{"level":"ERROR"}},{"match":{"app":"order"}}]}}' />
      </Form.Item>

      <Form.Item name="os_watermark_field" label="水位字段（防漏跑）" initialValue="@timestamp">
        <Input placeholder="@timestamp" />
      </Form.Item>

      <Form.Item name="os_poll_interval_seconds" label="轮询间隔（秒）" initialValue={30}>
        <InputNumber style={{ width: '100%' }} min={5} max={3600} />
      </Form.Item>

      <Form.Item name="os_lookback_seconds" label="启动回溯（秒）" initialValue={300}>
        <InputNumber style={{ width: '100%' }} min={0} max={86400} />
      </Form.Item>

      <Form.Item name="os_tls_skip" label="跳过 TLS 证书校验" valuePropName="checked">
        <Switch />
      </Form.Item>

      <Form.Item
        name="os_consumer_concurrency"
        label="消费并发度（goroutine 数）"
        initialValue={1}
        // Same rationale as KafkaForm.tsx: InputNumber.max alone won't
        // stop arbitrary keystroke input, so values >32 would silently
        // get clamped to 32 by clampConcurrency() in serialize.ts and
        // the operator wouldn't notice.  A hard rule blocks save first.
        rules={[{ type: 'integer', min: 1, max: 32, message: '消费并发度必须在 1 - 32 之间' }]}
        extra={
          <span style={{ color: '#666', fontSize: 11 }}>
            默认 1，上限 32。{c.title} 消费器实现于 Phase 4（README §4.1.4 路线图），
            <b>当前保存即生效，等待消费器上线后自动按 N 并行轮询</b>。
            建议先填 1，待 Phase 4 后根据日志摄入速率调整。
          </span>
        }
      >
        <InputNumber style={{ width: '100%' }} min={1} max={32} />
      </Form.Item>
    </>
  )
}
