package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadBootstrapResultYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bootstrap.yaml")
	content := []byte("vm_name: devvm-01\nip: 192.168.1.50\nssh_user: dev\nssh_key_path: /home/dev/.ssh/id_ed25519\nssh_host_fingerprint: SHA256:abc123def456\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	got, err := LoadBootstrapResult(path)
	if err != nil {
		t.Fatalf("load bootstrap result: %v", err)
	}
	if got.VMName != "devvm-01" || got.SSHPort != 22 {
		t.Fatalf("unexpected parsed result: %+v", got)
	}
	if got.SSHHostFingerprint != "SHA256:abc123def456" {
		t.Fatalf("unexpected fingerprint: %q", got.SSHHostFingerprint)
	}
}

func TestSaveAndLoadBootstrapResultJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bootstrap.json")
	in := BootstrapResult{
		VMName:             "devvm-01",
		IPAddress:          "192.168.1.50",
		SSHUser:            "dev",
		SSHPrivateKey:      "/k",
		SSHPort:            2222,
		SSHHostFingerprint: "SHA256:abc123def456",
	}
	if err := SaveBootstrapResult(path, in); err != nil {
		t.Fatalf("save bootstrap result: %v", err)
	}
	out, err := LoadBootstrapResult(path)
	if err != nil {
		t.Fatalf("load bootstrap result: %v", err)
	}
	if out.SSHPort != 2222 || out.IPAddress != in.IPAddress {
		t.Fatalf("unexpected loaded result: %+v", out)
	}
}
