package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func makeRuntimeFile(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("runtime fixture"), 0644); err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(path)
}

func TestRuntimeSettingsResolveDirectoriesAndPreserveHotkey(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "data"), 0755); err != nil {
		t.Fatal(err)
	}
	initial, _ := json.Marshal(appSettings{QuickHotkey: "Ctrl+Alt+Space"})
	if err := os.WriteFile(filepath.Join(root, "data", "settings.json"), initial, 0644); err != nil {
		t.Fatal(err)
	}

	python := makeRuntimeFile(t, filepath.Join(root, "Python", "python.exe"))
	java8 := makeRuntimeFile(t, filepath.Join(root, "Java8", "bin", "javaw.exe"))
	java11 := makeRuntimeFile(t, filepath.Join(root, "Java11", "bin", "java.exe"))
	a := &App{root: root}

	if err := a.SaveRuntimeSettings(RuntimeSettings{
		PythonPath: filepath.Dir(python),
		Java8Path:  filepath.Dir(filepath.Dir(java8)),
		Java11Path: filepath.Dir(java11),
	}); err != nil {
		t.Fatal(err)
	}

	got := a.GetRuntimeSettings()
	if got.PythonPath != python || got.Java8Path != java8 || got.Java11Path != java11 {
		t.Fatalf("resolved paths mismatch: %#v", got)
	}
	if hotkey := a.GetQuickHotkey(); hotkey != "Ctrl+Alt+Space" {
		t.Fatalf("hotkey was overwritten: %q", hotkey)
	}
	if resolved, err := a.runtimeExecutable("java11"); err != nil || resolved != java11 {
		t.Fatalf("runtimeExecutable(java11) = %q, %v", resolved, err)
	}
}

func TestRuntimeSettingsRejectMissingPath(t *testing.T) {
	root := t.TempDir()
	a := &App{root: root}
	if err := a.SaveRuntimeSettings(RuntimeSettings{PythonPath: filepath.Join(root, "missing")}); err == nil {
		t.Fatal("expected invalid Python path error")
	}
}
