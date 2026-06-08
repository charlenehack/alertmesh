import { Alert, Typography } from 'antd'
import { InfoCircleOutlined } from '@ant-design/icons'
import { SevTag } from './SevTag'
import { useTheme } from '../../../hooks/useTheme'

const { Text } = Typography

// 默认通知矩阵（v3 dispatcher 硬编码语义）
//
// IM/Email 列由通知策略的 severities 过滤决定，仅当联系人配置过相应 webhook /
// 邮筱、且通知策略的 severities 包含该等级时才分发；Voice/SMS 列由 dispatcher
// 在 resolveRecipients 阶段硬编码为 severity == "P0"，与通知策略无关。
type Cell = '✓' | '—'
const MATRIX: Array<{ sev: 'P0' | 'P1' | 'P2' | 'P3'; im: Cell; email: Cell; voice: Cell; sms: Cell }> = [
  { sev: 'P0', im: '✓', email: '✓', voice: '✓', sms: '✓' },
  { sev: 'P1', im: '✓', email: '✓', voice: '—', sms: '—' },
  { sev: 'P2', im: '✓', email: '✓', voice: '—', sms: '—' },
  { sev: 'P3', im: '✓', email: '✓', voice: '—', sms: '—' },
]

export function NotificationMatrixCard() {
  const { c } = useTheme()
  const cell = (v: Cell) => (
    <span style={{ color: v === '✓' ? '#52c41a' : c.textTertiary, fontWeight: 700 }}>{v}</span>
  )
  return (
    <Alert
      type="info"
      icon={<InfoCircleOutlined />}
      message={<Text style={{ fontSize: 13 }}>默认通知矩阵 (v3)</Text>}
      description={
        <div style={{ marginTop: 8 }}>
          <table style={{ borderCollapse: 'collapse', fontSize: 12, marginBottom: 8 }}>
            <thead>
              <tr style={{ color: c.textHint }}>
                <th style={{ padding: '4px 12px', textAlign: 'left' }}>严重级</th>
                <th style={{ padding: '4px 12px' }}>IM (DingTalk/Feishu/Slack)</th>
                <th style={{ padding: '4px 12px' }}>Email</th>
                <th style={{ padding: '4px 12px' }}>Voice 电话</th>
                <th style={{ padding: '4px 12px' }}>SMS 短信</th>
              </tr>
            </thead>
            <tbody>
              {MATRIX.map((r) => (
                <tr key={r.sev} style={{ borderTop: `1px solid ${c.border}` }}>
                  <td style={{ padding: '4px 12px' }}><SevTag s={r.sev} /></td>
                  <td style={{ padding: '4px 12px', textAlign: 'center' }}>{cell(r.im)}</td>
                  <td style={{ padding: '4px 12px', textAlign: 'center' }}>{cell(r.email)}</td>
                  <td style={{ padding: '4px 12px', textAlign: 'center' }}>{cell(r.voice)}</td>
                  <td style={{ padding: '4px 12px', textAlign: 'center' }}>{cell(r.sms)}</td>
                </tr>
              ))}
            </tbody>
          </table>
          <div style={{ color: c.textHint, fontSize: 12, lineHeight: '20px' }}>
            • IM / Email：仍按通知策略 <Text code>severities</Text> 路由，dispatcher 自动按 channel target 合并 @mention / To 列表，
            一次 webhook / SMTP 调用覆盖多人。
            <br />• Voice / SMS：dispatcher 层硬编码 <Text strong style={{ color: '#fadb14' }}>仅 P0</Text> 触发，无视通知策略 severities；
            需要在系统设置中配置 <Text code>notification.voice</Text> / <Text code>notification.sms</Text> provider，
            并保证联系人填写了手机号。
            <br />• v3 已下线后台 <Text code>escalation_policies</Text> 升级链；升级阶梯统一在
            <Text code> 系统配置 → 告警生命周期 → severity_chain </Text> 中配置（按驻留时长触发）。
          </div>
        </div>
      }
      style={{ marginBottom: 16, background: 'rgba(22,119,255,0.08)', border: '1px solid rgba(22,119,255,0.25)' }}
    />
  )
}
