import { useEffect, useRef, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Alert, App, Button, Drawer, Empty, Input, Modal, Popconfirm, Popover,
  Space, Spin, Tag, Tooltip, Typography, Divider, Card, Badge,
} from 'antd'
import {
  CheckCircleOutlined, CopyOutlined, DeleteOutlined, DownOutlined,
  EditOutlined, EyeOutlined, FileOutlined, FileTextOutlined,
  FolderOpenOutlined, FolderOutlined, PlusOutlined, ReloadOutlined,
  RightOutlined, SaveOutlined, SearchOutlined, SettingOutlined, DeploymentUnitOutlined,
  SendOutlined, LoadingOutlined, SyncOutlined,
} from '@ant-design/icons'
import { useTheme } from '../../hooks/useTheme'
import { useAuthStore } from '../../store/auth'
import { PageHeader } from '../../components/PageHeader'
import { SurfaceCard } from '../../components/SurfaceCard'
import { http } from '../../api/request'

const { Text, Paragraph } = Typography
const { TextArea } = Input

// ─── Types ────────────────────────────────────────────────────────────────────

interface NginxFile {
  name: string
  path: string
  content: string
  size: number
  mod_time: string
  is_dir: boolean
}

interface NginxServerEntry {
  ip: string
  label?: string
  port?: string
}

interface NginxServerGroup {
  id: number
  name: string
  env: string
  servers: NginxServerEntry[]
}

// ─── API ─────────────────────────────────────────────────────────────────────

const api = {
  listFiles:  (host: string, configPath: string) =>
    http.get<NginxFile[]>('/services/nginx/files', { params: { host, path: configPath } }),
  readFile:   (host: string, filePath: string) =>
    http.get<NginxFile>('/services/nginx/file', { params: { host, path: filePath } }),
  saveFile:   (host: string, filePath: string, content: string) =>
    http.put('/services/nginx/file', { host, path: filePath, content }),
  createFile: (host: string, filePath: string, content: string) =>
    http.post('/services/nginx/file', { host, path: filePath, content }),
  deleteFile: (host: string, filePath: string) =>
    http.delete('/services/nginx/file', { params: { host, path: filePath } }),
  reload:     (host: string) => http.post('/services/nginx/reload', { host }),
  test:       (host: string) => http.post<{ ok: boolean; output: string }>('/services/nginx/test', { host }),
  listServerGroups: () =>
    http.get<NginxServerGroup[]>('/services/nginx/server-groups'),
  updateServerGroup: (id: number, data: { name: string; servers: NginxServerEntry[] }) =>
    http.put(`/services/nginx/server-groups/${id}`, data),
}

// ─── localStorage ─────────────────────────────────────────────────────────────

const LS_KEY = 'nginx-config-settings'

interface Settings {
  host: string
  workDir: string
  configPath: string
}

function loadSettings(): Settings | null {
  try { const raw = localStorage.getItem(LS_KEY); return raw ? JSON.parse(raw) : null }
  catch { return null }
}
function saveSettings(s: Settings) { localStorage.setItem(LS_KEY, JSON.stringify(s)) }

// ─── Defaults ─────────────────────────────────────────────────────────────────

const DEFAULT_WORK_DIR = '/home/yzj/alertmesh/work'
const DEFAULT_CONFIG_PATH = '/usr/local/openresty/nginx/conf'

function getDefaultHost(): string {
  const h = window.location.hostname
  return (h && h !== 'localhost' && h !== '127.0.0.1') ? h : ''
}

// ─── DirNode ──────────────────────────────────────────────────────────────────

interface DirNodeProps {
  dirPath: string
  host: string
  depth: number
  expandedDirs: Set<string>
  toggleDir: (p: string) => void
  onViewFile: (f: NginxFile) => void
  onEditFile: (f: NginxFile) => void
  onDeleteFile: (f: NginxFile) => void
  onAddFile: (f: NginxFile) => void
  search: string
}

