package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func smokeSSHChecks(user, ip, keyPath string, sshPort int, dataMount string, swapSizeGB int) error {
	if ip == "" {
		return fmt.Errorf("missing VM IP")
	}
	if sshPort == 0 {
		sshPort = 22
	}

	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()

	// Basic connectivity + hostname (retry up to ~6 minutes)
	var lastErr error
	attempt := 0
	deadline := time.Now().Add(6 * time.Minute)
	for time.Now().Before(deadline) {
		attempt++
		if _, err := sshExecAuto(ctx, user, ip, sshPort, keyPath, "hostname"); err == nil {
			lastErr = nil
			break
		} else {
			lastErr = err
			fmt.Printf("  SSH not ready (attempt %d), retrying in 15s...\n", attempt)
			time.Sleep(15 * time.Second)
		}
	}
	if lastErr != nil {
		return fmt.Errorf("ssh hostname: %w", lastErr)
	}

	// Disk check
	if dataMount != "" {
		out, err := sshExecAuto(ctx, user, ip, sshPort, keyPath, "lsblk -o NAME,MOUNTPOINT -n")
		if err != nil {
			return fmt.Errorf("lsblk: %w", err)
		}
		if !strings.Contains(out, dataMount) {
			return fmt.Errorf("data disk mount not found: %s", dataMount)
		}
	}

	// Swap check
	if swapSizeGB > 0 {
		out, err := sshExecAuto(ctx, user, ip, sshPort, keyPath, "swapon --show --noheadings")
		if err != nil {
			return fmt.Errorf("swapon: %w", err)
		}
		if strings.TrimSpace(out) == "" {
			return fmt.Errorf("swap not enabled")
		}
	}

	// VMware Tools
	if _, err := sshExecAuto(ctx, user, ip, sshPort, keyPath, "systemctl is-active open-vm-tools"); err != nil {
		return fmt.Errorf("open-vm-tools not active: %w", err)
	}

	return nil
}

func sshExecAuto(ctx context.Context, user, ip string, sshPort int, keyPath, cmd string) (string, error) {
	// Try explicit key first (if provided), then fallback to default SSH config/agent.
	if keyPath != "" {
		if out, err := sshExecWithKey(ctx, user, ip, sshPort, keyPath, cmd); err == nil {
			return out, nil
		} else if isAuthError(err) || isKeyError(err) {
			// Fall back to default SSH (agent/config) on auth/key mismatch.
			return sshExecDefault(ctx, user, ip, sshPort, cmd)
		} else {
			return "", err
		}
	}
	return sshExecDefault(ctx, user, ip, sshPort, cmd)
}

func sshExecWithKey(ctx context.Context, user, ip string, sshPort int, keyPath, cmd string) (string, error) {
	sshCmd := exec.CommandContext(ctx, "ssh",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=10",
		"-o", "BatchMode=yes",
		"-o", "IdentitiesOnly=yes",
		"-p", fmt.Sprintf("%d", sshPort),
		"-i", keyPath,
		fmt.Sprintf("%s@%s", user, ip),
		cmd,
	)
	var out bytes.Buffer
	var stderr bytes.Buffer
	sshCmd.Stdout = &out
	sshCmd.Stderr = &stderr
	if err := sshCmd.Run(); err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

func sshExecDefault(ctx context.Context, user, ip string, sshPort int, cmd string) (string, error) {
	sshCmd := exec.CommandContext(ctx, "ssh",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=10",
		"-o", "BatchMode=yes",
		"-p", fmt.Sprintf("%d", sshPort),
		fmt.Sprintf("%s@%s", user, ip),
		cmd,
	)
	var out bytes.Buffer
	var stderr bytes.Buffer
	sshCmd.Stdout = &out
	sshCmd.Stderr = &stderr
	if err := sshCmd.Run(); err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

func isAuthError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "Permission denied") || strings.Contains(msg, "Authentication failed")
}

func isKeyError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "No such file or directory") ||
		strings.Contains(msg, "invalid format") ||
		strings.Contains(msg, "Identity file")
}
