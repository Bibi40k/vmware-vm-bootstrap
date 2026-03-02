package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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

func TestBootstrapResultValidateErrors(t *testing.T) {
	cases := []BootstrapResult{
		{},
		{VMName: "vm", IPAddress: "1.2.3.4", SSHUser: "u"},
		{VMName: "vm", IPAddress: "1.2.3.4", SSHUser: "u", SSHPrivateKey: "/k", SSHPort: 70000},
		{VMName: "vm", IPAddress: "1.2.3.4", SSHUser: "u", SSHPrivateKey: "/k", SSHHostFingerprint: "md5:abc"},
		{VMName: "vm", IPAddress: "1.2.3.4", SSHUser: "u", SSHPrivateKey: "/k", SSHHostFingerprint: "SHA256:abc"},
	}
	for _, c := range cases {
		if err := c.Validate(); err == nil {
			t.Fatalf("expected validate error for %+v", c)
		}
	}
}

func TestSaveBootstrapResult_DefaultPortAndYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "bootstrap.yaml")
	in := BootstrapResult{
		VMName:        "devvm-01",
		IPAddress:     "192.168.1.50",
		SSHUser:       "dev",
		SSHPrivateKey: "/k",
	}
	if err := SaveBootstrapResult(path, in); err != nil {
		t.Fatalf("save bootstrap result: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read bootstrap result: %v", err)
	}
	var got BootstrapResult
	if err := yaml.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal yaml: %v", err)
	}
	if got.SSHPort != 22 {
		t.Fatalf("expected default ssh_port=22, got %d", got.SSHPort)
	}
}

func TestLoadBootstrapResult_ParseAndReadErrors(t *testing.T) {
	dir := t.TempDir()

	_, err := LoadBootstrapResult(filepath.Join(dir, "missing.yaml"))
	if err == nil || !strings.Contains(err.Error(), "read bootstrap result") {
		t.Fatalf("expected read error, got: %v", err)
	}

	yamlPath := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(yamlPath, []byte(":\n-"), 0o600); err != nil {
		t.Fatalf("write bad yaml: %v", err)
	}
	_, err = LoadBootstrapResult(yamlPath)
	if err == nil || !strings.Contains(err.Error(), "parse bootstrap result") {
		t.Fatalf("expected parse yaml error, got: %v", err)
	}

	jsonPath := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(jsonPath, []byte("{bad"), 0o600); err != nil {
		t.Fatalf("write bad json: %v", err)
	}
	_, err = LoadBootstrapResult(jsonPath)
	if err == nil || !strings.Contains(err.Error(), "parse bootstrap result") {
		t.Fatalf("expected parse json error, got: %v", err)
	}
}

func TestSaveBootstrapResult_WriteAndValidateErrors(t *testing.T) {
	dir := t.TempDir()

	err := SaveBootstrapResult(filepath.Join(dir, "x.yaml"), BootstrapResult{})
	if err == nil {
		t.Fatal("expected validate error")
	}

	blocking := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(blocking, []byte("x"), 0o600); err != nil {
		t.Fatalf("write blocking file: %v", err)
	}
	err = SaveBootstrapResult(filepath.Join(blocking, "bootstrap.yaml"), BootstrapResult{
		VMName:        "devvm-01",
		IPAddress:     "192.168.1.50",
		SSHUser:       "dev",
		SSHPrivateKey: "/k",
	})
	if err == nil || !strings.Contains(err.Error(), "create dir") {
		t.Fatalf("expected create dir error, got: %v", err)
	}
}

func TestSaveAndLoadBootstrapResultJSON_ContentShape(t *testing.T) {
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

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read bootstrap result: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if obj["vm_name"] != "devvm-01" {
		t.Fatalf("unexpected json payload: %+v", obj)
	}
}
