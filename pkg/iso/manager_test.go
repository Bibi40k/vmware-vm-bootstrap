package iso

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestHashFileSuffix(t *testing.T) {
	path := "/tmp/example.iso"
	if got := hashFile(path); got != "/tmp/example.iso.uploaded.sha256" {
		t.Fatalf("hashFile() = %q", got)
	}
}

func TestComputeSHA256(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(p, []byte("hello"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	sum, err := computeSHA256(p)
	if err != nil {
		t.Fatalf("computeSHA256: %v", err)
	}
	// sha256("hello")
	const want = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if sum != want {
		t.Fatalf("hash mismatch: got %s", sum)
	}
}

func TestNeedsUpload_NoHashFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.iso")
	if err := os.WriteFile(p, []byte("data"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if !needsUpload(p) {
		t.Fatalf("expected needsUpload to be true when no hash file exists")
	}
}

func TestNeedsUpload_MatchAndChange(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.iso")
	if err := os.WriteFile(p, []byte("data"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	saveUploadedHash(p)
	if needsUpload(p) {
		t.Fatalf("expected needsUpload false when hash matches")
	}

	if err := os.WriteFile(p, []byte("changed"), 0644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if !needsUpload(p) {
		t.Fatalf("expected needsUpload true after content change")
	}
}

func TestNeedsUpload_MissingFile(t *testing.T) {
	if !needsUpload(filepath.Join(t.TempDir(), "missing.iso")) {
		t.Fatalf("expected needsUpload true when file missing")
	}
}

func TestVerifyChecksum_OK(t *testing.T) {
	mgr := NewManager(context.Background())
	dir := t.TempDir()
	p := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(p, []byte("hello"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// sha256("hello")
	const sum = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if err := mgr.verifyChecksum(p, sum); err != nil {
		t.Fatalf("verifyChecksum: %v", err)
	}
}

func TestVerifyChecksum_Mismatch(t *testing.T) {
	mgr := NewManager(context.Background())
	dir := t.TempDir()
	p := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(p, []byte("hello"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := mgr.verifyChecksum(p, "deadbeef"); err == nil {
		t.Fatal("expected checksum mismatch error")
	}
}
