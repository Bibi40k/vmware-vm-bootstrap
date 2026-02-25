//go:build windows
// +build windows

package main

// drainStdin is a no-op on Windows. survey's ANSI cursor queries are not used
// in the same way, and syscall non-blocking reads differ.
func drainStdin() {}

// restoreTTYOnExit is a no-op on Windows.
func restoreTTYOnExit() {}
