package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	stdruntime "runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-toast/toast"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"ariadesk/internal/aria2"
	"ariadesk/internal/autostart"
	"ariadesk/internal/config"
	"ariadesk/internal/history"
	"ariadesk/internal/rpc"
	"ariadesk/internal/secret"
	"ariadesk/internal/tray"
)

const appVersion = "0.1.0"

type Aria2State struct {
	Running   bool `json:"running"`
	PID       int  `json:"pid"`
	Port      int  `json:"port"`
	Connected bool `json:"connected"`
}

type AboutInfo struct {
	AppVersion   string `json:"appVersion"`
	Aria2Version string `json:"aria2Version"`
	DataDir      string `json:"dataDir"`
	CPUCores     int    `json:"cpuCores"`
}

type App struct {
	ctx  context.Context
	icon []byte

	mu       sync.RWMutex
	settings config.Settings
	detected aria2.DetectResult
	client   *rpc.Client

	proc      *aria2.Process
	adopted   atomic.Bool
	connected atomic.Bool
	updating  atomic.Bool

	pollMu   sync.Mutex
	pollStop chan struct{}

	knownMu       sync.Mutex
	knownStopped  map[string]struct{}
	stoppedSeeded atomic.Bool

	history *history.Store
}

func NewApp(icon []byte) *App {
	a := &App{
		icon:         icon,
		knownStopped: make(map[string]struct{}),
		history:      history.Load(),
	}
	a.proc = aria2.NewProcess()
	a.proc.SetOnExit(func() {
		a.setConnected(false)
		a.emitState()
	})
	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	s, err := config.Load()
	if err != nil {
		runtime.LogErrorf(ctx, "加载配置失败: %v", err)
	}
	a.settings = s
	a.history.SetMax(s.HistoryMax)

	detected := aria2.Detect(s.Aria2Path)
	a.detected = detected
	if detected.Installed && detected.Path != s.Aria2Path {
		s.Aria2Path = detected.Path
		a.settings = s
		_ = config.Save(s)
	}

	token, err := secret.LoadOrCreate()
	if err != nil {
		runtime.LogErrorf(ctx, "初始化 token 失败: %v", err)
	}
	a.client = rpc.NewClient(s.RPCPort, token)

	tray.Start(a.icon, tray.Handlers{
		OnShow: func() {
			runtime.WindowShow(a.ctx)
			runtime.WindowUnminimise(a.ctx)
		},
		OnStartAria2: func() { _ = a.StartAria2() },
		OnStopAria2:  func() { _ = a.StopAria2() },
		OnOpenDownloadDir: func() {
			a.mu.RLock()
			dir := a.settings.DownloadDir
			a.mu.RUnlock()
			openPath(dir)
		},
		OnQuit: func() {
			a.cleanup()
			os.Exit(0)
		},
	})
	a.emitState()

	if s.AutoStartAria2 {
		go func() {
			time.Sleep(1500 * time.Millisecond)
			if err := a.StartAria2(); err != nil {
				runtime.LogErrorf(a.ctx, "自动启动 aria2 失败: %v", err)
			}
		}()
	}
}

func (a *App) onShutdown(ctx context.Context) {
	a.cleanup()
}

func (a *App) onBeforeClose(ctx context.Context) bool {
	a.mu.RLock()
	closeToTray := a.settings.CloseToTray
	a.mu.RUnlock()
	if closeToTray {
		runtime.WindowHide(ctx)
		return true
	}
	return false
}

func (a *App) cleanup() {
	a.stopPolling()
	if a.proc.Running() {
		a.stopAria2Process()
	}
}

func (a *App) getClient() *rpc.Client {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.client
}

func (a *App) setConnected(v bool) {
	if a.connected.Swap(v) != v {
		a.emitState()
	}
}

func (a *App) emitState() {
	if a.ctx == nil {
		return
	}
	running := a.proc.Running() || a.connected.Load()
	runtime.EventsEmit(a.ctx, "aria2:state", map[string]any{
		"running":   running,
		"connected": a.connected.Load(),
	})
	tray.SetRunning(running)
}

// ---------- 设置 ----------

func (a *App) GetSettings() (config.Settings, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.settings, nil
}

