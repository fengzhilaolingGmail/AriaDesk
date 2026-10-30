package aria2

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"ariadesk/internal/config"
)

// ConfPath 返回 aria2 配置文件路径。
func ConfPath() string {
	return filepath.Join(config.Aria2Dir(), "aria2.conf")
}

// defaultConfTpl 是内置的默认配置模板，{{KEY}} 占位符由应用设置渲染。
// 模板内容参考 https://aria2.org/zh/config-generator/ 生成器输出。
const defaultConfTemplate = `# aria2.conf — 由 AriaDesk 生成与管理
# 以 # 开头的行是注释。key=value 等同于 aria2c --key=value。
# 注意：dir、rpc-listen-port、rpc-secret、max-concurrent-downloads、
# input-file、save-session、log 由 AriaDesk 统一管理，保存应用设置时自动覆盖。

# ── 文件 ──
# 下载完成的文件保存位置。
dir={{DIR}}
# 继续下载未完成的文件，而不是从头开始。
continue=true
# 写入前如何预留磁盘空间：none、prealloc、trunc 或 falloc。
file-allocation=falloc

# ── 会话 ──
# 重启后继续未完成的下载。文件不存在时 aria2 无法启动，AriaDesk 会自动创建。
input-file={{SESSION}}
save-session={{SESSION}}
save-session-interval=60

# ── 日志 ──
log={{LOG}}
log-level=warn

# ── 速度 ──
# 同时进行的下载数。
max-concurrent-downloads={{MAX_CONCURRENT}}
# 每个下载对同一服务器的连接数（1 到 16）。
max-connection-per-server=16
# 每个文件切成多少块并行下载。
split=16
# aria2 切出的最小分块。
min-split-size=4M
# 总速度限制，单位为字节/秒。0 表示不限速；可用 K 和 M（5M = 5 MiB/s）。
max-overall-download-limit=0
max-overall-upload-limit=0
# 用于合并磁盘写入的内存。0 表示关闭缓存。
disk-cache=32M

# ── 远程控制 (JSON-RPC) ──
enable-rpc=true
# RPC 密钥，请勿泄露。
rpc-secret={{SECRET}}
# 允许基于浏览器的 WebUI 访问 aria2。
rpc-allow-origin-all=true
# 只接受本机的连接。
rpc-listen-all=false
rpc-listen-port={{PORT}}

# ── BitTorrent ──
# DHT、节点交换和本地节点发现可以在没有 Tracker 时找到节点。PT 站请关闭。
enable-dht=true
enable-peer-exchange=true
bt-enable-lpd=true
# 不做种，下载完成即停止任务。
seed-time=0
# 额外的公共 Tracker（ngosang/trackerslist，best 列表）。最新列表：https://aria2.org/zh/trackers/
bt-tracker=udp://tracker.opentrackr.org:1337/announce,udp://open.stealth.si:80/announce,udp://open.demonii.com:1337/announce,udp://open.tracker.cl:1337/announce,udp://exodus.desync.com:6969/announce,udp://tracker.torrent.eu.org:451/announce,udp://tracker.skynetcloud.site:6969/announce,udp://tracker.qu.ax:6969/announce,udp://tracker.corpscorp.online:80/announce,udp://tracker-udp.gbitt.info:80/announce,udp://tracker.bittor.pw:1337/announce,udp://retracker01-msk-virt.corbina.net:80/announce,udp://tracker.tryhackx.org:6969/announce,udp://explodie.org:6969/announce,udp://tracker.ducks.party:1984/announce,udp://tracker2.dler.org:80/announce,udp://tracker.dler.org:6969/announce,http://tracker.dler.com:6969/announce,udp://tracker.nyaa.vc:6969/announce,udp://tracker.peerfect.org:6969/announce
`

// managedKeys 由应用设置统一控制的配置项，SyncConf 会用当前设置覆盖这些键。
var managedKeys = []string{
	"dir",
	"input-file",
	"save-session",
	"log",
	"log-level",
	"enable-rpc",
	"rpc-secret",
	"rpc-allow-origin-all",
	"rpc-listen-all",
	"rpc-listen-port",
	"max-concurrent-downloads",
}

var confKeyRe = regexp.MustCompile(`^\s*([a-zA-Z0-9-]+)\s*=`)

func managedValues(s config.Settings, token string) map[string]string {
	aria2Dir := config.Aria2Dir()
	session := filepath.Join(aria2Dir, "session.txt")
	return map[string]string{
		"dir":                      s.DownloadDir,
		"input-file":               session,
		"save-session":             session,
		"log":                      filepath.Join(aria2Dir, "aria2.log"),
		"log-level":                "warn",
		"enable-rpc":               "true",
		"rpc-secret":               token,
		"rpc-allow-origin-all":     "true",
		"rpc-listen-all":           "false",
		"rpc-listen-port":          fmt.Sprintf("%d", s.RPCPort),
		"max-concurrent-downloads": fmt.Sprintf("%d", s.MaxConcurrent),
	}
}

