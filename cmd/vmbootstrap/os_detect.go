package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func detectUbuntuVersion(user, ip, keyPath string, sshPort int) (string, error) {
	if ip == "" {
		return "", fmt.Errorf("missing IP")
	}
	if sshPort == 0 {
		sshPort = 22
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	out, err := sshExecAuto(ctx, user, ip, sshPort, keyPath, "lsb_release -rs")
	if err == nil {
		return strings.TrimSpace(out), nil
	}
	out, err = sshExecAuto(ctx, user, ip, sshPort, keyPath, "grep -E '^VERSION_ID=' /etc/os-release | cut -d= -f2 | tr -d '\"'")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}
