package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-toast/toast"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"ariadesk/internal/aria2"
	"ariadesk/internal/autostart"
	"ariadesk/internal/config"
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

	pollMu   sync.Mutex
	pollStop chan struct{}

	knownMu       sync.Mutex
	knownStopped  map[string]struct{}
	stoppedSeeded atomic.Bool
}

func NewApp(icon []byte) *App {
	a := &App{
		icon:         icon,
		knownStopped: make(map[string]struct{}),
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
	running := a.proc.Running()
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
				"line":  line,
				"phase": phase,
				"error": errStr,
			})
		}
		emit("", "running", "")
		err := aria2.Install(func(line string) { emit(line, "running", "") })
		if err != nil {
			emit("", "error", err.Error())
			return
		}
		if _, derr := a.DetectAria2(); derr != nil {
			runtime.LogErrorf(a.ctx, "安装后重新检测失败: %v", derr)
		}
		emit("", "done", "")
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
		Running:   a.proc.Running(),
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
	return client.GetGlobalStat(a.ctx)
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
		return client.TellStopped(a.ctx, 0, 100)
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
	return client.PurgeDownloadResult(a.ctx)
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
	a.setConnected(true)

	active, aerr := client.TellActive(ctx)
	waiting, werr := client.TellWaiting(ctx, 0, 1000)
	stopped, serr := client.TellStopped(ctx, 0, 100)
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
	if !a.stoppedSeeded.Swap(true) {
		for _, t := range stopped {
			a.knownStopped[t.Gid] = struct{}{}
		}
		a.knownMu.Unlock()
		return
	}
	var completed []string
	for _, t := range stopped {
		if _, seen := a.knownStopped[t.Gid]; seen {
			continue
		}
		a.knownStopped[t.Gid] = struct{}{}
		// FollowedBy 非空说明是磁力链元数据任务，真正的内容任务刚被创建，不视为下载完成
		if t.Status == "complete" && len(t.FollowedBy) == 0 {
			completed = append(completed, t.Name())
		}
	}
	a.knownMu.Unlock()

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
