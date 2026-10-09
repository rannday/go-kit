package confx

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type testConfig struct {
	Name    string   `toml:"name" json:"name" yaml:"name" ini:"name"`
	Port    *int     `toml:"port" json:"port" yaml:"port" ini:"port"`
	Enabled *bool    `toml:"enabled" json:"enabled" yaml:"enabled" ini:"enabled"`
	Tags    []string `toml:"tags" json:"tags" yaml:"tags" ini:"tags"`
}

type defaultConfig struct {
	Name    string `toml:"name" json:"name" yaml:"name" ini:"name"`
	Port    int    `toml:"port" json:"port" yaml:"port" ini:"port"`
	Enabled bool   `toml:"enabled" json:"enabled" yaml:"enabled" ini:"enabled"`
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write test file: %v", err)
	}
}

func TestLoadFormats(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		data     string
		want     testConfig
	}{
		{
			name:     "TOML",
			filename: "config.toml",
			data: `
name = "example"
port = 8080
enabled = true
tags = ["api", "worker"]
`,
			want: testConfig{
				Name:    "example",
				Port:    ptr(8080),
				Enabled: ptr(true),
				Tags:    []string{"api", "worker"},
			},
		},
		{
			name:     "JSON",
			filename: "config.json",
			data:     `{"name":"example","port":8080,"enabled":true,"tags":["api","worker"]}`,
			want: testConfig{
				Name:    "example",
				Port:    ptr(8080),
				Enabled: ptr(true),
				Tags:    []string{"api", "worker"},
			},
		},
		{
			name:     "JSONC",
			filename: "config.jsonc",
			data: `{
  // app settings
  "name": "example",
  "port": 8080,
  "enabled": true,
  "tags": ["api", "worker",],
}`,
			want: testConfig{
				Name:    "example",
				Port:    ptr(8080),
				Enabled: ptr(true),
				Tags:    []string{"api", "worker"},
			},
		},
		{
			name:     "YAML",
			filename: "config.yaml",
			data: `
name: example
port: 8080
enabled: true
tags:
  - api
  - worker
`,
			want: testConfig{
				Name:    "example",
				Port:    ptr(8080),
				Enabled: ptr(true),
				Tags:    []string{"api", "worker"},
			},
		},
		{
			name:     "YML",
			filename: "config.yml",
			data: `
name: example
port: 8080
enabled: true
tags:
  - api
  - worker
`,
			want: testConfig{
				Name:    "example",
				Port:    ptr(8080),
				Enabled: ptr(true),
				Tags:    []string{"api", "worker"},
			},
		},
		{
			name:     "INI",
			filename: "config.ini",
			data: `
name = example
port = 8080
enabled = true
tags = api, worker
`,
			want: testConfig{
				Name:    "example",
				Port:    ptr(8080),
				Enabled: ptr(true),
				Tags:    []string{"api", "worker"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), tt.filename)
			writeFile(t, path, tt.data)

			got, err := Load[testConfig](path)
			if err != nil {
				t.Fatalf("Load returned error: %v", err)
			}

			assertConfig(t, got, tt.want)
		})
	}
}

func TestLoadFS(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "config.toml"), `
name = "embedded"
port = 9090
`)

	got, err := LoadFS[testConfig](os.DirFS(dir), "config.toml")
	if err != nil {
		t.Fatalf("LoadFS returned error: %v", err)
	}

	assertConfig(t, got, testConfig{
		Name: "embedded",
		Port: ptr(9090),
	})
}

func TestLoadIntoPreservesDefaults(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		data     string
	}{
		{
			name:     "TOML",
			filename: "config.toml",
			data:     `name = "file"`,
		},
		{
			name:     "JSON",
			filename: "config.json",
			data:     `{"name":"file"}`,
		},
		{
			name:     "JSONC",
			filename: "config.jsonc",
			data: `{
  "name": "file",
}`,
		},
		{
			name:     "YAML",
			filename: "config.yaml",
			data:     "name: file\n",
		},
		{
			name:     "YML",
			filename: "config.yml",
			data:     "name: file\n",
		},
		{
			name:     "INI",
			filename: "config.ini",
			data:     "name = file\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), tt.filename)
			writeFile(t, path, tt.data)

			cfg := defaultConfig{
				Name:    "default",
				Port:    8080,
				Enabled: true,
			}

			if err := LoadInto(path, &cfg); err != nil {
				t.Fatalf("LoadInto returned error: %v", err)
			}

			assertDefaultConfig(t, cfg, defaultConfig{
				Name:    "file",
				Port:    8080,
				Enabled: true,
			})
		})
	}
}

