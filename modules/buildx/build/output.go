package build

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func validateArtifactName(label, name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "<>:\"/\\|?*\x00") || strings.TrimRight(name, ". ") != name {
		return fmt.Errorf("%s must be a plain filename: %q", label, name)
	}
	return nil
}

// Resolve existing ancestors too, so a nonexistent output under a symlink is checked.
func resolvedPath(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return "", err
	}
	resolved, err = resolvedPath(parent)
	return filepath.Join(resolved, filepath.Base(path)), err
}

func validateCleanOutput(repoRoot, output string) error {
	root, err := resolvedPath(repoRoot)
	if err != nil {
		return err
	}
	dir, err := resolvedPath(output)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(dir, root)
	if err != nil || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		return fmt.Errorf("refusing to clean module root or its ancestor: %s", output)
	}
	err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if path == dir && os.IsNotExist(walkErr) {
				return nil
			}
			return walkErr
		}
		name := strings.ToLower(entry.Name())
		if name == "go.mod" || name == ".git" || strings.HasSuffix(name, ".go") {
			return fmt.Errorf("refusing to clean directory containing source: %s", path)
		}
		return nil
	})
	return err
}

// Build and hash every artifact before replacing any existing release files.
// Return true only when rollback fails and recovery files must be retained.
func publishRelease(stage, output string, paths []string) (bool, error) {
	backupDir := filepath.Join(stage, ".backup")
	if err := os.Mkdir(backupDir, 0o700); err != nil {
		return false, err
	}
	var backedUp, published []string
	rollback := func(cause error) (bool, error) {
		var recoveryErrors []string
		for _, name := range published {
			if err := os.Remove(filepath.Join(output, name)); err != nil && !os.IsNotExist(err) {
				recoveryErrors = append(recoveryErrors, err.Error())
			}
		}
		for _, name := range backedUp {
			if err := os.Rename(filepath.Join(backupDir, name), filepath.Join(output, name)); err != nil {
				recoveryErrors = append(recoveryErrors, err.Error())
			}
		}
		if len(recoveryErrors) > 0 {
			return true, fmt.Errorf("%w; release rollback failed; recovery files retained at %s: %s", cause, stage, strings.Join(recoveryErrors, "; "))
		}
		return false, cause
	}
	for _, path := range paths {
		name := filepath.Base(path)
		dest := filepath.Join(output, name)
		info, err := os.Lstat(dest)
		if err == nil {
			if !info.Mode().IsRegular() {
				return rollback(fmt.Errorf("release destination is not a regular file: %s", dest))
			}
			if err := os.Rename(dest, filepath.Join(backupDir, name)); err != nil {
				return rollback(err)
			}
			backedUp = append(backedUp, name)
		} else if !os.IsNotExist(err) {
			return rollback(err)
		}
		if err := os.Rename(path, dest); err != nil {
			return rollback(err)
		}
		published = append(published, name)
	}
	return false, nil
}
