//go:build darwin

package main

import "os/exec"

// getlantern/systray 与 Wails v2 在 macOS 上都会声明 AppDelegate，无法同时链接。
// macOS 版本因此使用正常关闭行为，Windows/Linux 版本仍可关闭到托盘。
func hideWindowOnClose() bool { return false }

func darwinOpenPath(path string, reveal bool) bool {
	args := []string{path}
	if reveal {
		args = []string{path}
	} else {
		args = []string{"-R", path}
	}
	return exec.Command("open", args...).Start() == nil
}
