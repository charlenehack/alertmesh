// Single source of truth for theme palettes.
// `colors` = dark palette (kept for backward compat with all existing imports).
// Use `getColors(mode)` in dynamic-theme contexts.
import type { ThemeMode } from './ThemeContext'

const darkColors = {
  // Layered surfaces — page → card → elevated
  bgPage: '#0a0a0a',
  bgSurface: '#111111',
  bgElevated: '#1a1a1a',
  bgHover: '#222222',
  bgInput: '#1a1a1a',

  // Borders, dimmest → strongest
  borderSubtle: '#1e1e1e',
  border: '#2a2a2a',
  borderStrong: '#444444',
  borderInput: '#555555',

  // Text, dimmest → strongest
  textMuted: '#555555',
  textTertiary: '#777777',
  textSecondary: '#999999',
  textHint: '#bbbbbb',
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
}

const lightColors = {
  // Layered surfaces
  bgPage: '#f5f5f7',
  bgSurface: '#ffffff',
  bgElevated: '#f0f0f0',
  bgHover: '#e8e8e8',
  bgInput: '#ffffff',

  // Borders
  borderSubtle: '#e8e8e8',
  border: '#d9d9d9',
  borderStrong: '#bfbfbf',
  borderInput: '#d9d9d9',

  // Text
  textMuted: '#bfbfbf',
  textTertiary: '#8c8c8c',
  textSecondary: '#595959',
  textHint: '#434343',
  textBody: '#262626',
  textStrong: '#000000',

  // Accents
  accent: '#000000',
  primary: '#1677ff',
  danger: '#ff4d4f',
  warning: '#fa8c16',
  caution: '#d4a017',
  success: '#389e0d',
  ai: '#722ed1',
}

export type ColorPalette = typeof darkColors
export type ColorToken = keyof ColorPalette

export function getColors(mode: ThemeMode): ColorPalette {
  return mode === 'light' ? lightColors : darkColors
}

// Default export — dark palette (keeps backward compat with all existing imports)
export const colors: ColorPalette = darkColors
