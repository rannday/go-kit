package build

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPublishReleaseReplacesExistingArtifacts(t *testing.T) {
	out := t.TempDir()
	stage, err := os.MkdirTemp(out, ".build-release-*")
	if err != nil {
		t.Fatal(err)
	}
	names := []string{"app.tar.gz", DefaultChecksumName}
	var paths []string
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(out, name), []byte("old "+name), 0o644); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(stage, name)
		if err := os.WriteFile(path, []byte("new "+name), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	keep, err := publishRelease(stage, out, paths)
	if err != nil || keep {
		t.Fatalf("publish failed: keep=%v, err=%v", keep, err)
	}
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(out, name))
		if err != nil || string(data) != "new "+name {
			t.Fatalf("artifact not replaced: %q, %v", data, err)
		}
		data, err = os.ReadFile(filepath.Join(stage, ".backup", name))
		if err != nil || string(data) != "old "+name {
			t.Fatalf("original not backed up: %q, %v", data, err)
		}
	}
}

func TestPublishReleaseRestoresOverwritesOnFailure(t *testing.T) {
	for _, failure := range []string{"checksum-directory", "missing-staged-file"} {
		t.Run(failure, func(t *testing.T) {
			out := t.TempDir()
			stage, err := os.MkdirTemp(out, ".build-release-*")
			if err != nil {
				t.Fatal(err)
			}
			archive := "app.tar.gz"
			if err := os.WriteFile(filepath.Join(out, archive), []byte("old archive"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(stage, archive), []byte("new archive"), 0o644); err != nil {
				t.Fatal(err)
			}
			manifest := filepath.Join(out, DefaultChecksumName)
			if failure == "checksum-directory" {
				if err := os.Mkdir(manifest, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(stage, DefaultChecksumName), []byte("new sums"), 0o644); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.WriteFile(manifest, []byte("old sums"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			keep, err := publishRelease(stage, out, []string{filepath.Join(stage, archive), filepath.Join(stage, DefaultChecksumName)})
			if err == nil || keep {
				t.Fatalf("expected recoverable failure: keep=%v, err=%v", keep, err)
			}
			data, err := os.ReadFile(filepath.Join(out, archive))
			if err != nil || string(data) != "old archive" {
				t.Fatalf("archive not restored: %q, %v", data, err)
			}
			if failure == "missing-staged-file" {
				data, err = os.ReadFile(manifest)
				if err != nil || string(data) != "old sums" {
					t.Fatalf("manifest not restored: %q, %v", data, err)
				}
			} else if info, err := os.Stat(manifest); err != nil || !info.IsDir() {
				t.Fatalf("manifest directory changed: %v", err)
			}
		})
	}
}

func TestCleanRejectsSymlinkToModule(t *testing.T) {
	root := setupTestModule(t)
	link := filepath.Join(t.TempDir(), "module-link")
	if err := os.Symlink(root, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := validateCleanOutput(root, link); err == nil {
		t.Fatal("expected module symlink rejection")
	}
}
