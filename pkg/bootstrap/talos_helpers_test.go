package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTalosOVAHelpers(t *testing.T) {
	if got := normalizeTalosVersion("1.2.3"); got != "v1.2.3" {
		t.Fatalf("unexpected normalized version: %q", got)
	}
	if got := normalizeTalosVersion(" v1.2.3 "); got != "v1.2.3" {
		t.Fatalf("unexpected normalized version: %q", got)
	}
	if got := talosOVAURL("1.2.3", "schem"); got != "https://factory.talos.dev/image/schem/v1.2.3/vmware-amd64.ova" {
		t.Fatalf("unexpected OVA url: %q", got)
	}
	if got := talosLibraryItemName("v1.2.3", "1234567890abcdef"); got != "talos-1.2.3-1234567890ab" {
		t.Fatalf("unexpected item name: %q", got)
	}
	if got := talosLibraryItemName("", ""); got != "talos-latest-default" {
		t.Fatalf("unexpected default item name: %q", got)
	}
	if !govcErrContains(errors.New("invalid_library_item"), "invalid_library_item") {
		t.Fatal("expected contains match")
	}
	if govcErrContains(nil, "x") {
		t.Fatal("expected false on nil err")
	}
}

func TestGovcEnv(t *testing.T) {
	cfg := &VMConfig{
		VCenterHost:     "vc.example.local",
		VCenterUsername: "user",
		VCenterPassword: "pass",
		Datacenter:      "DC1",
		VCenterInsecure: true,
	}
	env := govcEnv(cfg)
	asMap := map[string]string{}
	for _, kv := range env {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			asMap[parts[0]] = parts[1]
		}
	}
	if asMap["GOVC_URL"] != "https://vc.example.local/sdk" {
		t.Fatalf("unexpected GOVC_URL: %q", asMap["GOVC_URL"])
	}
	if asMap["GOVC_USERNAME"] != "user" || asMap["GOVC_PASSWORD"] != "pass" {
		t.Fatalf("missing govc creds in env: %#v", asMap)
	}
}

func TestRunGovc_SuccessAndErrors(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "govc")
	content := `#!/usr/bin/env bash
set -euo pipefail
mode="${1:-ok}"
if [[ "$mode" == "ok" ]]; then
  echo "hello"
  exit 0
fi
if [[ "$mode" == "err-stderr" ]]; then
  echo "stderr-msg" 1>&2
  exit 2
fi
echo "stdout-msg"
exit 3
`
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}

	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", tmp+string(os.PathListSeparator)+oldPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("PATH", oldPath) })

	out, err := runGovc(context.Background(), nil, "ok")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if strings.TrimSpace(string(out)) != "hello" {
		t.Fatalf("unexpected out: %q", string(out))
	}

	_, err = runGovc(context.Background(), nil, "err-stderr")
	if err == nil || !strings.Contains(err.Error(), "stderr-msg") {
		t.Fatalf("expected stderr error, got: %v", err)
	}

	_, err = runGovc(context.Background(), nil, "err-stdout")
	if err == nil || !strings.Contains(err.Error(), "stdout-msg") {
		t.Fatalf("expected stdout fallback error, got: %v", err)
	}
}
