import { Result } from 'antd'
import { CloudOutlined } from '@ant-design/icons'
export default function TencentCloud() {
  return (
    <div style={{ padding: '80px 0' }}>
      <Result
        icon={<CloudOutlined style={{ color: '#1677ff' }} />}
        title="此功能正在开发中"
        subTitle="腾讯云资产管理功能即将上线，敬请期待。"
      />
    </div>
  )
}
