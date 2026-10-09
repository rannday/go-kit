package httpx

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rannday/go-kit/modules/logx"
)

type mockRoundTripper struct {
	resp    *http.Response
	err     error
	lastReq *http.Request
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	m.lastReq = req
	return m.resp, m.err
}

func captureHTTP(t *testing.T, fn func()) string {
	t.Helper()

	logx.Reset()

	var buf bytes.Buffer

	base := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level:     slog.LevelDebug,
		AddSource: false,
	})
	logx.SetLogger(slog.New(logx.WrapHandler(base, logx.Config{})))

	fn()

	return buf.String()
}

func TestTransport_Success(t *testing.T) {
	out := captureHTTP(t, func() {
		rt := &mockRoundTripper{
			resp: &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader("ok")),
			},
		}

		tr := Transport(rt)

		req := httptest.NewRequest("GET", "https://example.com", nil)
		_, _ = tr.RoundTrip(req)
	})

	if !strings.Contains(out, "status=200") {
		t.Fatalf("expected status log, got: %q", out)
	}
}

func TestTransport_PropagatesRequestIDHeader(t *testing.T) {
	rt := &mockRoundTripper{
		resp: &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader("ok")),
		},
	}

	tr := Transport(rt)

	req := httptest.NewRequest("GET", "https://example.com", nil)
	req = req.WithContext(logx.WithRequestID(req.Context(), "rid-123"))
	_, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := rt.lastReq.Header.Get("X-Request-ID"); got != "rid-123" {
		t.Fatalf("expected request id header to propagate, got %q", got)
	}
}

func TestTransport_Error(t *testing.T) {
	out := captureHTTP(t, func() {
		rt := &mockRoundTripper{
			err: fmt.Errorf("boom"),
		}

		tr := Transport(rt)

		req := httptest.NewRequest("GET", "https://example.com", nil)
		_, _ = tr.RoundTrip(req)
	})

	if !strings.Contains(out, "network_error=true") {
		t.Fatalf("expected network error log")
	}
}

func TestTransportLogger_LogsRequest(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{AddSource: false})
	logger := slog.New(handler)

	rt := &mockRoundTripper{
		resp: &http.Response{
			StatusCode:    200,
			Status:        "200 OK",
			Body:          io.NopCloser(strings.NewReader("ok")),
			ContentLength: 2,
		},
	}

	client := &http.Client{
		Transport: NewTransportLogger(rt, logger),
	}

	req, _ := http.NewRequest("GET", "https://example.com/?apikey=secret", nil)
	_, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "http request completed") {
		t.Fatalf("expected transport to log request, got: %s", out)
	}
}

func TestTransportLogger_RequestBodyRedaction(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{AddSource: false})
	logger := slog.New(handler)

	// redact "password"
	logx.ClearRedactedKeys()
	logx.AddRedactedKeys("password")

	rt := &mockRoundTripper{
		resp: &http.Response{
			StatusCode:    200,
			Status:        "200 OK",
			Body:          io.NopCloser(strings.NewReader(`{"ok":true}`)),
			Header:        http.Header{"Content-Type": []string{"application/json"}},
			ContentLength: 11,
		},
	}

	client := &http.Client{
		Transport: NewTransportLogger(rt, logger).EnableBodyLogging(4096),
	}

	body := `{"user":"admin","password":"secret"}`
	req, _ := http.NewRequest("POST", "https://example.com", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	out := buf.String()
	if !(strings.Contains(out, "password") && strings.Contains(out, "REDACTED")) {
		t.Fatalf("expected password to be redacted in logs, got: %s", out)
	}
	if strings.Contains(out, "secret") {
		t.Fatalf("expected cleartext secret to be removed, got: %s", out)
	}
}

func TestTransportLogger_RestoresBodies(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{AddSource: false})
	logger := slog.New(handler)

	rt := &mockRoundTripper{
		resp: &http.Response{
			StatusCode:    200,
			Status:        "200 OK",
			Body:          io.NopCloser(strings.NewReader("response-body")),
			Header:        http.Header{"Content-Type": []string{"text/plain"}},
			ContentLength: int64(len("response-body")),
		},
	}

	client := &http.Client{
		Transport: NewTransportLogger(rt, logger).EnableBodyLogging(4096),
	}

	reqBody := "request-body"
	req, _ := http.NewRequest("POST", "https://example.com", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "text/plain")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	gotReq, err := io.ReadAll(rt.lastReq.Body)
	if err != nil {
		t.Fatalf("read request body failed: %v", err)
	}
	if string(gotReq) != reqBody {
		t.Fatalf("expected request body to stay readable, got %q", gotReq)
	}

	gotResp, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body failed: %v", err)
	}
	if string(gotResp) != "response-body" {
		t.Fatalf("expected response body to stay readable, got %q", gotResp)
	}
}

