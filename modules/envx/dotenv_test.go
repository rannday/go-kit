package envx

import (
	"strings"
	"testing"
)

func TestParseDotEnv_ParsesKeyValues(t *testing.T) {
	content := "FOO=bar\nBAR=baz"
	path := writeTempDotEnv(t, content)

	values, err := parseDotEnv(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if values["FOO"] != "bar" {
		t.Fatalf("expected FOO=bar, got %q", values["FOO"])
	}
	if values["BAR"] != "baz" {
		t.Fatalf("expected BAR=baz, got %q", values["BAR"])
	}
}

func TestParseDotEnv_IgnoresCommentsAndEmptyLines(t *testing.T) {
	content := "\n# comment\nFOO=bar\n\n# another\nBAR=baz\n"
	path := writeTempDotEnv(t, content)

	values, err := parseDotEnv(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(values) != 2 {
		t.Fatalf("expected 2 values, got %d", len(values))
	}
}

func TestParseDotEnv_StripsInlineCommentOutsideQuotes(t *testing.T) {
	content := "FOO=bar # this is a comment"
	path := writeTempDotEnv(t, content)

	values, err := parseDotEnv(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if values["FOO"] != "bar" {
		t.Fatalf("expected FOO=bar, got %q", values["FOO"])
	}
}

func TestParseDotEnv_KeepsHashInsideQuotes(t *testing.T) {
	content := "FOO=\"abc #123\""
	path := writeTempDotEnv(t, content)

	values, err := parseDotEnv(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if values["FOO"] != "abc #123" {
		t.Fatalf("expected quoted hash to be preserved, got %q", values["FOO"])
	}
}

func TestParseDotEnv_SupportsSingleQuotes(t *testing.T) {
	content := "FOO='My App'"
	path := writeTempDotEnv(t, content)

	values, err := parseDotEnv(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if values["FOO"] != "My App" {
		t.Fatalf("expected single-quoted value, got %q", values["FOO"])
	}
}

func TestParseDotEnv_SupportsExportPrefix(t *testing.T) {
	content := "export FOO=bar"
	path := writeTempDotEnv(t, content)

	values, err := parseDotEnv(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if values["FOO"] != "bar" {
		t.Fatalf("expected export-prefixed key to parse, got %q", values["FOO"])
	}
}

func TestParseDotEnv_InvalidKeyErrors(t *testing.T) {
	content := " =bad"
	path := writeTempDotEnv(t, content)

	if _, err := parseDotEnv(path); err == nil {
		t.Fatal("expected invalid key error")
	}
}

func TestParseDotEnv_InvalidLineErrors(t *testing.T) {
	content := "not-a-key-value-line"
	path := writeTempDotEnv(t, content)

	_, err := parseDotEnv(path)
	if err == nil {
		t.Fatal("expected invalid line error")
	}
	if !strings.Contains(err.Error(), ":1:") {
		t.Fatalf("expected line number in error, got %v", err)
	}
}

func TestParseDotEnv_EmptyValue(t *testing.T) {
	content := "FOO="
	path := writeTempDotEnv(t, content)

	values, err := parseDotEnv(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := values["FOO"]; !ok {
		t.Fatal("expected FOO key to be present")
	}
	if values["FOO"] != "" {
		t.Fatalf("expected empty value, got %q", values["FOO"])
	}
}

func TestParseDotEnv_DuplicateKeysLastWins(t *testing.T) {
	content := "FOO=first\nFOO=second"
	path := writeTempDotEnv(t, content)

	values, err := parseDotEnv(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if values["FOO"] != "second" {
		t.Fatalf("expected last duplicate to win, got %q", values["FOO"])
	}
}

func TestParseDotEnv_MissingFileWrapsError(t *testing.T) {
	_, err := parseDotEnv("/nonexistent/path/.env")
	if err == nil {
		t.Fatal("expected missing file error")
	}
	if !strings.Contains(err.Error(), "read dotenv") {
		t.Fatalf("expected wrapped read error, got %v", err)
	}
}

func TestParseDotEnv_StripsBOM(t *testing.T) {
	content := "\ufeffFOO=bar"
	path := writeTempDotEnv(t, content)

	values, err := parseDotEnv(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if values["FOO"] != "bar" {
		t.Fatalf("expected BOM-prefixed key to parse, got %q", values["FOO"])
	}
}
