package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// sopsDecrypt decrypts a SOPS-encrypted file and returns the plaintext content.
func sopsDecrypt(path string) ([]byte, error) {
	out, err := exec.Command("sops", "-d", path).Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("sops -d %s: %s", filepath.Base(path), string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("sops -d %s: %w", filepath.Base(path), err)
	}
	return out, nil
}

// sopsEncrypt writes plaintext YAML to path and encrypts it in-place with SOPS.
// The file must match a creation_rule in .sops.yaml.
func sopsEncrypt(path string, plaintext []byte) error {
	cmd := exec.Command("sops",
		"-e",
		"--input-type", "yaml",
		"--output-type", "yaml",
		"--filename-override", path,
		"/dev/stdin",
	)
	cmd.Stdin = bytes.NewReader(plaintext)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if strings.Contains(msg, "no matching creation rules found") {
			return &userError{
				msg:  "sops config has no matching creation rules",
				hint: "Edit .sops.yaml (see .sops.yaml.example) so configs/*.sops.yaml matches",
			}
		}
		return fmt.Errorf("sops -e (stdin): %s", msg)
	}

	if err := os.WriteFile(path, out, 0600); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	return nil
}

// checkRequirements verifies SOPS is available and the AGE key is accessible.
func checkRequirements() error {
	// Check sops binary
	if _, err := exec.LookPath("sops"); err != nil {
		return &userError{
			msg:  "'sops' not found in PATH",
			hint: "make install-requirements",
		}
	}

	// Check .sops.yaml
	if _, err := os.Stat(".sops.yaml"); os.IsNotExist(err) {
		if _, exErr := os.Stat(".sops.yaml.example"); exErr == nil {
			return &userError{
				msg:  "'.sops.yaml' not found — run from project root or create it",
				hint: "cp .sops.yaml.example .sops.yaml",
			}
		}
		return fmt.Errorf("'.sops.yaml' not found in current directory — run from project root")
	}

	// Check .sopsrc (optional but recommended)
	if _, err := os.Stat(".sopsrc"); os.IsNotExist(err) {
		if _, exErr := os.Stat(".sopsrc.example"); exErr == nil {
			return &userError{
				msg:  "'.sopsrc' not found — create it",
				hint: "cp .sopsrc.example .sopsrc",
			}
		}
	}

	// Check AGE key (standard location)
	ageKeyFile := os.ExpandEnv("$HOME/.config/sops/age/keys.txt")
	if envKey := os.Getenv("SOPS_AGE_KEY_FILE"); envKey != "" {
		ageKeyFile = envKey
	}
	if _, err := os.Stat(ageKeyFile); os.IsNotExist(err) {
		return fmt.Errorf("AGE key not found at %s — set SOPS_AGE_KEY_FILE or create the key", ageKeyFile)
	}

	return nil
}
