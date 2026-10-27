package aria2

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

func WingetAvailable() bool {
	_, err := exec.LookPath("winget")
	return err == nil
}

// Install 通过 winget 安装 aria2，每行输出通过 onLine 回调。
func Install(onLine func(line string)) error {
	if !WingetAvailable() {
		return fmt.Errorf("未找到 winget，请先安装微软商店中的“应用安装程序”")
	}
	cmd := newHiddenCmd("winget",
		"install", "--id", "aria2.aria2", "--exact",
		"--accept-source-agreements", "--accept-package-agreements",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	var stderrBuf bytes.Buffer
	go func() {
		io.Copy(&stderrBuf, stderr)
	}()
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if onLine != nil {
			onLine(line)
		}
	}
	err = cmd.Wait()
	if err != nil {
		msg := strings.TrimSpace(stderrBuf.String())
		if msg != "" {
			return fmt.Errorf("winget 安装失败: %s", msg)
		}
		return fmt.Errorf("winget 安装失败: %w", err)
	}
	return nil
}