func (a *App) SaveSettings(s config.Settings) error {
	a.mu.Lock()
	old := a.settings
	a.settings = s
	client := a.client
	a.mu.Unlock()

	if client != nil && s.RPCPort != old.RPCPort {
		client.SetPort(s.RPCPort)
	}
	if err := config.Save(s); err != nil {
		return err
	}
	if client != nil && s.DownloadDir != old.DownloadDir {
		// 立即同步到运行中的 aria2，新建任务无需重启即生效
		_ = client.ChangeGlobalOption(a.ctx, map[string]string{"dir": s.DownloadDir})
	}
	if s.HistoryMax != old.HistoryMax {
		a.history.SetMax(s.HistoryMax)
	}
	if client != nil && s.MaxConcurrent != old.MaxConcurrent {
		_ = client.ChangeGlobalOption(a.ctx, map[string]string{"max-concurrent-downloads": fmt.Sprintf("%d", s.MaxConcurrent)})
	}
	if s.AutoStartApp != old.AutoStartApp {
		if err := autostart.Set(s.AutoStartApp); err != nil {
			return fmt.Errorf("更新开机自启失败: %w", err)
		}
	}
	return nil
}

func (a *App) GetAbout() (AboutInfo, error) {
	a.mu.RLock()
	version := a.detected.Version
	a.mu.RUnlock()
	return AboutInfo{
		AppVersion:   appVersion,
		Aria2Version: version,
		DataDir:      config.DataDir(),
		CPUCores:     stdruntime.NumCPU(),
	}, nil
}

// ---------- 检测与安装 ----------

func (a *App) DetectAria2() (aria2.DetectResult, error) {
	a.mu.RLock()
	cached := a.settings.Aria2Path
	a.mu.RUnlock()
	r := aria2.Detect(cached)
	a.mu.Lock()
	a.detected = r
	if r.Installed && r.Path != a.settings.Aria2Path {
		a.settings.Aria2Path = r.Path
		_ = config.Save(a.settings)
	}
	a.mu.Unlock()
	return r, nil
}

func (a *App) InstallAria2() error {
	go func() {
		emit := func(line, phase, errStr string) {
			if a.ctx == nil {
				return
			}
			runtime.EventsEmit(a.ctx, "aria2:install", map[string]any{
				"line":    line,
				"phase":   phase,
				"error":   errStr,
				"percent": -1,
			})
		}
		emit("", "running", "")
		err := aria2.Install(func(line string) { emit(line, "running", "") })
		if err != nil {
			emit("", "error", err.Error())
			return
		}
		d, derr := a.DetectAria2()
		if derr != nil {
			runtime.LogErrorf(a.ctx, "安装后重新检测失败: %v", derr)
		}
		if !d.Installed {
			emit("", "error", "winget 报告安装成功，但未检测到 aria2c.exe，请重试或手动安装 aria2")
			return
		}
		emit("", "done", "")
	}()
	return nil
}

// ---------- 版本管理（检查更新 / 切换版本 / 卸载） ----------

// CheckAria2Updates 从 GitHub 获取最近的 aria2 发布列表，含正式版/测试版标记。
func (a *App) CheckAria2Updates() ([]aria2.ReleaseInfo, error) {
	return aria2.FetchReleases(12)
}

func (a *App) emitUpdate(line, phase, errStr string, percent int) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, "aria2:update", map[string]any{
		"line":    line,
		"phase":   phase,
		"error":   errStr,
		"percent": percent,
	})
}

// UpdateAria2ToVersion 切换到指定版本：版本更高即升级，更低即回退。
// 从 GitHub 下载该版本到 AriaDesk 托管目录并切换路径，之前在运行则自动重启。
func (a *App) UpdateAria2ToVersion(version string) error {
	version = strings.TrimSpace(version)
	if version == "" {
		return fmt.Errorf("版本号不能为空")
	}
	if !a.updating.CompareAndSwap(false, true) {
		return fmt.Errorf("已有版本切换任务在进行中")
	}
	go func() {
		defer a.updating.Store(false)
		a.emitUpdate("", "running", "", -1)
		emit := func(line string) { a.emitUpdate(line, "running", "", -1) }
		fail := func(msg string) { a.emitUpdate("", "error", msg, -1) }

		wasRunning := a.proc.Running() || a.connected.Load()
		if wasRunning {
			emit("正在停止 aria2…")
			if err := a.StopAria2(); err != nil {
				fail("停止 aria2 失败: " + err.Error())
				return
			}
		}

		exe, err := aria2.EnsureVersion(version, func(line string, pct int) {
			a.emitUpdate(line, "running", "", pct)
		})
		if err != nil {
			fail(err.Error())
			return
		}

		a.mu.Lock()
		a.settings.Aria2Path = exe
		_ = config.Save(a.settings)
		a.mu.Unlock()
		if _, derr := a.DetectAria2(); derr != nil {
			runtime.LogErrorf(a.ctx, "切换版本后重新检测失败: %v", derr)
		}
		emit(fmt.Sprintf("已切换到 aria2 %s", version))

		if wasRunning {
			emit("正在启动 aria2…")
			if err := a.StartAria2(); err != nil {
				fail("启动失败: " + err.Error())
				return
			}
		}
		a.emitUpdate("", "done", "", 100)
	}()
	return nil
}

