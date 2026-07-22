//go:build windows

package main

import (
	"context"
	_ "embed"

	"github.com/getlantern/systray"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed build/windows/icon.ico
var trayIcon []byte

// startTray 在 Windows 通知区域常驻 DRGBOX 图标。
func startTray(ctx context.Context) {
	systray.Run(func() {
		systray.SetIcon(trayIcon)
		systray.SetTooltip("DRGBOX · 本地分类工具箱")
		systray.SetOnClick(func() {
			wailsruntime.WindowShow(ctx)
			wailsruntime.WindowUnminimise(ctx)
		})
		show := systray.AddMenuItem("显示 DRGBOX", "唤出工具箱窗口")
		systray.AddSeparator()
		quit := systray.AddMenuItem("退出 DRGBOX", "完全退出程序")
		go func() {
			for {
				select {
				case <-show.ClickedCh:
					wailsruntime.WindowShow(ctx)
					wailsruntime.WindowUnminimise(ctx)
				case <-quit.ClickedCh:
					wailsruntime.Quit(ctx)
					systray.Quit()
					return
				}
			}
		}()
	}, func() {})
}
