package main

import (
	"bufio"
	"compress/gzip"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// buildTestRouter builds a router through the same installMiddleware(r) that
// main() calls — not a hand-copied mirror of the chain — so these tests
// actually fail if the Compress line is removed or moved out of main.go.
func buildTestRouter() *chi.Mux {
	r := chi.NewRouter()
	installMiddleware(r)
	return r
}

// TestCompressMiddlewareChain is AK4: it fails if the Compress inkoppling is
// removed or moved past a handler that needs an unwrapped ResponseWriter.
func TestCompressMiddlewareChain(t *testing.T) {
	r := buildTestRouter()

	body := strings.Repeat(`{"q":1,"r":2,"terrain":"plains"},`, 2000) // map-shaped, compressible JSON
	r.Get("/api/v1/worlds/x/map", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	})

	t.Run("AK1: compresses JSON when Accept-Encoding: gzip is sent", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/worlds/x/map", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
			t.Fatalf("Content-Encoding = %q, want gzip", got)
		}
		gz, err := gzip.NewReader(rec.Body)
		if err != nil {
			t.Fatalf("response body is not valid gzip: %v", err)
		}
		defer gz.Close()
		decoded, err := io.ReadAll(gz)
		if err != nil {
			t.Fatalf("gzip decode: %v", err)
		}
		if string(decoded) != body {
			t.Fatalf("decompressed body does not match original (got %d bytes, want %d)", len(decoded), len(body))
		}
		if rec.Body.Len() >= len(body) {
			t.Fatalf("compressed body (%d bytes) is not smaller than plain (%d bytes)", rec.Body.Len(), len(body))
		}
	})

	t.Run("AK1: identical unchanged body without Accept-Encoding", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/worlds/x/map", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if got := rec.Header().Get("Content-Encoding"); got != "" {
			t.Fatalf("Content-Encoding = %q, want empty (no compression requested)", got)
		}
		if rec.Body.String() != body {
			t.Fatalf("plain body altered: got %d bytes, want %d", rec.Body.Len(), len(body))
		}
	})
}

// hijackRecorder is a minimal http.ResponseWriter + http.Hijacker, standing in
// for the real *http.response the way gorilla/websocket's Upgrade sees it —
// httptest.ResponseRecorder does not implement http.Hijacker, so it can't
// exercise this path itself.
type hijackRecorder struct {
	http.ResponseWriter
	hijacked bool
}

func (h *hijackRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h.hijacked = true
	server, _ := net.Pipe()
	return server, bufio.NewReadWriter(bufio.NewReader(server), bufio.NewWriter(server)), nil
}

// TestCompressDoesNotBreakHijack is AK2: the fear is that wrapping every
// ResponseWriter in the Compress middleware breaks the /ws/{worldID} upgrade,
// which needs w.(http.Hijacker) to succeed (gorilla/websocket calls this
// assertion directly in server.go's Upgrade — see api/handlers/ws.go).
// chi's compressResponseWriter implements Hijack() by delegating to its
// current writer, and stays on the plain ResponseWriter until something
// compressible is actually written (WriteHeader not yet called => Hijack
// happens before any compression decision) — so the assertion must still
// succeed through the middleware.
func TestCompressDoesNotBreakHijack(t *testing.T) {
	r := buildTestRouter()

	rec := &hijackRecorder{ResponseWriter: httptest.NewRecorder()}
	var hijackErr error
	r.Get("/ws/{worldID}", func(w http.ResponseWriter, req *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			hijackErr = errNotHijacker
			return
		}
		_, _, hijackErr = hj.Hijack()
	})

	req := httptest.NewRequest(http.MethodGet, "/ws/abc", nil)
	r.ServeHTTP(rec, req)

	if hijackErr != nil {
		t.Fatalf("Hijack failed through the Compress middleware: %v", hijackErr)
	}
	if !rec.hijacked {
		t.Fatal("underlying ResponseWriter.Hijack was never reached")
	}
}

var errNotHijacker = &hijackAssertionError{}

type hijackAssertionError struct{}

func (*hijackAssertionError) Error() string {
	return "http.ResponseWriter wrapped by Compress does not implement http.Hijacker"
}

// TestCompressStaticFiles is AK3: /static/* must keep serving correct bodies
// and its handler-set Cache-Control header (main.go:214-218 sets it before
// delegating to http.FileServer) once wrapped by Compress.
func TestCompressStaticFiles(t *testing.T) {
	r := buildTestRouter()

	const cssBody = "body{color:red}\n"
	r.Handle("/static/*", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = io.WriteString(w, cssBody)
	}))

	req := httptest.NewRequest(http.MethodGet, "/static/megaron.css", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("Cache-Control = %q, want no-cache (handler-set header must survive wrapping)", got)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip (text/css is in chi's default compressible list)", got)
	}
	gz, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("static response is not valid gzip: %v", err)
	}
	defer gz.Close()
	decoded, err := io.ReadAll(gz)
	if err != nil {
		t.Fatalf("gzip decode: %v", err)
	}
	if string(decoded) != cssBody {
		t.Fatalf("decompressed static body = %q, want %q", decoded, cssBody)
	}
}
