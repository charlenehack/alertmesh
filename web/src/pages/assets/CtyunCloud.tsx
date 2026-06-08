import { Result } from 'antd'
import { CloudOutlined } from '@ant-design/icons'
export default function CtyunCloud() {
  return (
    <div style={{ padding: '80px 0' }}>
      <Result
        icon={<CloudOutlined style={{ color: '#1677ff' }} />}
        title="此功能正在开发中"
        subTitle="天翼云资产管理功能即将上线，敬请期待。"
      />
    </div>
  )
}
