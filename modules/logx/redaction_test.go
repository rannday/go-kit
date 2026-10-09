package logx

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"testing"
)

func TestRedactAnyValueBoundsCycles(t *testing.T) {
	m := map[string]any{"token": "secret"}
	m["self"] = m
	a, b := map[string]any{}, map[string]any{}
	a["next"], b["next"] = b, a
	s := make([]any, 1)
	s[0] = s
	for name, value := range map[string]any{"map": m, "mutual maps": a, "slice": s} {
		t.Run(name, func(t *testing.T) {
			changed, result := redactAnyValue(value, map[string]struct{}{"token": {}})
			if !changed {
				t.Fatal("expected bounded container to be replaced")
			}
			data, err := json.Marshal(result)
			if err != nil {
				t.Fatalf("redacted result is still cyclic: %v", err)
			}
			if strings.Contains(string(data), "secret") || !strings.Contains(string(data), redactedValue) {
				t.Fatalf("unsafe result: %s", data)
			}
		})
	}
	if m["token"] != "secret" || m["self"].(map[string]any)["token"] != "secret" {
		t.Fatal("input map was mutated")
	}
	if _, ok := s[0].([]any); !ok {
		t.Fatal("input slice was mutated")
	}
}

func TestRedactAnyValueSharedChild(t *testing.T) {
	child := map[string]any{"token": "secret", "safe": "value"}
	input := []any{child, child}
	_, result := redactAnyValue(input, map[string]struct{}{"token": {}})
	for _, item := range result.([]any) {
		got := item.(map[string]any)
		if got["token"] != redactedValue || got["safe"] != "value" {
			t.Fatalf("unexpected shared child: %v", got)
		}
	}
	if child["token"] != "secret" {
		t.Fatal("input child was mutated")
	}
}

func TestRedactAnyValueBoundsDepth(t *testing.T) {
	var input any = map[string]any{"token": "secret"}
	for i := 0; i < maxRedactionDepth+1; i++ {
		input = []any{input}
	}
	changed, result := redactAnyValue(input, map[string]struct{}{"token": {}})
	if !changed {
		t.Fatal("expected depth limit to mask container")
	}
	for i := 0; i < maxRedactionDepth; i++ {
		result = result.([]any)[0]
	}
	if result != redactedValue {
		t.Fatalf("depth limit not masked: %v", result)
	}
}

func TestConcurrentSetRedactedKeys(t *testing.T) {
	ClearRedactedKeys()

	var wg sync.WaitGroup
	add := func(prefix string) {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			SetRedactedKeys(fmt.Sprintf("%s%d", prefix, i))
		}
	}

	wg.Add(2)
	go add("a")
	go add("b")
	wg.Wait()

	// ensure there is at least one key present
	if len(ListRedactedKeys()) == 0 {
		t.Fatalf("expected redacted keys to be present")
	}
}

func TestRedactionHandler_RedactsKeys(t *testing.T) {
	out := capture(t, slog.LevelInfo, func() {
		SetRedactedKeys("password")
		Info("login", "password", "secret", "user", "admin")
	})

	if !strings.Contains(out, "password=REDACTED") {
		t.Fatalf("expected password to be redacted, got: %s", out)
	}
}

func TestRedactionHandler_DefaultRedactsPassword(t *testing.T) {
	out := capture(t, slog.LevelInfo, func() {
		Info("login", "password", "secret")
	})

	if !strings.Contains(out, "password=REDACTED") {
		t.Fatalf("expected default password redaction, got: %s", out)
	}
	if strings.Contains(out, "secret") {
		t.Fatalf("expected secret value removed, got: %s", out)
	}
}

func TestClearRedactedKeys_DisablesStructuredRedaction(t *testing.T) {
	out := capture(t, slog.LevelInfo, func() {
		ClearRedactedKeys()
		Info("login", "password", "secret")
	})

	if !strings.Contains(out, "password=secret") {
		t.Fatalf("expected password to remain unredacted, got: %s", out)
	}
	if strings.Contains(out, "password=REDACTED") {
		t.Fatalf("expected structured redaction disabled, got: %s", out)
	}
}

func TestSetRedactedKeys_ReplacesDefaults(t *testing.T) {
	out := capture(t, slog.LevelInfo, func() {
		SetRedactedKeys("secret")
		Info("fields", "password", "pw", "secret", "value")
	})

	if !strings.Contains(out, "password=pw") {
		t.Fatalf("expected password default to be replaced, got: %s", out)
	}
	if !strings.Contains(out, "secret=REDACTED") {
		t.Fatalf("expected configured key to be redacted, got: %s", out)
	}
}

func TestRedactionHandler_RedactsMixedCaseKeys(t *testing.T) {
	out := capture(t, slog.LevelInfo, func() {
		SetRedactedKeys("password")
		Info("login", "Password", "secret", "user", "admin")
	})

	if !strings.Contains(out, "Password=REDACTED") {
		t.Fatalf("expected mixed-case password to be redacted, got: %s", out)
	}
}

