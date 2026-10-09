package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/rannday/go-kit/modules/buildx/checksum"
)

func TestWriteAtomicZipDeterministic(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "bin.txt")
	if err := os.WriteFile(src, []byte("hello"), 0o755); err != nil {
		t.Fatal(err)
	}

	out1 := filepath.Join(dir, "one.zip")
	out2 := filepath.Join(dir, "two.zip")
	items := []Item{{Name: "myapp", Path: src}}

	if err := WriteAtomic(out1, FormatZip, items); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(out2, FormatZip, items); err != nil {
		t.Fatal(err)
	}

	sum1, err := checksum.SumFile(out1)
	if err != nil {
		t.Fatal(err)
	}
	sum2, err := checksum.SumFile(out2)
	if err != nil {
		t.Fatal(err)
	}
	if sum1 != sum2 {
		t.Fatalf("zip sums differ: %s %s", sum1, sum2)
	}

	zr, err := zip.OpenReader(out1)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	if len(zr.File) != 1 || zr.File[0].Name != "myapp" {
		t.Fatalf("zip entries = %#v", zr.File)
	}
	info, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	if got := zr.File[0].Mode().Perm(); got != info.Mode().Perm() {
		t.Fatalf("zip permissions = %o, want source permissions %o", got, info.Mode().Perm())
	}
}

func TestWriteAtomicTarGzDeterministic(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "bin.txt")
	if err := os.WriteFile(src, []byte("hello"), 0o755); err != nil {
		t.Fatal(err)
	}

	out1 := filepath.Join(dir, "one.tar.gz")
	out2 := filepath.Join(dir, "two.tar.gz")
	items := []Item{{Name: "myapp", Path: src}}

	if err := WriteAtomic(out1, FormatTarGz, items); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(out2, FormatTarGz, items); err != nil {
		t.Fatal(err)
	}

	sum1, err := checksum.SumFile(out1)
	if err != nil {
		t.Fatal(err)
	}
	sum2, err := checksum.SumFile(out2)
	if err != nil {
		t.Fatal(err)
	}
	if sum1 != sum2 {
		t.Fatalf("tar sums differ: %s %s", sum1, sum2)
	}

	file, err := os.Open(out1)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	hdr, err := tr.Next()
	if err != nil {
		t.Fatal(err)
	}
	if hdr.Name != "myapp" {
		t.Fatalf("tar entry = %q", hdr.Name)
	}
	info, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	if got := hdr.FileInfo().Mode().Perm(); got != info.Mode().Perm() {
		t.Fatalf("tar permissions = %o, want source permissions %o", got, info.Mode().Perm())
	}
}

func TestWriteAtomicExecutableMode(t *testing.T) {
	for _, format := range []Format{FormatZip, FormatTarGz} {
		t.Run(format.String(), func(t *testing.T) {
			dir := t.TempDir()
			src := filepath.Join(dir, "myapp")
			if err := os.WriteFile(src, []byte("binary"), 0o644); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(src)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm()&0o111 != 0 {
				t.Fatalf("source unexpectedly executable: %o", info.Mode().Perm())
			}
			out := filepath.Join(dir, "release."+format.String())
			if err := WriteAtomic(out, format, []Item{{Name: "myapp", Path: src, Mode: 0o755}}); err != nil {
				t.Fatal(err)
			}

			var mode os.FileMode
			if format == FormatZip {
				zr, err := zip.OpenReader(out)
				if err != nil {
					t.Fatal(err)
				}
				defer zr.Close()
				if len(zr.File) != 1 {
					t.Fatalf("zip entry count = %d", len(zr.File))
				}
				mode = zr.File[0].Mode()
			} else {
				file, err := os.Open(out)
				if err != nil {
					t.Fatal(err)
				}
				defer file.Close()
				gz, err := gzip.NewReader(file)
				if err != nil {
					t.Fatal(err)
				}
				defer gz.Close()
				hdr, err := tar.NewReader(gz).Next()
				if err != nil {
					t.Fatal(err)
				}
				mode = hdr.FileInfo().Mode()
			}
			if mode.Perm() != 0o755 {
				t.Fatalf("archive permissions = %o, want 755", mode.Perm())
			}
		})
	}
}
