package main

import (
	"os"
	"path/filepath"
	"strings"
)

func configRepoRoot() string {
	return strings.TrimSpace(os.Getenv("CONFIG_REPO_ROOT"))
}

func resolveConfigPath(path string) string {
	p := strings.TrimSpace(path)
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	root := configRepoRoot()
	if root == "" {
		return p
	}
	clean := filepath.Clean(p)
	if clean == "configs" || strings.HasPrefix(clean, "configs"+string(filepath.Separator)) {
		return filepath.Join(root, clean)
	}
	if clean == "config" || strings.HasPrefix(clean, "config"+string(filepath.Separator)) {
		return filepath.Join(root, clean)
	}
	return p
}
