package router

import (
	"bufio"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	restful "github.com/emicklei/go-restful/v3"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/kuzane/alertmesh/internal/config"
	"github.com/kuzane/alertmesh/internal/httputil"
	"github.com/kuzane/alertmesh/internal/label"
)

// ansiEscape 匹配所有 ANSI 转义序列，用于在输出日志前过滤掉颜色码
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[mGKHF]`)

func stripANSI(s string) string { return ansiEscape.ReplaceAllString(s, "") }

// ─── Types ──────────────────────────────────────────────────────────────────────

type nginxFile struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Content string `json:"content,omitempty"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"`
	IsDir   bool   `json:"is_dir"`
}

type nginxFileBody struct {
	Host    string `json:"host"`
	Path    string `json:"path"`
	Content string `json:"content"`
}

// NginxServerEntry 是 JSONB 字段里每个服务器的一条记录
type NginxServerEntry struct {
	IP    string `json:"ip"`
	Label string `json:"label,omitempty"`
	Port  string `json:"port,omitempty"`
}

type NginxServerEntries []NginxServerEntry

func (n NginxServerEntries) Value() (driver.Value, error) {
	b, err := json.Marshal(n)
	return string(b), err
}
func (n *NginxServerEntries) Scan(src any) error {
	switch v := src.(type) {
	case []byte:
		return json.Unmarshal(v, n)
	case string:
		return json.Unmarshal([]byte(v), n)
	}
	return fmt.Errorf("unsupported type: %T", src)
}

// NginxServerGroup 对应 nginx_server_groups 表
type NginxServerGroup struct {
	ID        uint64             `gorm:"primaryKey;autoIncrement" json:"id"`
	Name      string             `gorm:"column:name"              json:"name"`
	Env       string             `gorm:"column:env"               json:"env"`
	Servers   NginxServerEntries `gorm:"column:servers;type:jsonb" json:"servers"`
	CreatedAt time.Time          `gorm:"column:created_at"        json:"created_at"`
	UpdatedAt time.Time          `gorm:"column:updated_at"        json:"updated_at"`
}

func (NginxServerGroup) TableName() string { return "nginx_server_groups" }

type nginxServerGroupBody struct {
	Name    string             `json:"name"`
	Servers NginxServerEntries `json:"servers"`
}

// ─── Handler ──────────────────────────────────────────────────────────────────────

type nginxHandler struct {
	workDir string
	db      *gorm.DB
	cfg     *config.Config
}

func newNginxHandler(workDir string, db *gorm.DB, cfg *config.Config) *nginxHandler {
	return &nginxHandler{workDir: workDir, db: db, cfg: cfg}
}

