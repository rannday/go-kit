package logx

import (
	"context"
	"log/slog"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

const redactedValue = "REDACTED"

// defaultURLRedactKeys are always applied to URL and form query names.
// Structured field redaction starts from the same names and then follows SetRedactedKeys.
var defaultURLRedactKeys = []string{
	"apikey",
	"password",
	"token",
	"key",
	"access_token",
	"refresh_token",
	"id_token",
	"client_secret",
}

var (
	redactedKeys         = defaultRedactedKeySet()
	redactedKeysMu       sync.RWMutex
	redactedKeysSnapshot atomic.Value // map[string]struct{}
)

func init() {
	redactedKeysSnapshot.Store(cloneKeySet(redactedKeys))
}

// SetRedactedKeys replaces the global structured redaction set exactly.
// Keys are normalized to lowercase. Built-in defaults are not retained unless
// they are passed explicitly.
func SetRedactedKeys(keys ...string) {
	redactedKeysMu.Lock()
	defer redactedKeysMu.Unlock()
	redactedKeys = make(map[string]struct{}, len(keys))
	for _, k := range keys {
		redactedKeys[strings.ToLower(k)] = struct{}{}
	}
	redactedKeysSnapshot.Store(cloneKeySet(redactedKeys))
}

// AddRedactedKeys appends keys to the redaction set (concurrency-safe).
func AddRedactedKeys(keys ...string) {
	redactedKeysMu.Lock()
	defer redactedKeysMu.Unlock()
	for _, k := range keys {
		redactedKeys[strings.ToLower(k)] = struct{}{}
	}
	redactedKeysSnapshot.Store(cloneKeySet(redactedKeys))
}

// ClearRedactedKeys removes all structured redaction keys, including defaults.
func ClearRedactedKeys() {
	redactedKeysMu.Lock()
	defer redactedKeysMu.Unlock()
	redactedKeys = map[string]struct{}{}
	redactedKeysSnapshot.Store(map[string]struct{}{})
}

func resetRedactedKeysToDefault() {
	redactedKeysMu.Lock()
	defer redactedKeysMu.Unlock()
	redactedKeys = defaultRedactedKeySet()
	redactedKeysSnapshot.Store(cloneKeySet(redactedKeys))
}

// RedactedKeySet returns a snapshot of configured redacted keys as a set.
// The returned map is a clone and can be safely mutated by callers.
func RedactedKeySet() map[string]struct{} {
	return cloneKeySet(redactedKeySetSnapshot())
}

func redactedKeySetSnapshot() map[string]struct{} {
	keys, _ := redactedKeysSnapshot.Load().(map[string]struct{})
	return keys
}

// ListRedactedKeys returns a sorted snapshot of configured redacted keys.
func ListRedactedKeys() []string {
	keys := RedactedKeySet()
	out := make([]string, 0, len(keys))
	for k := range keys {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// RedactQueryValues masks values for sensitive query names (case-insensitive).
// A name matches when it equals a key in keys, or when it ends with "_" plus a
// key of at least 5 characters. Short keys such as "key" stay exact-match so
// names like "sort_key" are left alone. Structured field redaction stays exact-match.
func RedactQueryValues(q url.Values, keys map[string]struct{}) {
	if len(keys) == 0 || len(q) == 0 {
		return
	}
	for k := range q {
		if queryKeySensitive(k, keys) {
			q.Set(k, redactedValue)
		}
	}
}

func queryKeySensitive(name string, keys map[string]struct{}) bool {
	lk := strings.ToLower(name)
	if _, ok := keys[lk]; ok {
		return true
	}
	for base := range keys {
		if len(base) < 5 {
			continue
		}
		if strings.HasSuffix(lk, "_"+base) {
			return true
		}
	}
	return false
}

// SanitizeURL returns a redacted URL string with sensitive query parameters
// and user credentials masked. Query redaction uses SetRedactedKeys plus the
// built-in default set, including access_token, refresh_token, id_token, and
// client_secret. Query names also match a "_" suffix of those keys. Structured
// fields use exact key matches only.
func SanitizeURL(u *url.URL) string {
	if u == nil {
		return ""
	}

	clone := *u
	if clone.User != nil {
		clone.User = url.UserPassword(redactedValue, redactedValue)
	}

	q := clone.Query()
	RedactQueryValues(q, urlRedactionKeySet())
	clone.RawQuery = q.Encode()
	return clone.String()
}

func urlNeedsRedaction(u *url.URL) bool {
	if u == nil {
		return false
	}
	if u.User != nil {
		return true
	}
	q := u.Query()
	if len(q) == 0 {
		return false
	}
	keys := urlRedactionKeySet()
	for k := range q {
		if queryKeySensitive(k, keys) {
			return true
		}
	}
	return false
}

func urlRedactionKeySet() map[string]struct{} {
	configured := redactedKeySetSnapshot()
	merged := make(map[string]struct{}, len(configured)+len(defaultURLRedactKeys))
	for k := range configured {
		merged[k] = struct{}{}
	}
	for _, k := range defaultURLRedactKeys {
		merged[k] = struct{}{}
	}
	return merged
}

var attrSlicePool = sync.Pool{
	New: func() any {
		s := make([]slog.Attr, 0, 32)
		return &s
	},
}

func borrowAttrSlice(estimate int) []slog.Attr {
	p, ok := attrSlicePool.Get().(*[]slog.Attr)
	if !ok || p == nil {
		return make([]slog.Attr, 0, estimate)
	}
	s := *p
	if cap(s) < estimate {
		return make([]slog.Attr, 0, estimate)
	}
	return s[:0]
}

func releaseAttrSlice(s []slog.Attr) {
	if cap(s) > 128 {
		return
	}
	s = s[:0]
	attrSlicePool.Put(&s)
}

type redactionHandler struct {
	baseDecorator
}

func newRedactionHandler(next slog.Handler) slog.Handler {
	h := &redactionHandler{}
	h.init(next, newRedactionHandler)
	return h
}

// WithAttrs redacts before the inner handler stores the attrs. Handle never
// sees attrs bound with Logger.With, so leaving this to baseDecorator leaks secrets.
func (h *redactionHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	keys := redactedKeySetSnapshot()
	if len(keys) == 0 {
		return h.baseDecorator.WithAttrs(attrs)
	}
	redacted := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		redacted[i], _ = redactAttr(a, keys)
	}
	return h.clone(h.next.WithAttrs(redacted))
}

func (h *redactionHandler) Handle(ctx context.Context, r slog.Record) error {
	keys := redactedKeySetSnapshot()
	if len(keys) == 0 {
		return h.next.Handle(ctx, r)
	}

	attrs := borrowAttrSlice(r.NumAttrs())
	changed := false
	r.Attrs(func(a slog.Attr) bool {
		redacted, ok := redactAttr(a, keys)
		if ok {
			changed = true
		}
		attrs = append(attrs, redacted)
		return true
	})

	if !changed {
		releaseAttrSlice(attrs)
		return h.next.Handle(ctx, r)
	}

	newRec := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	newRec.AddAttrs(attrs...)
	releaseAttrSlice(attrs)

	return h.next.Handle(ctx, newRec)
}

func cloneKeySet(src map[string]struct{}) map[string]struct{} {
	dst := make(map[string]struct{}, len(src))
	for k := range src {
		dst[k] = struct{}{}
	}
	return dst
}

func defaultRedactedKeySet() map[string]struct{} {
	keys := make(map[string]struct{}, len(defaultURLRedactKeys))
	for _, k := range defaultURLRedactKeys {
		keys[k] = struct{}{}
	}
	return keys
}

func redactAttr(a slog.Attr, keys map[string]struct{}) (slog.Attr, bool) {
	if _, ok := keys[strings.ToLower(a.Key)]; ok {
		a.Value = slog.StringValue(redactedValue)
		return a, true
	}

	if a.Value.Kind() == slog.KindLogValuer {
		a.Value = a.Value.Resolve()
		next, ok := redactResolvedAttr(a, keys)
		if ok {
			return next, true
		}
		return a, true
	}
	return redactResolvedAttr(a, keys)
}

func redactResolvedAttr(a slog.Attr, keys map[string]struct{}) (slog.Attr, bool) {
	switch a.Value.Kind() {
	case slog.KindGroup:
		group := a.Value.Group()
		redacted := make([]slog.Attr, 0, len(group))
		changed := false
		for _, child := range group {
			next, ok := redactAttr(child, keys)
			if ok {
				changed = true
			}
			redacted = append(redacted, next)
		}
		if !changed {
			return a, false
		}
		a.Value = slog.GroupValue(redacted...)
		return a, true
	case slog.KindAny:
		if changed, v := redactAnyValue(a.Value.Any(), keys); changed {
			a.Value = slog.AnyValue(v)
			return a, true
		}
	}

	return a, false
}

func redactAnyValue(v any, keys map[string]struct{}) (bool, any) {
	return redactAnyValueDepth(v, keys, 0, make(map[redactionVisit]bool))
}

// Bound traversal so cyclic or excessively deep containers cannot exhaust the stack.
const maxRedactionDepth = 64

type redactionVisit struct {
	kind reflect.Kind
	ptr  uintptr
	len  int
}

func redactAnyValueDepth(v any, keys map[string]struct{}, depth int, path map[redactionVisit]bool) (bool, any) {
	var visit redactionVisit
	switch x := v.(type) {
	case map[string]any:
		visit = redactionVisit{kind: reflect.Map, ptr: uintptr(reflect.ValueOf(x).UnsafePointer())}
	case []any:
		visit = redactionVisit{kind: reflect.Slice, ptr: reflect.ValueOf(x).Pointer(), len: len(x)}
	}
	if visit.ptr != 0 {
		if path[visit] || depth >= maxRedactionDepth {
			return true, redactedValue
		}
		path[visit] = true
		defer delete(path, visit)
	}
	switch x := v.(type) {
	case map[string]any:
		if depth >= maxRedactionDepth {
			return true, redactedValue
		}
		changed := false
		var redacted map[string]any
		for k, child := range x {
			if _, ok := keys[strings.ToLower(k)]; ok {
				if redacted == nil {
					redacted = cloneAnyMap(x)
				}
				redacted[k] = redactedValue
				changed = true
				continue
			}
			if c, next := redactAnyValueDepth(child, keys, depth+1, path); c {
				if redacted == nil {
					redacted = cloneAnyMap(x)
				}
				redacted[k] = next
				changed = true
			}
		}
		if !changed {
			return false, v
		}
		return true, redacted
	case map[string]string:
		changed := false
		var redacted map[string]string
		for k := range x {
			if _, ok := keys[strings.ToLower(k)]; ok {
				if redacted == nil {
					redacted = cloneStringMap(x)
				}
				redacted[k] = redactedValue
				changed = true
			}
		}
		if !changed {
			return false, v
		}
		return true, redacted
	case *url.URL:
		if !urlNeedsRedaction(x) {
			return false, v
		}
		return true, SanitizeURL(x)
	case url.URL:
		if !urlNeedsRedaction(&x) {
			return false, v
		}
		return true, SanitizeURL(&x)
	case []any:
		if depth >= maxRedactionDepth {
			return true, redactedValue
		}
		changed := false
		var redacted []any
		for i, child := range x {
			if c, next := redactAnyValueDepth(child, keys, depth+1, path); c {
				if redacted == nil {
					redacted = cloneAnySlice(x)
				}
				redacted[i] = next
				changed = true
			}
		}
		if !changed {
			return false, v
		}
		return true, redacted
	default:
		return false, v
	}
}

func cloneStringMap(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func cloneAnyMap(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func cloneAnySlice(src []any) []any {
	dst := make([]any, len(src))
	copy(dst, src)
	return dst
}
