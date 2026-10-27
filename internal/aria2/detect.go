package aria2

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type DetectResult struct {
	Installed bool   `json:"installed"`
	Version   string `json:"version"`
	Path      string `json:"path"`
}

var versionRe = regexp.MustCompile(`aria2 version\s+(\S+)`)

func candidatePaths() []string {
	var paths []string
	local := os.Getenv("LOCALAPPDATA")
	if local != "" {
		base := filepath.Join(local, "Microsoft", "WinGet", "Packages")
		patterns := []string{
			filepath.Join(base, "aria2.aria2_*", "aria2c.exe"),
			filepath.Join(base, "aria2.aria2_*", "*", "aria2c.exe"),
		}
		for _, p := range patterns {
			if matches, err := filepath.Glob(p); err == nil {
				paths = append(paths, matches...)
			}
		}
		paths = append(paths, filepath.Join(local, "Microsoft", "WinGet", "Links", "aria2c.exe"))
	}
	paths = append(paths,
		filepath.Join(os.Getenv("ProgramFiles"), "aria2", "aria2c.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "aria2", "aria2c.exe"),
	)
	return paths
}

// Detect 依次尝试：缓存路径、PATH、winget Packages、Links shim、Program Files。
func Detect(cachedPath string) DetectResult {
	if cachedPath != "" {
		if r := check(cachedPath); r.Installed {
			return r
		}
	}
	if p, err := exec.LookPath("aria2c"); err == nil {
		if r := check(p); r.Installed {
			return r
		}
	}
	for _, p := range candidatePaths() {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		if r := check(p); r.Installed {
			return r
		}
	}
	return DetectResult{Installed: false}
}

func check(path string) DetectResult {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	version, err := Version(abs)
	if err != nil {
		return DetectResult{Installed: false, Path: abs}
	}
	return DetectResult{Installed: true, Version: version, Path: abs}
}

func Version(path string) (string, error) {
	cmd := exec.Command(path, "--version")
	hideWindow(cmd)
	type result struct {
		out []byte
		err error
	}
	ch := make(chan result, 1)
	go func() {
		out, err := cmd.Output()
		ch <- result{out, err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			return "", r.err
		}
		m := versionRe.FindSubmatch(r.out)
		if m == nil {
			lines := strings.SplitN(string(r.out), "\n", 2)
			return strings.TrimSpace(lines[0]), nil
		}
		return string(m[1]), nil
	case <-time.After(10 * time.Second):
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		return "", os.ErrDeadlineExceeded
	}
}
