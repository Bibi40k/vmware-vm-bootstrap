package bootstrap

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// TalosNodeUpdateConfig controls talosctl-based node upgrades.
type TalosNodeUpdateConfig struct {
	NodeIP         string
	Endpoint       string
	Version        string
	Talosconfig    string
	TalosctlPath   string
	Preserve       bool
	Insecure       bool
	AdditionalArgs []string
}

// UpgradeTalosNode upgrades a Talos node using talosctl.
func UpgradeTalosNode(ctx context.Context, cfg *TalosNodeUpdateConfig) error {
	if cfg == nil {
		return fmt.Errorf("update config is required")
	}

	nodeIP := strings.TrimSpace(cfg.NodeIP)
	if nodeIP == "" {
		return fmt.Errorf("nodeIP is required")
	}
	version := strings.TrimSpace(cfg.Version)
	if version == "" {
		return fmt.Errorf("version is required")
	}
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}

	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		endpoint = nodeIP
	}

	talosctlPath := strings.TrimSpace(cfg.TalosctlPath)
	if talosctlPath == "" {
		talosctlPath = "talosctl"
	}

	image := fmt.Sprintf("ghcr.io/siderolabs/installer:%s", version)
	args := []string{
		"upgrade",
		"--nodes", nodeIP,
		"--endpoints", endpoint,
		"--image", image,
	}
	if cfg.Preserve {
		args = append(args, "--preserve")
	}
	if cfg.Insecure {
		args = append(args, "--insecure")
	}
	if cfg.Talosconfig != "" {
		args = append(args, "--talosconfig", cfg.Talosconfig)
	}
	if len(cfg.AdditionalArgs) > 0 {
		args = append(args, cfg.AdditionalArgs...)
	}

	cmd := exec.CommandContext(ctx, talosctlPath, args...)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errOut := strings.TrimSpace(stderr.String())
		if errOut == "" {
			errOut = strings.TrimSpace(out.String())
		}
		if errOut != "" {
			return fmt.Errorf("talosctl upgrade failed: %w: %s", err, errOut)
		}
		return fmt.Errorf("talosctl upgrade failed: %w", err)
	}

	return nil
}
