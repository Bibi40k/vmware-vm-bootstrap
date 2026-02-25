package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

func computeSSHHostFingerprint(host string, port int) (string, error) {
	if port <= 0 {
		port = 22
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	args := []string{"-H", "-p", fmt.Sprintf("%d", port), host}
	out, err := exec.CommandContext(ctx, "ssh-keyscan", args...).Output()
	if err != nil {
		return "", fmt.Errorf("ssh-keyscan %s:%d: %w", host, port, err)
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		keyLine := fields[1] + " " + fields[2]
		key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(keyLine))
		if err != nil {
			continue
		}
		return ssh.FingerprintSHA256(key), nil
	}
	return "", fmt.Errorf("no valid host key found for %s:%d", host, port)
}
