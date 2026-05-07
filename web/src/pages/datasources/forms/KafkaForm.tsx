// Kafka data-source form, split into two sections so the orchestrator
// can render them under separate drawer tabs:
//
//   1. KafkaBasicSection — connection (brokers / topic / consumer
//      group / SASL / TLS / rate-limit / consumer concurrency).  This
//      is what an operator who only wants alerts streamed in needs.
//   2. KafkaMappingSection — filter expression, per-field mapping
//      (alertname / severity / fingerprint / starts_at / status /
//      resolved_when), free-form labels & annotations, and the
//      "测试一条样例消息" dry-run button.  This is the advanced tab.
//
// Splitting eliminates the previous 1-tab marathon scroll, and keeps
// each list row's controls (key / value / delete) on a flex row so the
// delete button is always reachable regardless of drawer width.

import { useState } from 'react'
import {
  Alert, Button, Col, Divider, Form, Input, InputNumber, message, Row, Select, Switch,
  Typography,
} from 'antd'
import { MinusCircleOutlined, PlayCircleOutlined, PlusOutlined } from '@ant-design/icons'

import { SECRET_MASK } from '../shared/constants'
import { MappingValueInput } from '../shared/MappingValueInput'
import { KafkaTestMessageModal } from '../shared/KafkaTestMessageModal'

const { Text } = Typography
const { Option } = Select

// tryUnwrapFilterJSON catches the most common operator mistake in the
// filter input box: pasting the entire `{"filter": "<expr>"}` JSON
// envelope (which is what the API stores) instead of just the
// expression body.  expr-lang parses the leading `{` as a map literal
// and bubbles up the unhelpful `expected bool, but got map` error.
//
// On focus loss we peek at the input — if it parses as a JSON object
// with a string `filter` field, we silently swap in `.filter` and
// notify with a non-blocking toast so the operator can confirm the
// auto-correction without the form clearing their work.  Anything that
// doesn't smell like our JSON envelope falls through unchanged so the
// real expr error path remains observable.
function tryUnwrapFilterJSON(input: string): { value: string; unwrapped: boolean } {
  const trimmed = input?.trim?.() ?? ''
  if (!trimmed.startsWith('{')) {
    return { value: input, unwrapped: false }
  }
  try {
    const parsed = JSON.parse(trimmed) as unknown
    if (
      parsed !== null &&
      typeof parsed === 'object' &&
      'filter' in parsed &&
      typeof (parsed as { filter: unknown }).filter === 'string'
    ) {
      return { value: (parsed as { filter: string }).filter, unwrapped: true }
    }
  } catch {
    // Fall through — the user is mid-typing or pasted something else;
    // let the backend report the real syntax error on save.
  }
  return { value: input, unwrapped: false }
}

// ─── Tab 1: connection / SASL / rate-limit ───────────────────────────────────

export interface KafkaBasicSectionProps {
  editing: boolean
}

