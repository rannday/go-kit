package httpx

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	logx "github.com/rannday/go-kit/modules/logx"
)

type trailerAtEOFBody struct {
	io.Reader
	trailer http.Header
}

func (b trailerAtEOFBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	if err == io.EOF {
		b.trailer.Set("X-Checksum", "complete")
	}
	return n, err
}

func (b trailerAtEOFBody) Close() error { return nil }

func TestTransportPreservesDynamicTrailers(t *testing.T) {
	for _, capture := range []bool{false, true} {
		t.Run(fmt.Sprint(capture), func(t *testing.T) {
			gotTrailer := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				gotTrailer <- r.Trailer.Get("X-Checksum")
			}))
			defer server.Close()
			trailer := http.Header{"X-Checksum": nil}
			req, err := http.NewRequest(http.MethodPost, server.URL, trailerAtEOFBody{strings.NewReader("body"), trailer})
			if err != nil {
				t.Fatal(err)
			}
			req.Trailer = trailer
			req.TransferEncoding = []string{"chunked"}
			tr := NewTransportLogger(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if capture {
				tr.EnableBodyLogging(1024)
			}
			client := &http.Client{Transport: tr}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if got := <-gotTrailer; got != "complete" {
				t.Fatalf("dynamic trailer lost: %q", got)
			}
		})
	}
}

func TestPropagateRequestIDClonesRequest(t *testing.T) {
	for _, nilHeader := range []bool{false, true} {
		req, err := http.NewRequest(http.MethodGet, "https://example.test", nil)
		if err != nil {
			t.Fatal(err)
		}
		if nilHeader {
			req.Header = nil
		}
		for _, id := range []string{"first", "second"} {
			input := req.WithContext(logx.WithRequestID(context.Background(), id))
			got := propagateRequestID(input)
			if got == input || got.Header.Get("X-Request-ID") != id {
				t.Fatalf("request not cloned with current ID: %v", got.Header)
			}
			if req.Header.Get("X-Request-ID") != "" {
				t.Fatal("original header mutated")
			}
		}
	}
}

func TestPropagateRequestIDPreservesExplicitHeader(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://example.test", nil)
	req.Header.Set("X-Request-ID", "explicit")
	req = req.WithContext(logx.WithRequestID(req.Context(), "context"))
	got := propagateRequestID(req)
	if got.Header.Get("X-Request-ID") != "explicit" {
		t.Fatal("explicit ID overwritten")
	}
	got.Header.Set("Other", "value")
	if req.Header.Get("Other") != "" {
		t.Fatal("headers share mutable state")
	}
	input := req.WithContext(context.Background())
	if got := propagateRequestID(input); got == input {
		t.Fatal("request without contextual ID must also be cloned")
	}
}

func TestTransportDoesNotMutateRequestMetadata(t *testing.T) {
	rt := &mockRoundTripper{resp: &http.Response{StatusCode: http.StatusOK}}
	tr := NewTransportLogger(rt, slog.New(slog.NewTextHandler(io.Discard, nil)))
	tr.EnableBodyLogging(1024)
	req, err := http.NewRequest(http.MethodPost, "https://example.test", strings.NewReader("abc"))
	if err != nil {
		t.Fatal(err)
	}
	req.ContentLength = -1
	req.GetBody = nil
	originalBody := req.Body
	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if req.ContentLength != -1 || req.GetBody != nil || req.Body != originalBody {
		t.Fatal("capture changed caller request metadata")
	}
	if rt.lastReq == req || rt.lastReq.ContentLength != 3 || rt.lastReq.GetBody == nil {
		t.Fatal("captured metadata not applied to forwarded clone")
	}
}

func TestHTTPStatusLevel(t *testing.T) {
	tests := []struct {
		code int
		want slog.Level
	}{
		{200, slog.LevelInfo},
		{404, slog.LevelWarn},
		{500, slog.LevelError},
	}
	for _, tc := range tests {
		if got := HTTPStatusLevel(tc.code); got != tc.want {
			t.Fatalf("HTTPStatusLevel(%d) = %v, want %v", tc.code, got, tc.want)
		}
	}
}
