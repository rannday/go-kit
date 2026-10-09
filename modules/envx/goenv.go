// Package envx provides a strict way to load environment variables into structs.
package envx

import (
	"encoding"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

var (
	durationType       = reflect.TypeOf(time.Duration(0))
	timeType           = reflect.TypeOf(time.Time{})
	byteSliceType      = reflect.TypeOf([]byte(nil))
	textUnmarshalerTyp = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
)

// MissingRequiredError is returned when a field tagged required:"true" has no
// resolved value from the process environment or dotenv file.
type MissingRequiredError struct {
	Key string
}

func (e MissingRequiredError) Error() string {
	return fmt.Sprintf("missing required env: %s", e.Key)
}

// EmptyRequiredError is returned when a required field resolves to an empty
// string and allow_empty:"true" is not set.
type EmptyRequiredError struct {
	Key string
}

func (e EmptyRequiredError) Error() string {
	return fmt.Sprintf("required env is empty: %s", e.Key)
}

// ParseError is returned when a resolved value cannot be parsed into the target field.
type ParseError struct {
	Key string
	Err error
}

func (e ParseError) Error() string {
	return fmt.Sprintf("invalid value for %s: %v", e.Key, e.Err)
}

func (e ParseError) Unwrap() error {
	return e.Err
}

// UnsupportedFieldError is returned when a resolved value cannot be written to its field.
type UnsupportedFieldError struct {
	Key  string
	Kind reflect.Kind
}

func (e UnsupportedFieldError) Error() string {
	return fmt.Sprintf("unsupported field type for %s: %s", e.Key, e.Kind)
}

// UnsettableFieldError is returned when a resolved value targets a field that cannot be set.
type UnsettableFieldError struct {
	Key string
}

func (e UnsettableFieldError) Error() string {
	return fmt.Sprintf("cannot set env %s: field is unsettable", e.Key)
}

// Options configures environment loading.
type Options struct {
	// DotEnvPath is an optional fallback file used when process env is missing.
	DotEnvPath string
}

// Load returns loaded config value of type T.
func Load[T any](opts ...Options) (T, error) {
	var value T

	if err := LoadInto(&value, opts...); err != nil {
		return value, err
	}

	return value, nil
}

// LoadInto overlays env values onto dst without zeroing existing fields.
func LoadInto(dst any, opts ...Options) error {
	if len(opts) > 1 {
		return fmt.Errorf("expected at most 1 Options value, got %d", len(opts))
	}

	var dotEnv map[string]string
	if len(opts) == 1 && opts[0].DotEnvPath != "" {
		parsed, err := parseDotEnv(opts[0].DotEnvPath)
		if err != nil {
			return err
		}
		dotEnv = parsed
	}

	v, err := validateDestination(dst)
	if err != nil {
		return err
	}

	state := traversalState{
		types:  make(map[reflect.Type]int),
		values: make(map[structVisit]bool),
	}
	_, err = loadStruct(v, dotEnv, &state)
	return err
}

func validateDestination(dst any) (reflect.Value, error) {
	if dst == nil {
		return reflect.Value{}, errors.New("dst must be non-nil pointer to struct")
	}

	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return reflect.Value{}, errors.New("dst must be non-nil pointer to struct")
	}

	elem := v.Elem()
	if elem.Kind() != reflect.Struct {
		return reflect.Value{}, errors.New("dst must be non-nil pointer to struct")
	}

	return elem, nil
}

type structVisit struct {
	typ  reflect.Type
	addr uintptr
}

type traversalState struct {
	types  map[reflect.Type]int
	values map[structVisit]bool
}

func loadStruct(v reflect.Value, dotEnv map[string]string, state *traversalState) (bool, error) {
	t := v.Type()
	visit := structVisit{typ: t, addr: v.Addr().Pointer()}
	if state.values[visit] {
		return false, nil // Stop an existing pointer cycle on the active path.
	}
	state.values[visit] = true
	state.types[t]++
	defer func() {
		delete(state.values, visit)
		state.types[t]--
	}()
	changed := false

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)
		key := fieldType.Tag.Get("env")
		if key == "-" {
			continue
		}

		if fieldType.PkgPath != "" {
			if fieldType.Anonymous {
				childChanged, err := loadContainer(field, dotEnv, state)
				if err != nil {
					return changed, err
				}
				changed = changed || childChanged
			}
			continue // unexported field
		}

		if key == "" {
			childChanged, err := loadContainer(field, dotEnv, state)
			if err != nil {
				return changed, err
			}
			changed = changed || childChanged
			continue
		}

		value, ok := resolveValue(key, dotEnv)

		required := tagTruthy(&fieldType, "required")
		allowEmpty := tagTruthy(&fieldType, "allow_empty")

		if !ok && required {
			return changed, MissingRequiredError{Key: key}
		}
		if required && value == "" && !allowEmpty {
			return changed, EmptyRequiredError{Key: key}
		}

		if !ok {
			continue
		}

		if !field.CanSet() {
			return changed, UnsettableFieldError{Key: key}
		}

		if err := setField(field, value); err != nil {
			var uk unsupportedKindError
			if errors.As(err, &uk) {
				return changed, UnsupportedFieldError{Key: key, Kind: uk.kind}
			}
			return changed, ParseError{Key: key, Err: err}
		}
		changed = true
	}

	return changed, nil
}