export function KafkaBasicSection({ editing }: KafkaBasicSectionProps) {
  return (
    <>
      <Form.Item name="endpoint" label="Brokers" rules={[{ required: true, message: 'Brokers 必填' }]} extra={
        <span style={{ color: '#666', fontSize: 11 }}>逗号分隔，例如 <Text code style={{ fontSize: 11 }}>kafka-1:9092,kafka-2:9092</Text></span>
      }>
        <Input placeholder="kafka-1:9092,kafka-2:9092" />
      </Form.Item>

      <Form.Item name="kafka_topic" label="Topic" rules={[{ required: true, message: 'Topic 必填' }]}>
        <Input placeholder="alerts.raw" />
      </Form.Item>

      <Form.Item name="kafka_group_id" label="Consumer Group ID" rules={[{ required: true, message: 'Group ID 必填' }]}>
        <Input placeholder="alertmesh-ingest" />
      </Form.Item>

      <Form.Item name="kafka_sasl_mechanism" label="SASL 机制（可选）" initialValue="">
        <Select>
          <Option value="">无（公开 Kafka 集群）</Option>
          <Option value="PLAIN">PLAIN</Option>
          <Option value="SCRAM-SHA-256">SCRAM-SHA-256</Option>
          <Option value="SCRAM-SHA-512">SCRAM-SHA-512</Option>
        </Select>
      </Form.Item>

      <Form.Item shouldUpdate={(p, c) => p.kafka_sasl_mechanism !== c.kafka_sasl_mechanism} noStyle>
        {({ getFieldValue }) => {
          const m = getFieldValue('kafka_sasl_mechanism')
          if (!m) return null
          return (
            <>
              <Form.Item name="kafka_sasl_user" label="SASL 用户名">
                <Input autoComplete="off" />
              </Form.Item>
              <Form.Item name="kafka_sasl_password" label="SASL 密码 (敏感)">
                <Input.Password placeholder={editing ? SECRET_MASK : '在浏览器侧 RSA 加密后传输'} autoComplete="new-password" />
              </Form.Item>
            </>
          )
        }}
      </Form.Item>

      <Form.Item name="kafka_tls_enabled" label="启用 TLS" valuePropName="checked">
        <Switch />
      </Form.Item>
      <Form.Item name="kafka_tls_skip" label="跳过 TLS 证书校验" valuePropName="checked">
        <Switch />
      </Form.Item>

      <Form.Item name="kafka_max_per_second" label="速率上限（消息/秒，0 = 不限）" initialValue={0}>
        <InputNumber style={{ width: '100%' }} min={0} max={1000000} step={100} />
      </Form.Item>

      <Form.Item
        name="kafka_consumer_concurrency"
        label="消费并发度（goroutine 数 / Reader 数）"
        initialValue={1}
        // Hard rule, not just InputNumber.max (which is only a stepper
        // bound and does NOT prevent arbitrary keystroke input).  Without
        // this rule a value like 99 would pass form validation, then get
        // silently clamped to 32 by clampConcurrency() in serialize.ts —
        // operator sees "save success" but reopens to find 32, with no
        // explanation.  The rule fires before mutationFn so they get an
        // inline red error under the field instead.
        rules={[{ type: 'integer', min: 1, max: 32, message: '消费并发度必须在 1 - 32 之间' }]}
        extra={
          <span style={{ color: '#666', fontSize: 11 }}>
            默认 1，上限 32。N&gt;1 时 alertmesh 会以同一 GroupID 启动 N 个独立 kafka.Reader，
            由 broker 自动分配 partition；<b>建议按 topic partition 数填</b>，
            超出 partition 数的 worker 会 idle 浪费连接。
            速率上限按 worker 独立生效（多 worker 总吞吐 ≈ N × 速率上限）。
          </span>
        }
      >
        <InputNumber style={{ width: '100%' }} min={1} max={32} />
      </Form.Item>
    </>
  )
}

// ─── Tab 2: filter / mapping / labels / annotations / test ───────────────────

export interface KafkaMappingSectionProps {
  editingId: string
}

