package tray

import (
	"encoding/binary"

	"github.com/energye/systray"
)

type Handlers struct {
	OnShow            func()
	OnStartAria2      func()
	OnStopAria2       func()
	OnOpenDownloadDir func()
	OnQuit            func()
}

var (
	mStart *systray.MenuItem
	mStop  *systray.MenuItem
)

// Start 在独立 goroutine 中启动托盘消息循环。
func Start(pngIcon []byte, h Handlers) {
	go systray.Run(func() {
		if len(pngIcon) > 0 {
			systray.SetIcon(pngToICO(pngIcon))
		}
		systray.SetTitle("AriaDesk")
		systray.SetTooltip("AriaDesk")
		mShow := systray.AddMenuItem("显示主窗口", "显示 AriaDesk 主窗口")
		systray.AddSeparator()
		mStart = systray.AddMenuItem("启动 aria2", "启动 aria2 下载服务")
		mStop = systray.AddMenuItem("停止 aria2", "停止 aria2 下载服务")
		systray.AddSeparator()
		mOpenDir := systray.AddMenuItem("打开下载目录", "在资源管理器中打开下载目录")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("退出", "退出 AriaDesk")

		mShow.Click(func() {
			if h.OnShow != nil {
				h.OnShow()
			}
		})
		mStart.Click(func() {
			if h.OnStartAria2 != nil {
				h.OnStartAria2()
			}
		})
		mStop.Click(func() {
			if h.OnStopAria2 != nil {
				h.OnStopAria2()
			}
		})
		mOpenDir.Click(func() {
			if h.OnOpenDownloadDir != nil {
				h.OnOpenDownloadDir()
			}
		})
		mQuit.Click(func() {
			if h.OnQuit != nil {
				h.OnQuit()
			}
		})
	}, func() {})
}

// SetRunning 根据 aria2 运行状态切换启动/停止菜单可用性。
func SetRunning(running bool) {
	if mStart == nil || mStop == nil {
		return
	}
	if running {
		mStart.Disable()
		mStop.Enable()
	} else {
		mStart.Enable()
		mStop.Disable()
	}
}

// pngToICO 将 PNG 数据包装为 ICO 容器（Windows Vista+ 支持 PNG 压缩图标）。
func pngToICO(png []byte) []byte {
	if len(png) < 24 || string(png[1:4]) != "PNG" {
		return png
	}
	w := binary.BigEndian.Uint32(png[16:20])
	h := binary.BigEndian.Uint32(png[20:24])
	ico := make([]byte, 6+16+len(png))
	binary.LittleEndian.PutUint16(ico[0:2], 0)
	binary.LittleEndian.PutUint16(ico[2:4], 1)
	binary.LittleEndian.PutUint16(ico[4:6], 1)
	if w < 256 {
		ico[6] = byte(w)
	}
	if h < 256 {
		ico[7] = byte(h)
	}
	binary.LittleEndian.PutUint16(ico[10:12], 1)
	binary.LittleEndian.PutUint16(ico[12:14], 32)
	binary.LittleEndian.PutUint32(ico[14:18], uint32(len(png)))
	binary.LittleEndian.PutUint32(ico[18:22], 6+16)
	copy(ico[22:], png)
	return ico
}
