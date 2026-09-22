package proxy

import (
	"bytes"
	"compress/gzip"
	"io"
	"sync"
)

// smallBufferPool pools reusable memory buffers for frequent, small RPC request payloads (< 4KB).
var smallBufferPool = sync.Pool{
	New: func() any {
		return bytes.NewBuffer(make([]byte, 0, 512))
	},
}

// GetSmallBuffer acquires a clean buffer from the small buffer pool.
func GetSmallBuffer() *bytes.Buffer {
	buf := smallBufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

// PutSmallBuffer resets and returns a buffer to the small buffer pool.
// Buffers that have expanded beyond 4KB are dropped to prevent memory bloat.
func PutSmallBuffer(buf *bytes.Buffer) {
	if buf == nil {
		return
	}
	if buf.Cap() > 4096 {
		return
	}
	buf.Reset()
	smallBufferPool.Put(buf)
}

// largeBufferPool pools reusable memory buffers for larger JSON payloads and stream buffers (< 256KB).
var largeBufferPool = sync.Pool{
	New: func() any {
		return bytes.NewBuffer(make([]byte, 0, 32*1024))
	},
}

// GetLargeBuffer acquires a clean buffer from the large buffer pool.
func GetLargeBuffer() *bytes.Buffer {
	buf := largeBufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

// PutLargeBuffer resets and returns a buffer to the large buffer pool.
// Buffers that have expanded beyond 256KB are dropped to prevent memory bloat.
func PutLargeBuffer(buf *bytes.Buffer) {
	if buf == nil {
		return
	}
	if buf.Cap() > 256*1024 {
		return
	}
	buf.Reset()
	largeBufferPool.Put(buf)
}

// gzipReaderPool pools gzip.Reader instances to eliminate repeated ~32KB decompression buffer allocations.
var gzipReaderPool sync.Pool

// GetGzipReader acquires or creates a gzip.Reader bound to the provided reader.
func GetGzipReader(r io.Reader) (*gzip.Reader, error) {
	if v := gzipReaderPool.Get(); v != nil {
		gz := v.(*gzip.Reader)
		if err := gz.Reset(r); err != nil {
			// Failed reset: drop and create new
			return gzip.NewReader(r)
		}
		return gz, nil
	}
	return gzip.NewReader(r)
}

// PutGzipReader closes and returns a gzip.Reader to the pool.
func PutGzipReader(gz *gzip.Reader) {
	if gz == nil {
		return
	}
	_ = gz.Close()
	gzipReaderPool.Put(gz)
}

// readBodyToPool reads up to limit bytes from r using a pooled buffer.
// The caller must call the returned cleanup function once it has finished using the returned bytes.
func readBodyToPool(r io.Reader, limit int64) ([]byte, func(), error) {
	buf := GetLargeBuffer()
	cleanup := func() {
		PutLargeBuffer(buf)
	}
	if r == nil {
		return nil, cleanup, nil
	}
	var reader io.Reader = r
	if limit > 0 {
		reader = io.LimitReader(r, limit)
	}
	if _, err := buf.ReadFrom(reader); err != nil {
		cleanup()
		return nil, nil, err
	}
	return buf.Bytes(), cleanup, nil
}

// pooledGzipReadCloser wraps a pooled gzip.Reader and the original body,
// returning the gzip reader to the pool on Close().
type pooledGzipReadCloser struct {
	gz   *gzip.Reader
	body io.ReadCloser
}

func (p *pooledGzipReadCloser) Read(b []byte) (int, error) {
	return p.gz.Read(b)
}

func (p *pooledGzipReadCloser) Close() error {
	PutGzipReader(p.gz)
	return p.body.Close()
}