func TestLoadIntoOverridesDefaults(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		data     string
		want     defaultConfig
	}{
		{
			name:     "TOML",
			filename: "config.toml",
			data: `
port = 9090
enabled = false
`,
			want: defaultConfig{
				Name:    "default",
				Port:    9090,
				Enabled: false,
			},
		},
		{
			name:     "JSON",
			filename: "config.json",
			data:     `{"port":9090,"enabled":false}`,
			want: defaultConfig{
				Name:    "default",
				Port:    9090,
				Enabled: false,
			},
		},
		{
			name:     "JSONC",
			filename: "config.jsonc",
			data: `{
  "port": 9090,
  "enabled": false,
}`,
			want: defaultConfig{
				Name:    "default",
				Port:    9090,
				Enabled: false,
			},
		},
		{
			name:     "YAML",
			filename: "config.yaml",
			data: `
port: 9090
enabled: false
`,
			want: defaultConfig{
				Name:    "default",
				Port:    9090,
				Enabled: false,
			},
		},
		{
			name:     "YML",
			filename: "config.yml",
			data: `
port: 9090
enabled: false
`,
			want: defaultConfig{
				Name:    "default",
				Port:    9090,
				Enabled: false,
			},
		},
		{
			name:     "INI",
			filename: "config.ini",
			data: `
port = 9090
enabled = false
`,
			want: defaultConfig{
				Name:    "default",
				Port:    9090,
				Enabled: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), tt.filename)
			writeFile(t, path, tt.data)

			cfg := defaultConfig{
				Name:    "default",
				Port:    8080,
				Enabled: true,
			}

			if err := LoadInto(path, &cfg); err != nil {
				t.Fatalf("LoadInto returned error: %v", err)
			}

			assertDefaultConfig(t, cfg, tt.want)
		})
	}
}

func TestDecodeIntoAndLoadFSInto(t *testing.T) {
	t.Run("DecodeInto", func(t *testing.T) {
		cfg := defaultConfig{
			Name:    "default",
			Port:    8080,
			Enabled: true,
		}

		if err := DecodeInto([]byte(`name = "decoded"`), "config.toml", &cfg); err != nil {
			t.Fatalf("DecodeInto returned error: %v", err)
		}

		assertDefaultConfig(t, cfg, defaultConfig{
			Name:    "decoded",
			Port:    8080,
			Enabled: true,
		})
	})

	t.Run("LoadFSInto", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "config.toml"), `
name = "embedded"
`)

		cfg := defaultConfig{
			Name:    "default",
			Port:    8080,
			Enabled: true,
		}

		if err := LoadFSInto(os.DirFS(dir), "config.toml", &cfg); err != nil {
			t.Fatalf("LoadFSInto returned error: %v", err)
		}

		assertDefaultConfig(t, cfg, defaultConfig{
			Name:    "embedded",
			Port:    8080,
			Enabled: true,
		})
	})
}

func TestInvalidDestination(t *testing.T) {
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "nil",
			call: func() error {
				return DecodeInto([]byte(`name = "x"`), "config.toml", nil)
			},
		},
		{
			name: "non-pointer",
			call: func() error {
				cfg := defaultConfig{}
				return DecodeInto([]byte(`name = "x"`), "config.toml", cfg)
			},
		},
		{
			name: "nil pointer",
			call: func() error {
				var cfg *defaultConfig
				return DecodeInto([]byte(`name = "x"`), "config.toml", cfg)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if err == nil {
				t.Fatal("DecodeInto returned nil error")
			}
			if !strings.Contains(err.Error(), "invalid destination") {
				t.Fatalf("error = %q, want invalid destination", err)
			}
		})
	}
}

