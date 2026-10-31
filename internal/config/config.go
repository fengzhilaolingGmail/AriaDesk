package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
)

const DirName = "AriaDesk"

type Settings struct {
	Aria2Path      string `json:"aria2Path"`
	RPCPort        int    `json:"rpcPort"`
	DownloadDir    string `json:"downloadDir"`
	AutoStartAria2 bool   `json:"autoStartAria2"`
	AutoStartApp   bool   `json:"autoStartApp"`
	CloseToTray    bool   `json:"closeToTray"`
	HistoryMax     int    `json:"historyMax"`
	MaxConcurrent  int    `json:"maxConcurrent"`
	// BrowserExtEnabled 浏览器插件联动开关：开启后在 20000~20005 监听本地推送服务
	BrowserExtEnabled bool `json:"browserExtEnabled"`
	// AutoShowOnPush 收到插件推送时自动把窗口显示到前台并切换到下载页
	AutoShowOnPush bool `json:"autoShowOnPush"`
}

const (
	DefaultHistoryMax = 1000
	MaxHistoryMax     = 5000
)

func DataDir() string {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		base = os.Getenv("APPDATA")
	}
	return filepath.Join(base, DirName)
}

func Aria2Dir() string {
	return filepath.Join(DataDir(), "aria2")
}

func SettingsPath() string {
	return filepath.Join(DataDir(), "settings.json")
}

// DefaultMaxConcurrent 同时下载数量默认值：CPU 核心数的一半，至少 1。
func DefaultMaxConcurrent() int {
	n := runtime.NumCPU() / 2
	if n < 1 {
		n = 1
	}
	return n
}

func Default() Settings {
	downloadDir := filepath.Join(os.Getenv("USERPROFILE"), "Downloads")
	return Settings{
		RPCPort:           6800,
		DownloadDir:       downloadDir,
		AutoStartAria2:    true,
		AutoStartApp:      false,
		CloseToTray:       true,
		HistoryMax:        DefaultHistoryMax,
		MaxConcurrent:     DefaultMaxConcurrent(),
		BrowserExtEnabled: true,
		AutoShowOnPush:    true,
	}
}

func Load() (Settings, error) {
	s := Default()
	if err := os.MkdirAll(DataDir(), 0o755); err != nil {
		return s, err
	}
	data, err := os.ReadFile(SettingsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, err
	}
	if s.RPCPort <= 0 || s.RPCPort > 65535 {
		s.RPCPort = 6800
	}
	if s.DownloadDir == "" {
		s.DownloadDir = Default().DownloadDir
	}
	if s.HistoryMax <= 0 || s.HistoryMax > MaxHistoryMax {
		s.HistoryMax = DefaultHistoryMax
	}
	if s.MaxConcurrent < 1 {
		s.MaxConcurrent = DefaultMaxConcurrent()
	}
	if s.MaxConcurrent > runtime.NumCPU() {
		s.MaxConcurrent = runtime.NumCPU()
	}
	return s, nil
}

func Save(s Settings) error {
	if err := os.MkdirAll(DataDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(SettingsPath(), data, 0o644)
}
