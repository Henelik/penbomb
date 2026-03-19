package handlers

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"

	"github.com/Henelik/penbomb/payloads"
)

// NetHTTPPenbomb is a standard net/http handler that returns a zip bomb.
//
// Brotli path: serves the embedded 100 GiB brotli bomb (~79 KiB compressed).
// Sending the pre-built payload has a near-zero server CPU cost and produces the
// highest possible expansion ratio (~1,340,000x).
//
// Gzip fallback: generates a 100 GiB gzip bomb on the fly via an io.Pipe so
// the server never buffers the full payload in memory.
//
// Goroutine lifecycle (gzip path only):
//   - Normal completion: compressor finishes, closes pw, http.ResponseWriter reads EOF.
//   - Client disconnect: the ResponseWriter write returns an error; io.Copy in the
//     goroutine propagates it via pw.CloseWithError, and the goroutine exits.
//   - Context cancellation: contextReader.Read returns io.ErrClosedPipe on the
//     next read, same exit path.
func NetHTTPPenbomb(w http.ResponseWriter, r *http.Request) {
	accept := r.Header.Get("Accept-Encoding")

	if strings.Contains(accept, "br") {
		w.Header().Set("Content-Encoding", "br")
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(payloads.Brotli100GiB)
		return
	}

	// Gzip fallback: generate on the fly.
	pr, pw := io.Pipe()

	src := contextReader{
		r:    io.LimitReader(zeroReader{}, decompressedSize),
		done: r.Context().Done(),
	}

	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Content-Type", "application/octet-stream")

	go func() {
		gz, err := gzip.NewWriterLevel(pw, gzip.BestCompression)
		if err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		_, err = io.Copy(gz, src)
		if err != nil {
			_ = gz.Close()
			_ = pw.CloseWithError(err)
			return
		}
		if err = gz.Close(); err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		_ = pw.Close()
	}()

	// Copy the compressed stream to the response. If the client disconnects,
	// the write to w will fail; io.Copy returns the error, we close pr with
	// it, and the goroutine exits on the next blocked write into pw.
	_, err := io.Copy(w, pr)
	if err != nil {
		_ = pr.CloseWithError(err)
	}
}