func (h *nginxHandler) registerRoutes(ws *restful.WebService) {
	ws.Route(ws.GET("/services/nginx/files").
		To(h.listFiles).
		Doc("List Nginx config files under the given path").
		Metadata(label.MetaIdentity, label.OpsAccess).
		Metadata(label.MetaModule, label.OpsModuleName).
		Metadata(label.MetaKind, "Nginx").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.GET("/services/nginx/file").
		To(h.readFile).
		Doc("Read a single Nginx config file").
		Metadata(label.MetaIdentity, label.OpsAccess).
		Metadata(label.MetaModule, label.OpsModuleName).
		Metadata(label.MetaKind, "Nginx").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.POST("/services/nginx/file").
		To(h.createFile).
		Doc("Create a new Nginx config file").
		Metadata(label.MetaIdentity, label.OpsAccess).
		Metadata(label.MetaModule, label.OpsModuleName).
		Metadata(label.MetaKind, "Nginx").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.PUT("/services/nginx/file").
		To(h.saveFile).
		Doc("Save (overwrite) a Nginx config file").
		Metadata(label.MetaIdentity, label.OpsAccess).
		Metadata(label.MetaModule, label.OpsModuleName).
		Metadata(label.MetaKind, "Nginx").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.DELETE("/services/nginx/file").
		To(h.deleteFile).
		Doc("Delete a Nginx config file").
		Metadata(label.MetaIdentity, label.OpsAccess).
		Metadata(label.MetaModule, label.OpsModuleName).
		Metadata(label.MetaKind, "Nginx").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.POST("/services/nginx/test").
		To(h.testConfig).
		Doc("Test Nginx configuration syntax").
		Metadata(label.MetaIdentity, label.OpsAccess).
		Metadata(label.MetaModule, label.OpsModuleName).
		Metadata(label.MetaKind, "Nginx").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.POST("/services/nginx/reload").
		To(h.reload).
		Doc("Reload Nginx configuration").
		Metadata(label.MetaIdentity, label.OpsAccess).
		Metadata(label.MetaModule, label.OpsModuleName).
		Metadata(label.MetaKind, "Nginx").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	// 服务器分组 CRUD
	ws.Route(ws.GET("/services/nginx/server-groups").
		To(h.listServerGroups).
		Doc("List all Nginx server groups").
		Metadata(label.MetaIdentity, label.OpsAccess).
		Metadata(label.MetaModule, label.OpsModuleName).
		Metadata(label.MetaKind, "Nginx").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.PUT("/services/nginx/server-groups/{id}").
		To(h.updateServerGroup).
		Doc("Update a Nginx server group").
		Metadata(label.MetaIdentity, label.OpsAccess).
		Metadata(label.MetaModule, label.OpsModuleName).
		Metadata(label.MetaKind, "Nginx").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	// 下发配置（SSE 流式日志）
	ws.Route(ws.POST("/services/nginx/deploy").
		To(h.deployConfig).
		Doc("Deploy Nginx config files to target servers via Ansible").
		Metadata(label.MetaIdentity, label.OpsAccess).
		Metadata(label.MetaModule, label.OpsModuleName).
		Metadata(label.MetaKind, "Nginx").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	// 从目标服务器同步配置到 work 目录（SSE 流式日志）
	ws.Route(ws.POST("/services/nginx/sync").
		To(h.syncConfig).
		Doc("Sync Nginx config files from target servers to local work directory").
		Metadata(label.MetaIdentity, label.OpsAccess).
		Metadata(label.MetaModule, label.OpsModuleName).
		Metadata(label.MetaKind, "Nginx").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))
}

// resolvePath joins workDir with the request path.
// It also validates that the resolved path stays within workDir (path traversal guard).
func (h *nginxHandler) resolvePath(relPath string) (string, error) {
	clean := filepath.Clean(relPath)
	// Must be an absolute-looking relative path, e.g. /usr/local/openresty/nginx/conf
	full := filepath.Join(h.workDir, clean)
	// Security: ensure resolved path is under workDir
	absWork, _ := filepath.Abs(h.workDir)
	absFull, _ := filepath.Abs(full)
	if !strings.HasPrefix(absFull, absWork) {
		return "", fmt.Errorf("path escapes work directory: %s", relPath)
	}
	return full, nil
}

// ─── Handlers ──────────────────────────────────────────────────────────────────────

func (h *nginxHandler) listFiles(req *restful.Request, resp *restful.Response) {
	_ = req.QueryParameter("host") // host is informational; backend always reads locally

	relPath := req.QueryParameter("path")
	if relPath == "" {
		httputil.BadRequest(resp, "missing path parameter")
		return
	}

	dirPath, err := h.resolvePath(relPath)
	if err != nil {
		httputil.BadRequest(resp, err.Error())
		return
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			httputil.Success(resp, []nginxFile{})
			return
		}
		httputil.InternalError(resp, fmt.Sprintf("read dir: %s", err))
		return
	}

	var files []nginxFile
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		fullPath := filepath.Join(dirPath, e.Name())
		// Strip workDir prefix so the client sends back a relative path
		relPath, err := filepath.Rel(h.workDir, fullPath)
		if err != nil {
			relPath = fullPath // fallback
		}
		// Ensure relative path starts with / so resolvePath can re-join correctly
		if !strings.HasPrefix(relPath, "/") {
			relPath = "/" + relPath
		}
		files = append(files, nginxFile{
			Name:    e.Name(),
			Path:    relPath,
			Size:    info.Size(),
			ModTime: info.ModTime().Format(time.RFC3339),
			IsDir:   e.IsDir(),
		})
	}

	if files == nil {
		files = []nginxFile{}
	}
	httputil.Success(resp, files)
}

