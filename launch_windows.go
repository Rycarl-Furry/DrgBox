//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func runtimeExecutableFilters() []runtime.FileFilter {
	return []runtime.FileFilter{{DisplayName: "可执行文件", Pattern: "*.exe;*.cmd;*.bat"}}
}

func hideWindowOnClose() bool { return true }

func openPathLocation(path string, isDir bool) error {
	if isDir {
		return exec.Command("explorer.exe", path).Start()
	}
	return exec.Command("explorer.exe", "/select,", path).Start()
}

// consoleCommand 生成交给 cmd.exe /k 的单条命令。工作目录由 exec.Cmd.Dir 设置，
// 避免 start、cd 和多层引号在中文/空格路径下被二次解析。
func consoleCommand(executable, arguments string) string {
	command := `call "` + strings.ReplaceAll(filepath.Clean(executable), `"`, `""`) + `"`
	if arguments = strings.TrimSpace(arguments); arguments != "" {
		command += " " + arguments
	}
	return command
}

// commandPrompt 使用 Windows 原生命令行字符串，避免 os/exec 再把内部引号转义。
func commandPrompt(command string, keepOpen bool) *exec.Cmd {
	mode := "/c"
	if keepOpen {
		mode = "/k"
	}
	cmd := exec.Command("cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: "cmd.exe /d " + mode + " " + command}
	return cmd
}

func (a *App) launch(t *Tool) error {
	info, err := os.Stat(t.Path)
	if err != nil {
		return fmt.Errorf("启动路径不存在：%w", err)
	}
	if info.IsDir() {
		return exec.Command("cmd.exe", "/c", "start", "", filepath.Clean(t.Path)).Start()
	}
	args := strings.Fields(t.Args)
	dir := filepath.Dir(t.Path)
	typ := strings.ToLower(t.Type)
	var cmd *exec.Cmd
	switch typ {
	case "python":
		py, runtimeErr := a.runtimeExecutable("python")
		if runtimeErr != nil {
			return runtimeErr
		}
		pythonArgs := `"` + strings.ReplaceAll(filepath.Clean(t.Path), `"`, `""`) + `"`
		if strings.TrimSpace(t.Args) != "" {
			pythonArgs += " " + strings.TrimSpace(t.Args)
		}
		cmd = commandPrompt(consoleCommand(py, pythonArgs), true)
	case "java8", "java11":
		java, runtimeErr := a.runtimeExecutable(typ)
		if runtimeErr != nil {
			return runtimeErr
		}
		cmd = exec.Command(java, append([]string{"-jar", t.Path}, args...)...)
	case "批处理":
		command := fmt.Sprintf(`call "%s"`, t.Path)
		if strings.TrimSpace(t.Args) != "" {
			command += " " + t.Args
		}
		cmd = commandPrompt(command, true)
	case "命令行":
		cmd = commandPrompt(consoleCommand(t.Path, t.Args), true)
	case "powershell":
		psArgs := []string{"-NoExit", "-ExecutionPolicy", "Bypass", "-File", t.Path}
		psArgs = append(psArgs, args...)
		cmd = exec.Command("powershell.exe", psArgs...)
	default:
		cmd = exec.Command(t.Path, args...)
	}
	cmd.Dir = dir
	return cmd.Start()
}
