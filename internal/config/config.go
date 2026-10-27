package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const DirName = "AriaDesk"

type Settings struct {
	Aria2Path      string `json:"aria2Path"`
	RPCPort        int    `json:"rpcPort"`
	DownloadDir    string `json:"downloadDir"`
	AutoStartAria2 bool   `json:"autoStartAria2"`
	AutoStartApp   bool   `json:"autoStartApp"`
	CloseToTray    bool   `json:"closeToTray"`
}

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

func Default() Settings {
	downloadDir := filepath.Join(os.Getenv("USERPROFILE"), "Downloads")
	return Settings{
		RPCPort:        6800,
		DownloadDir:    downloadDir,
		AutoStartAria2: true,
		AutoStartApp:   false,
		CloseToTray:    true,
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