func TestPointerFields(t *testing.T) {
	got, err := Decode[testConfig]([]byte(`
port = 9000
enabled = true
`), "config.toml")
	if err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}

	assertConfig(t, got, testConfig{
		Port:    ptr(9000),
		Enabled: ptr(true),
	})

	got, err = Decode[testConfig]([]byte(`name = "minimal"`), "config.toml")
	if err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}

	assertConfig(t, got, testConfig{Name: "minimal"})
}

func TestSliceFields(t *testing.T) {
	got, err := Decode[testConfig]([]byte(`tags = ["alpha", "beta", "stable"]`), "config.toml")
	if err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}

	assertStringSlice(t, got.Tags, []string{"alpha", "beta", "stable"})
}

func TestDecodeErrorsIncludeSource(t *testing.T) {
	_, err := Decode[testConfig]([]byte(`name =`), "config.toml")
	if err == nil {
		t.Fatal("Decode returned nil error")
	}
	if !strings.Contains(err.Error(), `decode "config.toml"`) {
		t.Fatalf("error = %q, want decode source", err)
	}
}

func TestReadErrorsIncludePath(t *testing.T) {
	path := t.TempDir()

	_, err := Load[testConfig](path)
	if err == nil {
		t.Fatal("Load returned nil error")
	}
	if !strings.HasPrefix(err.Error(), "read "+strconv.Quote(path)+": ") {
		t.Fatalf("error = %q, want read path", err)
	}
}

func TestEmptyPathWrapsErrEmptyPath(t *testing.T) {
	tests := []struct {
		name string
		load func() error
	}{
		{
			name: "Load",
			load: func() error {
				_, err := Load[testConfig](" ")
				return err
			},
		},
		{
			name: "LoadFS",
			load: func() error {
				_, err := LoadFS[testConfig](os.DirFS(t.TempDir()), "\t")
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.load()
			if err == nil {
				t.Fatal("load returned nil error")
			}
			if !errors.Is(err, ErrEmptyPath) {
				t.Fatalf("error = %v, want ErrEmptyPath", err)
			}
		})
	}
}

func TestUnsupportedFormatWrapsErrUnsupportedFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.env")
	writeFile(t, path, "NAME=example")

	_, err := Load[testConfig](path)
	if err == nil {
		t.Fatal("Load returned nil error")
	}
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("error = %v, want ErrUnsupportedFormat", err)
	}
	if !strings.HasPrefix(err.Error(), "decode "+strconv.Quote(path)+": ") {
		t.Fatalf("error = %q, want decode source", err)
	}

	_, err = Decode[testConfig]([]byte(`name = "x"`), "config")
	if err == nil {
		t.Fatal("Decode returned nil error")
	}
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("error = %v, want ErrUnsupportedFormat", err)
	}
}

func TestMissingFilePreservesNotExist(t *testing.T) {
	_, err := LoadFS[testConfig](os.DirFS(t.TempDir()), "missing.toml")
	if err == nil {
		t.Fatal("LoadFS returned nil error")
	}
	if !strings.Contains(err.Error(), `read "missing.toml"`) {
		t.Fatalf("error = %q, want read path", err)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("error = %v, want fs.ErrNotExist", err)
	}
}

func ptr[T any](value T) *T {
	return &value
}

func assertConfig(t *testing.T, got, want testConfig) {
	t.Helper()

	if got.Name != want.Name {
		t.Fatalf("Name = %q, want %q", got.Name, want.Name)
	}
	assertPointer(t, "Port", got.Port, want.Port)
	assertPointer(t, "Enabled", got.Enabled, want.Enabled)
	assertStringSlice(t, got.Tags, want.Tags)
}

func assertPointer[T comparable](t *testing.T, name string, got, want *T) {
	t.Helper()

	switch {
	case got == nil && want == nil:
		return
	case got == nil || want == nil:
		t.Fatalf("%s = %v, want %v", name, got, want)
	case *got != *want:
		t.Fatalf("%s = %v, want %v", name, *got, *want)
	}
}

func assertDefaultConfig(t *testing.T, got, want defaultConfig) {
	t.Helper()

	if got != want {
		t.Fatalf("config = %#v, want %#v", got, want)
	}
}

func assertStringSlice(t *testing.T, got, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("len(slice) = %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("slice[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
