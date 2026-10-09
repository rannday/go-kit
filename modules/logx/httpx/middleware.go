package httpx

import (
	"bufio"
	"net"
	"net/http"
	"time"

	"github.com/rannday/go-kit/modules/logx"
)

// HTTPMiddleware returns an http.Handler that instruments requests with timing,
// status-level mapping, panic recovery, and request ID propagation through context.
func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		ctx := r.Context()
		var reqID string
		if id, ok := logx.RequestID(ctx); ok {
			reqID = id
		} else if id := r.Header.Get("X-Request-ID"); id != "" {
			reqID = id
		} else {
			reqID = logx.NewRequestID()
		}
		ctx = logx.WithRequestID(ctx, reqID)
		ctx = logx.WithLogger(ctx, logx.Logger())
		r = r.WithContext(ctx)

		rw := &responseWriter{
			ResponseWriter: w,
			status:         http.StatusOK,
		}
		rw.Header().Set("X-Request-ID", reqID)

		defer func() {
			if rec := recover(); rec != nil {
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				logx.LoggerFromContext(r.Context()).ErrorContext(
					r.Context(),
					"http handler panic",
					"panic", rec,
				)
				if !rw.wroteHeader {
					http.Error(rw, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				}
			}

			fields := []any{
				"method", r.Method,
				"url", logx.SanitizeURL(r.URL),
				"status", rw.status,
				"duration", time.Since(start),
				"remote_addr", r.RemoteAddr,
				"user_agent", r.UserAgent(),
				"bytes", rw.bytes,
			}
			fields = appendRequestIDField(r.Context(), fields)

			logx.LoggerFromContext(r.Context()).Log(
				r.Context(),
				HTTPStatusLevel(rw.status),
				"http request completed",
				fields...,
			)
		}()

		next.ServeHTTP(rw, r)
	})
}

type responseWriter struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if rw.wroteHeader {
		return
	}
	// An upgrade commits the response; other informational headers do not.
	if code >= 100 && code < 200 && code != http.StatusSwitchingProtocols {
		rw.ResponseWriter.WriteHeader(code)
		return
	}
	rw.wroteHeader = true
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.bytes += n
	return n, err
}

func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		if !rw.wroteHeader {
			rw.WriteHeader(http.StatusOK)
		}
		f.Flush()
	}
}

func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

func (rw *responseWriter) Push(target string, opts *http.PushOptions) error {
	if p, ok := rw.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}

func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}