func (h *nginxHandler) readFile(req *restful.Request, resp *restful.Response) {
	_ = req.QueryParameter("host")

	relPath := req.QueryParameter("path")
	if relPath == "" {
		httputil.BadRequest(resp, "missing path parameter")
		return
	}

	filePath, err := h.resolvePath(relPath)
	if err != nil {
		httputil.BadRequest(resp, err.Error())
		return
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			httputil.NotFound(resp)
			return
		}
		httputil.InternalError(resp, fmt.Sprintf("read file: %s", err))
		return
	}

	stat, _ := os.Stat(filePath)
	modTime := ""
	if stat != nil {
		modTime = stat.ModTime().Format(time.RFC3339)
	}

	httputil.Success(resp, nginxFile{
		Name:    filepath.Base(filePath),
		Path:    filePath,
		Content: string(data),
		Size:    int64(len(data)),
		ModTime: modTime,
	})
}

func (h *nginxHandler) createFile(req *restful.Request, resp *restful.Response) {
	var body nginxFileBody
	if err := req.ReadEntity(&body); err != nil {
		httputil.BadRequest(resp, "invalid request body")
		return
	}
	if body.Path == "" {
		httputil.BadRequest(resp, "missing path")
		return
	}

	filePath, err := h.resolvePath(body.Path)
	if err != nil {
		httputil.BadRequest(resp, err.Error())
		return
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		httputil.InternalError(resp, fmt.Sprintf("create dir: %s", err))
		return
	}

	if err := os.WriteFile(filePath, []byte(body.Content), 0644); err != nil {
		httputil.InternalError(resp, fmt.Sprintf("write file: %s", err))
		return
	}

	stat, _ := os.Stat(filePath)
	modTime := ""
	if stat != nil {
		modTime = stat.ModTime().Format(time.RFC3339)
	}

	httputil.Created(resp, nginxFile{
		Name:    filepath.Base(filePath),
		Path:    filePath,
		Content: body.Content,
		Size:    int64(len(body.Content)),
		ModTime: modTime,
	})
}

func (h *nginxHandler) saveFile(req *restful.Request, resp *restful.Response) {
	var body nginxFileBody
	if err := req.ReadEntity(&body); err != nil {
		httputil.BadRequest(resp, "invalid request body")
		return
	}
	if body.Path == "" {
		httputil.BadRequest(resp, "missing path")
		return
	}

	filePath, err := h.resolvePath(body.Path)
	if err != nil {
		httputil.BadRequest(resp, err.Error())
		return
	}

	if err := os.WriteFile(filePath, []byte(body.Content), 0644); err != nil {
		httputil.InternalError(resp, fmt.Sprintf("write file: %s", err))
		return
	}

	stat, _ := os.Stat(filePath)
	modTime := ""
	if stat != nil {
		modTime = stat.ModTime().Format(time.RFC3339)
	}

	httputil.Success(resp, nginxFile{
		Name:    filepath.Base(filePath),
		Path:    filePath,
		Content: body.Content,
		Size:    int64(len(body.Content)),
		ModTime: modTime,
	})
}

func (h *nginxHandler) deleteFile(req *restful.Request, resp *restful.Response) {
	_ = req.QueryParameter("host")

	relPath := req.QueryParameter("path")
	if relPath == "" {
		httputil.BadRequest(resp, "missing path parameter")
		return
	}

	filePath, err := h.resolvePath(relPath)
	if err != nil {
		httputil.BadRequest(resp, err.Error())
		return
	}

	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			httputil.NotFound(resp)
			return
		}
		httputil.InternalError(resp, fmt.Sprintf("delete file: %s", err))
		return
	}

	httputil.Success(resp, map[string]string{"deleted": filepath.Base(filePath)})
}

type nginxTestResult struct {
	OK     bool   `json:"ok"`
	Output string `json:"output"`
}

func (h *nginxHandler) testConfig(req *restful.Request, resp *restful.Response) {
	// Run nginx -t on the host
	cmd := exec.Command("sudo", "/usr/local/openresty/nginx/sbin/nginx", "-t")
	out, err := cmd.CombinedOutput()
	result := nginxTestResult{
		Output: string(out),
	}
	if err != nil {
		result.OK = false
		// Include error message if not already in output
		if !strings.Contains(result.Output, "test failed") && !strings.Contains(result.Output, "syntax is not ok") {
			result.Output += "\n" + err.Error()
		}
	} else {
		result.OK = true
	}
	httputil.Success(resp, result)
}

func (h *nginxHandler) reload(req *restful.Request, resp *restful.Response) {
	cmd := exec.Command("sudo", "/usr/local/openresty/nginx/sbin/nginx", "-s", "reload")
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Error().Err(err).Str("output", string(out)).Msg("nginx reload failed")
		httputil.InternalError(resp, fmt.Sprintf("reload failed: %s", string(out)))
		return
	}
	httputil.Success(resp, map[string]string{"status": "reloaded"})
}

