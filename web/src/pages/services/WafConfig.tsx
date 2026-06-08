import { Result } from 'antd'
import { SafetyCertificateOutlined } from '@ant-design/icons'

export default function WafConfig() {
  return (
    <div style={{ padding: '80px 0' }}>
      <Result
        icon={<SafetyCertificateOutlined style={{ color: '#1677ff' }} />}
        title="此功能正在开发中"
        subTitle="WAF 配置功能即将上线，敬请期待。"
      />
    </div>
  )
}
