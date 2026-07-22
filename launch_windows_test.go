//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConsoleCommandRunsFromChineseSpacedPath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "中文 目录 - tool")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "hello tool.cmd")
	if err := os.WriteFile(script, []byte("@echo DRGBOX_OK\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := commandPrompt(consoleCommand(script, ""), false)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "DRGBOX_OK") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestConsoleCommandRunsExeWithArguments(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "POC 扫描 - 工具")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	target := filepath.Join(dir, "sample tool.exe")
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, data, 0755); err != nil {
		t.Fatal(err)
	}
	cmd := commandPrompt(consoleCommand(target, "/d /c echo DRGBOX_EXE_OK"), false)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("exe command failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "DRGBOX_EXE_OK") {
		t.Fatalf("unexpected output: %q", out)
	}
}