// ─── Server Group Handlers ────────────────────────────────────────────────────

func (h *nginxHandler) listServerGroups(_ *restful.Request, resp *restful.Response) {
	var groups []NginxServerGroup
	if err := h.db.Order("id asc").Find(&groups).Error; err != nil {
		httputil.InternalError(resp, fmt.Sprintf("list server groups: %s", err))
		return
	}
	if groups == nil {
		groups = []NginxServerGroup{}
	}
	httputil.Success(resp, groups)
}

func (h *nginxHandler) updateServerGroup(req *restful.Request, resp *restful.Response) {
	id := req.PathParameter("id")
	var body nginxServerGroupBody
	if err := req.ReadEntity(&body); err != nil {
		httputil.BadRequest(resp, "invalid request body")
		return
	}

	servers := body.Servers
	if servers == nil {
		servers = NginxServerEntries{}
	}

	result := h.db.Model(&NginxServerGroup{}).Where("id = ?", id).
		Updates(map[string]any{
			"name":       body.Name,
			"servers":    servers,
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		httputil.InternalError(resp, fmt.Sprintf("update server group: %s", result.Error))
		return
	}
	if result.RowsAffected == 0 {
		httputil.NotFound(resp)
		return
	}

	var updated NginxServerGroup
	h.db.First(&updated, "id = ?", id)
	httputil.Success(resp, updated)
}

// ─── Deploy (Ansible SSE) ───────────────────────────────────────────────────────

type deployRequest struct {
	Env     string   `json:"env"`      // "prod" | "gray"
	Files   []string `json:"files"`    // 相对于 workDir 的文件路径列表
	DryRun  bool     `json:"dry_run"`  // true 时追加 --check 不实际执行
}

// deployConfig 以 SSE 将 ansible-playbook 执行日志实时推送给前端。
func (h *nginxHandler) deployConfig(req *restful.Request, resp *restful.Response) {
	var body deployRequest
	if err := req.ReadEntity(&body); err != nil {
		httputil.BadRequest(resp, "invalid request body")
		return
	}

	// 查询对应环境的服务器分组
	var group NginxServerGroup
	if err := h.db.Where("env = ?", body.Env).First(&group).Error; err != nil {
		httputil.NotFound(resp)
		return
	}
	if len(group.Servers) == 0 {
		httputil.BadRequest(resp, "no servers configured for this environment")
		return
	}

	// 构建 hosts 列表（逗号分隔）
	hostIPs := make([]string, 0, len(group.Servers))
	for _, s := range group.Servers {
		ip := strings.TrimSpace(s.IP)
		if ip != "" {
			hostIPs = append(hostIPs, ip)
		}
	}
	if len(hostIPs) == 0 {
		httputil.BadRequest(resp, "no valid IPs in server group")
		return
	}
	hostsList := strings.Join(hostIPs, ",")

	// 构建 src_files JSON：[ { "src": "/work/...", "dest": "/etc/nginx/..." }, ... ]
	type fileItem struct {
		Src  string `json:"src"`
		Dest string `json:"dest"`
	}
	var fileItems []fileItem
	for _, f := range body.Files {
		// f 是 listFiles 返回的相对路径，如 /usr/local/openresty/nginx/conf/vhost/b.conf
		// src = workDir + f（发布机上 work 目录中的副本）
		// dest = f 本身（目标服务器上的真实路径）
		clean := filepath.Clean(f)
		if !strings.HasPrefix(clean, "/") {
			clean = "/" + clean
		}
		fileItems = append(fileItems, fileItem{
			Src:  filepath.Join(h.workDir, clean),
			Dest: clean,
		})
	}
	srcFilesJSON, _ := json.Marshal(fileItems)

	// playbook 模板路径
	playbookPath := filepath.Join(h.workDir, "templates", "deploy_nginx.yml")

	// 构建 --extra-vars：包含 hosts、文件列表、SSH 账号密码、nginx 路径、备份时间戳
	backupTS := time.Now().Format("20060102_150405")
	extraVars := fmt.Sprintf(`hosts_list=%s src_files=%s backup_ts=%s`, hostsList, string(srcFilesJSON), backupTS)
	if h.cfg.AnsibleUser != "" {
		extraVars += fmt.Sprintf(` ansible_user=%s`, h.cfg.AnsibleUser)
	}
	if h.cfg.AnsiblePassword != "" {
		extraVars += fmt.Sprintf(` ansible_password=%s ansible_become_password=%s`, h.cfg.AnsiblePassword, h.cfg.AnsiblePassword)
	}
	if h.cfg.AnsibleNginxBin != "" {
		extraVars += fmt.Sprintf(` nginx_bin=%s`, h.cfg.AnsibleNginxBin)
	}

	// 构建 ansible-playbook 命令
	// --inventory 使用动态 hosts，格式: "ip1,ip2,"（尾部加逗号防止单渡时被解析为文件）
	cmdArgs := []string{
		playbookPath,
		"--inventory", hostsList + ",",
		"--extra-vars", extraVars,
		"-v",
	}
	if body.DryRun {
		cmdArgs = append(cmdArgs, "--check")
	}
	// 查找 ansible-playbook 可执行文件（支持 pip --user 安装的路径）
	ansibleBin, err := exec.LookPath("ansible-playbook")
	if err != nil {
		// LookPath 找不到，尝试常见的默认路径
		for _, candidate := range []string{
			"/home/yzj/.local/bin/ansible-playbook",
			"/usr/local/bin/ansible-playbook",
			"/usr/bin/ansible-playbook",
		} {
			if _, statErr := os.Stat(candidate); statErr == nil {
				ansibleBin = candidate
				break
			}
		}
	}
	if ansibleBin == "" {
		httputil.InternalError(resp, "ansible-playbook not found; please install ansible")
		return
	}
	cmd := exec.Command(ansibleBin, cmdArgs...) //nolint:gosec
	cmd.Env = append(os.Environ(), "ANSIBLE_FORCE_COLOR=1", "PYTHONUNBUFFERED=1", "ANSIBLE_HOST_KEY_CHECKING=False")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		httputil.InternalError(resp, "pipe error")
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		httputil.InternalError(resp, "pipe error")
		return
	}
	if err := cmd.Start(); err != nil {
		httputil.InternalError(resp, fmt.Sprintf("start ansible-playbook: %s", err))
		return
	}

	log.Info().Str("env", body.Env).Strs("hosts", hostIPs).Msg("ansible deploy started")

	// 切换为 SSE 响应
	w := resp.ResponseWriter
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, canFlush := w.(http.Flusher)

	sendLine := func(line string) {
		fmt.Fprintf(w, "data: %s\n\n", stripANSI(line))
		if canFlush {
			flusher.Flush()
		}
	}

	// 合并 stdout / stderr 输出
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			sendLine(scanner.Text())
		}
	}()

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		sendLine(scanner.Text())
	}

	_ = cmd.Wait()

	// 发送结束事件，附带 exit_code 和错误摘要
	exitCode := 0
	var errSummary string
	if cmd.ProcessState != nil && !cmd.ProcessState.Success() {
		exitCode = cmd.ProcessState.ExitCode()
		errSummary = "ansible-playbook 执行失败，请查看下方日志了解详情"
	}
	donePayload, _ := json.Marshal(map[string]any{
		"exit_code": exitCode,
		"error":     errSummary,
	})
	fmt.Fprintf(w, "event: done\ndata: %s\n\n", donePayload)
	if canFlush {
		flusher.Flush()
	}
	log.Info().Str("env", body.Env).Int("exit_code", exitCode).Msg("ansible deploy finished")
}

