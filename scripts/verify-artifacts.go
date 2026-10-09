// Run with: go run scripts/verify-artifacts.go DIRECTORY VERSION
package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	if len(os.Args) != 3 {
		panic("usage: verify-artifacts DIRECTORY VERSION")
	}
	if err := verify(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Five buildx archives, executable names/modes, and sorted SHA-256 manifest verified.")
}

func verify(dir, version string) error {
	targets := []string{"windows-amd64.zip", "linux-amd64.tar.gz", "linux-arm64.tar.gz", "darwin-amd64.tar.gz", "darwin-arm64.tar.gz"}
	sort.Strings(targets) // The existing writer sorts by artifact name, not hash.
	var lines []string
	for _, target := range targets {
		name := "buildx-" + version + "-" + target
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines = append(lines, fmt.Sprintf("%x  %s", sha256.Sum256(data), name))
		binary := "buildx"
		if strings.HasPrefix(target, "windows-") {
			binary += ".exe"
		}
		if strings.HasSuffix(target, ".zip") {
			z, err := zip.OpenReader(path)
			if err != nil {
				return err
			}
			if len(z.File) != 1 || z.File[0].Name != binary || z.File[0].Mode().Perm() != 0755 || z.File[0].UncompressedSize64 == 0 {
				z.Close()
				return fmt.Errorf("invalid archive %s", name)
			}
			r, err := z.File[0].Open()
			if err != nil {
				z.Close()
				return err
			}
			_, err = io.Copy(io.Discard, r)
			r.Close()
			z.Close()
			if err != nil {
				return err
			}
		} else {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			g, err := gzip.NewReader(f)
			if err != nil {
				f.Close()
				return err
			}
			t := tar.NewReader(g)
			h, err := t.Next()
			if err != nil || h.Name != binary || h.Mode != 0755 || h.Typeflag != tar.TypeReg || h.Size <= 0 {
				g.Close()
				f.Close()
				return fmt.Errorf("invalid archive %s", name)
			}
			_, err = io.Copy(io.Discard, t)
			if err == nil {
				_, err = t.Next()
				if err == io.EOF {
					err = nil
				} else if err == nil {
					err = fmt.Errorf("extra member in %s", name)
				}
			}
			if err == nil {
				_, err = io.Copy(io.Discard, g)
			}
			g.Close()
			f.Close()
			if err != nil {
				return err
			}
		}
	}
	manifest, err := os.ReadFile(filepath.Join(dir, "checksums.txt"))
	if err != nil {
		return err
	}
	if string(manifest) != strings.Join(lines, "\n")+"\n" {
		return fmt.Errorf("checksum manifest differs")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries) != 6 {
		return fmt.Errorf("unexpected release files: %d", len(entries))
	}
	return nil
}
