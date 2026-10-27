package aria2

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"ariadesk/internal/config"
)

// BuildArgs 按设计文档 §4.2 生成启动参数（不使用 --daemon）。
func BuildArgs(s config.Settings, token string) []string {
	aria2Dir := config.Aria2Dir()
	sessionFile := filepath.Join(aria2Dir, "session.txt")
	logFile := filepath.Join(aria2Dir, "aria2.log")
	return []string{
		"--enable-rpc",
		fmt.Sprintf("--rpc-listen-port=%d", s.RPCPort),
		"--rpc-secret=" + token,
		"--rpc-allow-origin-all=true",
		"--rpc-listen-all=false",
		"--dir=" + s.DownloadDir,
		"--input-file=" + sessionFile,
		"--save-session=" + sessionFile,
		"--save-session-interval=60",
		"--log=" + logFile,
		"--log-level=warn",
		"--max-concurrent-downloads=" + fmt.Sprintf("%d", s.MaxConcurrent),
		"--seed-time=0",
	}
}

func EnsureRuntimeFiles() error {
	aria2Dir := config.Aria2Dir()
	if err := os.MkdirAll(aria2Dir, 0o755); err != nil {
		return err
	}
	sessionFile := filepath.Join(aria2Dir, "session.txt")
	if _, err := os.Stat(sessionFile); os.IsNotExist(err) {
		if err := os.WriteFile(sessionFile, nil, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// PortInUse 检测本机端口是否已被占用。
func PortInUse(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

type Process struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	onExit func()
}

func NewProcess() *Process {
	return &Process{}
}

func (p *Process) Running() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cmd != nil && p.cmd.Process != nil
}

func (p *Process) PID() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd == nil || p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

func (p *Process) SetOnExit(fn func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onExit = fn
}

func (p *Process) Start(exePath string, args []string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil && p.cmd.Process != nil {
		return fmt.Errorf("aria2 已在运行 (PID %d)", p.cmd.Process.Pid)
	}
	cmd := newHiddenCmd(exePath, args...)
	cmd.Dir = config.Aria2Dir()
	if err := cmd.Start(); err != nil {
		return err
	}
	p.cmd = cmd
	go func() {
		cmd.Wait()
		p.mu.Lock()
		fn := p.onExit
		if p.cmd == cmd {
			p.cmd = nil
		}
		p.mu.Unlock()
		if fn != nil {
			fn()
		}
	}()
	return nil
}

// Stop 先 RPC 优雅停止，超时后 Kill 兜底。
func (p *Process) Stop(shutdown func() error, timeout time.Duration) error {
	p.mu.Lock()
	cmd := p.cmd
	p.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if shutdown != nil {
		_ = shutdown()
	}
	deadline := time.After(timeout)
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			_ = cmd.Process.Kill()
			return nil
		case <-tick.C:
			p.mu.Lock()
			done := p.cmd == nil
			p.mu.Unlock()
			if done {
				return nil
			}
		}
	}
}

func (p *Process) Kill() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
}
