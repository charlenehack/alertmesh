import { useState } from 'react'
import { Button, Checkbox, Input } from 'antd'
import { CloseOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons'
import type { NotificationContact, NotificationContactGroup } from '../../../types'

interface ContactGroupPickerProps {
  allGroups: NotificationContactGroup[]
  allContacts: NotificationContact[]
  value?: string[]
  onChange?: (ids: string[]) => void
}

// Left/right split panel for selecting contact groups (matches the design
// in the screenshot). Left side is the searchable / filterable list;
// right side is the selection summary with quick-remove chips.
export function ContactGroupPicker({
  allGroups, allContacts, value, onChange,
}: ContactGroupPickerProps) {
  const [leftSearch, setLeftSearch] = useState('')
  const selected = value ?? []

  const nameMap = Object.fromEntries(allContacts.map((c) => [c.id, c.name]))

  const filtered = allGroups.filter(
    (g) => !leftSearch || g.name.toLowerCase().includes(leftSearch.toLowerCase()),
  )

  const toggle = (id: string) => {
    const next = selected.includes(id) ? selected.filter((x) => x !== id) : [...selected, id]
    onChange?.(next)
  }
  const remove = (id: string) => onChange?.(selected.filter((x) => x !== id))

  const selectedGroups = allGroups.filter((g) => selected.includes(g.id))

  return (
    <div style={{ display: 'flex', gap: 12, height: 260 }}>
      {/* Left panel */}
      <div style={{ flex: 1, border: '1px solid #2a2a2a', borderRadius: 6, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
        <div style={{ padding: '8px 10px', borderBottom: '1px solid #2a2a2a', display: 'flex', alignItems: 'center', gap: 6 }}>
          <span style={{ color: '#999', fontSize: 13, flex: 1 }}>全部联系人组</span>
          <Button size="small" type="text" icon={<ReloadOutlined style={{ color: '#555', fontSize: 12 }} />} />
        </div>
        <div style={{ padding: '8px 10px', borderBottom: '1px solid #2a2a2a' }}>
          <Input
            size="small"
            placeholder="请输入名称"
            prefix={<SearchOutlined style={{ color: '#555' }} />}
            style={{ background: '#1a1a1a', border: '1px solid #2a2a2a', color: '#e8e8e8' }}
            value={leftSearch}
            onChange={(e) => setLeftSearch(e.target.value)}
          />
        </div>
        <div style={{ flex: 1, overflowY: 'auto' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr style={{ background: '#1a1a1a' }}>
                <th style={{ width: 32, padding: '6px 10px' }}>
                  <Checkbox
                    checked={filtered.length > 0 && filtered.every((g) => selected.includes(g.id))}
                    indeterminate={filtered.some((g) => selected.includes(g.id)) && !filtered.every((g) => selected.includes(g.id))}
                    onChange={(e) => {
                      if (e.target.checked) onChange?.([...new Set([...selected, ...filtered.map((g) => g.id)])])
                      else onChange?.(selected.filter((id) => !filtered.find((g) => g.id === id)))
                    }}
                  />
                </th>
                <th style={{ padding: '6px 0', color: '#666', fontSize: 12, fontWeight: 400, textAlign: 'left' }}>名称</th>
                <th style={{ padding: '6px 0', color: '#666', fontSize: 12, fontWeight: 400, textAlign: 'left' }}>联系人</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((g) => {
                const contactNames = (g.contact_ids || []).slice(0, 3).map((id) => nameMap[id] || id.slice(0, 6))
                const extra = (g.contact_ids || []).length - 3
                return (
                  <tr
                    key={g.id}
                    style={{ borderTop: '1px solid #1e1e1e', cursor: 'pointer' }}
                    onClick={() => toggle(g.id)}
                  >
                    <td style={{ padding: '7px 10px' }}>
                      <Checkbox checked={selected.includes(g.id)} onChange={() => toggle(g.id)} />
                    </td>
                    <td style={{ padding: '7px 0', color: '#e8e8e8', fontSize: 13 }}>{g.name}</td>
                    <td style={{ padding: '7px 0', color: '#888', fontSize: 12 }}>
                      {contactNames.join(' ')}
                      {extra > 0 && <span style={{ color: '#1677ff' }}> +{extra}</span>}
                    </td>
                  </tr>
                )
              })}
              {filtered.length === 0 && (
                <tr>
                  <td colSpan={3} style={{ textAlign: 'center', padding: 20, color: '#444', fontSize: 12 }}>暂无数据</td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Right panel */}
      <div style={{ width: 200, border: '1px solid #2a2a2a', borderRadius: 6, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
        <div style={{ padding: '8px 10px', borderBottom: '1px solid #2a2a2a', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <span style={{ color: '#999', fontSize: 13 }}>已选联系人组 ({selected.length})</span>
          <Button
            size="small"
            type="text"
            onClick={() => onChange?.([])}
            icon={<CloseOutlined style={{ color: '#555', fontSize: 11 }} />}
          />
        </div>
        <div style={{ flex: 1, overflowY: 'auto', padding: '6px 0' }}>
          {selectedGroups.map((g) => (
            <div
              key={g.id}
              style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '5px 12px' }}
            >
              <span style={{ color: '#e8e8e8', fontSize: 13 }}>{g.name}</span>
              <CloseOutlined
                style={{ color: '#555', fontSize: 11, cursor: 'pointer' }}
                onClick={() => remove(g.id)}
              />
            </div>
          ))}
          {selected.length === 0 && (
            <div style={{ textAlign: 'center', padding: 20, color: '#444', fontSize: 12 }}>未选择</div>
          )}
        </div>
      </div>
    </div>
  )
}