function DirNode({ dirPath, host, depth, expandedDirs, toggleDir, onViewFile, onEditFile, onDeleteFile, onAddFile, search }: DirNodeProps) {
  const { c } = useTheme()
  const isExpanded = expandedDirs.has(dirPath)

  const query = useQuery({
    queryKey: ['nginx-dir', host, dirPath],
    queryFn: () => api.listFiles(host, dirPath),
    enabled: isExpanded,
  })

  const rawItems = query.data ?? []
  const filtered = search
    ? rawItems.filter((f) => f.is_dir || f.name.toLowerCase().includes(search.toLowerCase()))
    : rawItems
  const sorted = [...filtered].sort((a, b) => {
    if (a.is_dir && !b.is_dir) return -1
    if (!a.is_dir && b.is_dir) return 1
    return a.name.localeCompare(b.name)
  })

  if (!isExpanded) return null

  return (
    <div style={{ marginLeft: depth * 20 }}>
      {query.isLoading ? (
        <div style={{ padding: '8px 0' }}><Spin size="small" /></div>
      ) : sorted.map((item) => (
        <div key={item.path} style={{ marginBottom: 2 }}>
          {item.is_dir ? (
            <div
              onClick={() => toggleDir(item.path)}
              style={{
                display: 'flex', alignItems: 'center', gap: 6,
                padding: '5px 8px', cursor: 'pointer', borderRadius: 4,
              }}
            >
              {expandedDirs.has(item.path)
                ? <DownOutlined style={{ fontSize: 10, color: c.textHint }} />
                : <RightOutlined style={{ fontSize: 10, color: c.textHint }} />
              }
              <FolderOutlined style={{ color: '#faad14', flexShrink: 0 }} />
              <span style={{ color: c.textBody, fontSize: 13 }}>{item.name}</span>
            </div>
          ) : (
            <div style={{
              display: 'flex', alignItems: 'flex-start', gap: 6,
              padding: '5px 8px', borderRadius: 4,
            }}>
              <span style={{ width: 10, flexShrink: 0, marginTop: 2 }} />
              <FileOutlined style={{ color: c.textHint, flexShrink: 0, marginTop: 2 }} />
              <span style={{
                fontSize: 13, color: c.textBody, flex: 1,
                wordBreak: 'break-all', lineHeight: '20px',
              }}>
                {item.name}
              </span>
              <Space size={0} style={{ flexShrink: 0, marginTop: 1 }}>
                <Tooltip title="查看内容">
                  <Button type="link" size="small" icon={<EyeOutlined />}
                    onClick={() => onViewFile(item)} />
                </Tooltip>
                <Tooltip title="编辑内容">
                  <Button type="link" size="small" icon={<EditOutlined />}
                    onClick={() => onEditFile(item)} />
                </Tooltip>
                <Tooltip title="加入下发">
                  <Button type="link" size="small" icon={<PlusOutlined />} style={{ color: c.primary }}
                    onClick={() => onAddFile(item)} />
                </Tooltip>
                <Popconfirm title="确认删除？" description={`删除 ${item.name}`}
                  onConfirm={() => onDeleteFile(item)}>
                  <Button type="link" size="small" danger icon={<DeleteOutlined />} />
                </Popconfirm>
              </Space>
            </div>
          )}
          {item.is_dir && (
            <DirNode
              dirPath={item.path} host={host} depth={depth + 1}
              expandedDirs={expandedDirs} toggleDir={toggleDir}
              onViewFile={onViewFile} onEditFile={onEditFile} onDeleteFile={onDeleteFile}
              onAddFile={onAddFile} search={search}
            />
          )}
        </div>
      ))}
    </div>
  )
}

// ─── Ansible 日志着色 ────────────────────────────────────────────────────────

