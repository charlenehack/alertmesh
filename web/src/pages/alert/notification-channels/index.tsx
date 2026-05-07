import { Tabs, Typography } from 'antd'
import { ContactGroupList } from './ContactGroupList'
import { ContactList } from './ContactList'
import { NotificationMatrixCard } from './NotificationMatrixCard'
import { PageCard } from './Toolbar'
import { PolicyList } from './PolicyList'

const { Title } = Typography

// 通知对象容器（联系人 + 联系人组两个子 Tab）
function NotificationObjects() {
  return (
    <Tabs
      size="small"
      tabBarStyle={{ padding: '0 16px', marginBottom: 0, borderBottom: '1px solid #1e1e1e' }}
      items={[
        { key: 'contacts', label: '联系人', children: <ContactList /> },
        { key: 'groups', label: '联系人组', children: <ContactGroupList /> },
      ]}
    />
  )
}

export default function NotificationChannels() {
  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 20 }}>
        <Title level={5} style={{ margin: 0, color: '#ffffff' }}>告警通知策略</Title>
      </div>

      <NotificationMatrixCard />

      <PageCard>
        <Tabs
          tabBarStyle={{
            padding: '0 16px',
            margin: 0,
            borderBottom: '1px solid #1e1e1e',
            background: '#0d0d0d',
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
