package aria2

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ariadesk/internal/config"
)

type ReleaseInfo struct {
	Version     string `json:"version"`
	Prerelease  bool   `json:"prerelease"`
	PublishedAt string `json:"publishedAt"`
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type ghRelease struct {
	TagName     string    `json:"tag_name"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt string    `json:"published_at"`
	Assets      []ghAsset `json:"assets"`
}

var ghClient = &http.Client{Timeout: 30 * time.Second}

func ghGet(url string, out any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	// GitHub API 要求携带 User-Agent
	req.Header.Set("User-Agent", "AriaDesk")
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := ghClient.Do(req)
	if err != nil {
		return fmt.Errorf("无法连接 GitHub: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub 返回 %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// stripTag 去掉 tag 的 release- 前缀，得到纯版本号。
func stripTag(tag string) string {
	return strings.TrimPrefix(tag, "release-")
}

// FetchReleases 获取最近的 aria2 发布列表（含正式版与测试版标记）。
func FetchReleases(limit int) ([]ReleaseInfo, error) {
	if limit <= 0 {
		limit = 10
	}
	var releases []ghRelease
	url := fmt.Sprintf("https://api.github.com/repos/aria2/aria2/releases?per_page=%d", limit)
	if err := ghGet(url, &releases); err != nil {
		return nil, err
	}
	infos := make([]ReleaseInfo, 0, len(releases))
	for _, r := range releases {
		infos = append(infos, ReleaseInfo{
			Version:     stripTag(r.TagName),
			Prerelease:  r.Prerelease,
			PublishedAt: r.PublishedAt,
		})
	}
	return infos, nil
}

// CompareVersions 比较两个点分版本号，a>b 返回 1，a<b 返回 -1，相等返回 0。
func CompareVersions(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		va, vb := 0, 0
		if i < len(pa) {
			va = leadingInt(pa[i])
		}
		if i < len(pb) {
			vb = leadingInt(pb[i])
		}
		if va != vb {
			if va > vb {
				return 1
			}
			return -1
		}
	}
	return 0
}

// leadingInt 取段首的数字部分，容忍 "0rc1" 之类的后缀。
func leadingInt(s string) int {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	v, _ := strconv.Atoi(s[:i])
	return v
}

// VersionsDir 是 AriaDesk 托管的 aria2 版本目录，切换/回退版本即在此目录内解压。
func VersionsDir() string {
	return filepath.Join(config.Aria2Dir(), "versions")
}

// ManagedExePath 返回指定版本的托管 aria2c.exe 路径。
func ManagedExePath(version string) string {
	return filepath.Join(VersionsDir(), "aria2-"+version, "aria2c.exe")
}

// IsManagedPath 判断路径是否位于 AriaDesk 托管的版本目录内。
func IsManagedPath(path string) bool {
	if path == "" {
		return false
	}
	rel, err := filepath.Rel(VersionsDir(), path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// EnsureVersion 确保指定版本的 aria2c.exe 可用：优先复用已下载的缓存，
// 否则从 GitHub Release 下载 win-64bit 压缩包并解压，返回可执行文件路径。
// onProgress 回调进度文本与百分比，百分比未知（非下载阶段）时传 -1。
func EnsureVersion(version string, onProgress func(line string, percent int)) (string, error) {
	exe := ManagedExePath(version)
	if _, err := os.Stat(exe); err == nil {
		if v, verr := Version(exe); verr == nil && v == version {
			if onProgress != nil {
				onProgress(fmt.Sprintf("使用已下载的 aria2 %s", version), 100)
			}
			return exe, nil
		}
	}

	var rel ghRelease
	tag := "release-" + version
	if err := ghGet("https://api.github.com/repos/aria2/aria2/releases/tags/"+tag, &rel); err != nil {
		// 兜底：少数 tag 可能没有 release- 前缀
		if err2 := ghGet("https://api.github.com/repos/aria2/aria2/releases/tags/"+version, &rel); err2 != nil {
			return "", fmt.Errorf("查询版本 %s 失败: %w", version, err)
		}
	}

	var asset *ghAsset
	for i := range rel.Assets {
		name := rel.Assets[i].Name
		if strings.Contains(name, "win-64bit") && strings.HasSuffix(name, ".zip") {
			asset = &rel.Assets[i]
			break
		}
	}
	if asset == nil {
		return "", fmt.Errorf("版本 %s 没有 Windows 64 位安装包", version)
	}

	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		return "", err
	}
	tmp := exe + ".download"
	defer os.Remove(tmp)

	if onProgress != nil {
		onProgress(fmt.Sprintf("下载 %s", asset.Name), 0)
	}
	if err := download(asset.BrowserDownloadURL, tmp, onProgress); err != nil {
		return "", err
	}
	if err := extractAria2c(tmp, exe); err != nil {
		return "", err
	}
	if v, err := Version(exe); err != nil || v != version {
		os.Remove(exe)
		return "", fmt.Errorf("下载的文件校验失败（期望 %s，得到 %s）", version, v)
	}
	if onProgress != nil {
		onProgress(fmt.Sprintf("aria2 %s 已就绪", version), 100)
	}
	return exe, nil
}

func download(url, dest string, onProgress func(line string, percent int)) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "AriaDesk")
	// 下载大文件单独放宽超时：整体 10 分钟
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败: GitHub 返回 %s", resp.Status)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	total := resp.ContentLength
	var done int64
	var lastReport int64
	buf := make([]byte, 64*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return werr
			}
			done += int64(n)
			if onProgress != nil && done-lastReport >= 512*1024 {
				lastReport = done
				if total > 0 {
					pct := int(done * 100 / total)
					onProgress(fmt.Sprintf("下载中 %.1f / %.1f MB", float64(done)/(1<<20), float64(total)/(1<<20)), pct)
				} else {
					onProgress(fmt.Sprintf("下载中 %.1f MB", float64(done)/(1<<20)), -1)
				}
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return fmt.Errorf("下载中断: %w", rerr)
		}
	}
	return nil
}

// extractAria2c 从 aria2 官方 zip 中解出 aria2c.exe（静态编译，单文件即可运行）。
func extractAria2c(zipPath, dest string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("安装包损坏: %w", err)
	}
	defer r.Close()
	for _, f := range r.File {
		if filepath.Base(f.Name) != "aria2c.exe" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		tmp := dest + ".tmp"
		out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			os.Remove(tmp)
			return err
		}
		if err := out.Close(); err != nil {
			os.Remove(tmp)
			return err
		}
		return os.Rename(tmp, dest)
	}
	return fmt.Errorf("安装包中未找到 aria2c.exe")
}