func TestRedactJSON_NestedAndCaseInsensitive(t *testing.T) {
	in := []byte(`{"Password":"secret","nested":{"token":"abc"},"items":[{"ApiKey":"k"},{"x":1}]}`)
	out := string(redactJSON(in, map[string]struct{}{
		"password": {},
		"token":    {},
		"apikey":   {},
	}))

	if strings.Contains(out, "secret") || strings.Contains(out, "abc") || strings.Contains(out, `"k"`) {
		t.Fatalf("expected nested secrets to be redacted, got: %s", out)
	}
	if strings.Count(out, "REDACTED") != 3 {
		t.Fatalf("expected exactly 3 redacted fields, got: %s", out)
	}
}

func TestRedactJSON_InvalidJSONFallback(t *testing.T) {
	in := []byte(`{"password":"secret"`)
	out := redactJSON(in, map[string]struct{}{"password": {}})
	if string(out) != "REDACTED" {
		t.Fatalf("expected invalid JSON log placeholder, got %q", out)
	}
	if string(out) == string(in) {
		t.Fatalf("expected invalid JSON not to be logged raw")
	}
}

func TestTransportLogger_SkipsLargeRequestAndResponse(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{AddSource: false})
	logger := slog.New(handler)

	rt := &mockRoundTripper{
		resp: &http.Response{
			StatusCode:    200,
			Status:        "200 OK",
			Body:          io.NopCloser(strings.NewReader(strings.Repeat("x", 1024))),
			ContentLength: 1024,
		},
	}

	client := &http.Client{Transport: NewTransportLogger(rt, logger).EnableBodyLogging(1)}

	req, _ := http.NewRequest("POST", "https://example.com", strings.NewReader(strings.Repeat("a", 1024)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	out := buf.String()
	if !strings.Contains(out, "req_body_skipped") {
		t.Fatalf("expected req_body_skipped in logs, got: %s", out)
	}
	if !strings.Contains(out, "resp_body_skipped") {
		t.Fatalf("expected resp_body_skipped in logs, got: %s", out)
	}
}

type trackCloser struct {
	io.Reader
	closed bool
	closes int
}

func (t *trackCloser) Close() error {
	t.closed = true
	t.closes++
	return nil
}

type failAfterReader struct {
	n   int
	err error
}

func (f *failAfterReader) Read(p []byte) (int, error) {
	if f.n > 0 {
		p[0] = 'x'
		f.n--
		return 1, nil
	}
	if f.err == nil {
		f.err = fmt.Errorf("read failed")
	}
	return 0, f.err
}

func TestCaptureBodyForLog_ClosesOriginalOnSuccess(t *testing.T) {
	orig := &trackCloser{Reader: strings.NewReader("abc")}
	captured := captureBodyForLog(orig, 3, "text/plain", 32)
	if captured.err != nil {
		t.Fatal(captured.err)
	}
	if !orig.closed {
		t.Fatal("expected original body to be closed")
	}
	got, err := io.ReadAll(captured.body)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "abc" {
		t.Fatalf("expected buffered body, got %q", got)
	}
}

func TestCaptureBodyForLog_ReadErrorRestoresBody(t *testing.T) {
	orig := &trackCloser{Reader: &failAfterReader{n: 1}}
	captured := captureBodyForLog(orig, -1, "text/plain", 32)
	if captured.err == nil {
		t.Fatal("expected read error")
	}
	if orig.closed {
		t.Fatal("expected original closer preserved for the caller")
	}
	got, err := io.ReadAll(captured.body)
	if err == nil {
		t.Fatal("expected restored reader to surface the read error")
	}
	if string(got) != "x" {
		t.Fatalf("expected already-read byte restored, got %q", got)
	}
}

func TestCaptureBodyForLog_CapsUnderstatedContentLength(t *testing.T) {
	orig := &trackCloser{Reader: strings.NewReader("abcdef")}
	captured := captureBodyForLog(orig, 1, "text/plain", 3)
	if captured.err != nil {
		t.Fatal(captured.err)
	}
	if !captured.skipped || !captured.rewound {
		t.Fatalf("expected oversized peek to rewind, got %#v", captured)
	}
	if orig.closed {
		t.Fatal("expected original body left open when skipping")
	}
	got, err := io.ReadAll(captured.body)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "abcdef" {
		t.Fatalf("expected full body restored, got %q", got)
	}
}

func TestTransportLogger_BodyReadErrorFailsRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{AddSource: false}))
	rt := &mockRoundTripper{
		resp: &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))},
	}
	tr := NewTransportLogger(rt, logger).EnableBodyLogging(32)
	body := &trackCloser{Reader: &failAfterReader{n: 1}}
	req, err := http.NewRequest("POST", "https://example.com", body)
	if err != nil {
		t.Fatal(err)
	}
	req.ContentLength = -1
	req.Header.Set("Content-Type", "text/plain")

	_, err = tr.RoundTrip(req)
	if err == nil {
		t.Fatal("expected body read error")
	}
	if rt.lastReq != nil {
		t.Fatal("expected transport not to send a truncated body")
	}
	if body.closes != 1 {
		t.Fatalf("expected failed request body closed once, got %d", body.closes)
	}
	if !strings.Contains(buf.String(), "http request failed") {
		t.Fatalf("expected failure log, got: %s", buf.String())
	}
	if strings.Contains(buf.String(), "http request completed") {
		t.Fatalf("expected no completion log, got: %s", buf.String())
	}
}

