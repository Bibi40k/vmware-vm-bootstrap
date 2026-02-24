package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Stage1Result is the normalized output contract from Stage 1 provisioning.
type Stage1Result struct {
	VMName        string `json:"vm_name" yaml:"vm_name"`
	IPAddress     string `json:"ip" yaml:"ip"`
	SSHUser       string `json:"ssh_user" yaml:"ssh_user"`
	SSHPrivateKey string `json:"ssh_key_path" yaml:"ssh_key_path"`
	SSHPort       int    `json:"ssh_port,omitempty" yaml:"ssh_port,omitempty"`
}

// Validate checks the minimum contract required by Stage 2.
func (r Stage1Result) Validate() error {
	if strings.TrimSpace(r.VMName) == "" {
		return fmt.Errorf("stage1 vm_name is required")
	}
	if strings.TrimSpace(r.IPAddress) == "" {
		return fmt.Errorf("stage1 ip is required")
	}
	if strings.TrimSpace(r.SSHUser) == "" {
		return fmt.Errorf("stage1 ssh_user is required")
	}
	if strings.TrimSpace(r.SSHPrivateKey) == "" {
		return fmt.Errorf("stage1 ssh_key_path is required")
	}
	if r.SSHPort < 0 || r.SSHPort > 65535 {
		return fmt.Errorf("stage1 ssh_port must be in range 0..65535")
	}
	return nil
}

// LoadStage1Result reads Stage1Result from YAML or JSON.
func LoadStage1Result(path string) (Stage1Result, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Stage1Result{}, fmt.Errorf("read stage1 result %s: %w", path, err)
	}

	var out Stage1Result
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".json" {
		if err := json.Unmarshal(content, &out); err != nil {
			return Stage1Result{}, fmt.Errorf("parse stage1 result %s: %w", path, err)
		}
	} else {
		if err := yaml.Unmarshal(content, &out); err != nil {
			return Stage1Result{}, fmt.Errorf("parse stage1 result %s: %w", path, err)
		}
	}

	if err := out.Validate(); err != nil {
		return Stage1Result{}, err
	}
	if out.SSHPort == 0 {
		out.SSHPort = 22
	}
	return out, nil
}

// SaveStage1Result writes Stage1Result to YAML or JSON based on file extension.
func SaveStage1Result(path string, result Stage1Result) error {
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
		return fmt.Errorf("marshal stage1 result %s: %w", path, err)
	}

	if err := os.WriteFile(path, content, 0o600); err != nil {
		return fmt.Errorf("write stage1 result %s: %w", path, err)
	}
	return nil
}
