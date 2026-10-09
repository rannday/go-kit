// Package confx provides small helpers for loading configuration files into
// caller-defined structs. Supported formats are selected by file extension:
// .toml, .json, .jsonc, .yaml, .yml, and .ini.
package confx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/tailscale/hujson"
	"go.yaml.in/yaml/v4"
	"gopkg.in/ini.v1"
)

var (
	// ErrEmptyPath is returned when a path is empty or only whitespace.
	ErrEmptyPath = errors.New("invalid path: empty")

	// ErrUnsupportedFormat is returned when the file extension is not supported.
	ErrUnsupportedFormat = errors.New("unsupported config format")
)

type decoder func(data []byte, value any) error

// Load reads a config file and decodes it into T.
func Load[T any](path string) (T, error) {
	var zero T
	var value T
	if err := LoadInto(path, &value); err != nil {
		return zero, err
	}
	return value, nil
}

// LoadFS reads a config file from fsys and decodes it into T.
func LoadFS[T any](fsys fs.FS, path string) (T, error) {
	var zero T
	var value T
	if err := LoadFSInto(fsys, path, &value); err != nil {
		return zero, err
	}
	return value, nil
}

// Decode decodes data into T. The source name must include a supported file
// extension, which selects the decoder.
func Decode[T any](data []byte, source string) (T, error) {
	var value T
	var zero T
	if err := DecodeInto(data, source, &value); err != nil {
		return zero, err
	}
	return value, nil
}

// LoadInto reads a config file and decodes it into dst.
func LoadInto(path string, dst any) error {
	if err := validateDestination(dst); err != nil {
		return err
	}
	if err := validatePath(path); err != nil {
		return fmt.Errorf("read %q: %w", path, err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %q: %w", path, err)
	}

	return DecodeInto(data, path, dst)
}

// LoadFSInto reads a config file from fsys and decodes it into dst.
func LoadFSInto(fsys fs.FS, path string, dst any) error {
	if err := validateDestination(dst); err != nil {
		return err
	}
	if err := validatePath(path); err != nil {
		return fmt.Errorf("read %q: %w", path, err)
	}

	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return fmt.Errorf("read %q: %w", path, err)
	}

	return DecodeInto(data, path, dst)
}

// DecodeInto decodes data into dst. The source name must include a supported
// file extension, which selects the decoder.
func DecodeInto(data []byte, source string, dst any) error {
	if err := validateDestination(dst); err != nil {
		return err
	}

	decoder, err := decoderForSource(source)
	if err != nil {
		return fmt.Errorf("decode %q: %w", source, err)
	}

	if err := decoder(data, dst); err != nil {
		return fmt.Errorf("decode %q: %w", source, err)
	}

	return nil
}

func validatePath(path string) error {
	if strings.TrimSpace(path) == "" {
		return ErrEmptyPath
	}
	return nil
}

func validateDestination(dst any) error {
	if dst == nil {
		return fmt.Errorf("invalid destination: nil")
	}

	value := reflect.ValueOf(dst)
	if value.Kind() != reflect.Pointer {
		return fmt.Errorf("invalid destination: want non-nil pointer")
	}
	if value.IsNil() {
		return fmt.Errorf("invalid destination: nil pointer")
	}
	return nil
}

func decoderForSource(source string) (decoder, error) {
	switch formatForSource(source) {
	case "toml":
		return decodeTOML, nil
	case "json":
		return decodeJSON, nil
	case "jsonc":
		return decodeJSONC, nil
	case "yaml":
		return decodeYAML, nil
	case "ini":
		return decodeINI, nil
	default:
		return nil, fmt.Errorf("unsupported config format for %q: %w", source, ErrUnsupportedFormat)
	}
}

func formatForSource(source string) string {
	switch strings.ToLower(filepath.Ext(source)) {
	case ".toml":
		return "toml"
	case ".json":
		return "json"
	case ".jsonc":
		return "jsonc"
	case ".yaml", ".yml":
		return "yaml"
	case ".ini":
		return "ini"
	default:
		return ""
	}
}

func decodeTOML(data []byte, value any) error {
	return toml.Unmarshal(data, value)
}

func decodeJSON(data []byte, value any) error {
	return json.Unmarshal(data, value)
}

func decodeJSONC(data []byte, value any) error {
	standardized, err := hujson.Standardize(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(standardized, value)
}

func decodeYAML(data []byte, value any) error {
	return yaml.Unmarshal(data, value)
}

func decodeINI(data []byte, value any) error {
	return ini.MapTo(value, data)
}
