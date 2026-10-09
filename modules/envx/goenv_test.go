package envx

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

type recursiveConfigA struct {
	B     *recursiveConfigB
	Value string `env:"ENVX_TEST_RECURSIVE_A"`
}

type recursiveConfigB struct {
	A     *recursiveConfigA
	Value string `env:"ENVX_TEST_RECURSIVE_B"`
}

func TestLoadInto_RecursivePointers(t *testing.T) {
	type node struct {
		Next  *node
		Value string `env:"ENVX_TEST_RECURSIVE_NODE"`
	}
	t.Setenv("ENVX_TEST_RECURSIVE_NODE", "loaded")
	t.Run("nil", func(t *testing.T) {
		cfg, err := Load[node]()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Next != nil || cfg.Value != "loaded" {
			t.Fatalf("expected root loaded without recursive allocation, got %+v", cfg)
		}
	})
	for _, shape := range []string{"self cycle", "indirect cycle", "finite chain"} {
		t.Run(shape, func(t *testing.T) {
			cfg := &node{Value: "root"}
			child := &node{Value: "child"}
			switch shape {
			case "self cycle":
				cfg.Next = cfg
			case "indirect cycle":
				cfg.Next, child.Next = child, cfg
			case "finite chain":
				cfg.Next = child
			}
			before := cfg.Next
			if err := LoadInto(cfg); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.Value != "loaded" || cfg.Next != before {
				t.Fatal("expected root value loaded and pointer preserved")
			}
			if shape != "self cycle" && child.Value != "loaded" {
				t.Fatalf("expected child value loaded, got %q", child.Value)
			}
			if shape == "finite chain" && child.Next != nil {
				t.Fatal("expected recursive nil tail to remain nil")
			}
			if shape == "indirect cycle" && child.Next != cfg {
				t.Fatal("expected cycle preserved")
			}
		})
	}
	t.Run("siblings", func(t *testing.T) {
		var cfg struct{ First, Second *node }
		if err := LoadInto(&cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.First == nil || cfg.Second == nil || cfg.First == cfg.Second {
			t.Fatal("expected independent sibling allocations")
		}
		if cfg.First.Value != "loaded" || cfg.Second.Value != "loaded" || cfg.First.Next != nil || cfg.Second.Next != nil {
			t.Fatal("expected both siblings loaded without recursive tails")
		}
	})
}

func TestLoadInto_MutuallyRecursivePointers(t *testing.T) {
	t.Setenv("ENVX_TEST_RECURSIVE_A", "a")
	t.Setenv("ENVX_TEST_RECURSIVE_B", "b")
	cfg, err := Load[recursiveConfigA]()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Value != "a" || cfg.B == nil || cfg.B.Value != "b" || cfg.B.A != nil {
		t.Fatal("expected both types loaded without recursive back allocation")
	}
}

func TestLoadInto_UnexportedEmbeddedPointer(t *testing.T) {
	type inner struct {
		Host string `env:"ENVX_TEST_PRIVATE_EMBEDDED_HOST"`
	}
	type config struct {
		*inner
		Port int `env:"ENVX_TEST_PRIVATE_EMBEDDED_PORT"`
	}
	t.Setenv("ENVX_TEST_PRIVATE_EMBEDDED_HOST", "loaded")
	t.Setenv("ENVX_TEST_PRIVATE_EMBEDDED_PORT", "8080")
	for _, prepopulated := range []bool{false, true} {
		t.Run(fmt.Sprint(prepopulated), func(t *testing.T) {
			var cfg config
			if prepopulated {
				cfg.inner = &inner{Host: "preset"}
			}
			if err := LoadInto(&cfg); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if prepopulated {
				if cfg.Host != "loaded" {
					t.Fatalf("expected existing embedded pointer to load, got %q", cfg.Host)
				}
			} else if cfg.inner != nil {
				t.Fatal("expected inaccessible nil embedded pointer to remain nil")
			}
			if cfg.Port != 8080 {
				t.Fatalf("expected sibling to load, got %d", cfg.Port)
			}
		})
	}
}

func TestLoadInto_UnexportedEmbeddingSkipped(t *testing.T) {
	type inner struct {
		Host string `env:"ENVX_TEST_PRIVATE_SKIP_HOST" required:"true"`
	}
	t.Setenv("ENVX_TEST_PRIVATE_SKIP_HOST", "loaded")
	t.Run("value", func(t *testing.T) {
		cfg := struct {
			inner `env:"-"`
		}{inner{Host: "preset"}}
		if err := LoadInto(&cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Host != "preset" {
			t.Fatalf("skipped value changed: %q", cfg.Host)
		}
	})
	t.Run("pointer", func(t *testing.T) {
		cfg := struct {
			*inner `env:"-"`
		}{&inner{Host: "preset"}}
		if err := LoadInto(&cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Host != "preset" {
			t.Fatalf("skipped pointer changed: %q", cfg.Host)
		}
	})
}

type testConfig struct {
	Port  int    `env:"ENVX_TEST_PORT"`
	Host  string `env:"ENVX_TEST_HOST"`
	Debug bool   `env:"ENVX_TEST_DEBUG"`
}

func TestLoadInto_PreservesPrepopulatedValues(t *testing.T) {
	cfg := testConfig{
		Port:  8080,
		Host:  "preset",
		Debug: true,
	}

	if err := LoadInto(&cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Port != 8080 {
		t.Fatalf("expected Port=8080, got %d", cfg.Port)
	}
	if cfg.Host != "preset" {
		t.Fatalf("expected Host=preset, got %q", cfg.Host)
	}
	if !cfg.Debug {
		t.Fatalf("expected Debug=true, got false")
	}
}

func TestLoadInto_EnvOverridesPrepopulatedValues(t *testing.T) {
	t.Setenv("ENVX_TEST_PORT", "9090")
	t.Setenv("ENVX_TEST_HOST", "from_env")
	t.Setenv("ENVX_TEST_DEBUG", "false")

	cfg := testConfig{
		Port:  8080,
		Host:  "preset",
		Debug: true,
	}

	if err := LoadInto(&cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Port != 9090 {
		t.Fatalf("expected Port=9090, got %d", cfg.Port)
	}
	if cfg.Host != "from_env" {
		t.Fatalf("expected Host=from_env, got %q", cfg.Host)
	}
	if cfg.Debug {
		t.Fatalf("expected Debug=false, got true")
	}
}

func TestLoadInto_DotEnvFallback(t *testing.T) {
	content := "ENVX_TEST_DOTENV_KEY=\"secret-key-123\"\nENVX_TEST_DOTENV_NAME='My App'"
	path := writeTempDotEnv(t, content)

	type cfg struct {
		Key  string `env:"ENVX_TEST_DOTENV_KEY"`
		Name string `env:"ENVX_TEST_DOTENV_NAME"`
	}

	var c cfg
	if err := LoadInto(&c, Options{DotEnvPath: path}); err != nil {
		t.Fatalf("failed to load dotenv: %v", err)
	}

	if c.Key != "secret-key-123" {
		t.Errorf("expected secret-key-123, got %s", c.Key)
	}
	if c.Name != "My App" {
		t.Errorf("expected My App, got %s", c.Name)
	}
}

func TestLoadInto_EnvOverridesDotEnv(t *testing.T) {
	content := "ENVX_TEST_ORDER_KEY=from_file"
	path := writeTempDotEnv(t, content)

	t.Setenv("ENVX_TEST_ORDER_KEY", "from_env")

	type cfg struct {
		Key string `env:"ENVX_TEST_ORDER_KEY"`
	}

	var c cfg
	if err := LoadInto(&c, Options{DotEnvPath: path}); err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if c.Key != "from_env" {
		t.Fatalf("expected env to win, got %q", c.Key)
	}
}

func TestLoadInto_DotEnvUsedWhenEnvMissing(t *testing.T) {
	content := "ENVX_TEST_DOTENV_ONLY=from_file"
	path := writeTempDotEnv(t, content)

	type cfg struct {
		Value string `env:"ENVX_TEST_DOTENV_ONLY"`
	}

	var c cfg
	if err := LoadInto(&c, Options{DotEnvPath: path}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.Value != "from_file" {
		t.Fatalf("expected dotenv value, got %q", c.Value)
	}
}

func TestLoadInto_RequiredMissing(t *testing.T) {
	type cfg struct {
		Host string `env:"ENVX_TEST_REQUIRED_MISSING" required:"true"`
	}

	var c cfg
	if err := LoadInto(&c); err == nil {
		t.Fatal("expected error for missing required field")
	}
}

func TestLoadInto_RequiredRejectsExplicitEmpty(t *testing.T) {
	type cfg struct {
		Value string `env:"ENVX_TEST_REQUIRED_EMPTY" required:"true"`
	}

	t.Setenv("ENVX_TEST_REQUIRED_EMPTY", "")

	var c cfg
	if err := LoadInto(&c); err == nil {
		t.Fatal("expected error for empty required value")
	}
}

func TestLoadInto_RequiredAllowsExplicitEmptyWhenAllowEmpty(t *testing.T) {
	type cfg struct {
		Value string `env:"ENVX_TEST_REQUIRED_ALLOW_EMPTY" required:"true" allow_empty:"true"`
	}

	t.Setenv("ENVX_TEST_REQUIRED_ALLOW_EMPTY", "")

	var c cfg
	if err := LoadInto(&c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.Value != "" {
		t.Fatalf("expected empty string, got %q", c.Value)
	}
}

func TestLoadInto_InvalidInt(t *testing.T) {
	t.Setenv("ENVX_TEST_HOST", "localhost")
	t.Setenv("ENVX_TEST_PORT", "notanumber")

	var cfg testConfig
	err := LoadInto(&cfg)
	if err == nil {
		t.Fatal("expected error for invalid int")
	}
	var parseErr ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("expected ParseError, got %T: %v", err, err)
	}
}

func TestLoadInto_InvalidDstNil(t *testing.T) {
	err := LoadInto(nil)
	if err == nil {
		t.Fatal("expected error for nil dst")
	}
}

func TestLoadInto_InvalidDstNonPointer(t *testing.T) {
	var cfg testConfig
	err := LoadInto(cfg)
	if err == nil {
		t.Fatal("expected error for non-pointer dst")
	}
}

func TestLoadInto_InvalidDstNilPointer(t *testing.T) {
	var cfg *testConfig
	err := LoadInto(cfg)
	if err == nil {
		t.Fatal("expected error for nil pointer dst")
	}
}

func TestLoadInto_InvalidDstPointerToNonStruct(t *testing.T) {
	value := 123
	err := LoadInto(&value)
	if err == nil {
		t.Fatal("expected error for pointer to non-struct")
	}
}

func TestLoad_Generic(t *testing.T) {
	t.Setenv("ENVX_TEST_GENERIC_HOST", "localhost")

	type cfg struct {
		Host string `env:"ENVX_TEST_GENERIC_HOST"`
	}

	got, err := Load[cfg]()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Host != "localhost" {
		t.Fatalf("expected localhost, got %q", got.Host)
	}
}

func TestLoadInt64(t *testing.T) {
	t.Setenv("ENVX_TEST_TIMEOUT_I64", "60")

	type cfg struct {
		Timeout int64 `env:"ENVX_TEST_TIMEOUT_I64"`
	}

	got, err := Load[cfg]()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Timeout != 60 {
		t.Fatalf("expected 60, got %d", got.Timeout)
	}
}

func TestLoad_Duration(t *testing.T) {
	t.Setenv("ENVX_TEST_TIMEOUT_DURATION", "5s")

	type cfg struct {
		Timeout time.Duration `env:"ENVX_TEST_TIMEOUT_DURATION"`
	}

	got, err := Load[cfg]()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Timeout != 5*time.Second {
		t.Fatalf("expected 5s, got %v", got.Timeout)
	}
}

func TestLoad_StringSlice(t *testing.T) {
	t.Setenv("ENVX_TEST_HOSTS", "a.com,b.com")

	type cfg struct {
		Hosts []string `env:"ENVX_TEST_HOSTS"`
	}

	got, err := Load[cfg]()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got.Hosts) != 2 {
		t.Fatalf("expected 2 hosts, got %d", len(got.Hosts))
	}

	if got.Hosts[0] != "a.com" || got.Hosts[1] != "b.com" {
		t.Fatalf("unexpected slice values: %v", got.Hosts)
	}
}

func TestLoad_IntSlice(t *testing.T) {
	t.Setenv("ENVX_TEST_PORTS", "80,443")

	type cfg struct {
		Ports []int `env:"ENVX_TEST_PORTS"`
	}

	got, err := Load[cfg]()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got.Ports) != 2 || got.Ports[0] != 80 || got.Ports[1] != 443 {
		t.Fatalf("unexpected ports: %v", got.Ports)
	}
}

func TestLoad_EmptySliceValueYieldsEmptySlice(t *testing.T) {
	t.Setenv("ENVX_TEST_EMPTY_SLICE", "")

	type cfg struct {
		List []string `env:"ENVX_TEST_EMPTY_SLICE"`
	}

	got, err := Load[cfg]()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.List == nil {
		t.Fatal("expected non-nil empty slice")
	}

	if len(got.List) != 0 {
		t.Fatalf("expected empty slice, got %v", got.List)
	}
}

func TestLoad_OptionalEmptyIntLeavesZero(t *testing.T) {
	t.Setenv("ENVX_TEST_OPTIONAL_EMPTY_INT", "")

	type cfg struct {
		Port int `env:"ENVX_TEST_OPTIONAL_EMPTY_INT"`
	}

	got, err := Load[cfg]()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Port != 0 {
		t.Fatalf("expected zero value, got %d", got.Port)
	}
}

func TestLoad_TimeValue(t *testing.T) {
	t.Setenv("ENVX_TEST_TIME", "2026-05-15T12:34:56Z")

	type cfg struct {
		CreatedAt time.Time `env:"ENVX_TEST_TIME"`
	}

	got, err := Load[cfg]()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.CreatedAt.Format(time.RFC3339) != "2026-05-15T12:34:56Z" {
		t.Fatalf("unexpected time value: %s", got.CreatedAt.Format(time.RFC3339))
	}
}

func TestLoad_FloatValue(t *testing.T) {
	t.Setenv("ENVX_TEST_FLOAT", "1.25")

	type cfg struct {
		Ratio float64 `env:"ENVX_TEST_FLOAT"`
	}

	got, err := Load[cfg]()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Ratio != 1.25 {
		t.Fatalf("expected 1.25, got %v", got.Ratio)
	}
}

func TestLoad_UintValue(t *testing.T) {
	t.Setenv("ENVX_TEST_UINT", "42")

	type cfg struct {
		Count uint `env:"ENVX_TEST_UINT"`
	}

	got, err := Load[cfg]()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Count != 42 {
		t.Fatalf("expected 42, got %d", got.Count)
	}
}

func TestLoad_ByteSlice(t *testing.T) {
	t.Setenv("ENVX_TEST_BYTES", "abc")

	type cfg struct {
		Token []byte `env:"ENVX_TEST_BYTES"`
	}

	got, err := Load[cfg]()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(got.Token) != "abc" {
		t.Fatalf("expected abc, got %q", string(got.Token))
	}
}

type customString struct {
	value string
}

func (c *customString) UnmarshalText(text []byte) error {
	c.value = strings.ToUpper(string(text))
	return nil
}

type failingText struct{}

func (f *failingText) UnmarshalText([]byte) error {
	return errors.New("bad text")
}

func TestLoad_TextUnmarshaler(t *testing.T) {
	t.Setenv("ENVX_TEST_TEXT_UNMARSHALER", "hello")

	type cfg struct {
		Value customString `env:"ENVX_TEST_TEXT_UNMARSHALER"`
	}

	got, err := Load[cfg]()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Value.value != "HELLO" {
		t.Fatalf("unexpected unmarshaled value: %q", got.Value.value)
	}
}

func TestLoad_TextUnmarshalerError(t *testing.T) {
	t.Setenv("ENVX_TEST_TEXT_FAIL", "x")

	type cfg struct {
		Value failingText `env:"ENVX_TEST_TEXT_FAIL"`
	}

	var c cfg
	err := LoadInto(&c)
	if err == nil {
		t.Fatal("expected text unmarshaler error")
	}
	var parseErr ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("expected ParseError, got %T: %v", err, err)
	}
}

func TestLoad_UnexportedIgnored(t *testing.T) {
	type cfg struct {
		host string `env:"ENVX_TEST_UNEXPORTED_HOST"`
	}

	t.Setenv("ENVX_TEST_UNEXPORTED_HOST", "ignored")

	var c cfg
	if err := LoadInto(&c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.host != "" {
		t.Fatalf("expected unexported field to remain unset, got %q", c.host)
	}
}

func TestLoad_PointerScalarFromEnv(t *testing.T) {
	t.Setenv("ENVX_TEST_PTR_PORT", "8080")

	type cfg struct {
		Port *int `env:"ENVX_TEST_PTR_PORT"`
	}

	var c cfg
	if err := LoadInto(&c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.Port == nil {
		t.Fatal("expected Port pointer to be initialized")
	}

	if *c.Port != 8080 {
		t.Fatalf("expected 8080, got %d", *c.Port)
	}
}

func TestLoad_PointerScalarPreservesExistingWhenMissing(t *testing.T) {
	value := 55

	type cfg struct {
		Port *int `env:"ENVX_TEST_PTR_MISSING"`
	}

	c := cfg{Port: &value}
	if err := LoadInto(&c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.Port == nil || *c.Port != 55 {
		t.Fatalf("expected existing pointer preserved, got %v", c.Port)
	}
}

func TestLoad_PointerScalarEmptyEnvSetsZero(t *testing.T) {
	t.Setenv("ENVX_TEST_PTR_EMPTY", "")

	type cfg struct {
		Port *int `env:"ENVX_TEST_PTR_EMPTY"`
	}

	var c cfg
	if err := LoadInto(&c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.Port == nil {
		t.Fatal("expected pointer to be initialized")
	}
	if *c.Port != 0 {
		t.Fatalf("expected zero value, got %d", *c.Port)
	}
}

func TestLoad_PointerStructFromEnv(t *testing.T) {
	t.Setenv("ENVX_TEST_PTR_DB_HOST", "db-host")

	type dbConfig struct {
		Host string `env:"ENVX_TEST_PTR_DB_HOST"`
	}

	type cfg struct {
		DB *dbConfig
	}

	var c cfg
	if err := LoadInto(&c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.DB == nil {
		t.Fatal("expected nested pointer struct to be allocated")
	}
	if c.DB.Host != "db-host" {
		t.Fatalf("expected db-host, got %q", c.DB.Host)
	}
}

func TestLoad_PointerStructPreservesExistingWhenMissing(t *testing.T) {
	type dbConfig struct {
		Host string `env:"ENVX_TEST_PTR_DB_HOST_MISSING"`
	}

	existing := &dbConfig{Host: "preset"}
	type cfg struct {
		DB *dbConfig
	}

	c := cfg{DB: existing}
	if err := LoadInto(&c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.DB == nil {
		t.Fatal("expected existing pointer preserved")
	}
	if c.DB.Host != "preset" {
		t.Fatalf("expected preset host, got %q", c.DB.Host)
	}
}

func TestLoad_EmbeddedStruct(t *testing.T) {
	t.Setenv("ENVX_TEST_EMBEDDED_HOST", "embedded-host")
	t.Setenv("ENVX_TEST_EMBEDDED_PORT", "443")

	type inner struct {
		Host string `env:"ENVX_TEST_EMBEDDED_HOST"`
	}

	type cfg struct {
		inner
		Port int `env:"ENVX_TEST_EMBEDDED_PORT"`
	}

	var c cfg
	if err := LoadInto(&c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.Host != "embedded-host" {
		t.Fatalf("expected embedded host, got %q", c.Host)
	}
	if c.Port != 443 {
		t.Fatalf("expected port 443, got %d", c.Port)
	}
}

func TestLoad_NestedStruct(t *testing.T) {
	t.Setenv("ENVX_TEST_NESTED_HOST", "db-host")
	t.Setenv("ENVX_TEST_NESTED_PORT", "5432")

	type cfg struct {
		DB struct {
			Host string `env:"ENVX_TEST_NESTED_HOST"`
			Port int    `env:"ENVX_TEST_NESTED_PORT"`
		}
	}

	var c cfg
	if err := LoadInto(&c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.DB.Host != "db-host" {
		t.Fatalf("expected db-host, got %q", c.DB.Host)
	}
	if c.DB.Port != 5432 {
		t.Fatalf("expected port 5432, got %d", c.DB.Port)
	}
}

func TestLoad_NestedPointerStruct(t *testing.T) {
	t.Setenv("ENVX_TEST_NESTED_PTR_HOST", "ptr-host")

	type cfg struct {
		DB *struct {
			Host string `env:"ENVX_TEST_NESTED_PTR_HOST"`
		}
	}

	var c cfg
	if err := LoadInto(&c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.DB == nil {
		t.Fatal("expected nested pointer struct to be allocated")
	}
	if c.DB.Host != "ptr-host" {
		t.Fatalf("expected ptr-host, got %q", c.DB.Host)
	}
}

func TestLoad_NestedStructSkippedWithEnvDash(t *testing.T) {
	type cfg struct {
		DB struct {
			Host string `env:"ENVX_TEST_SKIPPED_HOST"`
		} `env:"-"`
		Port int `env:"ENVX_TEST_SKIPPED_PORT"`
	}

	t.Setenv("ENVX_TEST_SKIPPED_HOST", "should-not-load")
	t.Setenv("ENVX_TEST_SKIPPED_PORT", "80")

	var c cfg
	if err := LoadInto(&c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.DB.Host != "" {
		t.Fatalf("expected skipped nested field to remain zero, got %q", c.DB.Host)
	}
	if c.Port != 80 {
		t.Fatalf("expected port 80, got %d", c.Port)
	}
}

func TestLoad_DotEnvDoesNotMutateProcessEnv(t *testing.T) {
	content := "ENVX_TEST_NO_MUTATE=from_file"
	path := writeTempDotEnv(t, content)

	type cfg struct {
		Value string `env:"ENVX_TEST_NO_MUTATE"`
	}

	var c cfg
	if err := LoadInto(&c, Options{DotEnvPath: path}); err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if c.Value != "from_file" {
		t.Fatalf("expected value from dotenv fallback, got %q", c.Value)
	}

	if _, ok := os.LookupEnv("ENVX_TEST_NO_MUTATE"); ok {
		t.Fatal("expected dotenv load not to mutate process environment")
	}
}

func TestLoad_SliceTrimming(t *testing.T) {
	t.Setenv("ENVX_TEST_LIST", " one, two , three ")

	type cfg struct {
		List []string `env:"ENVX_TEST_LIST"`
	}

	var c cfg
	if err := LoadInto(&c); err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	expected := []string{"one", "two", "three"}
	for i, v := range c.List {
		if v != expected[i] {
			t.Errorf("expected %q, got %q", expected[i], v)
		}
	}
}

func TestLoad_OptionalEmptyIntPreservesZero(t *testing.T) {
	type cfg struct {
		Port int `env:"ENVX_TEST_OPTIONAL_INT"`
	}

	var c cfg
	if err := LoadInto(&c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.Port != 0 {
		t.Fatalf("expected zero value, got %d", c.Port)
	}
}

func TestLoad_TimeValuePreservesPrepopulatedValueWhenMissing(t *testing.T) {
	type cfg struct {
		CreatedAt time.Time `env:"ENVX_TEST_TIME_MISSING"`
	}

	existing := time.Date(2026, time.May, 15, 12, 34, 56, 0, time.UTC)
	c := cfg{CreatedAt: existing}
	if err := LoadInto(&c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !c.CreatedAt.Equal(existing) {
		t.Fatalf("expected existing time preserved, got %s", c.CreatedAt.Format(time.RFC3339))
	}
}

func TestLoad_UnsupportedType(t *testing.T) {
	type cfg struct {
		Value map[string]string `env:"ENVX_TEST_UNSUPPORTED_MAP"`
	}

	t.Setenv("ENVX_TEST_UNSUPPORTED_MAP", "a=b")

	var c cfg
	err := LoadInto(&c)
	if err == nil {
		t.Fatal("expected unsupported type error")
	}
	var unsupported UnsupportedFieldError
	if !errors.As(err, &unsupported) {
		t.Fatalf("expected UnsupportedFieldError, got %T: %v", err, err)
	}
}
