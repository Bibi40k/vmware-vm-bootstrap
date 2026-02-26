package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Bibi40k/vmware-vm-bootstrap/configs"
	pkgconfig "github.com/Bibi40k/vmware-vm-bootstrap/pkg/config"
	"github.com/Bibi40k/vmware-vm-bootstrap/pkg/bootstrap"
)

func writeStage1Result(path string, cfg *bootstrap.VMConfig, sshKeyPath string, sshPort int, vm *bootstrap.VM) error {
	keyPath := resolveSSHPrivateKeyPath(sshKeyPath)
	if keyPath == "" {
		return fmt.Errorf("cannot write stage1 result: vm.ssh_key_path is required (private key path not available)")
	}
	fp, err := computeSSHHostFingerprint(vm.IPAddress, sshPort)
	if err != nil {
		return fmt.Errorf("compute ssh host fingerprint: %w", err)
	}

	result := pkgconfig.Stage1Result{
		VMName:             cfg.Name,
		IPAddress:          vm.IPAddress,
		SSHUser:            cfg.Username,
		SSHPrivateKey:      keyPath,
		SSHPort:            sshPort,
		SSHHostFingerprint: fp,
	}
	if result.SSHPort == 0 {
		result.SSHPort = 22
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create stage1 result dir: %w", err)
	}
	if err := pkgconfig.SaveStage1Result(path, result); err != nil {
		return fmt.Errorf("save stage1 result: %w", err)
	}
	return nil
}

func resolveSSHPrivateKeyPath(path string) string {
	p := strings.TrimSpace(path)
	if p == "" {
		return ""
	}
	if strings.HasSuffix(strings.ToLower(p), ".pub") {
		priv := strings.TrimSuffix(p, ".pub")
		if st, err := os.Stat(priv); err == nil && !st.IsDir() {
			return priv
		}
	}
	return p
}

func resolveStage1ResultPath(explicitPath, vmName string) string {
	path := strings.TrimSpace(explicitPath)
	if path == "" {
		if !configs.Defaults.Output.Enable {
			return ""
		}
		path = strings.TrimSpace(configs.Defaults.Output.Stage1ResultPath)
	}
	if path == "" {
		return ""
	}
	if strings.Contains(path, "{vm}") {
		path = strings.ReplaceAll(path, "{vm}", vmName)
	}
	return path
}
