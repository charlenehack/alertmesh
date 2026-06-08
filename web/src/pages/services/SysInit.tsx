import { Result } from 'antd'
import { ToolOutlined } from '@ant-design/icons'

export default function SysInit() {
  return (
    <div style={{ padding: '80px 0' }}>
      <Result
        icon={<ToolOutlined style={{ color: '#1677ff' }} />}
        title="此功能正在开发中"
        subTitle="系统初始化功能即将上线，敬请期待。"
      />
    </div>
  )
}
