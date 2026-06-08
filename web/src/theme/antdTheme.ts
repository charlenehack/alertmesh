import type { ThemeConfig } from 'antd'
import { theme as antdAlgorithm } from 'antd'
import { getColors } from './tokens'
import type { ThemeMode } from './ThemeContext'

// Single antd ThemeConfig built from the design tokens. Per-component
// overrides here replace the bulk of the ~365-line `!important` stack that
// used to live in `index.css` for the dark theme.
export function getAntdTheme(mode: ThemeMode): ThemeConfig {
  const c = getColors(mode)
  const isDark = mode === 'dark'

  return {
    algorithm: isDark ? antdAlgorithm.darkAlgorithm : antdAlgorithm.defaultAlgorithm,
    token: {
      colorPrimary: c.primary,
      colorBgBase: c.bgPage,
      colorTextBase: c.textBody,
      colorBorder: c.borderStrong,
      colorBgContainer: c.bgSurface,
      colorBgElevated: c.bgElevated,
      colorBgLayout: c.bgPage,
      colorText: c.textBody,
      colorTextSecondary: c.textSecondary,
      colorTextPlaceholder: c.textTertiary,
      colorFill: c.bgElevated,
      colorFillSecondary: c.bgHover,
      colorLink: c.textHint,
      colorLinkHover: c.textBody,
      colorLinkActive: c.textBody,
      colorOutline: 'rgba(22,119,255,0.15)',
      colorOutlineBg: 'transparent',
      colorPrimaryBg: 'rgba(22,119,255,0.08)',
      colorPrimaryBorder: 'rgba(22,119,255,0.3)',
      colorPrimaryBgHover: 'rgba(22,119,255,0.12)',
      colorPrimaryBorderHover: 'rgba(22,119,255,0.4)',
      borderRadius: 6,
      fontFamily: "'Inter', 'PingFang SC', 'Microsoft YaHei', sans-serif",
    },
    components: {
      Table: {
        headerBg: c.bgElevated,
        headerColor: c.textHint,
        rowHoverBg: c.bgElevated,
        borderColor: c.border,
        colorBgContainer: c.bgSurface,
      },
      Menu: isDark
        ? {
            darkItemBg: 'transparent',
            darkSubMenuItemBg: 'transparent',
            darkItemSelectedBg: 'rgba(255,255,255,0.08)',
            darkItemHoverBg: 'rgba(255,255,255,0.04)',
          }
        : {
            itemBg: 'transparent',
            subMenuItemBg: 'transparent',
            itemSelectedBg: 'rgba(22,119,255,0.08)',
            itemHoverBg: 'rgba(0,0,0,0.04)',
          },
      Modal: {
        contentBg: c.bgSurface,
        headerBg: c.bgSurface,
        titleColor: c.textBody,
      },
      Card: {
        colorBgContainer: c.bgSurface,
        colorBorderSecondary: c.border,
      },
      Pagination: {
        itemActiveBg: c.bgElevated,
      },
      Select: {
        selectorBg: c.bgInput,
        optionSelectedBg: isDark ? '#2a2a2a' : '#e6f4ff',
        optionActiveBg: isDark ? '#2a2a2a' : '#f0f7ff',
      },
      Input: {
        colorBgContainer: c.bgInput,
        activeBorderColor: c.borderInput,
        activeShadow: isDark ? '0 0 0 2px rgba(255,255,255,0.08)' : '0 0 0 2px rgba(22,119,255,0.12)',
        colorBorder: c.borderStrong,
        hoverBorderColor: c.borderInput,
      },
      InputNumber: {
        colorBgContainer: c.bgInput,
      },
      Button: {
        defaultBg: c.bgElevated,
        defaultColor: c.textBody,
        defaultBorderColor: c.borderStrong,
        defaultHoverBorderColor: c.textTertiary,
        defaultHoverColor: c.textStrong,
        primaryColor: '#ffffff',
        colorPrimaryHover: '#4096ff',
      },
      Tabs: {
        itemColor: c.textSecondary,
        itemHoverColor: c.textBody,
        itemSelectedColor: c.textStrong,
        inkBarColor: c.textStrong,
        colorBorderSecondary: c.border,
      },
      Descriptions: {
        colorTextSecondary: c.textSecondary,
        titleColor: c.textBody,
      },
      Timeline: {
        tailColor: c.borderStrong,
      },
      Divider: {
        colorSplit: c.border,
        colorTextHeading: c.textSecondary,
      },
      Alert: {
        colorInfoBg: 'rgba(22,119,255,0.08)',
        colorInfoBorder: 'rgba(22,119,255,0.25)',
        colorWarningBg: 'rgba(250,140,22,0.08)',
        colorWarningBorder: 'rgba(250,140,22,0.25)',
        colorErrorBg: 'rgba(255,77,79,0.08)',
        colorErrorBorder: 'rgba(255,77,79,0.25)',
        colorSuccessBg: 'rgba(82,196,26,0.08)',
        colorSuccessBorder: 'rgba(82,196,26,0.25)',
      },
      Popover: {
        colorBgElevated: c.bgElevated,
      },
      Dropdown: {
        colorBgElevated: c.bgElevated,
      },
      Statistic: {
        titleFontSize: 12,
        contentFontSize: 24,
      },
      Empty: {
        colorTextDisabled: c.textTertiary,
      },
      Form: {
        labelColor: c.textHint,
        labelFontSize: 13,
      },
      Tag: {
        defaultBg: c.bgElevated,
        defaultColor: c.textHint,
      },
      Message: {
        contentBg: c.bgElevated,
      },
      Tooltip: {
        colorBgSpotlight: isDark ? '#2a2a2a' : '#434343',
      },
      Switch: {
        colorPrimary: c.primary,
        colorPrimaryHover: '#4096ff',
        colorBgContainer: isDark ? '#444444' : '#d9d9d9',
        colorText: c.textStrong,
        trackHeight: 22,
        trackMinWidth: 44,
      },
      Checkbox: {
        colorBgContainer: 'transparent',
        colorBorder: c.borderInput,
        colorPrimary: c.primary,
        colorPrimaryHover: '#4096ff',
        colorCheckPrimary: c.textStrong,
      },
      Spin: {
        colorPrimary: c.textTertiary,
      },
    },
  }
}

// Keep backward compat for any non-dynamic import
export const antdTheme = getAntdTheme('dark')