export function KafkaMappingSection({ editingId }: KafkaMappingSectionProps) {
  const [testOpen, setTestOpen] = useState(false)
  const form = Form.useFormInstance()
  return (
    <>
      <Form.Item
        name="kafka_filter"
        label="过滤表达式"
        extra={
          <span style={{ color: '#666', fontSize: 11 }}>
            空表示全部放行。语法：<Text code style={{ fontSize: 11 }}>expr-lang</Text>，
            <b>强烈建议用安全 helper</b>（缺字段时返回 false，避免误放行整条消息）。示例：
            <br />
            <Text code style={{ fontSize: 11 }}>{`not_empty("response_body") && gte("status_code", 500)`}</Text>
            <br />
            <Text code style={{ fontSize: 11 }}>{`oneof("severity", "P0", "P1") && neq("namespace", "kube-system")`}</Text>
            <br />
            详见下方「安全 filter helper」段，或 <Text code style={{ fontSize: 11 }}>docs/data-sources.md</Text>。
          </span>
        }
      >
        <Input.TextArea
          rows={3}
          style={{ fontFamily: 'monospace', fontSize: 12 }}
          placeholder={`留空 = 全部放行\n例：not_empty("response_body") && gte("status_code", 500)`}
          onBlur={(e) => {
            // Auto-unwrap the `{"filter": "..."}` JSON envelope when the
            // operator copies it out of the API payload by mistake.  See
            // tryUnwrapFilterJSON for the why.
            const raw = e.target.value
            const { value, unwrapped } = tryUnwrapFilterJSON(raw)
            if (unwrapped) {
              form.setFieldValue('kafka_filter', value)
              message.info('检测到 JSON 包装，已自动提取 .filter 字段。')
            }
          }}
        />
      </Form.Item>

      <Alert
        type="warning"
        showIcon
        style={{ marginBottom: 12 }}
        message="安全 filter helper（强烈建议用于排除型条件）"
        description={
          <>
            <span style={{ color: '#a8071a' }}>
              <b>注意</b>：<Text code>{`level != "DEBUG"`}</Text> 在 payload 缺 <Text code>level</Text> 字段时会评估为 <b>true</b>，
              <b>整条消息会被放过去</b>。这是 expr-lang 的 <Text code>AllowUndefinedVariables</Text> 默认行为，与你之前可能用过的 DSL（缺字段返回 false）不一致。
              请用下面的 helper 替代任何 <Text code>!=</Text> / <Text code>not in</Text> / <Text code>not matches</Text> 风格表达式。
            </span>
            <ul style={{ margin: '8px 0 6px 18px', padding: 0, listStyle: 'disc' }}>
              <li><Text code>has(path)</Text> — 字段是否存在</li>
              <li><Text code>eq(path, v)</Text> / <Text code>neq(path, v)</Text> — 严格相等 / 严格不等（缺字段一律 false）</li>
              <li><Text code>gt / gte / lt / lte(path, n)</Text> — 数字比较，自动接受字符串数字（如 <Text code>"500"</Text>）</li>
              <li><Text code>oneof(path, v1, v2, …)</Text> — 字符串值在白名单内（用 <Text code>oneof</Text> 是因为 <Text code>in</Text> 是 expr 关键字）</li>
              <li><Text code>regex_match(path, pattern)</Text> — RE2 正则；命名为 <Text code>regex_match</Text> 是因为 <Text code>matches</Text> 是 expr 内置中缀操作符</li>
              <li><Text code>not_empty(path)</Text> — 非空字符串，剔除 <Text code>""</Text> / <Text code>"-"</Text> / <Text code>"null"</Text> 三种占位</li>
            </ul>
            <b>等价改写示例</b>：
            <ul style={{ margin: '6px 0 6px 18px', padding: 0, listStyle: 'disc' }}>
              <li>排除 DEBUG 日志：<Text code>{`level != "DEBUG"`}</Text> → <Text code>{`neq("level", "DEBUG")`}</Text></li>
              <li>只看 P0/P1：<Text code>{`severity in ["P0","P1"]`}</Text> → <Text code>{`oneof("severity", "P0", "P1")`}</Text></li>
              <li>5xx 错误：<Text code>{`status_code >= 500`}</Text> → <Text code>{`gte("status_code", 500)`}</Text></li>
              <li>只看 /api/ 前缀：<Text code>{`path matches "^/api/"`}</Text> → <Text code>{`regex_match("path", "^/api/")`}</Text></li>
            </ul>
            第一个参数始终是 <b>gjson 路径字符串</b>（与下方 mapping 字段路径写法一致：<Text code>labels.namespace</Text>、<Text code>tags.0</Text>），不要写裸标识符。
          </>
        }
      />

      <Divider plain>字段映射</Divider>
      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 16 }}
        message="路径语法 = gjson 或 expr 表达式（任一切换）"
        description={
          <>
            每个字段都可在左侧 Segmented 切换 <Text code>gjson</Text> 路径（点分隔，例
            <Text code>alert.severity</Text>、<Text code>tags.0</Text>）或 <Text code>expr</Text> 表达式
            （expr-lang 语法，可访问 JSON 顶层字段）。空值表示该字段不映射；
            <b>alertname</b> 和 <b>severity</b> 必填。
            <br />
            <b>内置函数</b>（同时可用于 <Text code>filter</Text> / <Text code>resolved_when</Text> / mapping 表达式）：
            <ul style={{ margin: '6px 0 6px 18px', padding: 0, listStyle: 'disc' }}>
              <li><Text code>strip_query(s)</Text> — 去掉 <Text code>?</Text> 之后的 query string</li>
              <li><Text code>normalize_path(s)</Text> — 把路径里的 id 段（数字 / UUID / Eth 地址 / 长 hex / Base58）统一替换为 <Text code>{'{id}'}</Text></li>
              <li><Text code>regex_replace(s, pattern, repl)</Text> — 任意 RE2 替换</li>
              <li><Text code>coalesce(...)</Text> — 取首个非空 / 非 <Text code>"-"</Text> / 非 <Text code>"null"</Text> 值（适配 Higress 占位）</li>
            </ul>
            <b>常用配方</b>：
            <ul style={{ margin: '6px 0 6px 18px', padding: 0, listStyle: 'disc' }}>
              <li>组合 fingerprint：<Text code>{`route_name + "|" + normalize_path(strip_query(path))`}</Text></li>
              <li>动态 severity：<Text code>{`gte("response_code", 500) ? "P2" : "P3"`}</Text></li>
              <li>取真实 IP：<Text code>{`coalesce(true_client_ip, x_forwarded_for, downstream_remote_address)`}</Text></li>
              <li>filter 排除健康检查：<Text code>{`neq("path", "/healthz")`}</Text></li>
            </ul>
            <b>resolved 信号</b>：当 <Text code>status_path</Text> 命中 <Text code>resolved</Text>/<Text code>ok</Text>/<Text code>recovered</Text>，
            或 <Text code>resolved_when</Text> 表达式为真时，将走 v2 lifecycle 自动归档（无需等待 staleness reaper）。
            <br />
            <b>自动注入</b>：当 <Text code>filter</Text> 非空时，系统会在每条告警上自动追加一条 annotation
            {' '}<Text code>kafka_filter_expr</Text>，内容等于当前 filter 表达式，方便通知/排障时一眼看到“为什么这条被告警”。
            如需自定义可在下方 annotations 列表中显式声明同名 key 来覆盖。
          </>
        }
      />

      <Form.Item name="kafka_map_alertname" label="alertname" rules={[{ required: true, message: '必填' }]}>
        <MappingValueInput
          gjsonPlaceholder="alertname 或 alert.name"
          exprPlaceholder={`route_name 或 alertname + ":" + service`}
        />
      </Form.Item>
      <Form.Item name="kafka_map_severity" label="severity" rules={[{ required: true, message: '必填' }]}>
        <MappingValueInput
          gjsonPlaceholder="severity 或 alert.level"
          exprPlaceholder={`response_code >= "500" ? "P2" : "P3"`}
        />
      </Form.Item>
      <Form.Item name="kafka_map_summary" label="summary">
        <MappingValueInput
          gjsonPlaceholder="summary / msg"
          exprPlaceholder={`method + " " + path + " -> " + response_code`}
        />
      </Form.Item>
      <Form.Item name="kafka_map_description" label="description">
        <MappingValueInput
          gjsonPlaceholder="description / stack"
          exprPlaceholder="任意 expr 表达式"
        />
      </Form.Item>
      <Form.Item
        name="kafka_map_starts_at"
        label="starts_at"
        extra={
          <Text type="secondary" style={{ fontSize: 12 }}>
            字段名以 <code>@</code>/<code>-</code> 开头（如 <code>@timestamp</code>、<code>x-trace-id</code>）请用
            <b> gjson 模式</b>直接填字段名；在 expr 模式中需写成 <code>this[&quot;@timestamp&quot;]</code>。
          </Text>
        }
      >
        <MappingValueInput
          gjsonPlaceholder="start_time（支持 RFC3339 / Unix 秒/毫秒）"
          exprPlaceholder="start_time"
        />
      </Form.Item>
      <Form.Item name="kafka_map_ends_at" label="ends_at">
        <MappingValueInput
          gjsonPlaceholder="ends_at（可选）"
          exprPlaceholder="任意 expr 表达式"
        />
      </Form.Item>
      <Form.Item name="kafka_map_fingerprint" label="fingerprint">
        <MappingValueInput
          gjsonPlaceholder="留空则按 labels 计算"
          exprPlaceholder={`route_name + "|" + normalize_path(strip_query(path))`}
        />
      </Form.Item>
      <Form.Item name="kafka_map_status_path" label="status_path">
        <MappingValueInput
          gjsonPlaceholder='例：state；命中 "resolved" 即视为已恢复'
          exprPlaceholder='response_code >= "500" ? "firing" : "resolved"'
        />
      </Form.Item>
      <Form.Item
        name="kafka_map_resolved_when"
        label="resolved_when 表达式"
        extra={
          <span style={{ color: '#666', fontSize: 11 }}>
            布尔表达式，true 视为已恢复（不支持 <Text code style={{ fontSize: 11 }}>expr:</Text> 前缀）。
            建议用同样的安全 helper，例：
            <br />
            <Text code style={{ fontSize: 11 }}>{`oneof("level", "INFO", "DEBUG")`}</Text>
            {' / '}
            <Text code style={{ fontSize: 11 }}>{`eq("status", "resolved")`}</Text>
          </span>
        }
      >
        <Input.TextArea rows={2} style={{ fontFamily: 'monospace', fontSize: 12 }} />
      </Form.Item>

      <Divider plain>labels（key → gjson 路径 或 expr 表达式）</Divider>
      <KafkaPairList
        listName="kafka_labels"
        keyPlaceholder="label key（如 service）"
        gjsonPlaceholder="路径（如 svc）"
        exprPlaceholder={`normalize_path(strip_query(path))`}
        addLabel="添加 label 映射"
      />

      <Divider plain>annotations（key → gjson 路径 或 expr 表达式）</Divider>
      <KafkaPairList
        listName="kafka_annotations"
        keyPlaceholder="annotation key（如 runbook_url）"
        gjsonPlaceholder="路径（如 runbook）"
        exprPlaceholder={`method + " " + path + " -> " + response_code`}
        addLabel="添加 annotation 映射"
      />

      <Divider plain />
      <Button icon={<PlayCircleOutlined />} onClick={() => setTestOpen(true)}>
        测试一条样例消息
      </Button>

      {testOpen && (
        <KafkaTestMessageModal
          open={testOpen}
          onClose={() => setTestOpen(false)}
          editingId={editingId}
        />
      )}
    </>
  )
}

