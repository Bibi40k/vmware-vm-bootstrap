package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// BootstrapResult is the normalized output contract from a completed bootstrap.
type BootstrapResult struct {
	VMName             string `json:"vm_name" yaml:"vm_name"`
	IPAddress          string `json:"ip" yaml:"ip"`
	SSHUser            string `json:"ssh_user" yaml:"ssh_user"`
	SSHPrivateKey      string `json:"ssh_key_path" yaml:"ssh_key_path"`
	SSHPort            int    `json:"ssh_port,omitempty" yaml:"ssh_port,omitempty"`
	SSHHostFingerprint string `json:"ssh_host_fingerprint,omitempty" yaml:"ssh_host_fingerprint,omitempty"`
}

// Validate checks the minimum contract required by Stage 2.
func (r BootstrapResult) Validate() error {
	if strings.TrimSpace(r.VMName) == "" {
		return fmt.Errorf("bootstrap vm_name is required")
	}
	if strings.TrimSpace(r.IPAddress) == "" {
		return fmt.Errorf("bootstrap ip is required")
	}
	if strings.TrimSpace(r.SSHUser) == "" {
		return fmt.Errorf("bootstrap ssh_user is required")
	}
	if strings.TrimSpace(r.SSHPrivateKey) == "" {
		return fmt.Errorf("bootstrap ssh_key_path is required")
	}
	if r.SSHPort < 0 || r.SSHPort > 65535 {
		return fmt.Errorf("bootstrap ssh_port must be in range 0..65535")
	}
	if fp := strings.TrimSpace(r.SSHHostFingerprint); fp != "" {
		if !strings.HasPrefix(fp, "SHA256:") {
			return fmt.Errorf("bootstrap ssh_host_fingerprint must be in SHA256:... format")
		}
		if len(fp) < len("SHA256:")+8 {
			return fmt.Errorf("bootstrap ssh_host_fingerprint is too short")
		}
	}
	return nil
}

// LoadBootstrapResult reads BootstrapResult from YAML or JSON.
func LoadBootstrapResult(path string) (BootstrapResult, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("read bootstrap result %s: %w", path, err)
	}

	var out BootstrapResult
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".json" {
		if err := json.Unmarshal(content, &out); err != nil {
			return BootstrapResult{}, fmt.Errorf("parse bootstrap result %s: %w", path, err)
		}
	} else {
		if err := yaml.Unmarshal(content, &out); err != nil {
			return BootstrapResult{}, fmt.Errorf("parse bootstrap result %s: %w", path, err)
		}
	}

	if err := out.Validate(); err != nil {
		return BootstrapResult{}, err
	}
	if out.SSHPort == 0 {
		out.SSHPort = 22
	}
	return out, nil
}

// SaveBootstrapResult writes BootstrapResult to YAML or JSON based on file extension.
func SaveBootstrapResult(path string, result BootstrapResult) error {
	if err := result.Validate(); err != nil {
		return err
	}
	if result.SSHPort == 0 {
		result.SSHPort = 22
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create dir for %s: %w", path, err)
	}

	ext := strings.ToLower(filepath.Ext(path))
	var content []byte
	var err error
	if ext == ".json" {
		content, err = json.MarshalIndent(result, "", "  ")
	} else {
		content, err = yaml.Marshal(result)
	}
	if err != nil {
		return fmt.Errorf("marshal bootstrap result %s: %w", path, err)
	}

	if err := os.WriteFile(path, content, 0o600); err != nil {
		return fmt.Errorf("write bootstrap result %s: %w", path, err)
	}
	return nil
}
