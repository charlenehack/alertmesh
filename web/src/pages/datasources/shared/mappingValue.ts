// Mode helpers for the per-cell mapping editor. The underlying form value
// stays a single string (so the persisted JSON shape is unchanged: gjson
// cells are stored as `"path"`, expr cells as `"expr: <body>"`).

export const EXPR_PREFIX = 'expr:'

export function detectMode(v: string | undefined): 'gjson' | 'expr' {
  if (!v) return 'gjson'
  return v.trimStart().toLowerCase().startsWith(EXPR_PREFIX) ? 'expr' : 'gjson'
}

export function bodyOf(v: string | undefined): string {
  if (!v) return ''
  const t = v.trimStart()
  if (t.toLowerCase().startsWith(EXPR_PREFIX)) {
    return t.slice(EXPR_PREFIX.length).replace(/^\s+/, '')
  }
  return v
}

export function joinValue(mode: 'gjson' | 'expr', body: string): string {
  if (mode === 'expr') {
    if (!body) return 'expr:'
    return `expr: ${body}`
  }
  return body
}
