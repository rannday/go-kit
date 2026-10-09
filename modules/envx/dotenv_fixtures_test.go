package envx

import (
	"os"
	"testing"
)

func writeTempDotEnv(t *testing.T, content string) string {
	t.Helper()

	file, err := os.CreateTemp("", ".env")
	if err != nil {
		t.Fatalf("failed creating temp file: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(file.Name())
	})

	if _, err := file.WriteString(content); err != nil {
		t.Fatalf("failed writing temp file: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("failed closing temp file: %v", err)
	}

	return file.Name()
}
