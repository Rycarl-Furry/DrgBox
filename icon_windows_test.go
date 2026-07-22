//go:build windows

package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractAssociatedIcon(t *testing.T) {
	source := filepath.Join(os.Getenv("WINDIR"), "System32", "notepad.exe")
	destination := filepath.Join(t.TempDir(), "notepad.png")
	if err := extractAssociatedIcon(source, destination); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	img, err := png.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 48 || img.Bounds().Dy() != 48 {
		t.Fatalf("unexpected icon size: %v", img.Bounds())
	}
}
