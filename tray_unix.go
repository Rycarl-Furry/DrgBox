//go:build linux || darwin

package main

import (
	"context"
	_ "embed"

	"github.com/getlantern/systray"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed build/appicon.png
var trayIconPNG []byte

func startTray(ctx context.Context) {
	systray.Run(func() {
		systray.SetIcon(trayIconPNG)
		systray.SetTitle("DRGBOX")
		systray.SetTooltip("DRGBOX · 本地分类工具箱")
		show := systray.AddMenuItem("显示 DRGBOX", "打开主界面")
		quit := systray.AddMenuItem("退出", "完全退出 DRGBOX")
		go func() {
			for {
				select {
				case <-show.ClickedCh:
					wailsruntime.WindowShow(ctx)
					wailsruntime.WindowUnminimise(ctx)
				case <-quit.ClickedCh:
					systray.Quit()
					wailsruntime.Quit(ctx)
					return
				}
			}
		}()
	}, func() {})
}