func TestBodyForLog_MediaTypes(t *testing.T) {
	logx.Reset()
	t.Cleanup(logx.Reset)
	for _, contentType := range []string{
		"application/json", "Application/JSON; charset=utf-8",
		"application/problem+json", "APPLICATION/vnd.example+JSON; charset=utf-8",
		"application/json; charset", "invalid content type",
	} {
		t.Run(contentType, func(t *testing.T) {
			got := bodyForLog([]byte(`{"token":"secret","ok":true}`), contentType, 4096)
			if strings.Contains(got, "secret") || !strings.Contains(got, "REDACTED") {
				t.Fatalf("expected redaction, got %q", got)
			}
		})
	}
	got := bodyForLog([]byte("token=secret&ok=true"), "Application/X-WWW-Form-Urlencoded; charset=utf-8", 4096)
	if strings.Contains(got, "secret") || !strings.Contains(got, "REDACTED") {
		t.Fatalf("expected form redaction, got %q", got)
	}
	for _, contentType := range []string{"", "text/plain", "text/plain; note=\"application/json\""} {
		if got := bodyForLog([]byte("ordinary text"), contentType, 4096); got != "ordinary text" {
			t.Fatalf("expected raw text for %q, got %q", contentType, got)
		}
	}
}

func TestTransportLogger_ResponseBodyReadErrorClosesBody(t *testing.T) {
	body := &trackCloser{Reader: &failAfterReader{n: 1}}
	rt := &mockRoundTripper{resp: &http.Response{StatusCode: 200, Body: body, ContentLength: -1}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tr := NewTransportLogger(rt, logger).EnableBodyLogging(32)
	req := httptest.NewRequest("GET", "https://example.com", nil)
	resp, err := tr.RoundTrip(req)
	if err == nil {
		t.Fatal("expected response body read error")
	}
	if resp != nil {
		t.Fatal("expected failed captured response not to escape")
	}
	if body.closes != 1 {
		t.Fatalf("expected failed response body closed once, got %d", body.closes)
	}
}

func TestTransportLogger_InvalidJSONNotLoggedRaw(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{AddSource: false}))
	rt := &mockRoundTripper{
		resp: &http.Response{
			StatusCode:    200,
			Body:          io.NopCloser(strings.NewReader("ok")),
			ContentLength: 2,
		},
	}
	tr := NewTransportLogger(rt, logger).EnableBodyLogging(4096)
	raw := `{"password":"secret"`
	req, err := http.NewRequest("POST", "https://example.com", strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	sent, err := io.ReadAll(rt.lastReq.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(sent) != raw {
		t.Fatalf("expected on-wire body unchanged, got %q", sent)
	}
	out := buf.String()
	if strings.Contains(out, "secret") {
		t.Fatalf("expected invalid JSON secret omitted from log, got: %s", out)
	}
	if !strings.Contains(out, "REDACTED") {
		t.Fatalf("expected placeholder in log, got: %s", out)
	}
}

func TestTransportLogger_PropagatesRequestIDHeader(t *testing.T) {
	rt := &mockRoundTripper{
		resp: &http.Response{
			StatusCode:    200,
			Status:        "200 OK",
			Body:          io.NopCloser(strings.NewReader("ok")),
			ContentLength: 2,
		},
	}

	client := &http.Client{Transport: NewTransportLogger(rt, nil)}

	req, _ := http.NewRequest("GET", "https://example.com", nil)
	ctx := logx.WithRequestID(req.Context(), "rid-123")
	req = req.WithContext(ctx)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if got := rt.lastReq.Header.Get("X-Request-ID"); got != "rid-123" {
		t.Fatalf("expected request id header to propagate, got %q", got)
	}
}
