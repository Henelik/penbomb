package handlers

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- zeroReader tests ---

// TestZeroReaderFillsBufferWithZeros verifies that every byte produced by
// zeroReader is 0x00.
func TestZeroReaderFillsBufferWithZeros(t *testing.T) {
	var r zeroReader
	buf := make([]byte, 256)
	// Pre-fill with non-zero value to detect if any bytes remain unchanged.
	for i := range buf {
		buf[i] = 0xFF
	}

	n, err := r.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 256, n)
	for i, b := range buf {
		assert.Equalf(t, byte(0), b, "buf[%d] = 0x%02X, want 0x00", i, b)
	}
}

// TestZeroReaderSmallBuffer checks that zeroReader works correctly for a
// single-byte buffer.
func TestZeroReaderSmallBuffer(t *testing.T) {
	var r zeroReader
	buf := make([]byte, 1)
	buf[0] = 0xAB

	n, err := r.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, byte(0), buf[0])
}

// TestZeroReaderEmptyBuffer confirms that a zero-length read returns 0 bytes
// and no error; the reader should not block or panic.
func TestZeroReaderEmptyBuffer(t *testing.T) {
	var r zeroReader
	n, err := r.Read(nil)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

// TestZeroReaderIsInfinite verifies that zeroReader never returns io.EOF on
// its own; total bytes read across multiple calls should match expectation.
func TestZeroReaderIsInfinite(t *testing.T) {
	var r zeroReader
	buf := make([]byte, 1024)
	totalReads := 10
	for i := range totalReads {
		n, err := r.Read(buf)
		require.NoErrorf(t, err, "read %d: unexpected error", i)
		assert.Equalf(t, 1024, n, "read %d: expected 1024 bytes", i)
	}
}

// --- contextReader tests ---

// TestContextReaderPassesThroughWhenOpen verifies that contextReader forwards
// reads to the underlying reader when its done channel is not closed.
func TestContextReaderPassesThroughWhenOpen(t *testing.T) {
	done := make(chan struct{})
	cr := contextReader{
		r:    io.LimitReader(zeroReader{}, 8),
		done: done,
	}

	buf := make([]byte, 8)
	n, err := cr.Read(buf)
	if err != nil {
		require.ErrorIs(t, err, io.EOF)
	}
	assert.Equal(t, 8, n)
	for i, b := range buf {
		assert.Equalf(t, byte(0), b, "buf[%d] = 0x%02X, want 0x00", i, b)
	}
}

// TestContextReaderReturnsErrWhenDoneClosed verifies that contextReader returns
// io.ErrClosedPipe immediately once the done channel is closed, without
// forwarding the call to the underlying reader.
func TestContextReaderReturnsErrWhenDoneClosed(t *testing.T) {
	done := make(chan struct{})
	close(done)

	cr := contextReader{
		r:    zeroReader{},
		done: done,
	}

	buf := make([]byte, 64)
	n, err := cr.Read(buf)
	require.ErrorIs(t, err, io.ErrClosedPipe)
	assert.Equal(t, 0, n)
}

// TestContextReaderPropagatesUnderlyingEOF ensures that an io.EOF from the
// wrapped reader is transparently returned to the caller while the context is
// still live.
func TestContextReaderPropagatesUnderlyingEOF(t *testing.T) {
	done := make(chan struct{})
	// LimitReader will emit io.EOF after 0 bytes.
	cr := contextReader{
		r:    io.LimitReader(zeroReader{}, 0),
		done: done,
	}

	buf := make([]byte, 8)
	n, err := cr.Read(buf)
	require.ErrorIs(t, err, io.EOF)
	assert.Equal(t, 0, n)
}

// TestContextReaderNilBufWhenOpen checks the zero-length read path with an
// open context.
func TestContextReaderNilBufWhenOpen(t *testing.T) {
	done := make(chan struct{})
	cr := contextReader{
		r:    zeroReader{},
		done: done,
	}

	n, err := cr.Read(nil)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

// TestDecompressedSizeConstant guards against accidental changes to the target
// decompressed payload size (100 GiB).
func TestDecompressedSizeConstant(t *testing.T) {
	const want = 100 << 30
	assert.Equal(t, int64(want), int64(decompressedSize), "decompressedSize should be 100 GiB")
}