// renderDefault 用当前设置渲染默认模板。
func renderDefault(s config.Settings, token string) string {
	v := managedValues(s, token)
	return strings.NewReplacer(
		"{{DIR}}", v["dir"],
		"{{SESSION}}", v["input-file"],
		"{{LOG}}", v["log"],
		"{{MAX_CONCURRENT}}", v["max-concurrent-downloads"],
		"{{SECRET}}", v["rpc-secret"],
		"{{PORT}}", v["rpc-listen-port"],
	).Replace(defaultConfTemplate)
}

// SyncConf 确保配置文件存在，并将托管键同步为当前应用设置的值，
// 其余用户自定义内容保持不变。应用每次启动 aria2 前调用。
func SyncConf(s config.Settings, token string) error {
	path := ConfPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return os.WriteFile(path, []byte(renderDefault(s, token)), 0o644)
	}
	if err != nil {
		return err
	}

	values := managedValues(s, token)
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	seen := make(map[string]bool, len(managedKeys))
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		m := confKeyRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if v, ok := values[m[1]]; ok {
			lines[i] = m[1] + "=" + v
			seen[m[1]] = true
		}
	}
	var missing []string
	for _, k := range managedKeys {
		if !seen[k] {
			missing = append(missing, k+"="+values[k])
		}
	}
	if len(missing) > 0 {
		lines = append(lines, "", "# ── AriaDesk 托管项（由应用设置统一管理） ──")
		lines = append(lines, missing...)
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

// ReadConf 读取配置文件内容。
func ReadConf() (string, error) {
	data, err := os.ReadFile(ConfPath())
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// WriteConf 覆盖写入配置文件内容。
func WriteConf(content string) error {
	if err := os.MkdirAll(config.Aria2Dir(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(ConfPath(), []byte(content), 0o644)
}

// ConfItem 是配置文件中的一个键值对，Comment 为紧邻上方的注释（作为字段说明）。
type ConfItem struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Comment string `json:"comment"`
}

// ConfSection 是按 "# ── 名称 ──" 注释划分的配置分区。
type ConfSection struct {
	Name  string     `json:"name"`
	Items []ConfItem `json:"items"`
}

var confSectionRe = regexp.MustCompile(`^#\s*──\s*(.+?)\s*──\s*$`)

// ParseConf 将配置内容解析为分区结构，用于配置页卡片展示。
func ParseConf(content string) []ConfSection {
	sections := []ConfSection{}
	var comments []string
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			comments = nil
			continue
		}
		if m := confSectionRe.FindStringSubmatch(trimmed); m != nil {
			sections = append(sections, ConfSection{Name: m[1]})
			comments = nil
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			comments = append(comments, strings.TrimSpace(strings.TrimPrefix(trimmed, "#")))
			continue
		}
		m := confKeyRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		value := strings.TrimSpace(line[strings.Index(line, "=")+1:])
		if len(sections) == 0 {
			sections = append(sections, ConfSection{Name: "常规"})
		}
		sec := &sections[len(sections)-1]
		sec.Items = append(sec.Items, ConfItem{Key: m[1], Value: value, Comment: strings.Join(comments, "\n")})
		comments = nil
	}
	return sections
}

// UpdateConfValues 就地更新配置文件中指定键的值，保留注释与其它内容；
// 文件中不存在的键追加到文件末尾。
func UpdateConfValues(values map[string]string) error {
	data, err := os.ReadFile(ConfPath())
	if err != nil {
		return err
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	seen := make(map[string]bool, len(values))
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		m := confKeyRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if v, ok := values[m[1]]; ok {
			lines[i] = m[1] + "=" + v
			seen[m[1]] = true
		}
	}
	var missing []string
	for k, v := range values {
		if !seen[k] {
			missing = append(missing, k+"="+v)
		}
	}
	if len(missing) > 0 {
		lines = append(lines, "", "# ── 自定义 ──")
		lines = append(lines, missing...)
	}
	return os.WriteFile(ConfPath(), []byte(strings.Join(lines, "\n")), 0o644)
}

// ResetConf 用当前设置重新生成默认配置，覆盖现有文件。
func ResetConf(s config.Settings, token string) error {
	if err := os.MkdirAll(config.Aria2Dir(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(ConfPath(), []byte(renderDefault(s, token)), 0o644)
}

// trackersURL 是公共 Tracker 列表源（ngosang/trackerslist 的 best 列表，jsdelivr CDN）。
const trackersURL = "https://cdn.jsdelivr.net/gh/ngosang/trackerslist@master/trackers_best.txt"

// FetchTrackers 拉取最新公共 Tracker 列表，返回 Tracker 地址数组。
func FetchTrackers() ([]string, error) {
	req, err := http.NewRequest(http.MethodGet, trackersURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "AriaDesk")
	resp, err := ghClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("无法连接 Tracker 列表源: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取 Tracker 列表失败: %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var trackers []string
	for _, f := range strings.Fields(string(body)) {
		if strings.Contains(f, "://") {
			trackers = append(trackers, f)
		}
	}
	if len(trackers) == 0 {
		return nil, fmt.Errorf("Tracker 列表为空")
	}
	return trackers, nil
}
