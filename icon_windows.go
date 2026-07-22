//go:build windows

package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	shgfiIcon      = 0x000000100
	shgfiLargeIcon = 0x000000000
	dibRGBColors   = 0
	diNormal       = 0x0003
	swShowNormal   = 1
)

var (
	iconShell32 = syscall.NewLazyDLL("shell32.dll")
	iconUser32  = syscall.NewLazyDLL("user32.dll")
	iconGDI32   = syscall.NewLazyDLL("gdi32.dll")

	shGetFileInfoW   = iconShell32.NewProc("SHGetFileInfoW")
	shellExecuteW    = iconShell32.NewProc("ShellExecuteW")
	destroyIcon      = iconUser32.NewProc("DestroyIcon")
	getDC            = iconUser32.NewProc("GetDC")
	releaseDC        = iconUser32.NewProc("ReleaseDC")
	drawIconEx       = iconUser32.NewProc("DrawIconEx")
	createCompatible = iconGDI32.NewProc("CreateCompatibleDC")
	deleteDC         = iconGDI32.NewProc("DeleteDC")
	createDIBSection = iconGDI32.NewProc("CreateDIBSection")
	selectObject     = iconGDI32.NewProc("SelectObject")
	deleteObject     = iconGDI32.NewProc("DeleteObject")
)

type shellFileInfo struct {
	HIcon       uintptr
	IIcon       int32
	Attributes  uint32
	DisplayName [260]uint16
	TypeName    [80]uint16
}

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type bitmapInfo struct {
	Header bitmapInfoHeader
	Colors [1]uint32
}

// extractAssociatedIcon 读取 EXE 自带图标，其他类型则读取 Windows 文件关联图标。
func extractAssociatedIcon(source, destination string) error {
	path, err := syscall.UTF16PtrFromString(filepath.Clean(source))
	if err != nil {
		return err
	}
	var info shellFileInfo
	ret, _, callErr := shGetFileInfoW.Call(
		uintptr(unsafe.Pointer(path)),
		0,
		uintptr(unsafe.Pointer(&info)),
		unsafe.Sizeof(info),
		shgfiIcon|shgfiLargeIcon,
	)
	if ret == 0 || info.HIcon == 0 {
		return fmt.Errorf("无法读取系统图标：%v", callErr)
	}
	defer destroyIcon.Call(info.HIcon)

	const size = 48
	screenDC, _, _ := getDC.Call(0)
	if screenDC == 0 {
		return fmt.Errorf("无法获取屏幕绘图上下文")
	}
	defer releaseDC.Call(0, screenDC)
	memDC, _, _ := createCompatible.Call(screenDC)
	if memDC == 0 {
		return fmt.Errorf("无法创建图标绘图上下文")
	}
	defer deleteDC.Call(memDC)

	bmi := bitmapInfo{Header: bitmapInfoHeader{
		Size:     uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		Width:    size,
		Height:   -size,
		Planes:   1,
		BitCount: 32,
	}}
	var bits uintptr
	bitmap, _, _ := createDIBSection.Call(memDC, uintptr(unsafe.Pointer(&bmi)), dibRGBColors, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bitmap == 0 || bits == 0 {
		return fmt.Errorf("无法创建图标位图")
	}
	defer deleteObject.Call(bitmap)
	old, _, _ := selectObject.Call(memDC, bitmap)
	defer selectObject.Call(memDC, old)
	if ok, _, _ := drawIconEx.Call(memDC, 0, 0, info.HIcon, size, size, 0, 0, diNormal); ok == 0 {
		return fmt.Errorf("无法绘制系统图标")
	}

	raw := unsafe.Slice((*byte)(unsafe.Pointer(bits)), size*size*4)
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	hasAlpha := false
	for i := 0; i < size*size; i++ {
		if raw[i*4+3] != 0 {
			hasAlpha = true
			break
		}
	}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			i := (y*size + x) * 4
			a := raw[i+3]
			if !hasAlpha {
				if raw[i] != 0 || raw[i+1] != 0 || raw[i+2] != 0 {
					a = 255
				}
			}
			img.SetNRGBA(x, y, color.NRGBA{R: raw[i+2], G: raw[i+1], B: raw[i], A: a})
		}
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return err
	}
	out, err := os.Create(destination)
	if err != nil {
		return err
	}
	if err := png.Encode(out, img); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func shellRunAs(file, parameters, directory string) error {
	verb, _ := syscall.UTF16PtrFromString("runas")
	filePtr, err := syscall.UTF16PtrFromString(file)
	if err != nil {
		return err
	}
	paramsPtr, _ := syscall.UTF16PtrFromString(parameters)
	dirPtr, _ := syscall.UTF16PtrFromString(directory)
	ret, _, callErr := shellExecuteW.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(filePtr)), uintptr(unsafe.Pointer(paramsPtr)), uintptr(unsafe.Pointer(dirPtr)), swShowNormal)
	if ret <= 32 {
		return fmt.Errorf("管理员启动失败（代码 %d）：%v", ret, callErr)
	}
	return nil
}

func quoteWindowsArg(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}

func (a *App) RunToolAsAdmin(id string) (string, error) {
	items, err := a.GetTools()
	if err != nil {
		return "", err
	}
	for i := range items {
		if items[i].ID != id {
			continue
		}
		tool := &items[i]
		info, err := os.Stat(tool.Path)
		if err != nil {
			return "", fmt.Errorf("启动路径不存在：%w", err)
		}
		if info.IsDir() {
			return "", fmt.Errorf("目录不需要管理员方式运行")
		}
		dir, typ := filepath.Dir(tool.Path), strings.ToLower(tool.Type)
		file, params := tool.Path, strings.TrimSpace(tool.Args)
		switch typ {
		case "python":
			file, err = a.runtimeExecutable("python")
			if err != nil {
				return "", err
			}
			params = quoteWindowsArg(tool.Path) + " " + params
		case "java8", "java11":
			file, err = a.runtimeExecutable(typ)
			if err != nil {
				return "", err
			}
			params = "-jar " + quoteWindowsArg(tool.Path) + " " + params
		case "批处理":
			file = "cmd.exe"
			params = `/d /k call ` + quoteWindowsArg(tool.Path) + " " + params
		case "命令行":
			file = "cmd.exe"
			params = `/d /k ` + consoleCommand(tool.Path, params)
		case "powershell":
			file = "powershell.exe"
			params = `-NoExit -ExecutionPolicy Bypass -File ` + quoteWindowsArg(tool.Path) + " " + params
		}
		if err := shellRunAs(file, strings.TrimSpace(params), dir); err != nil {
			return "", err
		}
		items[i].LastRun = time.Now().Unix()
		_ = a.saveTools(items)
		return items[i].Name, nil
	}
	return "", fmt.Errorf("未找到工具：%s", id)
}
