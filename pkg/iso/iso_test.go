package iso

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetUbuntuReleases(t *testing.T) {
	releases := GetUbuntuReleases()

	if len(releases) == 0 {
		t.Fatal("GetUbuntuReleases() returned empty map")
	}

	for _, version := range []string{"24.04", "22.04"} {
		t.Run("Ubuntu "+version, func(t *testing.T) {
			r, ok := releases[version]
			if !ok {
				t.Fatalf("Ubuntu %s not found in releases", version)
			}
			if r.URL == "" {
				t.Errorf("Ubuntu %s has no download URL", version)
			}
			if r.Version != version {
				t.Errorf("Ubuntu %s Version field = %q, want %q", version, r.Version, version)
			}
		})
	}
}

func TestNewManager(t *testing.T) {
	ctx := context.Background()
	mgr := NewManager(ctx)

	if mgr == nil {
		t.Fatal("NewManager() returned nil")
	}
	if mgr.ctx == nil {
		t.Error("Manager context is nil")
	}
	if mgr.cacheDir == "" {
		t.Error("Manager cacheDir is empty")
	}
}

func TestSetCacheDir(t *testing.T) {
	mgr := NewManager(context.Background())
	tmpDir := t.TempDir()

	if err := mgr.SetCacheDir(tmpDir); err != nil {
		t.Fatalf("SetCacheDir() failed: %v", err)
	}
	if mgr.cacheDir != tmpDir {
		t.Errorf("cacheDir = %q, want %q", mgr.cacheDir, tmpDir)
	}
}

func TestDownloadUbuntu_unsupportedVersion(t *testing.T) {
	mgr := NewManager(context.Background())
	if err := mgr.SetCacheDir(t.TempDir()); err != nil {
		t.Fatalf("SetCacheDir: %v", err)
	}

	_, err := mgr.DownloadUbuntu("99.99")

	if err == nil {
		t.Fatal("DownloadUbuntu() should fail for unsupported version")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("error should mention 'unsupported', got: %v", err)
	}
}

func TestCreateNoCloudISO(t *testing.T) {
	mgr := NewManager(context.Background())
	if err := mgr.SetCacheDir(t.TempDir()); err != nil {
		t.Fatalf("SetCacheDir: %v", err)
	}

	isoPath, err := mgr.CreateNoCloudISO(
		"#cloud-config\nhostname: test-vm\n",
		"instance-id: test-001\nlocal-hostname: test-vm\n",
		"version: 2\nethernets:\n  ens192:\n    dhcp4: false\n",
		"test-vm",
	)
	if err != nil {
		t.Fatalf("CreateNoCloudISO() failed: %v", err)
	}
	defer func() {
		_ = os.Remove(isoPath)
	}()

	info, err := os.Stat(isoPath)
	if err != nil {
		t.Fatalf("ISO file not created at %s: %v", isoPath, err)
	}
	if info.Size() == 0 {
		t.Error("ISO file is empty")
	}
	if filepath.Ext(isoPath) != ".iso" {
		t.Errorf("ISO extension = %q, want .iso", filepath.Ext(isoPath))
	}
}
