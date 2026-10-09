package logx

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestFileRotator_OversizedSingleWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")

	r, err := newFileRotator(path, 8, 2)
	if err != nil {
		t.Fatalf("newFileRotator failed: %v", err)
	}
	defer r.Close()

	payload := strings.Repeat("x", 32)
	n, err := r.Write([]byte(payload))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if n != len(payload) {
		t.Fatalf("expected full write, got %d", n)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if len(data) != 0 {
		t.Fatalf("expected active file to be empty after oversized write rotation, got %d bytes", len(data))
	}

	matches, err := filepath.Glob(rotatedLogGlob(path))
	if err != nil {
		t.Fatalf("glob failed: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one rotated backup, got %d: %v", len(matches), matches)
	}
	backup, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read backup failed: %v", err)
	}
	if len(backup) != len(payload) {
		t.Fatalf("expected backup to contain oversized payload, got %d bytes", len(backup))
	}
}

func TestFileRotator_WriteAfterClose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")

	r, err := newFileRotator(path, 8, 2)
	if err != nil {
		t.Fatalf("newFileRotator failed: %v", err)
	}

	if err := r.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	if _, err := r.Write([]byte("boom")); err == nil {
		t.Fatalf("expected write after close to fail")
	}
}

func TestFileRotator_BackupPruning(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")

	r, err := newFileRotator(path, 4, 2)
	if err != nil {
		t.Fatalf("newFileRotator failed: %v", err)
	}
	defer r.Close()

	for i := 0; i < 5; i++ {
		if _, err := r.Write([]byte("abcd")); err != nil {
			t.Fatalf("write %d failed: %v", i, err)
		}
		time.Sleep(time.Millisecond)
	}

	matches, err := filepath.Glob(rotatedLogGlob(path))
	if err != nil {
		t.Fatalf("glob failed: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 backups after pruning, got %d: %v", len(matches), matches)
	}
}

func TestFileRotator_RotationErrorReturnsError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Chmod does not make a directory unwritable on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat temp dir failed: %v", err)
	}

	r, err := newFileRotator(path, 4, 2)
	if err != nil {
		t.Fatalf("newFileRotator failed: %v", err)
	}
	defer r.Close()

	if _, err := r.Write([]byte("abcd")); err != nil {
		t.Fatalf("initial write failed: %v", err)
	}

	origPerm := info.Mode().Perm()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("chmod failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, origPerm) })

	if _, err := r.Write([]byte("efgh")); err == nil {
		t.Fatalf("expected rotation error when log path is not writable")
	}
}

func TestFileRotator_MaxBackups(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")

	r, err := newFileRotator(path, 4, 1)
	if err != nil {
		t.Fatalf("newFileRotator failed: %v", err)
	}
	defer r.Close()

	for i := 0; i < 4; i++ {
		if _, err := r.Write([]byte("abcd")); err != nil {
			t.Fatalf("write %d failed: %v", i, err)
		}
		time.Sleep(time.Millisecond)
	}

	matches, err := filepath.Glob(rotatedLogGlob(path))
	if err != nil {
		t.Fatalf("glob failed: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 backup with max backups=1, got %d: %v", len(matches), matches)
	}
}

func TestFileRotator_RenameCollisionUsesCounter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")

	r, err := newFileRotator(path, 4, 5)
	if err != nil {
		t.Fatalf("newFileRotator failed: %v", err)
	}
	defer r.Close()

	if _, err := r.Write([]byte("abcd")); err != nil {
		t.Fatalf("initial write failed: %v", err)
	}

	fixed := time.Date(2026, 10, 3, 12, 0, 0, 123456789, time.UTC)
	r.now = func() time.Time { return fixed }
	occupied := rotatedName(path, fixed.UTC().Format("20060102T150405.000000000"), 0)
	if err := os.WriteFile(occupied, []byte("taken"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := r.Write([]byte("efgh")); err != nil {
		t.Fatalf("expected rotation to retry on name collision, got: %v", err)
	}

	taken, err := os.ReadFile(occupied)
	if err != nil {
		t.Fatal(err)
	}
	if string(taken) != "taken" {
		t.Fatalf("expected occupied backup to stay, got %q", taken)
	}

	alt := rotatedName(path, fixed.UTC().Format("20060102T150405.000000000"), 1)
	backup, err := os.ReadFile(alt)
	if err != nil {
		t.Fatalf("expected counter backup %s: %v", alt, err)
	}
	if string(backup) != "abcd" {
		t.Fatalf("expected rotated payload in counter backup, got %q", backup)
	}
}

func TestFileRotator_GlobErrorReturned(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app[.log")

	r, err := newFileRotator(path, 4, 1)
	if err != nil {
		t.Fatalf("newFileRotator failed: %v", err)
	}
	defer r.Close()

	if _, err := r.Write([]byte("abcd")); err != nil {
		t.Fatalf("initial write failed: %v", err)
	}
	if _, err := r.Write([]byte("efgh")); err == nil {
		t.Fatal("expected glob error while pruning backups")
	}
}

func TestFileRotator_MaxBackupsZeroMeansUnlimited(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")

	r, err := newFileRotator(path, 4, 0)
	if err != nil {
		t.Fatalf("newFileRotator failed: %v", err)
	}
	defer r.Close()

	for i := 0; i < 4; i++ {
		if _, err := r.Write([]byte("abcd")); err != nil {
			t.Fatalf("write %d failed: %v", i, err)
		}
		time.Sleep(time.Millisecond)
	}

	matches, err := filepath.Glob(rotatedLogGlob(path))
	if err != nil {
		t.Fatalf("glob failed: %v", err)
	}
	if len(matches) != 3 {
		t.Fatalf("expected unlimited backups when max backups=0, got %d: %v", len(matches), matches)
	}
}
