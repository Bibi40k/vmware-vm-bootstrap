package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"unicode"
)

const (
	clrReset  = "\033[0m"
	clrDim    = "\033[2m"
	clrBold   = "\033[1m"
	clrRed    = "\033[31m"
	clrYellow = "\033[33m"
	clrGreen  = "\033[32m"
	clrCyan   = "\033[36m"
	clrGray   = "\033[90m"
	clrWhite  = "\033[97m"
)

// prettyHandler is a slog.Handler that formats log records with ANSI colors.
// Designed for CLI output: no timestamps, colored level indicators, highlighted values.
type prettyHandler struct {
	mu    sync.Mutex
	out   io.Writer
	level slog.Level
	attrs []slog.Attr // pre-set attrs from WithAttrs
}

func newPrettyLogger(w io.Writer) *slog.Logger {
	return slog.New(&prettyHandler{out: w, level: slog.LevelInfo})
}

func (h *prettyHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *prettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)
	return &prettyHandler{out: h.out, level: h.level, attrs: newAttrs}
}

func (h *prettyHandler) WithGroup(_ string) slog.Handler {
	return h
}

func (h *prettyHandler) Handle(_ context.Context, r slog.Record) error {
	var prefix, msgColor string
	switch r.Level {
	case slog.LevelInfo:
		prefix = clrGray + "  → " + clrReset
		msgColor = clrWhite
	case slog.LevelWarn:
		prefix = clrYellow + "  ⚠ " + clrReset
		msgColor = clrYellow
	case slog.LevelError:
		prefix = clrRed + "  ✗ " + clrReset
		msgColor = clrRed
	default:
		prefix = clrGray + "  · " + clrReset
		msgColor = clrGray
	}

	var sb strings.Builder
	sb.WriteString(prefix)
	sb.WriteString(msgColor)
	sb.WriteString(clrBold)
	sb.WriteString(r.Message)
	sb.WriteString(clrReset)

	writeAttr := func(a slog.Attr) bool {
		sb.WriteString("  ")
		sb.WriteString(clrGray)
		sb.WriteString(a.Key)
		sb.WriteString("=")
		sb.WriteString(clrReset)
		sb.WriteString(colorForValue(a))
		sb.WriteString(a.Value.String())
		sb.WriteString(clrReset)
		return true
	}

	for _, a := range h.attrs {
		writeAttr(a)
	}
	r.Attrs(writeAttr)

	sb.WriteString("\n")

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := fmt.Fprint(h.out, sb.String())
	return err
}

// colorForValue picks an ANSI color based on the attribute key and value.
func colorForValue(a slog.Attr) string {
	if a.Key == "error" {
		return clrRed
	}
	val := a.Value.String()
	// Paths and ISO filenames
	if strings.Contains(val, "/") || strings.HasSuffix(val, ".iso") || strings.HasSuffix(val, ".yaml") {
		return clrCyan
	}
	// IP addresses, hostnames, IDs
	switch a.Key {
	case "name", "host", "vcenter", "datastore", "iso_datastore", "folder",
		"network", "datacenter", "hostname", "path":
		return clrCyan
	}
	// Numbers
	if isNumericVal(val) {
		return clrYellow
	}
	return clrCyan
}

func isNumericVal(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !unicode.IsDigit(c) && c != '.' && c != '-' {
			return false
		}
	}
	return true
}
