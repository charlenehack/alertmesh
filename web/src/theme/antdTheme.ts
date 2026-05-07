import type { ThemeConfig } from 'antd'
import { colors } from './tokens'

// Single antd ThemeConfig built from the design tokens. Per-component
// overrides here replace the bulk of the ~365-line `!important` stack that
// used to live in `index.css` for the dark theme.
export const antdTheme: ThemeConfig = {
  token: {
    colorPrimary: colors.accent,
    colorBgBase: colors.bgPage,
    colorTextBase: colors.textBody,
    colorBorder: colors.borderStrong,
    colorBgContainer: colors.bgSurface,
    colorBgElevated: colors.bgElevated,
    colorBgLayout: colors.bgPage,
    colorText: colors.textBody,
    colorTextSecondary: colors.textSecondary,
    colorTextPlaceholder: colors.textTertiary,
    colorFill: colors.bgElevated,
    colorFillSecondary: colors.bgHover,
    borderRadius: 6,
    fontFamily: "'PingFang SC', 'Microsoft YaHei', sans-serif",
  },
  components: {
    Table: {
      headerBg: colors.bgElevated,
      headerColor: colors.textHint,
      rowHoverBg: colors.bgElevated,
      borderColor: colors.border,
      colorBgContainer: colors.bgSurface,
    },
    Menu: {
      darkItemBg: 'transparent',
      darkSubMenuItemBg: 'transparent',
      darkItemSelectedBg: 'rgba(255,255,255,0.08)',
      darkItemHoverBg: 'rgba(255,255,255,0.04)',
    },
    Modal: {
      contentBg: colors.bgSurface,
      headerBg: colors.bgSurface,
      titleColor: colors.textBody,
    },
    Card: {
      colorBgContainer: colors.bgSurface,
      colorBorderSecondary: colors.border,
    },
    Pagination: {
      itemActiveBg: colors.bgElevated,
    },
    Select: {
      selectorBg: colors.bgInput,
      optionSelectedBg: '#2a2a2a',
      optionActiveBg: '#2a2a2a',
    },
    Input: {
      colorBgContainer: colors.bgInput,
      activeBorderColor: '#555555',
      activeShadow: '0 0 0 2px rgba(255,255,255,0.06)',
    },
    InputNumber: {
      colorBgContainer: colors.bgInput,
    },
    Button: {
      defaultBg: colors.bgElevated,
      defaultColor: colors.textBody,
      defaultBorderColor: colors.borderStrong,
      defaultHoverBorderColor: colors.textTertiary,
      defaultHoverColor: colors.textStrong,
      primaryColor: '#000000',
      colorPrimaryHover: '#e0e0e0',
    },
    Tabs: {
      itemColor: colors.textSecondary,
      itemHoverColor: colors.textBody,
      itemSelectedColor: colors.textStrong,
      inkBarColor: colors.textStrong,
      colorBorderSecondary: colors.border,
    },
    Descriptions: {
      colorTextSecondary: colors.textSecondary,
      titleColor: colors.textBody,
    },
    Timeline: {
      tailColor: colors.borderStrong,
    },
    Divider: {
      colorSplit: colors.border,
      colorTextHeading: colors.textSecondary,
    },
    Alert: {
      colorInfoBg: colors.bgElevated,
      colorInfoBorder: colors.borderStrong,
    },
    Popover: {
      colorBgElevated: colors.bgElevated,
    },
    Dropdown: {
      colorBgElevated: colors.bgElevated,
    },
    Statistic: {
      titleFontSize: 12,
      contentFontSize: 24,
    },
    Empty: {
      colorTextDisabled: colors.textTertiary,
    },
    Form: {
      labelColor: colors.textHint,
      labelFontSize: 13,
    },
    Tag: {
      defaultBg: colors.bgElevated,
      defaultColor: colors.textHint,
    },
    Message: {
      contentBg: colors.bgElevated,
    },
    Tooltip: {
      colorBgSpotlight: '#2a2a2a',
    },
    Spin: {
      colorPrimary: colors.textTertiary,
    },
  },
}
