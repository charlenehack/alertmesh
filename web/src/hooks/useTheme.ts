import { useThemeMode } from '../theme/ThemeContext'
import { getColors } from '../theme/tokens'

/**
 * Convenience hook — returns { c, mode, isDark, toggle }.
 * Use `c.xxx` instead of `colors.xxx` to get theme-aware color values.
 *
 * Usage:
 *   const { c, isDark } = useTheme()
 */
export function useTheme() {
  const { mode, toggle } = useThemeMode()
  const c = getColors(mode)
  const isDark = mode === 'dark'
  return { c, mode, isDark, toggle }
}
