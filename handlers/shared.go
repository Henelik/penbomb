package handlers

import "io"

const (
	// decompressedSize is the target uncompressed size for generated gzip bombs: 100 GiB.
	decompressedSize = 100 << 30
)

// zeroReader is an io.Reader that produces an infinite stream of zero bytes
// without allocating a backing buffer per read.
type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

// contextReader wraps an io.Reader and returns io.ErrClosedPipe if the done
// channel is closed, preventing the compressor goroutine from continuing after
// the client has disconnected or the context has been cancelled.
type contextReader struct {
	r    io.Reader
	done <-chan struct{}
}

func (cr contextReader) Read(p []byte) (int, error) {
	select {
	case <-cr.done:
		return 0, io.ErrClosedPipe
	default:
		return cr.r.Read(p)
	}
}
