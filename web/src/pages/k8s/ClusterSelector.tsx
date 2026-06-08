/**
 * ClusterSelector – top-bar dropdown to switch between configured k8s clusters.
 */
import { Select, Tag, Space } from 'antd'
import { ClusterOutlined } from '@ant-design/icons'
import type { ClusterRow } from './useCluster'

interface Props {
  clusters: ClusterRow[]
  value: string
  onChange: (id: string) => void
}

export function ClusterSelector({ clusters, value, onChange }: Props) {
  return (
    <Space size={8}>
      <ClusterOutlined style={{ color: '#6f6f6f' }} />
      <Select
        style={{ minWidth: 200 }}
        placeholder="选择集群"
        value={value || undefined}
        onChange={onChange}
        options={clusters.map(c => ({
          value: c.id,
          label: (
            <Space size={6}>
              <span>{c.name}</span>
              {c.last_test_ok === true && <Tag color="success" style={{ margin: 0, lineHeight: '16px', fontSize: 11 }}>正常</Tag>}
              {c.last_test_ok === false && <Tag color="error" style={{ margin: 0, lineHeight: '16px', fontSize: 11 }}>异常</Tag>}
              {c.last_test_ok === null && <Tag style={{ margin: 0, lineHeight: '16px', fontSize: 11 }}>未测试</Tag>}
            </Space>
          ),
        }))}
      />
    </Space>
  )
}
