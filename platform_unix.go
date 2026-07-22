//go:build linux || darwin

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func runtimeExecutableFilters() []runtime.FileFilter {
	return []runtime.FileFilter{{DisplayName: "可执行文件", Pattern: "*"}}
}

// Wails v2 没有统一的跨平台全局快捷键 API。Linux/macOS 版本保留设置，
// 快捷启动入口通过托盘打开；Windows 版本仍使用原生 RegisterHotKey。
func registerToggleHotkey(_ context.Context, _ string) {}
func updateQuickHotkey(combo string) bool              { return strings.TrimSpace(combo) != "" }

func extractAssociatedIcon(_, _ string) error {
	return fmt.Errorf("当前平台不支持自动提取关联图标，请设置自定义图标")
}

func (a *App) RunToolAsAdmin(_ string) (string, error) {
	return "", fmt.Errorf("当前平台不提供应用内提权启动，请从终端使用 sudo 运行需要权限的工具")
}

func openPathLocation(path string, isDir bool) error {
	if darwinOpenPath(path, isDir) {
		return nil
	}
	target := path
	if !isDir {
		target = filepath.Dir(path)
	}
	return exec.Command("xdg-open", target).Start()
}

func (a *App) launch(t *Tool) error {
	info, err := os.Stat(t.Path)
	if err != nil {
		return fmt.Errorf("启动路径不存在：%w", err)
	}
	if info.IsDir() {
		return openPathLocation(filepath.Clean(t.Path), true)
	}

	args := strings.Fields(t.Args)
	dir := filepath.Dir(t.Path)
	typ := strings.ToLower(t.Type)
	var cmd *exec.Cmd
	switch typ {
	case "python":
		python, runtimeErr := a.runtimeExecutable("python")
		if runtimeErr != nil {
			return runtimeErr
		}
		cmd = exec.Command(python, append([]string{t.Path}, args...)...)
	case "java8", "java11":
		java, runtimeErr := a.runtimeExecutable(typ)
		if runtimeErr != nil {
			return runtimeErr
		}
		cmd = exec.Command(java, append([]string{"-jar", t.Path}, args...)...)
	case "批处理":
		cmd = exec.Command("sh", append([]string{t.Path}, args...)...)
	case "powershell":
		cmd = exec.Command("pwsh", append([]string{"-File", t.Path}, args...)...)
	default:
		cmd = exec.Command(t.Path, args...)
	}
	cmd.Dir = dir
	return cmd.Start()
}
