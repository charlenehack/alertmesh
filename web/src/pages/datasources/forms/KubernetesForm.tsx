// Kubernetes connection + detector toggle fields.  Renders the
// in-cluster switch first because flipping it hides the endpoint /
// token / TLS-skip block (the in-pod ServiceAccount supersedes them).

import { Checkbox, Divider, Form, Input, InputNumber, Space, Switch, Typography } from 'antd'

import { K8S_EVENT_OPTIONS, SECRET_MASK } from '../shared/constants'

const { Text } = Typography

export interface KubernetesFormProps {
  editing: boolean
}

export function KubernetesForm({ editing }: KubernetesFormProps) {
  return (
    <>
      <Form.Item name="k8s_in_cluster" label="In-Cluster 模式" valuePropName="checked" extra={
        <span style={{ color: '#666', fontSize: 11 }}>
          打开后忽略下方 endpoint / token，连接器在 Pod 内通过 ServiceAccount 自动鉴权。
        </span>
      }>
        <Switch />
      </Form.Item>

      <Form.Item shouldUpdate={(p, c) => p.k8s_in_cluster !== c.k8s_in_cluster} noStyle>
        {({ getFieldValue }) => {
          const inCluster = getFieldValue('k8s_in_cluster')
          if (inCluster) return null
          return (
            <>
              <Form.Item name="endpoint" label="API Server URL" rules={[{ required: true, message: 'API Server 必填' }]}>
                <Input placeholder="https://kubernetes.default.svc:6443" />
              </Form.Item>
              <Form.Item name="k8s_token" label="Bearer Token (敏感)" rules={editing ? [] : [{ required: true, message: 'Token 必填' }]}>
                <Input.Password placeholder={editing ? SECRET_MASK : 'kubectl create token … 生成的 ServiceAccount Token'} autoComplete="new-password" />
              </Form.Item>
              <Form.Item name="k8s_tls_skip" label="跳过 TLS 证书校验" valuePropName="checked">
                <Switch />
              </Form.Item>
            </>
          )
        }}
      </Form.Item>

      <Divider style={{ borderColor: '#1e1e1e', margin: '8px 0' }} />

      <Form.Item
        name="k8s_events"
        label="检测的事件类型"
        rules={[{ required: true, type: 'array', min: 1, message: '至少勾选一种事件' }]}
        extra={
          <span style={{ color: '#666', fontSize: 11 }}>
            连接器为每种类型启动对应的 Informer / 检测器；具体触发逻辑见 README §4.1.4。
          </span>
        }
      >
        <Checkbox.Group style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
          {K8S_EVENT_OPTIONS.map((opt) => (
            <Checkbox key={opt.value} value={opt.value}>
              <Space>
                <span>{opt.label}</span>
                <Text type="secondary" style={{ fontSize: 11 }}>{opt.hint}</Text>
              </Space>
            </Checkbox>
          ))}
        </Checkbox.Group>
      </Form.Item>

      <Form.Item name="k8s_watched_namespaces" label="监听的命名空间（逗号分隔，留空 = 全部）">
        <Input placeholder="prod, infra, monitoring" />
      </Form.Item>

      <Form.Item name="k8s_ignored_namespaces" label="忽略的命名空间（逗号分隔）">
        <Input placeholder="kube-system, kube-public" />
      </Form.Item>

      <Form.Item name="k8s_ignored_pods_re" label="忽略 Pod 名称正则（RE2）">
        <Input placeholder="^(canary|sandbox)-" />
      </Form.Item>

      <Form.Item name="k8s_mute_seconds" label="同一 Pod 静默窗口（秒）" initialValue={1800} extra={
        <span style={{ color: '#666', fontSize: 11 }}>
          相同 Pod 连续告警的最小间隔，避免重启风暴；参考 airwallex/k8s-pod-restart-info-collector 的 per_pod_seconds。
        </span>
      }>
        <InputNumber style={{ width: '100%' }} min={0} max={86400} step={60} />
      </Form.Item>

      <Form.Item name="k8s_ignore_restart_count_above" label="忽略 RestartCount 大于" initialValue={20}>
        <InputNumber style={{ width: '100%' }} min={0} max={1000} />
      </Form.Item>

      <Form.Item name="k8s_pending_threshold_seconds" label="Pending 阈值（秒）" initialValue={300}>
        <InputNumber style={{ width: '100%' }} min={30} max={86400} step={30} />
      </Form.Item>
    </>
  )
}