func loadContainer(field reflect.Value, dotEnv map[string]string, state *traversalState) (bool, error) {
	switch field.Kind() {
	case reflect.Struct:
		return loadStruct(field, dotEnv, state)
	case reflect.Pointer:
		if field.Type().Elem().Kind() != reflect.Struct {
			return false, nil
		}
		if field.IsNil() {
			// Inaccessible embeddings cannot be allocated. Repeated types must
			// not expand a recursive schema into an unbounded chain of nils.
			if !field.CanSet() || state.types[field.Type().Elem()] > 0 {
				return false, nil
			}
			tmp := reflect.New(field.Type().Elem()).Elem()
			changed, err := loadStruct(tmp, dotEnv, state)
			if err != nil {
				return false, err
			}
			if !changed {
				return false, nil
			}
			field.Set(reflect.New(field.Type().Elem()))
			field.Elem().Set(tmp)
			return true, nil
		}
		return loadStruct(field.Elem(), dotEnv, state)
	default:
		return false, nil
	}
}

func tagTruthy(fieldType *reflect.StructField, key string) bool {
	return strings.EqualFold(fieldType.Tag.Get(key), "true")
}

func resolveValue(key string, dotEnv map[string]string) (string, bool) {
	if value, ok := os.LookupEnv(key); ok {
		return value, true
	}

	if dotEnv != nil {
		if value, ok := dotEnv[key]; ok {
			return value, true
		}
	}

	return "", false
}

func setField(field reflect.Value, value string) error {
	if field.Kind() == reflect.Pointer {
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		field = field.Elem()
	}

	switch {
	case field.Type() == durationType:
		if value == "" {
			field.SetInt(0)
			return nil
		}
		d, err := time.ParseDuration(value)
		if err != nil {
			return err
		}
		field.SetInt(int64(d))
		return nil
	case field.Type() == timeType:
		if value == "" {
			field.Set(reflect.ValueOf(time.Time{}))
			return nil
		}
		t, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			t, err = time.Parse(time.RFC3339, value)
			if err != nil {
				return err
			}
		}
		field.Set(reflect.ValueOf(t))
		return nil
	case field.CanAddr() && field.Addr().Type().Implements(textUnmarshalerTyp):
		return field.Addr().Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(value))
	case field.Type() == byteSliceType:
		field.SetBytes([]byte(value))
		return nil
	case field.Kind() == reflect.Slice:
		return setSlice(field, value)
	}

	return setScalar(field, value)
}

func setSlice(field reflect.Value, value string) error {
	if strings.TrimSpace(value) == "" {
		field.Set(reflect.MakeSlice(field.Type(), 0, 0))
		return nil
	}

	parts := strings.Split(value, ",")
	slice := reflect.MakeSlice(field.Type(), 0, len(parts))

	for _, part := range parts {
		elem := reflect.New(field.Type().Elem()).Elem()
		if err := setField(elem, strings.TrimSpace(part)); err != nil {
			return err
		}
		slice = reflect.Append(slice, elem)
	}

	field.Set(slice)
	return nil
}

func setScalar(field reflect.Value, value string) error {
	switch field.Kind() {
	case reflect.String:
		field.SetString(value)
	case reflect.Bool:
		if value == "" {
			field.SetBool(false)
			return nil
		}
		b, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}
		field.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if value == "" {
			field.SetInt(0)
			return nil
		}
		i, err := strconv.ParseInt(value, 10, field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetInt(i)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if value == "" {
			field.SetUint(0)
			return nil
		}
		u, err := strconv.ParseUint(value, 10, field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetUint(u)
	case reflect.Float32, reflect.Float64:
		if value == "" {
			field.SetFloat(0)
			return nil
		}
		f, err := strconv.ParseFloat(value, field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetFloat(f)
	default:
		return unsupportedKindError{kind: field.Kind()}
	}

	return nil
}

type unsupportedKindError struct {
	kind reflect.Kind
}

func (e unsupportedKindError) Error() string {
	return fmt.Sprintf("unsupported type: %s", e.kind)
}