func TestRedactionHandler_RedactsNestedGroup(t *testing.T) {
	out := capture(t, slog.LevelInfo, func() {
		SetRedactedKeys("token")
		Info("login", "user", "admin", slog.Group("auth", slog.Group("nested",
			slog.String("token", "secret"),
			slog.String("ok", "yes"),
		)))
	})

	if !strings.Contains(out, "token=REDACTED") {
		t.Fatalf("expected nested token to be redacted, got: %s", out)
	}
	if strings.Contains(out, "secret") {
		t.Fatalf("expected nested secret to be removed, got: %s", out)
	}
}

func TestSanitizeURL_RedactsQueryParams(t *testing.T) {
	u, _ := url.Parse("https://fw/api?apikey=abc123&name=test")

	s := SanitizeURL(u)

	if strings.Contains(s, "abc123") {
		t.Fatalf("expected apikey to be redacted")
	}

	if !strings.Contains(s, "apikey=REDACTED") {
		t.Fatalf("expected apikey=REDACTED, got: %s", s)
	}
}

func TestSanitizeURL_RedactsConfiguredKeys(t *testing.T) {
	ClearRedactedKeys()
	defer Reset()
	SetRedactedKeys("secret")

	u, _ := url.Parse("https://fw/api?secret=abc&name=test")
	s := SanitizeURL(u)

	if strings.Contains(s, "abc") {
		t.Fatalf("expected configured secret param to be redacted, got: %s", s)
	}
	if !strings.Contains(s, "secret=REDACTED") {
		t.Fatalf("expected secret=REDACTED, got: %s", s)
	}
}

func TestSanitizeURL_RedactsDefaultKeysAfterClear(t *testing.T) {
	ClearRedactedKeys()
	defer Reset()

	u, _ := url.Parse("https://fw/api?password=secret&name=test")
	s := SanitizeURL(u)

	if strings.Contains(s, "secret") {
		t.Fatalf("expected default password param to be redacted, got: %s", s)
	}
	if !strings.Contains(s, "password=REDACTED") {
		t.Fatalf("expected password=REDACTED, got: %s", s)
	}
}

func TestSanitizeURL_RedactsUserCredentials(t *testing.T) {
	u, _ := url.Parse("https://admin:pass@fw/api?name=test")
	s := SanitizeURL(u)

	if strings.Contains(s, "admin") || strings.Contains(s, "pass") {
		t.Fatalf("expected user credentials to be redacted, got: %s", s)
	}
	if !strings.Contains(s, "REDACTED:REDACTED@") {
		t.Fatalf("expected redacted userinfo, got: %s", s)
	}
}

func TestRedactionHandler_RedactsAnyMap(t *testing.T) {
	out := capture(t, slog.LevelInfo, func() {
		SetRedactedKeys("token")
		Info("payload", "data", map[string]any{"token": "secret", "ok": true})
	})

	if !strings.Contains(out, "token:REDACTED") {
		t.Fatalf("expected map token to be redacted, got: %s", out)
	}
	if strings.Contains(out, "secret") {
		t.Fatalf("expected secret value removed, got: %s", out)
	}
}

func TestRedactedKeySet_ReturnsClone(t *testing.T) {
	SetRedactedKeys("password")
	defer Reset()

	keys := RedactedKeySet()
	delete(keys, "password")
	keys["token"] = struct{}{}

	fresh := RedactedKeySet()
	if _, ok := fresh["password"]; !ok {
		t.Fatalf("expected internal redaction set to keep password")
	}
	if _, ok := fresh["token"]; ok {
		t.Fatalf("expected mutation of returned map not to affect internal state")
	}
}

func TestRedactAnyValue_DoesNotMutateMap(t *testing.T) {
	SetRedactedKeys("password")
	defer Reset()

	original := map[string]any{"password": "secret"}
	changed, redacted := redactAnyValue(original, redactedKeySetSnapshot())

	if !changed {
		t.Fatalf("expected redaction to change returned value")
	}
	if original["password"] != "secret" {
		t.Fatalf("expected original map to remain unchanged, got: %#v", original)
	}
	got := redacted.(map[string]any)
	if got["password"] != redactedValue {
		t.Fatalf("expected redacted copy, got: %#v", got)
	}
}

func TestRedactAnyValue_DoesNotMutateNestedMap(t *testing.T) {
	SetRedactedKeys("token")
	defer Reset()

	nested := map[string]any{"token": "secret"}
	original := map[string]any{"outer": nested}
	changed, redacted := redactAnyValue(original, redactedKeySetSnapshot())

	if !changed {
		t.Fatalf("expected nested redaction to change returned value")
	}
	if nested["token"] != "secret" {
		t.Fatalf("expected nested map to remain unchanged, got: %#v", nested)
	}
	got := redacted.(map[string]any)
	gotNested := got["outer"].(map[string]any)
	if gotNested["token"] != redactedValue {
		t.Fatalf("expected redacted nested copy, got: %#v", gotNested)
	}
}

