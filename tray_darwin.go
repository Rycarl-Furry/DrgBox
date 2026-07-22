//go:build darwin

package main

import "context"

// Wails v2 自带 macOS AppDelegate；第三方 systray 同样声明 AppDelegate 会造成
// duplicate symbol 链接失败，因此 macOS 发行版暂不启用托盘。
func startTray(_ context.Context) {}
