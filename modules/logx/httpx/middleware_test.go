package httpx

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rannday/go-kit/modules/logx"
)

func captureMiddleware(t *testing.T, fn func()) string {
	t.Helper()

	logx.Reset()
	defer logx.Reset()

	var buf bytes.Buffer
	base := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug, AddSource: false})
	logx.SetLogger(slog.New(logx.WrapHandler(base, logx.Config{})))

	fn()

	return buf.String()
}

func TestMiddleware_LogsStatusAndBytes(t *testing.T) {
	out := captureMiddleware(t, func() {
		handler := HTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(404)
			_, _ = w.Write([]byte("hey"))
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
	})

	if !strings.Contains(out, "status=404") {
		t.Fatalf("expected status 404 log, got: %s", out)
	}
	if !strings.Contains(out, "bytes=3") {
		t.Fatalf("expected byte count in log, got: %s", out)
	}
}

func TestMiddleware_PropagatesRequestIDFromHeader(t *testing.T) {
	var got string
	handler := HTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := logx.RequestID(r.Context())
		if !ok {
			t.Fatalf("expected request id in context")
		}
		got = id
	}))

	req := httptest.NewRequest("GET", "/header", nil)
	req.Header.Set("X-Request-ID", "rid-header")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if got != "rid-header" {
		t.Fatalf("expected request id from header, got %q", got)
	}
	if rec.Header().Get("X-Request-ID") != "rid-header" {
		t.Fatalf("expected response header to match request id")
	}
}

func TestMiddleware_PropagatesRequestIDFromContext(t *testing.T) {
	var got string
	handler := HTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := logx.RequestID(r.Context())
		if !ok {
			t.Fatalf("expected request id in context")
		}
		got = id
	}))

	req := httptest.NewRequest("GET", "/ctx", nil)
	req = req.WithContext(logx.WithRequestID(req.Context(), "rid-ctx"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if got != "rid-ctx" {
		t.Fatalf("expected request id from context, got %q", got)
	}
	if rec.Header().Get("X-Request-ID") != "rid-ctx" {
		t.Fatalf("expected response header to match request id")
	}
}

func TestMiddleware_KeepsFirstStatus(t *testing.T) {
	var code int
	out := captureMiddleware(t, func() {
		handler := HTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		req := httptest.NewRequest("GET", "/status", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		code = rec.Code
	})

	if code != http.StatusCreated {
		t.Fatalf("expected client status 201, got %d", code)
	}
	if !strings.Contains(out, "status=201") {
		t.Fatalf("expected logged status 201, got: %s", out)
	}
	if strings.Contains(out, "status=500") {
		t.Fatalf("expected later status ignored, got: %s", out)
	}
}

func TestMiddleware_PanicAfterWriteKeepsStatus(t *testing.T) {
	var code int
	var body string
	out := captureMiddleware(t, func() {
		handler := HTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte("ok"))
			panic("boom")
		}))
		req := httptest.NewRequest("GET", "/panic-after", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		code = rec.Code
		body = rec.Body.String()
	})

	if code != http.StatusCreated {
		t.Fatalf("expected client status 201, got %d", code)
	}
	if strings.Contains(body, "Internal Server Error") {
		t.Fatalf("expected no error body appended, got %q", body)
	}
	if !strings.Contains(out, "status=201") {
		t.Fatalf("expected logged status 201, got: %s", out)
	}
	if !strings.Contains(out, "http handler panic") {
		t.Fatalf("expected panic log, got: %s", out)
	}
}

func TestMiddleware_LogsRequestIDFromContextOnce(t *testing.T) {
	out := captureMiddleware(t, func() {
		handler := HTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		req := httptest.NewRequest("GET", "/rid", nil)
		req.Header.Set("X-Request-ID", "rid-once")
		handler.ServeHTTP(httptest.NewRecorder(), req)
	})

	if strings.Count(out, "request_id=") != 1 {
		t.Fatalf("expected request_id once, got: %s", out)
	}
	if !strings.Contains(out, "request_id=rid-once") {
		t.Fatalf("expected request id from context, got: %s", out)
	}
}

func TestMiddleware_RecoversPanic(t *testing.T) {
	out := captureMiddleware(t, func() {
		handler := HTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("boom")
		}))

		req := httptest.NewRequest("GET", "/panic", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
	})

	if !strings.Contains(out, "http handler panic") {
		t.Fatalf("expected panic log, got: %s", out)
	}
	if !strings.Contains(out, "status=500") {
		t.Fatalf("expected panic completion log, got: %s", out)
	}
	if strings.Contains(out, "stack=") {
		t.Fatalf("expected middleware not to add an explicit stack field, got: %s", out)
	}
}

func TestMiddleware_PropagatesAbortHandler(t *testing.T) {
	var recovered any
	rec := httptest.NewRecorder()
	out := captureMiddleware(t, func() {
		func() {
			defer func() { recovered = recover() }()
			handler := HTTPMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				panic(http.ErrAbortHandler)
			}))
			handler.ServeHTTP(rec, httptest.NewRequest("GET", "/abort", nil))
		}()
	})
	if recovered != http.ErrAbortHandler {
		t.Fatalf("expected abort sentinel, got %v", recovered)
	}
	if rec.Body.Len() != 0 || out != "" {
		t.Fatalf("abort wrote a response or log: body=%q log=%q", rec.Body.String(), out)
	}
}

func TestMiddleware_InformationalHeadersAllowFinalStatus(t *testing.T) {
	var status int
	out := captureMiddleware(t, func() {
		server := httptest.NewServer(HTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusEarlyHints)
			w.WriteHeader(http.StatusProcessing)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte("created"))
		})))
		defer server.Close()
		resp, err := server.Client().Get(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			t.Fatal(err)
		}
		status = resp.StatusCode
	})
	if status != http.StatusCreated || !strings.Contains(out, "status=201") {
		t.Fatalf("expected final client/log status 201, got %d and %q", status, out)
	}
}

func TestResponseWriter_SwitchingProtocolsCommitsStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, status: http.StatusOK}
	rw.WriteHeader(http.StatusSwitchingProtocols)
	rw.WriteHeader(http.StatusInternalServerError)
	if !rw.wroteHeader || rw.status != http.StatusSwitchingProtocols || rec.Code != http.StatusSwitchingProtocols {
		t.Fatalf("expected upgrade status retained, got %+v and client status %d", rw, rec.Code)
	}
}

func TestMiddleware_FlushCommitsImplicitStatus(t *testing.T) {
	for _, panicAfterFlush := range []bool{false, true} {
		name := "later-header"
		if panicAfterFlush {
			name = "later-panic"
		}
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			out := captureMiddleware(t, func() {
				handler := HTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.(http.Flusher).Flush()
					if panicAfterFlush {
						panic("boom")
					}
					w.WriteHeader(http.StatusInternalServerError)
				}))
				handler.ServeHTTP(rec, httptest.NewRequest("GET", "/flush", nil))
			})
			if rec.Code != http.StatusOK || !strings.Contains(out, "status=200") || strings.Contains(out, "status=500") {
				t.Fatalf("expected flushed client/log status 200, got %d and %q", rec.Code, out)
			}
			if rec.Body.Len() != 0 {
				t.Fatalf("expected no appended error body, got %q", rec.Body.String())
			}
		})
	}
}
