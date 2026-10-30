package aria2

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func WingetAvailable() bool {
	_, err := exec.LookPath("winget")
	return err == nil
}

// decodeLocale 还原 winget 输出：中文 Windows 上 winget 使用 GBK 代码页，
// 直接按 UTF-8 显示会乱码。
func decodeLocale(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	if s, err := simplifiedchinese.GBK.NewDecoder().Bytes(b); err == nil {
		return string(s)
	}
	return string(b)
}

// Install 通过 winget 安装 aria2，每行输出通过 onLine 回调。
// --source winget 跳过 msstore 源：部分网络环境无法连接微软商店，
// 不指定源时 winget 会因 msstore 查询失败而中止。
// --force 让 winget 记录与实际文件不一致（如卸载残留）时也能强制重装自愈。
func Install(onLine func(line string)) error {
	return runWinget([]string{
		"install", "--id", "aria2.aria2", "--exact", "--source", "winget", "--force",
		"--accept-source-agreements", "--accept-package-agreements",
	}, "安装", onLine)
}

// Uninstall 通过 winget 卸载 aria2，每行输出通过 onLine 回调。
func Uninstall(onLine func(line string)) error {
	return runWinget([]string{
		"uninstall", "--id", "aria2.aria2", "--exact", "--source", "winget",
	}, "卸载", onLine)
}

func runWinget(args []string, op string, onLine func(line string)) error {
	if !WingetAvailable() {
		return fmt.Errorf("未找到 winget，请先安装微软商店中的“应用安装程序”")
	}
	cmd := newHiddenCmd("winget", args...)
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
		line := strings.TrimRight(decodeLocale(scanner.Bytes()), "\r")
		if onLine != nil {
			onLine(line)
		}
	}
	err = cmd.Wait()
	if err != nil {
		msg := strings.TrimSpace(decodeLocale(stderrBuf.Bytes()))
		if msg != "" {
			return fmt.Errorf("winget %s失败: %s", op, msg)
		}
		return fmt.Errorf("winget %s失败: %w", op, err)
	}
	return nil
}
