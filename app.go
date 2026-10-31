package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	stdruntime "runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/go-toast/toast"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"ariadesk/internal/aria2"
	"ariadesk/internal/autostart"
	"ariadesk/internal/config"
	"ariadesk/internal/extsrv"
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

	extMu sync.Mutex
	ext   *extsrv.Server
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

	// 启动时生成/同步 aria2.conf，保证配置页随时可读、启动时使用最新设置
	if err := aria2.SyncConf(s, token); err != nil {
		runtime.LogErrorf(ctx, "生成 aria2 配置失败: %v", err)
	}

	// 每次启动自动更新公共 Tracker 列表
	go a.autoUpdateTrackers()

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

	if s.BrowserExtEnabled {
		a.startExtServer()
	}

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
	a.stopExtServer()
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
	if s.DownloadDir != old.DownloadDir || s.RPCPort != old.RPCPort || s.MaxConcurrent != old.MaxConcurrent {
		// 托管键统一写入 aria2.conf，重启 aria2 后生效
		token, err := secret.LoadOrCreate()
		if err == nil {
			if err := aria2.SyncConf(s, token); err != nil {
				return fmt.Errorf("更新 aria2 配置失败: %w", err)
			}
		}
	}
	if s.AutoStartApp != old.AutoStartApp {
		if err := autostart.Set(s.AutoStartApp); err != nil {
			return fmt.Errorf("更新开机自启失败: %w", err)
		}
	}
	if s.BrowserExtEnabled != old.BrowserExtEnabled {
		if s.BrowserExtEnabled {
			a.startExtServer()
		} else {
			a.stopExtServer()
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
	s := a.settings
	a.mu.RUnlock()
	if client != nil {
		client.SetToken(token)
	}
	// rpc-secret 是托管键，同步到 aria2.conf 保持重启后一致
	if err := aria2.SyncConf(s, token); err != nil {
		return fmt.Errorf("更新 aria2 配置失败: %w", err)
	}
	return nil
}

// ---------- aria2 配置文件 ----------

// GetAria2ConfSections 按分区返回 aria2.conf 内容，供配置页卡片展示。
func (a *App) GetAria2ConfSections() ([]aria2.ConfSection, error) {
	a.mu.RLock()
	s := a.settings
	a.mu.RUnlock()
	token, err := secret.LoadOrCreate()
	if err != nil {
		return nil, err
	}
	if err := aria2.SyncConf(s, token); err != nil {
		return nil, err
	}
	text, err := aria2.ReadConf()
	if err != nil {
		return nil, err
	}
	return aria2.ParseConf(text), nil
}

// SaveAria2ConfValues 就地更新配置文件中的指定键，重启 aria2 后生效。
func (a *App) SaveAria2ConfValues(values map[string]string) error {
	if len(values) == 0 {
		return fmt.Errorf("没有需要保存的配置")
	}
	for k := range values {
		if strings.TrimSpace(k) == "" {
			return fmt.Errorf("配置键不能为空")
		}
	}
	return aria2.UpdateConfValues(values)
}

// ResetAria2Conf 重置为默认配置。
func (a *App) ResetAria2Conf() error {
	a.mu.RLock()
	s := a.settings
	a.mu.RUnlock()
	token, err := secret.LoadOrCreate()
	if err != nil {
		return err
	}
	return aria2.ResetConf(s, token)
}

// UpdateBtTrackers 拉取最新公共 Tracker 列表，写入 aria2.conf 的 bt-tracker，
// 并实时应用到运行中的 aria2，返回列表内容（逗号分隔）。
func (a *App) UpdateBtTrackers() (string, error) {
	trackers, err := aria2.FetchTrackers()
	if err != nil {
		return "", err
	}
	list := strings.Join(trackers, ",")
	if err := aria2.UpdateConfValues(map[string]string{"bt-tracker": list}); err != nil {
		return "", err
	}
	a.applyTrackersLive(list)
	return list, nil
}

// autoUpdateTrackers 每次启动时自动更新 bt-tracker，失败仅记录日志。
func (a *App) autoUpdateTrackers() {
	trackers, err := aria2.FetchTrackers()
	if err != nil {
		runtime.LogErrorf(a.ctx, "自动更新 Tracker 失败: %v", err)
		return
	}
	list := strings.Join(trackers, ",")
	if err := aria2.UpdateConfValues(map[string]string{"bt-tracker": list}); err != nil {
		runtime.LogErrorf(a.ctx, "写入 Tracker 配置失败: %v", err)
		return
	}
	runtime.LogInfof(a.ctx, "已自动更新 %d 个 Tracker", len(trackers))
	a.applyTrackersLive(list)
}

// applyTrackersLive 将最新 bt-tracker 应用到运行中的 aria2，无需重启。
func (a *App) applyTrackersLive(list string) {
	client := a.getClient()
	if client == nil || !a.connected.Load() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_ = client.ChangeGlobalOption(ctx, map[string]string{"bt-tracker": list})
	cancel()
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

	// 重启场景：旧进程刚退出，端口可能尚未完全释放，先等待其空闲，
	// 否则会误入下方的“复用外部进程”分支而报错
	const portWaitRounds = 25 // 最多等待 5 秒
	for i := 0; i < portWaitRounds && aria2.PortInUse(s.RPCPort); i++ {
		time.Sleep(200 * time.Millisecond)
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
	// 启动前将应用设置同步到 aria2.conf，由配置文件统一控制启动参数
	if err := aria2.SyncConf(s, token); err != nil {
		return fmt.Errorf("生成 aria2 配置失败: %w", err)
	}

	client := a.getClient()
	if client == nil {
		a.mu.Lock()
		a.client = rpc.NewClient(s.RPCPort, token)
		client = a.client
		a.mu.Unlock()
	}

	// 启动并等待 RPC 就绪；进程立即退出（如端口绑定竞争失败）时重试
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Second)
		}
		if err := a.proc.Start(exePath, aria2.BuildArgs()); err != nil {
			return err
		}
		a.adopted.Store(false)
		a.emitState()

		connected := false
		deadline := time.Now().Add(10 * time.Second)
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
				lastErr = fmt.Errorf("aria2 启动后立即退出，请查看日志 %s", filepath.Join(config.Aria2Dir(), "aria2.log"))
				break
			}
			time.Sleep(300 * time.Millisecond)
		}
		if connected {
			a.setConnected(true)
			a.resetCompletionTracking()
			a.startPolling()
			return nil
		}
		if a.proc.Running() {
			return fmt.Errorf("aria2 已启动但 RPC 未就绪")
		}
	}
	return lastErr
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
func (a *App) dirOption() map[string]any {
	a.mu.RLock()
	dir := a.settings.DownloadDir
	a.mu.RUnlock()
	if dir == "" {
		return nil
	}
	return map[string]any{"dir": dir}
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
		if err := client.RemoveDownloadResult(a.ctx, gid); err != nil {
			return err
		}
	} else if err := client.Remove(a.ctx, gid); err != nil {
		if err := client.ForceRemove(a.ctx, gid); err != nil {
			return err
		}
	}
	// 同一 gid 的本地历史记录一并删除，否则 aria2 侧删除后历史记录会顶上来，需要再删一次
	if a.history.Remove(gid) {
		a.emitHistory()
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

// OpenLocation 打开历史记录的位置：文件用 explorer /select 定位并选中，目录直接打开
// （历史记录中的任务已从 aria2 消失，只能依赖记录里保存的路径）
func (a *App) OpenLocation(path string) error {
	if path == "" {
		return fmt.Errorf("该记录没有位置信息")
	}
	p := filepath.Clean(path)
	info, err := os.Stat(p)
	if err != nil {
		return fmt.Errorf("文件或目录不存在（可能已被移动或删除）：%s", p)
	}
	if info.IsDir() {
		openPath(p)
	} else {
		openSelect(p)
	}
	return nil
}

// ---------- 目录与日志 ----------

func openPath(path string) {
	// explorer 即使成功也常返回退出码 1，忽略错误
	_ = exec.Command("explorer.exe", path).Run()
}

func openSelect(path string) {
	// explorer 的命令行解析与 Go 的参数转义不兼容：路径含空格时，由 Args 拼出的
	// "/select,..." 会被 explorer 错误解析而打开无关目录，必须自行拼接原始命令行 /select,"path"
	cmd := exec.Command("explorer.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer.exe /select,"` + path + `"`}
	// explorer 即使成功也常返回退出码 1，忽略错误
	_ = cmd.Run()
}

// taskTarget 计算任务“打开文件位置”的目标路径（aria2 风格 "/" 分隔，未做本地化处理）：
// 多文件 BT 任务定位到以任务名命名的目录，其余定位到第一个在磁盘上实际存在的文件
// （部分下载时 Files[0] 可能未被选中而不存在）；找不到时返回空串。
func taskTarget(t rpc.Task) string {
	if len(t.Files) == 0 {
		return ""
	}
	if len(t.Files) > 1 && t.BitTorrent.Info.Name != "" {
		return t.Dir + "/" + t.BitTorrent.Info.Name
	}
	for _, f := range t.Files {
		if f.Path == "" {
			continue
		}
		if _, err := os.Stat(filepath.Clean(f.Path)); err == nil {
			return f.Path
		}
	}
	return ""
}

func (a *App) OpenTaskDir(gid string) error {
	client, err := a.requireClient()
	if err != nil {
		return err
	}
	t, err := client.TellStatus(a.ctx, gid, "files", "dir", "bittorrent")
	if err != nil {
		return err
	}
	// aria2 返回的路径统一使用 "/" 分隔符，explorer /select 需要 Windows 反斜杠路径
	if target := taskTarget(t); target != "" {
		p := filepath.Clean(target)
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
			FilePath:        taskTarget(t),
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

// ---------- 浏览器插件推送服务 ----------

type ExtInfo struct {
	Enabled bool `json:"enabled"`
	Running bool `json:"running"`
	Port    int  `json:"port"`
	PortMin int  `json:"portMin"`
	PortMax int  `json:"portMax"`
}

func (a *App) GetExtInfo() (ExtInfo, error) {
	a.mu.RLock()
	enabled := a.settings.BrowserExtEnabled
	a.mu.RUnlock()
	a.extMu.Lock()
	running := a.ext != nil && a.ext.Running()
	port := 0
	if running {
		port = a.ext.Port()
	}
	a.extMu.Unlock()
	return ExtInfo{
		Enabled: enabled,
		Running: running,
		Port:    port,
		PortMin: extsrv.PortMin,
		PortMax: extsrv.PortMax,
	}, nil
}

func (a *App) RevealExtToken() (string, error) {
	return secret.LoadOrCreateExt()
}

// RegenerateExtToken 重新生成配对 token，旧 token 立即失效，插件侧需重新填写
func (a *App) RegenerateExtToken() error {
	token, err := secret.Generate()
	if err != nil {
		return err
	}
	return secret.SaveExt(token)
}

func (a *App) startExtServer() {
	a.extMu.Lock()
	defer a.extMu.Unlock()
	if a.ext != nil && a.ext.Running() {
		return
	}
	srv := extsrv.New(appVersion, secret.LoadOrCreateExt, extPushHandler{a: a})
	if err := srv.Start(); err != nil {
		if a.ctx != nil {
			runtime.LogErrorf(a.ctx, "浏览器插件服务启动失败: %v", err)
		}
		return
	}
	a.ext = srv
	if a.ctx != nil {
		runtime.LogInfof(a.ctx, "浏览器插件服务已监听 127.0.0.1:%d", srv.Port())
	}
}

func (a *App) stopExtServer() {
	a.extMu.Lock()
	defer a.extMu.Unlock()
	if a.ext != nil {
		a.ext.Stop()
		a.ext = nil
	}
}

// readyClient 返回可用的 aria2 客户端；未运行时先尝试拉起，失败则告知插件
func (a *App) readyClient() (*rpc.Client, error) {
	if a.connected.Load() {
		if c := a.getClient(); c != nil {
			return c, nil
		}
	}
	if err := a.StartAria2(); err != nil {
		return nil, fmt.Errorf("%w: %v", extsrv.ErrAria2NotRunning, err)
	}
	c := a.getClient()
	if c == nil || !a.connected.Load() {
		return nil, extsrv.ErrAria2NotRunning
	}
	return c, nil
}

func (a *App) notifyExtTask(name string) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "task:ext-added", map[string]any{"name": name})
	}
	go func() {
		notification := toast.Notification{
			AppID:   "AriaDesk",
			Title:   "已接收浏览器下载任务",
			Message: name,
		}
		_ = notification.Push()
	}()
}

func extTaskName(raw string) string {
	if strings.HasPrefix(raw, "magnet:") {
		return "磁力链接任务"
	}
	if u, err := url.Parse(raw); err == nil {
		if b := path.Base(u.Path); b != "." && b != "/" && b != "" {
			return b
		}
	}
	return raw
}

type extPushHandler struct{ a *App }

func (h extPushHandler) AddDownload(req extsrv.DownloadRequest) (string, error) {
	client, err := h.a.readyClient()
	if err != nil {
		return "", err
	}
	opts := h.a.dirOption()
	if opts == nil {
		opts = map[string]any{}
	}
	var headers []string
	if req.Cookies != "" {
		headers = append(headers, "Cookie: "+req.Cookies)
	}
	if req.Referer != "" {
		headers = append(headers, "Referer: "+req.Referer)
	}
	if req.UserAgent != "" {
		headers = append(headers, "User-Agent: "+req.UserAgent)
	}
	if len(headers) > 0 {
		opts["header"] = headers
	}
	if req.Filename != "" {
		opts["out"] = req.Filename
	}
	gid, err := client.AddURI(h.a.ctx, req.URL, opts)
	if err != nil {
		return "", err
	}
	h.a.notifyExtTask(extTaskName(req.URL))
	return gid, nil
}

func (h extPushHandler) AddTorrent(name string, data []byte) (string, error) {
	client, err := h.a.readyClient()
	if err != nil {
		return "", err
	}
	gid, err := client.AddTorrent(h.a.ctx, data, h.a.dirOption())
	if err != nil {
		return "", err
	}
	if name == "" {
		name = "种子任务"
	}
	h.a.notifyExtTask(name)
	return gid, nil
}