// KafkaPairList renders the {key, path} editable list used by both
// `kafka_labels` and `kafka_annotations`.  The row layout is a flex
// `<Row>` so the delete button stays anchored to the right edge no
// matter how narrow the drawer is — the previous fixed `width: 160 +
// 420` <Space> would clip the trailing button when the responsive
// drawer settled below ~880px content width.
interface KafkaPairListProps {
  listName: 'kafka_labels' | 'kafka_annotations'
  keyPlaceholder: string
  gjsonPlaceholder: string
  exprPlaceholder: string
  addLabel: string
}

function KafkaPairList({
  listName, keyPlaceholder, gjsonPlaceholder, exprPlaceholder, addLabel,
}: KafkaPairListProps) {
  return (
    <Form.List name={listName}>
      {(fields, { add, remove }) => (
        <>
          {fields.map((field) => (
            <Row key={field.key} gutter={8} align="middle" style={{ marginBottom: 8 }} wrap={false}>
              <Col flex="160px">
                <Form.Item name={[field.name, 'key']} noStyle>
                  <Input placeholder={keyPlaceholder} />
                </Form.Item>
              </Col>
              <Col flex="auto" style={{ minWidth: 0 }}>
                <Form.Item name={[field.name, 'path']} noStyle>
                  <MappingValueInput
                    gjsonPlaceholder={gjsonPlaceholder}
                    exprPlaceholder={exprPlaceholder}
                  />
                </Form.Item>
              </Col>
              <Col flex="36px" style={{ textAlign: 'right' }}>
                <Button
                  type="text"
                  danger
                  aria-label="删除"
                  icon={<MinusCircleOutlined />}
                  onClick={() => remove(field.name)}
                />
              </Col>
            </Row>
          ))}
          <Button onClick={() => add({ key: '', path: '' })} icon={<PlusOutlined />} style={{ marginBottom: 8 }}>
            {addLabel}
          </Button>
        </>
      )}
    </Form.List>
  )
}
