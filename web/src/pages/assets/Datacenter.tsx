import { Result } from 'antd'
import { BankOutlined } from '@ant-design/icons'
export default function Datacenter() {
  return (
    <div style={{ padding: '80px 0' }}>
      <Result
        icon={<BankOutlined style={{ color: '#1677ff' }} />}
        title="此功能正在开发中"
        subTitle="本地机房资产管理功能即将上线，敬请期待。"
      />
    </div>
  )
}
