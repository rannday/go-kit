package httpx

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rannday/go-kit/modules/logx"
)

const defaultMaxBodyLogBytes = 32 * 1024

// TransportLogger wraps an existing RoundTripper and logs outbound requests.
// Body capture is size-limited and redacted with the package redaction keys.
type TransportLogger struct {
	rt     http.RoundTripper
	logger *slog.Logger

	// LogBody controls whether request/response bodies are captured and logged.
	LogBody bool
	// MaxBodyLogBytes limits how many bytes are read from bodies for logging.
	// Bodies larger than the limit are left readable and logged as skipped.
	// A missing or understated ContentLength does not raise the read cap.
	// If 0, default is 32*1024.
	MaxBodyLogBytes int
}

// NewTransportLogger constructs a TransportLogger. If rt is nil, http.DefaultTransport
// is used. If logger is nil, the transport will use the request context logger or
// the package global logger.
func NewTransportLogger(rt http.RoundTripper, logger *slog.Logger) *TransportLogger {
	if rt == nil {
		rt = http.DefaultTransport
	}
	return &TransportLogger{rt: rt, logger: logger}
}

// EnableBodyLogging enables body capture and sets a maximum capture size.
// Bodies larger than the limit are skipped instead of partially logged.
func (t *TransportLogger) EnableBodyLogging(maxBytes int) *TransportLogger {
	t.LogBody = true
	if maxBytes <= 0 {
		t.MaxBodyLogBytes = defaultMaxBodyLogBytes
	} else {
		t.MaxBodyLogBytes = maxBytes
	}
	return t
}

func bodyLogLimit(max int) int {
	if max <= 0 {
		return defaultMaxBodyLogBytes
	}
	return max
}

func redactJSON(b []byte, keys map[string]struct{}) []byte {
	if len(keys) == 0 || len(b) == 0 {
		return b
	}

	var payload any
	if err := json.Unmarshal(b, &payload); err != nil {
		// Unparsed JSON can still contain secrets. Log a placeholder, not the buffer.
		return []byte("REDACTED")
	}

	redactJSONValue(payload, keys)

	out, err := json.Marshal(payload)
	if err != nil {
		return []byte("REDACTED")
	}
	return out
}

func redactJSONValue(v any, keySet map[string]struct{}) {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			if _, ok := keySet[strings.ToLower(k)]; ok {
				x[k] = "REDACTED"
				continue
			}
			redactJSONValue(child, keySet)
		}
	case []any:
		for _, child := range x {
			redactJSONValue(child, keySet)
		}
	}
}

func redactForm(s string, keys map[string]struct{}) string {
	vals, _ := url.ParseQuery(s)
	logx.RedactQueryValues(vals, keys)
	return vals.Encode()
}

type capturedBody struct {
	body    io.ReadCloser
	raw     []byte
	text    string
	skipped bool
	full    bool
	rewound bool
	err     error
}

type prefixReadCloser struct {
	r      io.Reader
	closer io.Closer
}

func (p prefixReadCloser) Read(b []byte) (int, error) { return p.r.Read(b) }

func (p prefixReadCloser) Close() error {
	if p.closer == nil {
		return nil
	}
	return p.closer.Close()
}

func captureBodyForLog(body io.ReadCloser, contentLength int64, contentType string, max int) capturedBody {
	if body == nil {
		return capturedBody{}
	}

	limit := bodyLogLimit(max)
	if contentLength > int64(limit) {
		return capturedBody{body: body, skipped: true}
	}

	buf, err := io.ReadAll(io.LimitReader(body, int64(limit)+1))
	if err != nil {
		return capturedBody{
			body: prefixReadCloser{r: io.MultiReader(bytes.NewReader(buf), body), closer: body},
			err:  err,
		}
	}
	if len(buf) > limit {
		return capturedBody{
			body:    prefixReadCloser{r: io.MultiReader(bytes.NewReader(buf), body), closer: body},
			skipped: true,
			rewound: true,
		}
	}

	closeErr := body.Close()
	return capturedBody{
		body: io.NopCloser(bytes.NewReader(buf)),
		raw:  buf,
		text: bodyForLog(buf, contentType, limit),
		full: true,
		err:  closeErr,
	}
}

func bodyForLog(bodyBytes []byte, contentType string, limit int) string {
	keys := logx.RedactedKeySet()
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil && contentType != "" && len(keys) > 0 {
		// Malformed metadata must not turn a structured secret into raw text.
		return "REDACTED"
	}
	if mediaType == "application/json" || strings.HasPrefix(mediaType, "application/") && strings.HasSuffix(mediaType, "+json") {
		return string(redactJSON(bodyBytes, keys))
	}
	if mediaType == "application/x-www-form-urlencoded" {
		return redactForm(string(bodyBytes), keys)
	}
	if len(bodyBytes) > limit {
		return string(bodyBytes[:limit])
	}
	return string(bodyBytes)
}

func (t *TransportLogger) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return t.rt.RoundTrip(req)
	}

	l := roundTripLogger(req, t.logger)
	req = propagateRequestID(req)
	start := time.Now()

	fields := []any{
		"method", req.Method,
		"url", logx.SanitizeURL(req.URL),
	}

	if req.URL != nil {
		fields = append(fields, "host", req.URL.Host)
	}

	if t.LogBody {
		captured := captureBodyForLog(req.Body, req.ContentLength, req.Header.Get("Content-Type"), t.MaxBodyLogBytes)
		if captured.body != nil {
			req.Body = captured.body
		}
		if captured.err != nil {
			if captured.body != nil {
				_ = captured.body.Close()
			}
			fields = append(fields, "duration", time.Since(start))
			fields = appendRequestIDField(req.Context(), fields)
			logHTTPFailure(l, req.Context(), fields, captured.err, false)
			return nil, captured.err
		}
		applyCapturedRequestBody(req, captured)
		if captured.skipped {
			fields = append(fields, "req_body_skipped", true)
		} else if captured.text != "" {
			fields = append(fields, "req_body", captured.text)
		}
	}

	resp, err := t.rt.RoundTrip(req)
	fields = append(fields, "duration", time.Since(start))
	fields = appendRequestIDField(req.Context(), fields)

	if err != nil {
		logHTTPFailure(l, req.Context(), fields, err, true)
		return resp, err
	}

	if t.LogBody && resp != nil {
		captured := captureBodyForLog(resp.Body, resp.ContentLength, resp.Header.Get("Content-Type"), t.MaxBodyLogBytes)
		if captured.body != nil {
			resp.Body = captured.body
		}
		if captured.err != nil {
			if captured.body != nil {
				_ = captured.body.Close()
			}
			logHTTPFailure(l, req.Context(), fields, captured.err, false)
			return nil, captured.err
		}
		if captured.skipped {
			fields = append(fields, "resp_body_skipped", true)
		} else if captured.text != "" {
			fields = append(fields, "resp_body", captured.text)
		}
	}

	logHTTPCompletion(l, req.Context(), resp.StatusCode, fields)
	return resp, nil
}

func applyCapturedRequestBody(req *http.Request, captured capturedBody) {
	if req == nil {
		return
	}
	if captured.full {
		req.ContentLength = int64(len(captured.raw))
		raw := captured.raw
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(raw)), nil
		}
		return
	}
	if captured.rewound && req.ContentLength >= 0 {
		req.ContentLength = -1
		req.GetBody = nil
	}
}
