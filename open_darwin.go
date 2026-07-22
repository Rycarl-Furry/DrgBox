//go:build darwin

package main

import "os/exec"

func darwinOpenPath(path string, reveal bool) bool {
	args := []string{path}
	if reveal {
		args = []string{path}
	} else {
		args = []string{"-R", path}
	}
	return exec.Command("open", args...).Start() == nil
}