// ─── Sync (Ansible SSE) ──────────────────────────────────────────────

type syncRequest struct {
	Env   string   `json:"env"`    // "prod" | "gray"
	Paths []string `json:"paths"`  // 要同步的远程路径，如 ["/etc/nginx/nginx.conf"]
}

// syncConfig 从目标服务器拉取配置文件到发布机 work 目录，以 SSE 实时推送日志。
func (h *nginxHandler) syncConfig(req *restful.Request, resp *restful.Response) {
	var body syncRequest
	if err := req.ReadEntity(&body); err != nil {
		httputil.BadRequest(resp, "invalid request body")
		return
	}
	if len(body.Paths) == 0 {
		httputil.BadRequest(resp, "paths is required")
		return
	}

	// 查询对应环境的服务器分组
	var group NginxServerGroup
	if err := h.db.Where("env = ?", body.Env).First(&group).Error; err != nil {
		httputil.NotFound(resp)
		return
	}
	if len(group.Servers) == 0 {
		httputil.BadRequest(resp, "no servers configured for this environment")
		return
	}

	// 只取第一台服务器作为同步源
	sourceIP := strings.TrimSpace(group.Servers[0].IP)
	if sourceIP == "" {
		httputil.BadRequest(resp, "no valid IP in server group")
		return
	}

	// 构建 dest_files JSON
	type destItem struct {
		RemotePath string `json:"remote_path"`
	}
	destItems := make([]destItem, 0, len(body.Paths))
	for _, p := range body.Paths {
		clean := filepath.Clean(p)
		if !strings.HasPrefix(clean, "/") {
			clean = "/" + clean
		}
		destItems = append(destItems, destItem{RemotePath: clean})
	}
	destFilesJSON, _ := json.Marshal(destItems)

	// playbook 模板路径
	playbookPath := filepath.Join(h.workDir, "templates", "sync_nginx.yml")

	// --extra-vars
	extraVars := fmt.Sprintf(`hosts_list=%s dest_files=%s local_dir=%s`,
		sourceIP, string(destFilesJSON), h.workDir)
	if h.cfg.AnsibleUser != "" {
		extraVars += fmt.Sprintf(` ansible_user=%s`, h.cfg.AnsibleUser)
	}
	if h.cfg.AnsiblePassword != "" {
		extraVars += fmt.Sprintf(` ansible_password=%s`, h.cfg.AnsiblePassword)
	}

	cmdArgs := []string{
		playbookPath,
		"--inventory", sourceIP + ",",
		"--extra-vars", extraVars,
		"-v",
	}

	// 查找 ansible-playbook
	ansibleBin, err := exec.LookPath("ansible-playbook")
	if err != nil {
		for _, candidate := range []string{
			"/home/yzj/.local/bin/ansible-playbook",
			"/usr/local/bin/ansible-playbook",
			"/usr/bin/ansible-playbook",
		} {
			if _, statErr := os.Stat(candidate); statErr == nil {
				ansibleBin = candidate
				break
			}
		}
	}
	if ansibleBin == "" {
		httputil.InternalError(resp, "ansible-playbook not found")
		return
	}

	cmd := exec.Command(ansibleBin, cmdArgs...) //nolint:gosec
	cmd.Env = append(os.Environ(), "ANSIBLE_FORCE_COLOR=1", "PYTHONUNBUFFERED=1", "ANSIBLE_HOST_KEY_CHECKING=False")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		httputil.InternalError(resp, "pipe error")
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		httputil.InternalError(resp, "pipe error")
		return
	}
	if err := cmd.Start(); err != nil {
		httputil.InternalError(resp, fmt.Sprintf("start ansible: %v", err))
		return
	}

	w := resp.ResponseWriter
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, canFlush := w.(http.Flusher)
	sendLine := func(line string) {
		fmt.Fprintf(w, "data: %s\n\n", stripANSI(line))
		if canFlush {
			flusher.Flush()
		}
	}

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			sendLine(scanner.Text())
		}
	}()

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		sendLine(scanner.Text())
	}

	_ = cmd.Wait()

	exitCode := 0
	var errSummary string
	if cmd.ProcessState != nil && !cmd.ProcessState.Success() {
		exitCode = cmd.ProcessState.ExitCode()
		errSummary = "ansible-playbook 同步失败，请查看下方日志了解详情"
	}
	donePayload, _ := json.Marshal(map[string]any{
		"exit_code": exitCode,
		"error":     errSummary,
	})
	fmt.Fprintf(w, "event: done\ndata: %s\n\n", donePayload)
	if canFlush {
		flusher.Flush()
	}
	log.Info().Str("env", body.Env).Int("exit_code", exitCode).Msg("ansible sync finished")
}

// ─── Helpers ────────────────────────────────────────────────────────────────────────

// ensureWorkDir creates the work directory structure if it doesn't exist.
func ensureWorkDir(workDir string) error {
	return os.MkdirAll(workDir, 0755)
}

// walkDir is a helper that recursively lists all files under a directory.
// Currently unused but kept for future sub-directory browsing.
func walkDir(root string) ([]nginxFile, error) {
	var files []nginxFile
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		files = append(files, nginxFile{
			Name:    d.Name(),
			Path:    path,
			Size:    info.Size(),
			ModTime: info.ModTime().Format(time.RFC3339),
		})
		return nil
	})
	return files, err
}
