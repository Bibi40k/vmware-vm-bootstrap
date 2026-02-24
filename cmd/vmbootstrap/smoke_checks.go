package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func smokeSSHChecks(user, ip, dataMount string, swapSizeGB int) error {
	if ip == "" {
		return fmt.Errorf("missing VM IP")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Basic connectivity + hostname (retry up to ~3 minutes)
	var lastErr error
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		if _, err := sshExec(ctx, user, ip, "hostname"); err == nil {
			lastErr = nil
			break
		} else {
			lastErr = err
			time.Sleep(10 * time.Second)
		}
	}
	if lastErr != nil {
		return fmt.Errorf("ssh hostname: %w", lastErr)
	}

	// Disk check
	if dataMount != "" {
		out, err := sshExec(ctx, user, ip, "lsblk -o NAME,MOUNTPOINT -n")
		if err != nil {
			return fmt.Errorf("lsblk: %w", err)
		}
		if !strings.Contains(out, dataMount) {
			return fmt.Errorf("data disk mount not found: %s", dataMount)
		}
	}

	// Swap check
	if swapSizeGB > 0 {
		out, err := sshExec(ctx, user, ip, "swapon --show --noheadings")
		if err != nil {
			return fmt.Errorf("swapon: %w", err)
		}
		if strings.TrimSpace(out) == "" {
			return fmt.Errorf("swap not enabled")
		}
	}

	// VMware Tools
	if _, err := sshExec(ctx, user, ip, "systemctl is-active open-vm-tools"); err != nil {
		return fmt.Errorf("open-vm-tools not active: %w", err)
	}

	return nil
}

func sshExec(ctx context.Context, user, ip, cmd string) (string, error) {
	sshCmd := exec.CommandContext(ctx, "ssh",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=10",
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