// UninstallAria2 卸载 aria2：优先删除 AriaDesk 托管的版本目录，
// 若仍存在 winget 安装的副本则通过 winget 卸载。
func (a *App) UninstallAria2() error {
	if !a.updating.CompareAndSwap(false, true) {
		return fmt.Errorf("已有版本切换任务在进行中")
	}
	go func() {
		defer a.updating.Store(false)
		a.emitUpdate("", "running", "", -1)
		emit := func(line string) { a.emitUpdate(line, "running", "", -1) }
		fail := func(msg string) { a.emitUpdate("", "error", msg, -1) }

		if a.proc.Running() || a.connected.Load() {
			emit("正在停止 aria2…")
			if err := a.StopAria2(); err != nil {
				fail("停止 aria2 失败: " + err.Error())
				return
			}
		}

		a.mu.Lock()
		a.settings.Aria2Path = ""
		_ = config.Save(a.settings)
		a.mu.Unlock()

		if _, err := os.Stat(aria2.VersionsDir()); err == nil {
			emit("删除 AriaDesk 下载的 aria2…")
			if err := os.RemoveAll(aria2.VersionsDir()); err != nil {
				fail("删除托管版本失败: " + err.Error())
				return
			}
		}

		r := aria2.Detect("")
		if r.Installed {
			if strings.Contains(r.Path, "WinGet") {
				emit("通过 winget 卸载 aria2…")
				if err := aria2.Uninstall(emit); err != nil {
					fail(err.Error())
					return
				}
				r = aria2.Detect("")
			}
			if r.Installed {
				fail(fmt.Sprintf("检测到非 AriaDesk 安装的 aria2（%s），请手动卸载", r.Path))
				return
			}
		}

		a.mu.Lock()
		a.detected = aria2.DetectResult{Installed: false}
		a.mu.Unlock()
		a.emitState()
		a.emitUpdate("", "done", "", 100)
	}()
	return nil
}

// ---------- token ----------

func (a *App) GetTokenMasked() (string, error) {
	token, err := secret.LoadOrCreate()
	if err != nil {
		return "", err
	}
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()
	if client != nil {
		client.SetToken(token)
	}
	return strings.Repeat("•", 8), nil
}

func (a *App) RevealToken() (string, error) {
	return secret.Load()
}

func (a *App) RegenerateToken() error {
	token, err := secret.Generate()
	if err != nil {
		return err
	}
	if err := secret.Save(token); err != nil {
		return err
	}
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()
	if client != nil {
		client.SetToken(token)
	}
	return nil
}

// ---------- aria2 进程 ----------

