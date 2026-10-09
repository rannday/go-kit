package logx

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const rotatedLogSuffix = ".logx-rotated-"

// fileRotator is a simple size-based log rotator.
// It is intentionally minimal: no time rotation, compression, or external deps.
// All methods are safe for concurrent use; writes and rotation share one mutex.
type fileRotator struct {
	path    string
	mu      sync.Mutex
	f       *os.File
	maxSize int
	backups int
	size    int64
	closed  bool
	now     func() time.Time
}

func newFileRotator(path string, maxSize int, backups int) (*fileRotator, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}

	info, err := f.Stat()
	var size int64
	if err == nil {
		size = info.Size()
	}
	r := &fileRotator{path: path, f: f, maxSize: maxSize, backups: backups, size: size, now: time.Now}
	return r, nil
}

func (r *fileRotator) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return 0, errors.New("file rotator closed")
	}

	if r.maxSize > 0 && r.size > 0 && r.size+int64(len(p)) > int64(r.maxSize) {
		if err := r.rotateLocked(); err != nil {
			return 0, err
		}
	}

	if r.f == nil {
		return 0, errors.New("file rotator closed")
	}

	n, err := r.f.Write(p)
	r.size += int64(n)
	if err != nil {
		return n, err
	}

	if r.maxSize > 0 && r.size > int64(r.maxSize) {
		if rotErr := r.rotateLocked(); rotErr != nil {
			return n, rotErr
		}
	}

	return n, nil
}

func (r *fileRotator) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	if r.f != nil {
		err := r.f.Close()
		r.f = nil
		return err
	}
	return nil
}

func rotatedName(path, ts string, seq int) string {
	if seq == 0 {
		return path + rotatedLogSuffix + ts
	}
	return fmt.Sprintf("%s%s%s-%d", path, rotatedLogSuffix, ts, seq)
}

func (r *fileRotator) rotateLocked() error {
	if r.f != nil {
		_ = r.f.Sync()
		_ = r.f.Close()
		r.f = nil
	}

	now := time.Now
	if r.now != nil {
		now = r.now
	}
	ts := now().UTC().Format("20060102T150405.000000000")

	rotated, err := r.renameToUnique(ts)
	if err != nil {
		f, err2 := r.reopenActiveFile()
		if err2 != nil {
			return fmt.Errorf("logx: rotate rename %q: %w", r.path, err)
		}
		r.f = f
		return err
	}

	f, err := r.reopenActiveFile()
	if err != nil {
		_ = os.Rename(rotated, r.path)
		fallback, reopenErr := r.reopenActiveFile()
		if reopenErr != nil {
			return fmt.Errorf("logx: rotate reopen %q after rollback: %w", r.path, err)
		}
		r.f = fallback
		return err
	}
	r.f = f
	r.size = 0

	if r.backups > 0 {
		entries, globErr := filepath.Glob(rotatedLogGlob(r.path))
		if globErr != nil {
			return globErr
		}
		sort.Strings(entries)
		for len(entries) > r.backups {
			_ = os.Remove(entries[0])
			entries = entries[1:]
		}
	}

	return nil
}

func (r *fileRotator) renameToUnique(ts string) (string, error) {
	var lastErr error
	for seq := 0; seq < 10000; seq++ {
		candidate := rotatedName(r.path, ts, seq)
		_, statErr := os.Stat(candidate)
		if statErr == nil {
			continue
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			return "", statErr
		}
		if err := os.Rename(r.path, candidate); err != nil {
			if _, again := os.Stat(candidate); again == nil {
				lastErr = err
				continue
			}
			return "", err
		}
		return candidate, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("logx: no unique rotated name for %q", r.path)
	}
	return "", lastErr
}

func (r *fileRotator) reopenActiveFile() (*os.File, error) {
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	info, statErr := f.Stat()
	if statErr == nil {
		r.size = info.Size()
	}
	return f, nil
}

func rotatedLogGlob(path string) string {
	if strings.HasSuffix(path, rotatedLogSuffix) {
		return path + "*"
	}
	return path + rotatedLogSuffix + "*"
}

var _ io.WriteCloser = (*fileRotator)(nil)
