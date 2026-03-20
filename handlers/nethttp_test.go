package handlers

import (
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Henelik/penbomb/payloads"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNetHTTPPenbombBrotliPath verifies that a request advertising brotli
// encoding receives the pre-built brotli payload with the correct headers.
func TestNetHTTPPenbombBrotliPath(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "br")

	rr := httptest.NewRecorder()
	NetHTTPPenbomb(rr, req)

	res := rr.Result()
	defer res.Body.Close()

	assert.Equal(t, "br", res.Header.Get("Content-Encoding"))
	assert.Equal(t, "text/plain", res.Header.Get("Content-Type"))

	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	assert.Len(t, body, len(payloads.Brotli100GiB), "body should match embedded brotli payload length")
}

// TestNetHTTPPenbombBrotliPreferredOverGzip confirms brotli is chosen when
// the Accept-Encoding header contains both "br" and "gzip".
func TestNetHTTPPenbombBrotliPreferredOverGzip(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")

	rr := httptest.NewRecorder()
	NetHTTPPenbomb(rr, req)

	res := rr.Result()
	defer res.Body.Close()

	assert.Equal(t, "br", res.Header.Get("Content-Encoding"))
}

// TestNetHTTPPenbombGzipFallbackHeaders confirms that when brotli is not
// advertised the handler falls back to gzip and sets the correct headers.
// We cancel the context immediately after the headers are observed so the
// goroutine exits without compressing the full 100 GiB.
func TestNetHTTPPenbombGzipFallbackHeaders(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req = req.WithContext(ctx)

	// cancelAfterNWriter cancels the context once >= threshold bytes have been
	// received, then discards subsequent writes so NetHTTPPenbomb can exit.
	rr := &cancelAfterNWriter{
		ResponseRecorder: httptest.NewRecorder(),
		cancel:           cancel,
		threshold:        2, // 2 bytes = gzip magic (0x1f 0x8b)
	}
	NetHTTPPenbomb(rr, req)

	assert.Equal(t, "gzip", rr.ResponseRecorder.Header().Get("Content-Encoding"))
	assert.Equal(t, "text/plain", rr.ResponseRecorder.Header().Get("Content-Type"))
}

// TestNetHTTPPenbombGzipFallbackBodyIsValidGzip verifies that the response
// body in the gzip fallback path starts with the gzip magic bytes (0x1f 0x8b).
func TestNetHTTPPenbombGzipFallbackBodyIsValidGzip(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req = req.WithContext(ctx)

	rr := &cancelAfterNWriter{
		ResponseRecorder: httptest.NewRecorder(),
		cancel:           cancel,
		threshold:        10,
	}
	NetHTTPPenbomb(rr, req)

	body := rr.ResponseRecorder.Body.Bytes()
	require.GreaterOrEqual(t, len(body), 2, "body too short; expected at least 2 gzip magic bytes")
	assert.Equal(t, byte(0x1f), body[0], "expected gzip magic byte 0x1f")
	assert.Equal(t, byte(0x8b), body[1], "expected gzip magic byte 0x8b")
}

// TestNetHTTPPenbombGzipDecompressesCorrectly reads a small number of
// decompressed bytes from the gzip stream to confirm the payload consists of
// zero bytes (consistent with the zeroReader source).
func TestNetHTTPPenbombGzipDecompressesCorrectly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pr, pw := io.Pipe()
	go func() {
		rw := &pipeRW{pw: pw, header: make(http.Header)}
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		req = req.WithContext(ctx)
		NetHTTPPenbomb(rw, req)
		_ = pw.Close()
	}()

	gr, err := gzip.NewReader(pr)
	require.NoError(t, err, "gzip.NewReader should not fail")

	const sampleSize = 4096
	buf := make([]byte, sampleSize)
	n, readErr := io.ReadFull(gr, buf)
	// Cancel context and close pipe to unblock the compressor goroutine.
	cancel()
	_ = pr.CloseWithError(io.ErrClosedPipe)

	require.Equal(t, sampleSize, n, "expected %d decompressed bytes (err: %v)", sampleSize, readErr)
	for i, b := range buf {
		if !assert.Equalf(t, byte(0), b, "decompressed buf[%d] = 0x%02X, want 0x00", i, b) {
			break
		}
	}
}

// TestNetHTTPPenbombNoAcceptEncodingFallsBackToGzip ensures that a client
// that sends no Accept-Encoding header still gets a gzip response.
func TestNetHTTPPenbombNoAcceptEncodingFallsBackToGzip(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// No Accept-Encoding header.
	req = req.WithContext(ctx)

	rr := &cancelAfterNWriter{
		ResponseRecorder: httptest.NewRecorder(),
		cancel:           cancel,
		threshold:        2,
	}
	NetHTTPPenbomb(rr, req)

	assert.Equal(t, "gzip", rr.ResponseRecorder.Header().Get("Content-Encoding"))
}

// TestNetHTTPPenbombMethodAgnostic verifies that the handler responds
// identically to POST requests (method does not affect the bomb logic).
func TestNetHTTPPenbombMethodAgnostic(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/wp-login.php", nil)
	req.Header.Set("Accept-Encoding", "br")

	rr := httptest.NewRecorder()
	NetHTTPPenbomb(rr, req)

	assert.Equal(t, "br", rr.Header().Get("Content-Encoding"))
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// cancelAfterNWriter is a minimal http.ResponseWriter that cancels a context
// once the body has received at least `threshold` bytes, then delegates to an
// embedded httptest.ResponseRecorder.
type cancelAfterNWriter struct {
	*httptest.ResponseRecorder
	cancel    context.CancelFunc
	threshold int
	received  int
}

func (w *cancelAfterNWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseRecorder.Write(b)
	w.received += n
	if w.received >= w.threshold {
		w.cancel()
	}
	return n, err
}
