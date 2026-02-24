package iso

import (
	"context"
	"strings"
	"testing"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"
)

func TestNewGovcCmd(t *testing.T) {
	ctx := context.Background()
	cmd := newGovcCmd(ctx, "vc.example", "user", "pass", true, "datastore.ls")

	if cmd == nil {
		t.Fatal("expected cmd to be created")
	}
	if len(cmd.Args) < 2 || cmd.Args[1] != "datastore.ls" {
		t.Fatalf("unexpected args: %v", cmd.Args)
	}

	env := strings.Join(cmd.Env, "\n")
	if !strings.Contains(env, "GOVC_URL=https://vc.example/sdk") {
		t.Fatalf("missing GOVC_URL, env=%s", env)
	}
	if !strings.Contains(env, "GOVC_USERNAME=user") {
		t.Fatalf("missing GOVC_USERNAME, env=%s", env)
	}
	if !strings.Contains(env, "GOVC_PASSWORD=pass") {
		t.Fatalf("missing GOVC_PASSWORD, env=%s", env)
	}
	if !strings.Contains(env, "GOVC_INSECURE=true") {
		t.Fatalf("missing GOVC_INSECURE, env=%s", env)
	}
}

func TestGetCDROMs(t *testing.T) {
	devices := object.VirtualDeviceList{
		&types.VirtualCdrom{VirtualDevice: types.VirtualDevice{Key: 1}},
		&types.VirtualDisk{VirtualDevice: types.VirtualDevice{Key: 2}},
	}

	cdroms := getCDROMs(devices)
	if len(cdroms) != 1 {
		t.Fatalf("expected 1 cdrom, got %d", len(cdroms))
	}
	if cdroms[0].Key != 1 {
		t.Fatalf("unexpected cdrom key: %d", cdroms[0].Key)
	}
}
