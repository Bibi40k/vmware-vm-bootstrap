package iso

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFakeExecutable(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestExtractISOWithProgress_Success(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFakeExecutable(t, bin, "xorriso", `#!/bin/sh
extractDir=""
for arg in "$@"; do
  extractDir="$arg"
done
mkdir -p "$extractDir"
echo ok > "$extractDir/.done"
exit 0
`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	mgr := NewManager(context.Background())
	extractDir := filepath.Join(tmp, "extract")
	if err := mgr.extractISOWithProgress("dummy.iso", extractDir); err != nil {
		t.Fatalf("extractISOWithProgress: %v", err)
	}
	if _, err := os.Stat(filepath.Join(extractDir, ".done")); err != nil {
		t.Fatalf("expected .done file: %v", err)
	}
}

func TestExtractISOWithProgress_Failure(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFakeExecutable(t, bin, "xorriso", "#!/bin/sh\nexit 1\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	mgr := NewManager(context.Background())
	err := mgr.extractISOWithProgress("dummy.iso", filepath.Join(tmp, "extract"))
	if err == nil {
		t.Fatal("expected error from xorriso failure")
	}
	if !strings.Contains(err.Error(), "xorriso extract failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRepackISO_SuccessWithUEFI(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFakeExecutable(t, bin, "genisoimage", `#!/bin/sh
out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then
    out="$2"
    shift 2
    continue
  fi
  shift
done
touch "$out"
exit 0
`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	extractDir := filepath.Join(tmp, "extract")
	if err := os.MkdirAll(filepath.Join(extractDir, "boot/grub/i386-pc"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(extractDir, "EFI/boot"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(extractDir, "boot/grub/i386-pc/eltorito.img"), []byte("x"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(extractDir, "EFI/boot/bootx64.efi"), []byte("x"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	mgr := NewManager(context.Background())
	out := filepath.Join(tmp, "out.iso")
	if err := mgr.repackISO(extractDir, out); err != nil {
		t.Fatalf("repackISO: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("expected output iso: %v", err)
	}
}

func TestRepackISO_SuccessNoUEFI(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFakeExecutable(t, bin, "genisoimage", `#!/bin/sh
out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then
    out="$2"
    shift 2
    continue
  fi
  shift
done
touch "$out"
exit 0
`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	extractDir := filepath.Join(tmp, "extract")
	if err := os.MkdirAll(filepath.Join(extractDir, "boot/grub/i386-pc"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(extractDir, "boot/grub/i386-pc/eltorito.img"), []byte("x"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	mgr := NewManager(context.Background())
	out := filepath.Join(tmp, "out.iso")
	if err := mgr.repackISO(extractDir, out); err != nil {
		t.Fatalf("repackISO: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("expected output iso: %v", err)
	}
}

func TestUploadWithGovc_SuccessAndFailure(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	writeFakeExecutable(t, bin, "govc", "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	mgr := NewManager(context.Background())
	if err := mgr.uploadWithGovc("ds", "local.iso", "remote.iso", "vc", "user", "pass", true); err != nil {
		t.Fatalf("uploadWithGovc success: %v", err)
	}

	// Failure case
	writeFakeExecutable(t, bin, "govc", "#!/bin/sh\necho fail 1>&2\nexit 1\n")
	err := mgr.uploadWithGovc("ds", "local.iso", "remote.iso", "vc", "user", "pass", true)
	if err == nil {
		t.Fatal("expected error from govc failure")
	}
	if !strings.Contains(err.Error(), "govc upload failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteFromDatastore_SuccessAndFailure(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	writeFakeExecutable(t, bin, "govc", "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	mgr := NewManager(context.Background())
	if err := mgr.DeleteFromDatastore("ds", "path.iso", "vc", "user", "pass", true); err != nil {
		t.Fatalf("DeleteFromDatastore success: %v", err)
	}

	// Failure case
	writeFakeExecutable(t, bin, "govc", "#!/bin/sh\necho fail 1>&2\nexit 1\n")
	err := mgr.DeleteFromDatastore("ds", "path.iso", "vc", "user", "pass", true)
	if err == nil {
		t.Fatal("expected error from govc failure")
	}
	if !strings.Contains(err.Error(), "govc datastore.rm failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestModifyUbuntuISO_SuccessAndCache(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFakeExecutable(t, bin, "xorriso", `#!/bin/sh
extractDir=""
for arg in "$@"; do
  extractDir="$arg"
done
mkdir -p "$extractDir/boot/grub/i386-pc"
mkdir -p "$extractDir/EFI/boot"
mkdir -p "$extractDir/isolinux"
cat > "$extractDir/boot/grub/grub.cfg" <<EOF
set timeout=30
menuentry "Install" {
 linux /casper/vmlinuz ---
}
EOF
echo ok > "$extractDir/boot/grub/i386-pc/eltorito.img"
echo ok > "$extractDir/EFI/boot/bootx64.efi"
echo ok > "$extractDir/isolinux/txt.cfg"
exit 0
`)
	writeFakeExecutable(t, bin, "genisoimage", `#!/bin/sh
out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then
    out="$2"
    shift 2
    continue
  fi
  shift
done
touch "$out"
exit 0
`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	isoPath := filepath.Join(tmp, "ubuntu.iso")
	if err := os.WriteFile(isoPath, []byte("dummy"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	mgr := NewManager(context.Background())
	if err := mgr.SetCacheDir(tmp); err != nil {
		t.Fatalf("SetCacheDir: %v", err)
	}

	modified, wasCreated, err := mgr.ModifyUbuntuISO(isoPath)
	if err != nil {
		t.Fatalf("ModifyUbuntuISO: %v", err)
	}
	if !wasCreated {
		t.Fatal("expected wasCreated true on first run")
	}
	if _, err := os.Stat(modified); err != nil {
		t.Fatalf("expected modified iso: %v", err)
	}

	modified2, wasCreated2, err := mgr.ModifyUbuntuISO(isoPath)
	if err != nil {
		t.Fatalf("ModifyUbuntuISO second run: %v", err)
	}
	if wasCreated2 {
		t.Fatal("expected wasCreated false on cached run")
	}
	if modified2 != modified {
		t.Fatalf("unexpected modified path: %s vs %s", modified2, modified)
	}
}
