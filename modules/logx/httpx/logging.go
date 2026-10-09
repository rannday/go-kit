package httpx

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/rannday/go-kit/modules/logx"
)

// HTTPStatusLevel maps an HTTP status code to a slog level.
func HTTPStatusLevel(statusCode int) slog.Level {
	switch {
	case statusCode >= 500:
		return slog.LevelError
	case statusCode >= 400:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}

func propagateRequestID(req *http.Request) *http.Request {
	if req == nil {
		return nil
	}
	trailer := req.Trailer
	req = req.Clone(req.Context())
	// Trailer values can be populated by the shared Body reader at EOF.
	req.Trailer = trailer
	if id, ok := logx.RequestID(req.Context()); ok && req.Header.Get("X-Request-ID") == "" {
		if req.Header == nil {
			req.Header = make(http.Header)
		}
		req.Header.Set("X-Request-ID", id)
	}
	return req
}

func roundTripLogger(req *http.Request, explicit *slog.Logger) *slog.Logger {
	if explicit != nil {
		return explicit
	}
	return logx.LoggerFromContext(req.Context())
}

// appendRequestIDField puts request_id on completion records from context.
// Binding it with Logger.With as well logs the id twice, and a replaced
// context logger would drop it from the completion line.
func appendRequestIDField(ctx context.Context, fields []any) []any {
	if id, ok := logx.RequestID(ctx); ok {
		return append(fields, "request_id", id)
	}
	return fields
}

func logHTTPFailure(l *slog.Logger, ctx context.Context, fields []any, err error, networkError bool) {
	fields = append(fields, "error", err)
	if networkError {
		fields = append(fields, "network_error", true)
	}
	l.ErrorContext(ctx, "http request failed", fields...)
}

func logHTTPCompletion(l *slog.Logger, ctx context.Context, statusCode int, fields []any) {
	fields = append(fields, "status", statusCode)
	l.Log(ctx, HTTPStatusLevel(statusCode), "http request completed", fields...)
}
