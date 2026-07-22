//go:build windows

// DRGBOX 安装程序：单文件嵌入主程序，不依赖外部安装器。
package main

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

//go:embed DrgBoxDesktop.exe
var drgboxBinary []byte

const targetDir = `D:\Car1N0tCat\DRGBOX`

var (
	user32         = syscall.NewLazyDLL("user32.dll")
	messageBoxProc = user32.NewProc("MessageBoxW")
)

func messageBox(title, body string, flags uintptr) {
	t, _ := syscall.UTF16PtrFromString(body)
	c, _ := syscall.UTF16PtrFromString(title)
	messageBoxProc.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), flags)
}

func writeShortcut(exePath string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	desktop := filepath.Join(home, "Desktop")
	if _, err := os.Stat(desktop); err != nil {
		return err
	}
	// .url 是 Windows 原生快捷方式格式，避免额外安装 COM 依赖。
	content := fmt.Sprintf("[InternetShortcut]\r\nURL=file:///%s\r\nIconFile=%s\r\nIconIndex=0\r\n", strings.ReplaceAll(exePath, `\`, "/"), exePath)
	return os.WriteFile(filepath.Join(desktop, "DRGBOX 工具箱.url"), []byte(content), 0644)
}

func install() (string, error) {
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", err
	}
	exePath := filepath.Join(targetDir, "DRGBOX.exe")
	tmpPath := exePath + ".new"
	if err := os.WriteFile(tmpPath, drgboxBinary, 0755); err != nil {
		return "", err
	}
	if err := os.Rename(tmpPath, exePath); err != nil {
		_ = os.Remove(exePath)
		if err := os.Rename(tmpPath, exePath); err != nil {
			return "", err
		}
	}
	_ = writeShortcut(exePath)
	return exePath, nil
}

func main() {
	silent := len(os.Args) > 1 && strings.EqualFold(os.Args[1], "/S")
	if !silent {
		messageBox("DRGBOX 安装", "将安装 DRGBOX 到 D:\\Car1N0tCat\\DRGBOX，并在桌面创建快捷方式。", 0x00000001|0x00000040)
	}
	exePath, err := install()
	if err != nil {
		if !silent {
			messageBox("DRGBOX 安装失败", err.Error(), 0x00000010)
		}
		return
	}
	if err := exec.Command(exePath).Start(); err != nil && !silent {
		messageBox("DRGBOX 已安装", "安装完成，但启动失败："+err.Error(), 0x00000030)
		return
	}
	if !silent {
		messageBox("DRGBOX 已安装", "安装完成。已启动 DRGBOX，并创建桌面快捷方式。", 0x00000040)
	}
}
