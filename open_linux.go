//go:build linux

package main

func darwinOpenPath(_ string, _ bool) bool { return false }