func TestRedactionHandler_RedactsWithAttrs(t *testing.T) {
	out := capture(t, slog.LevelInfo, func() {
		With("password", "from-with").Info("via with")
	})

	if strings.Contains(out, "from-with") {
		t.Fatalf("expected With attr to be redacted, got: %s", out)
	}
	if !strings.Contains(out, "password=REDACTED") {
		t.Fatalf("expected password=REDACTED, got: %s", out)
	}
}

type secretUser struct{}

func (secretUser) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("name", "ada"),
		slog.String("password", "hunter2"),
	)
}

func TestRedactionHandler_RedactsLogValuer(t *testing.T) {
	out := capture(t, slog.LevelInfo, func() {
		Info("login", "user", secretUser{})
	})

	if strings.Contains(out, "hunter2") {
		t.Fatalf("expected LogValuer secret removed, got: %s", out)
	}
	if !strings.Contains(out, "password=REDACTED") {
		t.Fatalf("expected nested password redacted, got: %s", out)
	}
	if !strings.Contains(out, "name=ada") {
		t.Fatalf("expected non-secret field kept, got: %s", out)
	}
}

func TestRedactionHandler_RedactsStringMap(t *testing.T) {
	out := capture(t, slog.LevelInfo, func() {
		Info("payload", "data", map[string]string{"password": "from-map", "ok": "yes"})
	})

	if strings.Contains(out, "from-map") {
		t.Fatalf("expected map secret removed, got: %s", out)
	}
	if !strings.Contains(out, "password:REDACTED") && !strings.Contains(out, "password=REDACTED") {
		t.Fatalf("expected map password redacted, got: %s", out)
	}
}

func TestRedactionHandler_RedactsURLValue(t *testing.T) {
	u, err := url.Parse("https://user:pass@example.com/api?token=abc")
	if err != nil {
		t.Fatal(err)
	}
	out := capture(t, slog.LevelInfo, func() {
		Info("call", "url", u)
	})

	if strings.Contains(out, "user:pass") || strings.Contains(out, "abc") {
		t.Fatalf("expected URL secrets removed, got: %s", out)
	}
	if !strings.Contains(out, "REDACTED") {
		t.Fatalf("expected redacted URL, got: %s", out)
	}
}

func TestSanitizeURL_RedactsOAuthQueryNames(t *testing.T) {
	ClearRedactedKeys()
	defer Reset()

	u, err := url.Parse("https://example.com/cb?access_token=a&refresh_token=b&id_token=c&client_secret=d&name=ok")
	if err != nil {
		t.Fatal(err)
	}
	s := SanitizeURL(u)
	for _, secret := range []string{"=a", "=b", "=c", "=d"} {
		if strings.Contains(s, secret) {
			t.Fatalf("expected OAuth query secret removed, got: %s", s)
		}
	}
	if !strings.Contains(s, "access_token=REDACTED") || !strings.Contains(s, "client_secret=REDACTED") {
		t.Fatalf("expected OAuth query names redacted, got: %s", s)
	}
	if !strings.Contains(s, "name=ok") {
		t.Fatalf("expected non-secret query kept, got: %s", s)
	}
}

func TestSanitizeURL_RedactsSuffixedToken(t *testing.T) {
	u, err := url.Parse("https://example.com/cb?session_token=abc&sort_key=id")
	if err != nil {
		t.Fatal(err)
	}
	s := SanitizeURL(u)
	if strings.Contains(s, "abc") {
		t.Fatalf("expected suffixed token redacted, got: %s", s)
	}
	if !strings.Contains(s, "session_token=REDACTED") {
		t.Fatalf("expected session_token=REDACTED, got: %s", s)
	}
	if !strings.Contains(s, "sort_key=id") {
		t.Fatalf("expected short-key suffix to stay exact, got: %s", s)
	}
}

func TestRedactionHandler_StructuredSuffixStaysExact(t *testing.T) {
	out := capture(t, slog.LevelInfo, func() {
		Info("fields", "session_token", "abc")
	})
	if !strings.Contains(out, "session_token=abc") {
		t.Fatalf("expected structured suffix key to stay exact, got: %s", out)
	}
}

func TestRedactAnyValue_DoesNotMutateSlice(t *testing.T) {
	SetRedactedKeys("token")
	defer Reset()

	child := map[string]any{"token": "secret"}
	original := []any{child}
	changed, redacted := redactAnyValue(original, redactedKeySetSnapshot())

	if !changed {
		t.Fatalf("expected slice redaction to change returned value")
	}
	if child["token"] != "secret" {
		t.Fatalf("expected original map in slice to remain unchanged, got: %#v", child)
	}
	if original[0].(map[string]any)["token"] != "secret" {
		t.Fatalf("expected original slice to remain unchanged, got: %#v", original)
	}
	got := redacted.([]any)
	gotChild := got[0].(map[string]any)
	if gotChild["token"] != redactedValue {
		t.Fatalf("expected redacted slice copy, got: %#v", got)
	}
}
