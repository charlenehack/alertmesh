// Two-mode editor for a single string-valued mapping cell. The
// underlying form value stays a single string (so the persisted JSON
// shape is unchanged: gjson cells are stored as `"path"`, expr cells as
// `"expr: <body>"`). Antd Form.Item can't pass `value` and `onChange`
// to a custom inner component unless we play the "controlled component"
// dance below — value comes in through props, onChange fires with the
// reconciled string back out.
//
// Mode is derived from the value (`expr:` prefix → expr; otherwise
// gjson) on every render so the component stays stateless and can't
// drift out of sync with `form.setFieldsValue` calls elsewhere.

import { Input, Segmented, Space } from 'antd'
import { bodyOf, detectMode, joinValue } from './mappingValue'

export interface MappingValueInputProps {
  value?: string
  onChange?: (next: string) => void
  gjsonPlaceholder?: string
  exprPlaceholder?: string
  allowExpr?: boolean
}

export function MappingValueInput({
  value, onChange, gjsonPlaceholder, exprPlaceholder, allowExpr = true,
}: MappingValueInputProps) {
  const mode = detectMode(value)
  const body = bodyOf(value)
  if (!allowExpr) {
    return (
      <Input
        value={value ?? ''}
        onChange={(e) => onChange?.(e.target.value)}
        placeholder={gjsonPlaceholder}
      />
    )
  }
  return (
    <Space.Compact style={{ width: '100%' }}>
      <Segmented
        size="middle"
        value={mode}
        onChange={(v) => onChange?.(joinValue(v as 'gjson' | 'expr', body))}
        options={[
          { value: 'gjson', label: 'gjson' },
          { value: 'expr',  label: 'expr' },
        ]}
        style={{ flex: '0 0 auto' }}
      />
      <Input
        value={body}
        onChange={(e) => onChange?.(joinValue(mode, e.target.value))}
        placeholder={mode === 'expr' ? exprPlaceholder : gjsonPlaceholder}
        style={{
          flex: 1,
          fontFamily: mode === 'expr' ? 'monospace' : undefined,
        }}
      />
    </Space.Compact>
  )
}
