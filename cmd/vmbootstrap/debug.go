package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
)

var debugLogger *slog.Logger
var debugCleanup func()

func initDebugLogger() func() {
	if !debugLogs {
		return nil
	}
	logger, cleanup, err := setupDebugLogger("tmp/vmbootstrap-debug.log")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to enable debug log: %v\n", err)
		return nil
	}
	debugLogger = logger
	debugCleanup = cleanup
	fmt.Println("  Debug log: tmp/vmbootstrap-debug.log")
	return cleanup
}

func getLogger() *slog.Logger {
	if debugLogs && debugLogger != nil {
		return debugLogger
	}
	return newPrettyLogger(os.Stdout)
}

func setupDebugLogger(path string) (*slog.Logger, func(), error) {
	if err := os.MkdirAll("tmp", 0700); err != nil {
		return nil, nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, nil, err
	}
	mw := io.MultiWriter(os.Stdout, f)
	return newDebugLogger(mw), func() { _ = f.Close() }, nil
}
