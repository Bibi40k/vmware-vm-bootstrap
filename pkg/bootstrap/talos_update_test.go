package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpgradeTalosNode_Validation(t *testing.T) {
	if err := UpgradeTalosNode(context.Background(), nil); err == nil || err.Error() != "update config is required" {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := UpgradeTalosNode(context.Background(), &TalosNodeUpdateConfig{Version: "1.0.0"}); err == nil || err.Error() != "nodeIP is required" {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := UpgradeTalosNode(context.Background(), &TalosNodeUpdateConfig{NodeIP: "1.2.3.4"}); err == nil || err.Error() != "version is required" {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestUpgradeTalosNode_DefaultsAndArgs(t *testing.T) {
	tmp := t.TempDir()
	argsFile := filepath.Join(tmp, "args.txt")
	talosctl := filepath.Join(tmp, "talosctl")
	script := "#!/usr/bin/env bash\nset -euo pipefail\nprintf '%s\\n' \"$@\" > \"" + argsFile + "\"\n"
	if err := os.WriteFile(talosctl, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", tmp+string(os.PathListSeparator)+oldPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("PATH", oldPath) })

	err := UpgradeTalosNode(context.Background(), &TalosNodeUpdateConfig{
		NodeIP:         "10.0.0.11",
		Version:        "1.2.3",
		Preserve:       true,
		Insecure:       true,
		Talosconfig:    "/tmp/talosconfig",
		AdditionalArgs: []string{"--stage", "install"},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	raw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	args := string(raw)
	for _, want := range []string{
		"upgrade",
		"--nodes\n10.0.0.11",
		"--endpoints\n10.0.0.11",
		"--image\nghcr.io/siderolabs/installer:v1.2.3",
		"--preserve",
		"--insecure",
		"--talosconfig\n/tmp/talosconfig",
		"--stage\ninstall",
	} {
		if !strings.Contains(args, want) {
			t.Fatalf("args missing %q\nargs:\n%s", want, args)
		}
	}
}

func TestUpgradeTalosNode_CustomBinaryAndError(t *testing.T) {
	tmp := t.TempDir()
	talosctl := filepath.Join(tmp, "talosctl-fail")
	script := "#!/usr/bin/env bash\nset -euo pipefail\necho 'boom-stderr' 1>&2\nexit 9\n"
	if err := os.WriteFile(talosctl, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	err := UpgradeTalosNode(context.Background(), &TalosNodeUpdateConfig{
		NodeIP:       "10.0.0.11",
		Endpoint:     "10.0.0.12",
		Version:      "v1.2.3",
		TalosctlPath: talosctl,
	})
	if err == nil || !strings.Contains(err.Error(), "boom-stderr") {
		t.Fatalf("expected stderr error, got: %v", err)
	}
}