func (a *App) StartAria2() error {
	if a.proc.Running() {
		return nil
	}
	a.mu.RLock()
	s := a.settings
	a.mu.RUnlock()

	exePath := s.Aria2Path
	if exePath == "" {
		r := aria2.Detect("")
		if !r.Installed {
			return fmt.Errorf("未检测到 aria2，请先安装")
		}
		exePath = r.Path
		a.mu.Lock()
		a.settings.Aria2Path = r.Path
		a.detected = r
		_ = config.Save(a.settings)
		a.mu.Unlock()
	}

	if aria2.PortInUse(s.RPCPort) {
		// 端口被占：探测是否是活着的 aria2，是则复用连接
		token, err := secret.LoadOrCreate()
		if err != nil {
			return err
		}
		probe := rpc.NewClient(s.RPCPort, token)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, err = probe.GetVersion(ctx)
		cancel()
		if err != nil {
			return fmt.Errorf("端口 %d 已被其它程序占用，请在设置中更换 RPC 端口", s.RPCPort)
		}
		a.adopted.Store(true)
		a.setConnected(true)
		a.resetCompletionTracking()
		a.startPolling()
		a.emitState()
		return nil
	}

	info, err := os.Stat(s.DownloadDir)
	if err != nil || !info.IsDir() {
		if mkErr := os.MkdirAll(s.DownloadDir, 0o755); mkErr != nil {
			return fmt.Errorf("下载目录不存在且无法创建: %s", s.DownloadDir)
		}
	}
	if err := aria2.EnsureRuntimeFiles(); err != nil {
		return err
	}
	token, err := secret.LoadOrCreate()
	if err != nil {
		return err
	}
	if err := a.proc.Start(exePath, aria2.BuildArgs(s, token)); err != nil {
		return err
	}
	a.adopted.Store(false)
	a.emitState()

	// 等待 RPC 就绪
	client := a.getClient()
	if client == nil {
		a.mu.Lock()
		a.client = rpc.NewClient(s.RPCPort, token)
		client = a.client
		a.mu.Unlock()
	}
	deadline := time.Now().Add(10 * time.Second)
	connected := false
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		v, err := client.GetVersion(ctx)
		cancel()
		if err == nil {
			a.mu.Lock()
			a.detected.Version = v.Version
			a.mu.Unlock()
			connected = true
			break
		}
		if !a.proc.Running() {
			return fmt.Errorf("aria2 启动后立即退出，请查看日志 %s", filepath.Join(config.Aria2Dir(), "aria2.log"))
		}
		time.Sleep(300 * time.Millisecond)
	}
	if !connected {
		return fmt.Errorf("aria2 已启动但 RPC 未就绪")
	}
	a.setConnected(true)
	a.resetCompletionTracking()
	a.startPolling()
	return nil
}

func (a *App) StopAria2() error {
	a.stopPolling()
	if a.proc.Running() {
		if err := a.stopAria2Process(); err != nil {
			return err
		}
	} else if a.adopted.Load() && a.connected.Load() {
		client := a.getClient()
		if client != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			_ = client.Shutdown(ctx)
			cancel()
		}
		a.adopted.Store(false)
	}
	a.setConnected(false)
	a.emitState()
	return nil
}

func (a *App) stopAria2Process() error {
	client := a.getClient()
	return a.proc.Stop(func() error {
		if client == nil {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return client.Shutdown(ctx)
	}, 5*time.Second)
}

func (a *App) GetAria2State() (Aria2State, error) {
	a.mu.RLock()
	port := a.settings.RPCPort
	a.mu.RUnlock()
	return Aria2State{
		// Running 语义为"aria2 正在运行"：本进程托管，或已接管外部进程（RPC 已连通即证明其在运行）
		Running:   a.proc.Running() || a.connected.Load(),
		PID:       a.proc.PID(),
		Port:      port,
		Connected: a.connected.Load(),
	}, nil
}

// ---------- RPC ----------

func (a *App) Connect() error {
	a.mu.RLock()
	s := a.settings
	a.mu.RUnlock()
	token, err := secret.LoadOrCreate()
	if err != nil {
		return err
	}
	a.mu.Lock()
	if a.client == nil {
		a.client = rpc.NewClient(s.RPCPort, token)
	} else {
		a.client.SetPort(s.RPCPort)
		a.client.SetToken(token)
	}
	client := a.client
	a.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	v, err := client.GetVersion(ctx)
	cancel()
	if err != nil {
		a.setConnected(false)
		return fmt.Errorf("连接 aria2 失败: %w", err)
	}
	a.mu.Lock()
	a.detected.Version = v.Version
	a.mu.Unlock()
	a.setConnected(true)
	a.resetCompletionTracking()
	a.startPolling()
	return nil
}

func (a *App) requireClient() (*rpc.Client, error) {
	client := a.getClient()
	if client == nil {
		return nil, fmt.Errorf("尚未连接 aria2")
	}
	return client, nil
}

func (a *App) GetGlobalStat() (rpc.GlobalStat, error) {
	client, err := a.requireClient()
	if err != nil {
		return rpc.GlobalStat{}, err
	}
	stat, err := client.GetGlobalStat(a.ctx)
	normalizeStat(&stat)
	return stat, err
}

// normalizeStat 归零无活动任务时的残留速率：aria2 的速率是滑动窗口平均值，
// 任务结束后会缓慢衰减约 10s；没有活动任务时不存在真实传输，直接归零避免误导。
// 做种中的任务 status 为 active 会计入 numActive，因此不影响做种上传速率的显示。
func normalizeStat(stat *rpc.GlobalStat) {
	if stat.NumActive == "0" {
		stat.DownloadSpeed = "0"
		stat.UploadSpeed = "0"
	}
}

func (a *App) GetTasks(tab string) ([]rpc.Task, error) {
	client, err := a.requireClient()
	if err != nil {
		return nil, err
	}
	switch tab {
	case "active":
		return client.TellActive(a.ctx)
	case "waiting":
		return client.TellWaiting(a.ctx, 0, 1000)
	case "stopped":
		return client.TellStopped(a.ctx, 0, 1000)
	default:
		return nil, fmt.Errorf("未知任务类型: %s", tab)
	}
}

func (a *App) AddURI(uri string) (string, error) {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return "", fmt.Errorf("下载地址不能为空")
	}
	client, err := a.requireClient()
	if err != nil {
		return "", err
	}
	return client.AddURI(a.ctx, uri, a.dirOption())
}