function getAnsibleLineStyle(line: string): React.CSSProperties {
  const t = line.trimStart()
  if (/^TASK\s*\[/.test(t) || /^PLAY\s/.test(t) || /^PLAY RECAP/.test(t)) {
    return { color: '#4096ff', fontWeight: 600 }          // 任务标题 - 蓝色加粗
  }
  if (/^ok:/.test(t)) {
    return { color: '#52c41a' }                             // ok - 绿色
  }
  if (/^changed:/.test(t)) {
    return { color: '#fa8c16' }                             // changed - 橙色
  }
  if (/^skipping:/.test(t)) {
    return { color: '#8c8c8c' }                             // skipping - 灰色
  }
  if (/^(fatal|UNREACHABLE|ERROR|FAILED)/.test(t) || /\[ERROR\]/.test(t)) {
    return { color: '#ff4d4f', fontWeight: 500 }           // 错误 - 红色
  }
  if (/^(\*+|\d{4}-\d{2}|星期)/.test(t)) {
    return { color: '#8c8c8c' }                             // 分隔线、时间戳 - 灰色
  }
  return {}                                                 // 默认继承容器颜色
}

// ─── Main ────────────────────────────────────────────────────────

export default function NginxConfig() {
  const qc = useQueryClient()
  const { message } = App.useApp()
  const { c } = useTheme()

  // ── Settings ──
  const saved = loadSettings()
  const defaultHost = saved?.host || getDefaultHost()
  const defaultWorkDir = saved?.workDir || DEFAULT_WORK_DIR
  const defaultPath = saved?.configPath || DEFAULT_CONFIG_PATH

  const [committedHost, setCommittedHost] = useState(defaultHost)
  const [committedPath, setCommittedPath] = useState(defaultPath)

  const [host, setHost] = useState(defaultHost)
  const [workDir, setWorkDir] = useState(defaultWorkDir)
  const [configPath, setConfigPath] = useState(defaultPath)
  const [settingsEditing, setSettingsEditing] = useState(false)

  // Nginx 目标服务器：Drawer 状态
  const [serverDrawerGroup, setServerDrawerGroup] = useState<NginxServerGroup | null>(null)
  const [draftServers, setDraftServers] = useState<NginxServerEntry[]>([])

  // ── Tree state ──
  const [expandedDirs, setExpandedDirs] = useState<Set<string>>(new Set([committedPath]))
  const [search, setSearch] = useState('')
  // ── Search visible state ──
  const [searchVisible, setSearchVisible] = useState(false)

  // ── Modal state: view / edit ──
  const [viewFile, setViewFile] = useState<NginxFile | null>(null)
  const [editFile, setEditFile] = useState<NginxFile | null>(null)
  const [editContent, setEditContent] = useState('')
  const [pendingFiles, setPendingFiles] = useState<NginxFile[]>([])

  const [createOpen, setCreateOpen] = useState(false)
  const [newFileName, setNewFileName] = useState('')
  const [newFileContent, setNewFileContent] = useState('')
  const [testResult, setTestResult] = useState<{ ok: boolean; output: string } | null>(null)

  // ── 下发 state ──
  const [deployEnv, setDeployEnv] = useState<'prod' | 'gray' | null>(null)
  const [deployLogs, setDeployLogs] = useState<string[]>([])
  const [deployDone, setDeployDone] = useState(true)
  const [deployFailed, setDeployFailed] = useState(false)
  const [deployDryRun, setDeployDryRun] = useState(false)
  const logsEndRef = useRef<HTMLDivElement>(null)

  // ── 同步 state（日志复用下发日志区）──
  const [syncDone, setSyncDone] = useState(true)

  // ── Queries ──
  const rootQuery = useQuery({
    queryKey: ['nginx-dir', committedHost, committedPath],
    queryFn: () => api.listFiles(committedHost, committedPath),
    enabled: !!committedHost && !!committedPath,
  })

  const viewQuery = useQuery({
    queryKey: ['nginx-file', committedHost, viewFile?.path],
    queryFn: () => api.readFile(committedHost, viewFile!.path),
    enabled: !!viewFile && !!committedHost,
  })

  const editQuery = useQuery({
    queryKey: ['nginx-file', committedHost, editFile?.path],
    queryFn: () => api.readFile(committedHost, editFile!.path),
    enabled: !!editFile && !!committedHost,
  })

  useEffect(() => { if (editQuery.data) setEditContent(editQuery.data.content) }, [editQuery.data])

  // ── Directory toggle ──
  const toggleDir = (path: string) => {
    setExpandedDirs((prev) => {
      const next = new Set(prev)
      if (next.has(path)) next.delete(path); else next.add(path)
      return next
    })
  }

  // ── File actions ──
  const handleViewFile = (f: NginxFile) => { setEditFile(null); setViewFile(f) }
  const handleEditFile = (f: NginxFile) => { setViewFile(null); setEditFile(f); setEditContent('') }
  const handleDeleteFile = (f: NginxFile) => deleteFileMut.mutate(f.path)
  const handleAddFile = (f: NginxFile) => {
    setPendingFiles((prev) => prev.find((x) => x.path === f.path) ? prev : [...prev, f])
    message.success(`「${f.name}」已加入下发列表`)
  }

  // ── Settings mutations ──
  const handleSaveSettings = () => {
    saveSettings({ host, workDir, configPath })
    setCommittedHost(host); setCommittedPath(configPath)
    setExpandedDirs(new Set([configPath])); setViewFile(null); setEditFile(null)
    message.success('设置已保存'); setSettingsEditing(false)
  }
  const handleCancelSettings = () => {
    const s = loadSettings()
    setHost(s?.host || getDefaultHost()); setWorkDir(s?.workDir || DEFAULT_WORK_DIR)
    setConfigPath(s?.configPath || DEFAULT_CONFIG_PATH); setSettingsEditing(false)
  }

  // ── 下发 ──
  const handleDeploy = (env: 'prod' | 'gray', dryRun = false) => {
    if (deployDone === false) return // 正在执行中
    if (pendingFiles.length === 0) { message.warning('请先添加待下发文件'); return }
    setDeployEnv(env)
    setDeployLogs([])
    setDeployDone(false)
    setDeployFailed(false)
    setDeployDryRun(dryRun)

    const files = pendingFiles.map((f) => f.path)

    const token = useAuthStore.getState().token
    fetch('/api/v1/services/nginx/deploy', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}) },
      body: JSON.stringify({ env, files, dry_run: dryRun }),
    }).then((res) => {
      const reader = res.body?.getReader()
      if (!reader) { setDeployDone(true); return }
      const decoder = new TextDecoder()
      let buf = ''

      const pump = () => {
        reader.read().then(({ done, value }) => {
          if (done) { setDeployDone(true); return }
          buf += decoder.decode(value, { stream: true })
          const parts = buf.split('\n\n')
          buf = parts.pop() ?? ''
          parts.forEach((part) => {
            const lines = part.split('\n')
            for (const l of lines) {
              if (l.startsWith('event: done')) {
                setDeployDone(true)
              } else if (l.startsWith('data: ')) {
                const text = l.slice(6)
                // done 事件的 JSON 不输入日志，但解析 error
                try {
                  const evt = JSON.parse(text)
                  if ('exit_code' in evt) {
                    if (evt.exit_code !== 0) {
                      setDeployFailed(true)
                      message.error({
                        content: evt.error || 'Ansible 执行失败，请查看日志了解详情',
                        duration: 8,
                      })
                    }
                    return
                  }
                } catch { /* 不是 JSON，按普通日志处理 */ }
                setDeployLogs((prev) => [...prev, text])
                setTimeout(() => logsEndRef.current?.scrollIntoView({ behavior: 'smooth' }), 50)
              }
            }
          })
          pump()
        }).catch(() => setDeployDone(true))
      }
      pump()
    }).catch(() => {
      message.error({ content: '下发请求失败，请检查网络连接', duration: 8 })
      setDeployFailed(true)
      setDeployDone(true)
    })
  }

  const handleSync = (env: 'prod' | 'gray') => {
    if (syncDone === false) return // 正在执行中
    setSyncDone(false)
    // 同步日志复用下发日志区
    setDeployEnv(env)
    setDeployLogs([])
    setDeployDone(false)
    setDeployFailed(false)
    setDeployDryRun(false)

    const paths = [configPath]

    const syncToken = useAuthStore.getState().token
    fetch('/api/v1/services/nginx/sync', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', ...(syncToken ? { Authorization: `Bearer ${syncToken}` } : {}) },
      body: JSON.stringify({ env, paths }),
    }).then((res) => {
      const reader = res.body?.getReader()
      if (!reader) { setSyncDone(true); setDeployDone(true); return }
      const decoder = new TextDecoder()
      let buf = ''

      const pump = () => {
        reader.read().then(({ done, value }) => {
          if (done) { setSyncDone(true); setDeployDone(true); return }
          buf += decoder.decode(value, { stream: true })
          const parts = buf.split('\n\n')
          buf = parts.pop() ?? ''
          parts.forEach((part) => {
            const lines = part.split('\n')
            for (const l of lines) {
              if (l.startsWith('event: done')) {
                setSyncDone(true)
                setDeployDone(true)
              } else if (l.startsWith('data: ')) {
                const text = l.slice(6)
                try {
                  const evt = JSON.parse(text)
                  if ('exit_code' in evt) {
                    if (evt.exit_code !== 0) {
                      setDeployFailed(true)
                      message.error({ content: evt.error || 'Ansible 同步失败，请查看日志了解详情', duration: 8 })
                    } else {
                      message.success('同步完成，请刷新目录树查看文件')
                      qc.invalidateQueries({ queryKey: ['nginx-dir', committedHost] })
                    }
                    return
                  }
                } catch { /* 不是 JSON */ }
                setDeployLogs((prev) => [...prev, text])
                setTimeout(() => logsEndRef.current?.scrollIntoView({ behavior: 'smooth' }), 50)
              }
            }
          })
          pump()
        }).catch(() => { setSyncDone(true); setDeployDone(true) })
      }
      pump()
    }).catch(() => {
      message.error({ content: '同步请求失败，请检查网络连接', duration: 8 })
      setDeployFailed(true)
      setSyncDone(true)
      setDeployDone(true)
    })
  }

  // ── Mutations ──
  const saveFileMut = useMutation({
    mutationFn: () => api.saveFile(committedHost, editFile!.path, editContent),
    onSuccess: () => {
      message.success('文件已保存'); setEditFile(null)
      qc.invalidateQueries({ queryKey: ['nginx-dir', committedHost] })
    },
  })

  const createFileMut = useMutation({
    mutationFn: () => {
      const fullPath = committedPath.replace(/\/+$/, '') + '/' + newFileName.replace(/^\/+/, '')
      return api.createFile(committedHost, fullPath, newFileContent)
    },
    onSuccess: () => {
      message.success('文件已创建'); setCreateOpen(false)
      setNewFileName(''); setNewFileContent('')
      qc.invalidateQueries({ queryKey: ['nginx-dir', committedHost] })
    },
  })

  const deleteFileMut = useMutation({
    mutationFn: (filePath: string) => api.deleteFile(committedHost, filePath),
    onSuccess: () => {
      message.success('文件已删除'); setViewFile(null); setEditFile(null)
      qc.invalidateQueries({ queryKey: ['nginx-dir', committedHost] })
    },
  })

  const reloadMut = useMutation({
    mutationFn: () => api.reload(committedHost),
    onSuccess: () => message.success('Nginx 已重载'),
    onError: (e: Error) => message.error(e.message || '重载失败'),
  })

  const testMut = useMutation({
    mutationFn: () => api.test(committedHost),
    onSuccess: (r) => setTestResult(r),
    onError: (e: Error) => message.error(e.message || '检测失败'),
  })

  // ── Server groups ──
  const serverGroupsQuery = useQuery({
    queryKey: ['nginx-server-groups'],
    queryFn: () => api.listServerGroups(),
  })
  const serverGroups = serverGroupsQuery.data ?? []

  const updateServerGroupMut = useMutation({
    mutationFn: ({ id, name, servers }: { id: number; name: string; servers: NginxServerEntry[] }) =>
      api.updateServerGroup(id, { name, servers }),
    onSuccess: () => {
      message.success('服务器已保存')
      qc.invalidateQueries({ queryKey: ['nginx-server-groups'] })
      setServerDrawerGroup(null)
    },
  })

  // ── Modal styles ──
  const modalStyles = {
    header: { background: c.bgSurface, borderBottom: `1px solid ${c.border}`, color: c.textBody },
    body: { background: c.bgSurface, padding: '16px 24px' },
    footer: { background: c.bgSurface, borderTop: `1px solid ${c.border}` },
  }

  // ── Render ──────────────────────────────────────────────────────────────────

  return (
    <div>
      <PageHeader
        title="Nginx 配置" icon={<FileTextOutlined />}
        description="编辑发布机上的 Nginx / OpenResty 配置文件，并下发配置到「生产」「灰度」 nginx 服务器"
        extra={
          <Space>
          </Space>
        }
      />

      {/* 配置区：发布机 + Nginx目标服务器 */}
      <SurfaceCard style={{ marginBottom: 12 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12, flexWrap: 'wrap' }}>
          {/* 发布机 */}
          <Space size={6} align="center">
            <SettingOutlined style={{ color: c.primary }} />
            <Text strong style={{ color: c.textBody, fontSize: 13 }}>发布机</Text>
          </Space>
          {settingsEditing ? (
            <>
              <Input value={host} onChange={(e) => setHost(e.target.value)}
                placeholder="发布机 IP" size="small" style={{ width: 160 }} />
              <Text style={{ color: c.textHint, fontSize: 12 }}>配置路径</Text>
              <Input value={configPath} onChange={(e) => setConfigPath(e.target.value)}
                placeholder="/usr/local/openresty/nginx/conf" size="small" style={{ width: 300 }} />
              <Button size="small" type="primary" onClick={handleSaveSettings}>保存</Button>
              <Button size="small" onClick={handleCancelSettings}>取消</Button>
            </>
          ) : (
            <>
              <Tag color="blue" style={{ cursor: 'pointer' }}
                onClick={() => setSettingsEditing(true)}>{host || '未设置'}</Tag>
              <Text style={{ color: c.textHint, fontSize: 12 }}>配置路径</Text>
              <Tag style={{ cursor: 'pointer' }} onClick={() => setSettingsEditing(true)}>{configPath}</Tag>
              <Tooltip title="编辑发布机">
                <Button size="small" type="text" icon={<EditOutlined />}
                  onClick={() => setSettingsEditing(true)} />
              </Tooltip>
            </>
          )}

          {/* 弹性占位，将服务器区域推到右侧 */}
          <div style={{ flex: 1 }} />

          {/* Nginx 服务器：靠右显示 */}
          <Space size={6} align="center" style={{ flexShrink: 0 }}>
            <DeploymentUnitOutlined style={{ color: c.primary, fontSize: 12 }} />
            <Text strong style={{ color: c.textBody, fontSize: 13 }}>Nginx 服务器</Text>
            {serverGroupsQuery.isLoading ? (
              <Spin size="small" />
            ) : (
              serverGroups.map((g) => {
                const ipList = (
                  <div style={{ minWidth: 160, maxWidth: 260 }}>
                    <div style={{ fontWeight: 600, marginBottom: 6, fontSize: 12, color: c.textBody }}>
                      {g.name}服务器列表
                    </div>
                    {g.servers.length === 0 ? (
                      <div style={{ color: c.textHint, fontSize: 12 }}>暂未配置 IP</div>
                    ) : (
                      g.servers.map((s, i) => (
                        <div key={i} style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
                          <Text style={{ fontFamily: 'monospace', fontSize: 12, color: c.textBody }}>{s.ip}</Text>
                          {s.label && <Text style={{ fontSize: 11, color: c.textHint }}>{s.label}</Text>}
                        </div>
                      ))
                    )}
                    <Divider style={{ margin: '8px 0 6px', borderColor: c.border }} />
                    <Button size="small" type="link" icon={<EditOutlined />} style={{ padding: 0, height: 'auto' }}
                      onClick={() => { setServerDrawerGroup(g); setDraftServers([...g.servers]) }}>
                      编辑
                    </Button>
                  </div>
                )
                return (
                  <Tooltip key={g.id} title="点击查看 / 编辑服务器 IP">
                    <Popover content={ipList} trigger="click" placement="bottomRight">
                      <Tag
                        color={g.env === 'prod' ? 'green' : g.env === 'gray' ? 'orange' : 'default'}
                        style={{
                          cursor: 'pointer',
                          userSelect: 'none',
                          margin: 0,
                          borderStyle: 'dashed',
                        }}
                      >
                        {g.name}
                        <Text style={{ color: 'inherit', marginLeft: 4, opacity: 0.85, fontSize: 11 }}>
                          {g.servers.length}台
                        </Text>
                      </Tag>
                    </Popover>
                  </Tooltip>
                )
              })
            )}
          </Space>
          
          {/* 同步配置按钮 - 直接从生产同步 */}
          {serverGroups.some((g) => g.env === 'prod') && (
            <Tooltip title="从生产服务器拉取配置到发布机">
              <Button
                size="small"
                icon={<SyncOutlined spin={!syncDone} />}
                loading={!syncDone}
                onClick={() => handleSync('prod')}
                style={{ flexShrink: 0 }}
              >
                同步配置
              </Button>
            </Tooltip>
          )}
        </div>
      </SurfaceCard>

      {/* Server Group Drawer */}
      <Drawer
        title={
          <Space>
            <DeploymentUnitOutlined />
            <span>编辑{serverDrawerGroup?.name}服务器</span>
          </Space>
        }
        open={!!serverDrawerGroup}
        onClose={() => setServerDrawerGroup(null)}
        width={480}
        extra={
          <Space>
            <Button onClick={() => setServerDrawerGroup(null)}>取消</Button>
            <Button type="primary" loading={updateServerGroupMut.isPending}
              onClick={() => {
                if (serverDrawerGroup) {
                  updateServerGroupMut.mutate({
                    id: serverDrawerGroup.id,
                    name: serverDrawerGroup.name,
                    servers: draftServers,
                  })
                }
              }}>保存</Button>
          </Space>
        }
      >
        <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <Text style={{ color: c.textHint, fontSize: 12 }}>共 {draftServers.length} 个服务器</Text>
            <Button size="small" type="dashed" icon={<PlusOutlined />}
              onClick={() => setDraftServers([...draftServers, { ip: '', label: '' }])}>
              添加服务器
            </Button>
          </div>
          {draftServers.length === 0 ? (
            <Empty description="暂无服务器，点击「添加」开始" image={Empty.PRESENTED_IMAGE_SIMPLE} />
          ) : (
            draftServers.map((s, i) => (
              <div key={i} style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <Input
                  value={s.ip}
                  placeholder="IP 地址，如 192.168.1.10"
                  style={{ flex: 1 }}
                  onChange={(e) => {
                    const next = [...draftServers]
                    next[i] = { ...next[i], ip: e.target.value }
                    setDraftServers(next)
                  }}
                />
                <Tooltip title="删除">
                  <Button danger type="text" icon={<DeleteOutlined />}
                    onClick={() => setDraftServers(draftServers.filter((_, j) => j !== i))} />
                </Tooltip>
              </div>
            ))
          )}
        </div>
      </Drawer>

      {/* 语法检测结果 */}
      {testResult && (
        <Alert
          type={testResult.ok ? 'success' : 'error'} showIcon closable
          message={testResult.ok ? 'Nginx 配置语法检测通过' : 'Nginx 配置语法错误'}
          description={
            <pre style={{ margin: 0, fontSize: 12, whiteSpace: 'pre-wrap', fontFamily: 'monospace' }}>
              {testResult.output}
            </pre>
          }
          onClose={() => setTestResult(null)}
          style={{ marginBottom: 12 }}
        />
      )}

      {/* 主体：左侧目录树 | 右侧内容区 */}
      <div style={{ display: 'flex', gap: 12, alignItems: 'stretch' }}>
        {/* 左侧：目录树 */}
        <SurfaceCard style={{ width: 480, flexShrink: 0 }}
          title={
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, width: '100%' }}>
              <FolderOpenOutlined style={{ color: '#faad14', flexShrink: 0 }} />
              {!searchVisible && (
                <Text style={{ color: c.textBody, fontSize: 12, flex: 1,
                  overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
                  title={committedPath}>{committedPath}</Text>
              )}
              {searchVisible ? (
                <Input
                  placeholder="搜索文件名" autoFocus
                  prefix={<SearchOutlined style={{ color: c.textHint, fontSize: 11 }} />}
                  size="small" allowClear
                  style={{ flex: 1, minWidth: 0 }}
                  onChange={(e) => setSearch(e.target.value)}
                  onBlur={(e) => { if (!e.target.value) { setSearchVisible(false); setSearch('') } }}
                />
              ) : (
                <Tooltip title="搜索文件名">
                  <Button size="small" type="text" icon={<SearchOutlined />}
                    onClick={() => setSearchVisible(true)} />
                </Tooltip>
              )}
              <Tooltip title="刷新">
                <Button size="small" type="text" icon={<ReloadOutlined />}
                  onClick={() => qc.invalidateQueries({ queryKey: ['nginx-dir', committedHost] })} />
              </Tooltip>
              <Tooltip title="新建文件">
                <Button size="small" type="text" icon={<PlusOutlined />}
                  onClick={() => { setNewFileName(''); setNewFileContent(''); setCreateOpen(true) }} />
              </Tooltip>
            </div>
          }>
          <div style={{ height: 520, overflowY: 'auto' }}>
          {rootQuery.isLoading ? (
            <div style={{ textAlign: 'center', padding: 40 }}><Spin /></div>
          ) : (
            <DirNode
              dirPath={committedPath} host={committedHost} depth={0}
              expandedDirs={expandedDirs} toggleDir={toggleDir}
              onViewFile={handleViewFile} onEditFile={handleEditFile} onDeleteFile={handleDeleteFile}
              onAddFile={handleAddFile} search={search}
            />
          )}
          </div>
        </SurfaceCard>
        
        {/* 右侧：待下发 + 执行日志 */}
        <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 12, minHeight: 0 }}>
          {/* 待下发文件区 */}
          <SurfaceCard
            title={
              <Space size={4} align="center">
                <Text style={{ color: c.textBody, fontSize: 13 }}>待下发文件</Text>
                {pendingFiles.length > 0 && (
                  <Badge count={pendingFiles.length} size="small" />
                )}
              </Space>
            }
            extra={
              pendingFiles.length > 0 && (
                <Space size={6}>
                  <Tooltip title="模拟执行不实际改动，验证下发流程">
                    <Button
                      size="small"
                      icon={<SendOutlined />}
                      disabled={!deployDone}
                      loading={!deployDone && deployDryRun && deployEnv === 'gray'}
                      style={{ borderColor: '#8c8c8c', color: '#8c8c8c', backgroundColor: 'transparent' }}
                      onClick={() => handleDeploy('gray', true)}
                    >
                      灰度预检
                    </Button>
                  </Tooltip>
                  <Tooltip title="模拟执行不实际改动，验证下发流程">
                    <Button
                      size="small"
                      icon={<SendOutlined />}
                      disabled={!deployDone}
                      loading={!deployDone && deployDryRun && deployEnv === 'prod'}
                      style={{ borderColor: '#8c8c8c', color: '#8c8c8c', backgroundColor: 'transparent' }}
                      onClick={() => handleDeploy('prod', true)}
                    >
                      生产预检
                    </Button>
                  </Tooltip>
                  <Tooltip title="下发到灰度服务器">
                    <Button
                      size="small"
                      icon={<SendOutlined />}
                      disabled={!deployDone}
                      loading={!deployDone && !deployDryRun && deployEnv === 'gray'}
                      style={{ borderColor: '#fa8c16', color: '#fa8c16', backgroundColor: 'transparent' }}
                      onClick={() => handleDeploy('gray')}
                    >
                      灰度下发
                    </Button>
                  </Tooltip>
                  <Tooltip title="下发到生产服务器">
                    <Button
                      size="small"
                      type="primary"
                      icon={<SendOutlined />}
                      disabled={!deployDone}
                      loading={!deployDone && !deployDryRun && deployEnv === 'prod'}
                      onClick={() => handleDeploy('prod')}
                    >
                      生产下发
                    </Button>
                  </Tooltip>
                </Space>
              )
            }
            style={{ flexShrink: 0 }}
          >
            {pendingFiles.length === 0 ? (
              <div style={{ color: c.textHint, fontSize: 12, padding: '8px 0', textAlign: 'center' }}>
                点击左侧目录树中的 + 将文件加入下发列表
              </div>
            ) : (
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                {pendingFiles.map((f) => (
                  <Tag
                    key={f.path}
                    closable
                    onClose={() => setPendingFiles((prev) => prev.filter((x) => x.path !== f.path))}
                    icon={<FileOutlined />}
                    style={{ fontSize: 12, padding: '2px 8px' }}
                  >
                    {f.name}
                  </Tag>
                ))}
              </div>
            )}
          </SurfaceCard>

          {/* 执行日志区 */}
          <SurfaceCard style={{ flex: 1, display: 'flex', flexDirection: 'column', minHeight: 0 }}
            title={
              <Space size={6} align="center">
                <Text style={{ color: c.textBody, fontSize: 13 }}>执行日志</Text>
                {!deployDone && <Spin indicator={<LoadingOutlined spin />} size="small" />}
                {!deployDone && deployDryRun && (
                  <Tag color="default" style={{ fontSize: 11, margin: 0 }}>dry-run</Tag>
                )}
                {deployDone && deployLogs.length > 0 && !deployFailed && deployDryRun && (
                  <Tag color="blue" style={{ fontSize: 11, margin: 0 }}>dry-run 完成</Tag>
                )}
                {deployDone && deployLogs.length > 0 && !deployFailed && !deployDryRun && (
                  <Tag color="green" style={{ fontSize: 11, margin: 0 }}>已完成</Tag>
                )}
                {deployDone && deployFailed && (
                  <Tag color="red" style={{ fontSize: 11, margin: 0 }}>失败</Tag>
                )}
              </Space>
            }
            extra={
              deployLogs.length > 0 && (
                <Button size="small" type="text" onClick={() => setDeployLogs([])}>清空</Button>
              )
            }
          >
            <div style={{
              flex: 1,
              overflowY: 'auto',
              fontFamily: 'monospace',
              fontSize: 12,
              lineHeight: 1.7,
              color: c.textBody,
              background: c.bgInput,
              borderRadius: 4,
              padding: '8px 12px',
              minHeight: 120,
              maxHeight: 360,
            }}>
              {deployLogs.length === 0 ? (
                <span style={{ color: c.textHint }}>执行日志将在此显示</span>
              ) : (
                deployLogs.map((line, i) => (
                  <div key={i} style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-all', ...getAnsibleLineStyle(line) }}>{line}</div>
                ))
              )}
              <div ref={logsEndRef} />
            </div>
          </SurfaceCard>


        </div>
      </div>

      {/* 查看内容 Modal */}
      <Modal
        title={
          <Space>
            <EyeOutlined style={{ color: c.primary }} />
            <Text style={{ color: c.textBody }}>{viewFile?.name}</Text>
            {viewFile && (
              <Tag style={{ fontSize: 10 }}>
                {viewFile.size < 1024 ? `${viewFile.size} B` : `${(viewFile.size / 1024).toFixed(1)} KB`}
              </Tag>
            )}
          </Space>
        }
        open={!!viewFile} onCancel={() => setViewFile(null)}
        footer={[
          <Button key="copy" icon={<CopyOutlined />}
            onClick={() => {
              const text = viewQuery.data?.content || ''
              if (navigator.clipboard && window.isSecureContext) {
                navigator.clipboard.writeText(text).then(() => message.success('已复制'))
              } else {
                const ta = document.createElement('textarea')
                ta.value = text
                ta.style.cssText = 'position:fixed;opacity:0;top:0;left:0'
                document.body.appendChild(ta)
                ta.focus(); ta.select()
                document.execCommand('copy')
                document.body.removeChild(ta)
                message.success('已复制')
              }
            }}>
            复制
          </Button>,
          <Button key="close" onClick={() => setViewFile(null)}>关闭</Button>,
        ]}
        width={800} styles={modalStyles}
      >
        {viewQuery.isLoading ? (
          <div style={{ textAlign: 'center', padding: 40 }}><Spin /></div>
        ) : (
          <pre style={{
            fontFamily: 'ui-monospace, Menlo, monospace', fontSize: 12, lineHeight: 1.6,
            margin: 0, whiteSpace: 'pre-wrap', wordBreak: 'break-all',
            color: c.textBody, maxHeight: 600, overflow: 'auto',
            background: c.bgInput, padding: 12, borderRadius: 6,
          }}>
            {viewQuery.data?.content}
          </pre>
        )}
      </Modal>

      {/* 编辑内容 Modal */}
      <Modal
        title={
          <Space>
            <EditOutlined style={{ color: c.primary }} />
            <Text style={{ color: c.textBody }}>{editFile?.name}</Text>
          </Space>
        }
        open={!!editFile}
        onCancel={() => setEditFile(null)}
        footer={[
          <Button key="cancel" onClick={() => setEditFile(null)}>取消</Button>,
          <Button key="save" type="primary" icon={<SaveOutlined />}
            onClick={() => saveFileMut.mutate()} loading={saveFileMut.isPending}>保存</Button>,
        ]}
        width={800} styles={modalStyles}
      >
        {editQuery.isLoading ? (
          <div style={{ textAlign: 'center', padding: 40 }}><Spin /></div>
        ) : (
          <TextArea
            value={editContent}
            onChange={(e) => setEditContent(e.target.value)}
            rows={28}
            style={{
              fontFamily: 'ui-monospace, Menlo, monospace', fontSize: 12, lineHeight: 1.6,
              background: c.bgInput, color: c.textBody, border: `1px solid ${c.borderInput}`,
            }}
          />
        )}
      </Modal>

      {/* 新建文件 Modal */}
      <Modal
        title="新建配置文件" open={createOpen}
        onCancel={() => setCreateOpen(false)} onOk={() => createFileMut.mutate()}
        okText="创建" confirmLoading={createFileMut.isPending} width={720}
        styles={modalStyles}
      >
        <div style={{ marginBottom: 12 }}>
          <Text style={{ color: c.textBody, fontSize: 13 }}>文件名</Text>
          <Input value={newFileName} onChange={(e) => setNewFileName(e.target.value)}
            placeholder="例如：api.example.com.conf" style={{ marginTop: 4 }}
            addonBefore={`${committedPath.replace(/\/+$/, '')}/`} />
        </div>
        <Divider style={{ borderColor: c.border, margin: '12px 0 16px' }} />
        <div>
          <Text style={{ color: c.textBody, fontSize: 13 }}>文件内容</Text>
          <TextArea value={newFileContent} onChange={(e) => setNewFileContent(e.target.value)} rows={16}
            style={{
              fontFamily: 'ui-monospace, Menlo, monospace', fontSize: 12, marginTop: 4,
              background: c.bgInput, color: c.textBody,
            }}
            placeholder="粘贴或编写 Nginx 配置..." />
        </div>
      </Modal>

      {/* 说明 */}
      <Card size="small"
        style={{ background: c.bgSurface, border: `1px solid ${c.borderSubtle}`, borderRadius: 8, marginTop: 12 }}>
        <Paragraph style={{ margin: 0, color: c.textHint, fontSize: 12, lineHeight: '22px' }}>
          <Text strong style={{ color: c.textBody }}>操作说明：</Text>
          &nbsp;执行下发动作前建议先点击「预检」进行 dry-run 验证下发流程，确认无误后再进行下发，避免重载后服务中断。
        </Paragraph>
      </Card>
    </div>
  )
}
