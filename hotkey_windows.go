//go:build windows

package main

import (
	"context"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	wmHotkey       = 0x0312
	wmHotkeyUpdate = 0x8001
	hotkeyID       = 0xF0B0
	modAlt         = 0x0001
	modControl     = 0x0002
	modShift       = 0x0004
	vkSpace        = 0x20
)

var (
	user32                 = syscall.NewLazyDLL("user32.dll")
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	registerHotKeyProc     = user32.NewProc("RegisterHotKey")
	unregisterHotKeyProc   = user32.NewProc("UnregisterHotKey")
	getMessageProc         = user32.NewProc("GetMessageW")
	postThreadMessageProc  = user32.NewProc("PostThreadMessageW")
	getCurrentThreadIDProc = kernel32.NewProc("GetCurrentThreadId")
	hotkeyMu               sync.RWMutex
	configuredModifiers    uintptr = modControl | modShift
	configuredVK           uintptr = vkSpace
	hotkeyThreadID         uint32
)

type winPoint struct{ X, Y int32 }
type winMessage struct {
	Hwnd           uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	Pt             winPoint
	LPrivate       uint32
}

func parseHotkey(combo string) (uintptr, uintptr, bool) {
	combo = strings.ToUpper(strings.ReplaceAll(combo, " ", ""))
	parts := strings.Split(combo, "+")
	var mods uintptr
	key := ""
	for _, p := range parts {
		switch p {
		case "CTRL":
			mods |= modControl
		case "ALT":
			mods |= modAlt
		case "SHIFT":
			mods |= modShift
		default:
			key = p
		}
	}
	if mods == 0 {
		return 0, 0, false
	}
	if key == "SPACE" {
		return mods, vkSpace, true
	}
	if len(key) == 1 && key[0] >= 'A' && key[0] <= 'Z' {
		return mods, uintptr(key[0]), true
	}
	return 0, 0, false
}

func updateQuickHotkey(combo string) bool {
	m, k, ok := parseHotkey(combo)
	if !ok {
		return false
	}
	hotkeyMu.Lock()
	configuredModifiers, configuredVK = m, k
	tid := hotkeyThreadID
	hotkeyMu.Unlock()
	if tid != 0 {
		postThreadMessageProc.Call(uintptr(tid), wmHotkeyUpdate, 0, 0)
	}
	return true
}

func showQuickLauncher(ctx context.Context) {
	wailsruntime.WindowSetSize(ctx, 820, 620)
	wailsruntime.WindowCenter(ctx)
	wailsruntime.WindowShow(ctx)
	wailsruntime.WindowUnminimise(ctx)
	wailsruntime.EventsEmit(ctx, "drgbox:quick-launcher")
	wailsruntime.WindowSetAlwaysOnTop(ctx, true)
	time.AfterFunc(180*time.Millisecond, func() { wailsruntime.WindowSetAlwaysOnTop(ctx, false) })
}

func registerToggleHotkey(ctx context.Context, combo string) {
	updateQuickHotkey(combo)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	tid, _, _ := getCurrentThreadIDProc.Call()
	hotkeyMu.Lock()
	hotkeyThreadID = uint32(tid)
	hotkeyMu.Unlock()
	defer func() { unregisterHotKeyProc.Call(0, hotkeyID); hotkeyMu.Lock(); hotkeyThreadID = 0; hotkeyMu.Unlock() }()
	register := func() {
		hotkeyMu.RLock()
		m, k := configuredModifiers, configuredVK
		hotkeyMu.RUnlock()
		registerHotKeyProc.Call(0, hotkeyID, m, k)
	}
	register()
	for {
		var msg winMessage
		ret, _, _ := getMessageProc.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ret) <= 0 {
			return
		}
		if msg.Message == wmHotkeyUpdate {
			unregisterHotKeyProc.Call(0, hotkeyID)
			register()
			continue
		}
		if msg.Message == wmHotkey && msg.WParam == hotkeyID {
			showQuickLauncher(ctx)
		}
	}
}
