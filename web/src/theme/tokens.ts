// Single source of truth for the dark-theme palette. Pages should import
// from here instead of hard-coding hex values; the colours below are the
// ones that appeared 3+ times across the original inline `style={{...}}`
// chunks and CSS overrides.

export const colors = {
  // Layered surfaces — page → card → elevated
  bgPage: '#0a0a0a',
  bgSurface: '#111111',
  bgElevated: '#1a1a1a',
  bgHover: '#222222',
  bgInput: '#1a1a1a',

  // Borders, dimmest → strongest
  borderSubtle: '#1e1e1e',
  border: '#222222',
  borderStrong: '#333333',

  // Text, dimmest → strongest
  textMuted: '#444444',
  textTertiary: '#555555',
  textSecondary: '#666666',
  textHint: '#999999',
  textBody: '#e8e8e8',
  textStrong: '#ffffff',

  // Accents (severity / state)
  accent: '#ffffff',
  primary: '#1677ff',
  danger: '#ff4d4f',
  warning: '#fa8c16',
  caution: '#fadb14',
  success: '#52c41a',
  ai: '#722ed1',
} as const

export type ColorToken = keyof typeof colors
