import { Tabs, Typography } from 'antd'
import { SoundOutlined } from '@ant-design/icons'
import { ContactGroupList } from './ContactGroupList'
import { ContactList } from './ContactList'
import { NotificationMatrixCard } from './NotificationMatrixCard'
import { PageCard } from './Toolbar'
import { PolicyList } from './PolicyList'
import { useTheme } from '../../../hooks/useTheme'

const { Title } = Typography

// 通知对象容器（联系人 + 联系人组两个子 Tab）
function NotificationObjects() {
  const { c } = useTheme()
  return (
    <Tabs
      size="small"
      tabBarStyle={{ padding: '0 16px', marginBottom: 0, borderBottom: `1px solid ${c.border}` }}
      items={[
        { key: 'contacts', label: '联系人', children: <ContactList /> },
        { key: 'groups', label: '联系人组', children: <ContactGroupList /> },
      ]}
    />
  )
}

export default function NotificationChannels() {
  const { c } = useTheme()
  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 20 }}>
        <SoundOutlined style={{ fontSize: 18, color: c.primary }} />
        <Title level={5} style={{ margin: 0, color: c.textBody }}>告警通知策略</Title>
      </div>

      <NotificationMatrixCard />

      <PageCard>
        <Tabs
          tabBarStyle={{
            padding: '0 16px',
            margin: 0,
            borderBottom: `1px solid ${c.border}`,
            background: c.bgElevated,
          }}
          items={[
            { key: 'policies', label: '策略列表', children: <PolicyList /> },
            { key: 'objects', label: '通知对象', children: <NotificationObjects /> },
          ]}
        />
      </PageCard>
    </div>
  )
}
