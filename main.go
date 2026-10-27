package main

import (
	"embed"
	"os"
	"unsafe"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"golang.org/x/sys/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

var (
	kernel32             = windows.NewLazySystemDLL("kernel32.dll")
	user32               = windows.NewLazySystemDLL("user32.dll")
	procCreateMutexW     = kernel32.NewProc("CreateMutexW")
	procFindWindowW      = user32.NewProc("FindWindowW")
	procShowWindow       = user32.NewProc("ShowWindow")
	procSetForegroundWnd = user32.NewProc("SetForegroundWindow")
)

func ensureSingleInstance() bool {
	name, _ := windows.UTF16PtrFromString(`Global\AriaDeskSingleInstanceMutex`)
	handle, _, err := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if handle == 0 {
		return true // 创建失败不阻塞启动
	}
	const errorAlreadyExists = 183
	if errno, ok := err.(windows.Errno); ok && errno == errorAlreadyExists {
		return false
	}
	return true
}

func activateExistingWindow() {
	title, _ := windows.UTF16PtrFromString("AriaDesk")
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(title)))
	if hwnd == 0 {
		return
	}
	const swRestore = 9
	procShowWindow.Call(hwnd, swRestore)
	procSetForegroundWnd.Call(hwnd)
}

func main() {
	if !ensureSingleInstance() {
		activateExistingWindow()
		return
	}

	app := NewApp(appIcon)

	minimized := false
	for _, arg := range os.Args[1:] {
		if arg == "--minimized" {
			minimized = true
			break
		}
	}

	err := wails.Run(&options.App{
		Title:     "AriaDesk",
		Width:     1280,
		Height:    800,
		MinWidth:  960,
		MinHeight: 640,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		StartHidden:      minimized,
		OnStartup:        app.startup,
		OnShutdown:       app.onShutdown,
		OnBeforeClose:    app.onBeforeClose,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