// dirOption 让新建任务始终使用当前设置的下载目录，与 aria2 启动时的 --dir 无关
func (a *App) dirOption() map[string]string {
	a.mu.RLock()
	dir := a.settings.DownloadDir
	a.mu.RUnlock()
	if dir == "" {
		return nil
	}
	return map[string]string{"dir": dir}
}

func (a *App) AddTorrentFile() (string, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择种子文件",
		Filters: []runtime.FileFilter{
			{DisplayName: "种子文件", Pattern: "*.torrent"},
		},
	})
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	client, err := a.requireClient()
	if err != nil {
		return "", err
	}
	return client.AddTorrent(a.ctx, data, a.dirOption())
}

func (a *App) PauseTask(gid string) error {
	client, err := a.requireClient()
	if err != nil {
		return err
	}
	return client.Pause(a.ctx, gid)
}

func (a *App) UnpauseTask(gid string) error {
	client, err := a.requireClient()
	if err != nil {
		return err
	}
	return client.Unpause(a.ctx, gid)
}

func (a *App) RemoveTask(gid string, stopped bool) error {
	client, err := a.requireClient()
	if err != nil {
		return err
	}
	if stopped {
		return client.RemoveDownloadResult(a.ctx, gid)
	}
	if err := client.Remove(a.ctx, gid); err != nil {
		return client.ForceRemove(a.ctx, gid)
	}
	return nil
}

func (a *App) PauseAll() error {
	client, err := a.requireClient()
	if err != nil {
		return err
	}
	return client.PauseAll(a.ctx)
}

func (a *App) UnpauseAll() error {
	client, err := a.requireClient()
	if err != nil {
		return err
	}
	return client.UnpauseAll(a.ctx)
}

func (a *App) PurgeCompleted() error {
	client, err := a.requireClient()
	if err != nil {
		return err
	}
	if a.history.Clear() {
		a.emitHistory()
	}
	return client.PurgeDownloadResult(a.ctx)
}

// ---------- 下载历史（本地持久化） ----------

func (a *App) emitHistory() {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, "history", a.history.List())
}

func (a *App) GetHistory() []history.Record {
	return a.history.List()
}

func (a *App) DeleteHistoryRecord(gid string) error {
	if a.history.Remove(gid) {
		a.emitHistory()
	}
	return nil
}

// OpenFolder 打开指定目录（用于历史记录的“打开目录”，此时任务已从 aria2 消失）
func (a *App) OpenFolder(path string) error {
	if path == "" {
		return fmt.Errorf("该记录没有目录信息")
	}
	p := filepath.Clean(path)
	if _, err := os.Stat(p); err != nil {
		return fmt.Errorf("目录不存在（可能已被移动或删除）：%s", p)
	}
	openPath(p)
	return nil
}

// ---------- 目录与日志 ----------

func openPath(path string) {
	// explorer 即使成功也常返回退出码 1，忽略错误
	_ = exec.Command("explorer.exe", path).Run()
}

func openSelect(path string) {
	_ = exec.Command("explorer.exe", "/select,"+path).Run()
}

func (a *App) OpenTaskDir(gid string) error {
	client, err := a.requireClient()
	if err != nil {
		return err
	}
	t, err := client.TellStatus(a.ctx, gid, "files", "dir")
	if err != nil {
		return err
	}
	// aria2 返回的路径统一使用 "/" 分隔符，explorer /select 需要 Windows 反斜杠路径
	if len(t.Files) > 0 && t.Files[0].Path != "" {
		p := filepath.Clean(t.Files[0].Path)
		if _, err := os.Stat(p); err == nil {
			openSelect(p)
			return nil
		}
	}
	if t.Dir != "" {
		d := filepath.Clean(t.Dir)
		if _, err := os.Stat(d); err == nil {
			openPath(d)
			return nil
		}
		return fmt.Errorf("文件不存在（可能已被移动或删除）：%s", d)
	}
	return fmt.Errorf("任务没有可打开的文件")
}

