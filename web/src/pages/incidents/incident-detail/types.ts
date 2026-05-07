export interface AIConversation {
  id: string
  role: string
  content: string
  created_at: string
}

export interface AIReportData {
  report?: string
  summary?: string
  root_cause?: string
  status?: string
  created_at?: string
  conversations?: AIConversation[]
}

export interface RepeatRung {
  label: string
  key: 'normal' | 'attention' | 'repeat-low'
}
