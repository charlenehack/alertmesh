// Prometheus connection fields.  Used by both the create drawer and the
// edit drawer; the optional `editing` prop swaps the password / token
// placeholder so operators see the SECRET_MASK hint instead of the
// "RSA-encrypted" copy when amending an existing row.

import { Alert, Form, Input, Select, Switch, Typography } from 'antd'

import { SECRET_MASK } from '../shared/constants'

const { Text } = Typography
const { Option } = Select

export interface PrometheusFormProps {
  editing: boolean
}

export function PrometheusForm({ editing }: PrometheusFormProps) {
  return (
    <>
      <Form.Item name="endpoint" label="Endpoint" rules={[{ required: true, message: 'Prometheus URL 必填' }]} extra={
        <span style={{ color: '#666', fontSize: 11 }}>例如 <Text code style={{ fontSize: 11 }}>http://prometheus.monitoring:9090</Text></span>
      }>
        <Input placeholder="http://prometheus:9090" />
      </Form.Item>

      <Form.Item name="prom_auth_type" label="认证方式" initialValue="none">
        <Select>
          <Option value="none">无（公开访问）</Option>
          <Option value="basic">Basic（用户名 + 密码）</Option>
          <Option value="bearer">Bearer Token</Option>
        </Select>
      </Form.Item>

      <Form.Item shouldUpdate={(p, c) => p.prom_auth_type !== c.prom_auth_type} noStyle>
        {({ getFieldValue }) => {
          const at = getFieldValue('prom_auth_type')
          if (at === 'basic') {
            return (
              <>
                <Form.Item name="prom_username" label="用户名">
                  <Input autoComplete="off" />
                </Form.Item>
                <Form.Item name="prom_password" label="密码 (敏感)">
                  <Input.Password placeholder={editing ? SECRET_MASK : '在浏览器侧 RSA 加密后传输'} autoComplete="new-password" />
                </Form.Item>
              </>
            )
          }
          if (at === 'bearer') {
            return (
              <Form.Item name="prom_token" label="Bearer Token (敏感)">
                <Input.Password placeholder={editing ? SECRET_MASK : '在浏览器侧 RSA 加密后传输'} autoComplete="new-password" />
              </Form.Item>
            )
          }
          return null
        }}
      </Form.Item>

      <Form.Item name="prom_tls_skip" label="跳过 TLS 证书校验" valuePropName="checked">
        <Switch />
      </Form.Item>

      <Alert
        type="info" showIcon style={{ marginTop: 8 }}
        message="Prometheus 数据源仅供 AI 工具检索 + Explore 页查询"
        description="AlertMesh 不会基于 PromQL 创建告警规则；告警仍由 Prometheus / Alertmanager 经 webhook 推送进来。"
      />
    </>
  )
}