func (a *App) OpenDownloadDir() error {
	a.mu.RLock()
	dir := a.settings.DownloadDir
	a.mu.RUnlock()
	if dir == "" {
		return fmt.Errorf("下载目录未配置")
	}
	openPath(dir)
	return nil
}

func (a *App) OpenDataDir() error {
	openPath(config.DataDir())
	return nil
}

func (a *App) BrowseDownloadDir() (string, error) {
	a.mu.RLock()
	def := a.settings.DownloadDir
	a.mu.RUnlock()
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:            "选择下载目录",
		DefaultDirectory: def,
	})
}

func (a *App) ReadAria2Log() (string, error) {
	data, err := os.ReadFile(filepath.Join(config.Aria2Dir(), "aria2.log"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	const maxLines = 200
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n"), nil
}

// ---------- 轮询 ----------

func (a *App) startPolling() {
	a.stopPolling()
	a.pollMu.Lock()
	stop := make(chan struct{})
	a.pollStop = stop
	a.pollMu.Unlock()
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				a.pollOnce()
			}
		}
	}()
}

func (a *App) stopPolling() {
	a.pollMu.Lock()
	defer a.pollMu.Unlock()
	if a.pollStop != nil {
		close(a.pollStop)
		a.pollStop = nil
	}
}

func (a *App) pollOnce() {
	client := a.getClient()
	if client == nil || a.ctx == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stat, err := client.GetGlobalStat(ctx)
	if err != nil {
		a.setConnected(false)
		return
	}
	normalizeStat(&stat)
	a.setConnected(true)

	active, aerr := client.TellActive(ctx)
	waiting, werr := client.TellWaiting(ctx, 0, 1000)
	stopped, serr := client.TellStopped(ctx, 0, 1000)
	if aerr != nil {
		active = []rpc.Task{}
	}
	if werr != nil {
		waiting = []rpc.Task{}
	}
	if serr != nil {
		stopped = []rpc.Task{}
	} else {
		a.checkCompleted(stopped)
	}

	runtime.EventsEmit(a.ctx, "poll", map[string]any{
		"stat":      stat,
		"active":    active,
		"waiting":   waiting,
		"stopped":   stopped,
		"connected": true,
	})
}

// ---------- 下载完成通知 ----------

func (a *App) resetCompletionTracking() {
	a.stoppedSeeded.Store(false)
	a.knownMu.Lock()
	a.knownStopped = make(map[string]struct{})
	a.knownMu.Unlock()
}

func (a *App) checkCompleted(stopped []rpc.Task) {
	a.knownMu.Lock()
	first := !a.stoppedSeeded.Swap(true)
	var completed []string
	historyChanged := false
	for _, t := range stopped {
		if _, seen := a.knownStopped[t.Gid]; seen {
			continue
		}
		a.knownStopped[t.Gid] = struct{}{}
		// FollowedBy 非空说明是磁力链元数据任务，真正的内容任务刚被创建，不视为下载完成
		if len(t.FollowedBy) > 0 {
			continue
		}
		// 记入本地历史（包括基线播种时发现的、应用未运行期间完成的任务）
		rec := history.Record{
			Gid:             t.Gid,
			Name:            t.Name(),
			Status:          t.Status,
			TotalLength:     t.TotalLength,
			CompletedLength: t.CompletedLength,
			Dir:             t.Dir,
			CompletedAt:     time.Now(),
		}
		if a.history.Add(rec) {
			historyChanged = true
		}
		// 基线播种不弹通知，避免启动时对旧任务批量误报
		if !first && t.Status == "complete" {
			completed = append(completed, t.Name())
		}
	}
	a.knownMu.Unlock()

	if historyChanged {
		a.emitHistory()
	}
	for _, name := range completed {
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "task:completed", map[string]any{"name": name})
		}
		go func(n string) {
			notification := toast.Notification{
				AppID:   "AriaDesk",
				Title:   "下载完成",
				Message: n,
			}
			_ = notification.Push()
		}(name)
	}
}
