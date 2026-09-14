package proxy

import (
	"bytes"
	"compress/gzip"
	"io"
	"sync"
	"testing"
)

func TestBufferPools(t *testing.T) {
	// Small buffer test
	sBuf := GetSmallBuffer()
	if sBuf == nil {
		t.Fatal("expected non-nil small buffer")
	}
	sBuf.WriteString("hello small buffer")
	if sBuf.String() != "hello small buffer" {
		t.Fatalf("unexpected buffer content: %s", sBuf.String())
	}
	PutSmallBuffer(sBuf)

	// Reacquire to ensure Reset worked
	sBuf2 := GetSmallBuffer()
	if sBuf2.Len() != 0 {
		t.Fatalf("expected reset buffer with len 0, got %d", sBuf2.Len())
	}
	PutSmallBuffer(sBuf2)

	// Oversized small buffer dropped
	bigBuf := GetSmallBuffer()
	bigBuf.Grow(5000)
	PutSmallBuffer(bigBuf) // Should not panic and should be dropped

	// Large buffer test
	lBuf := GetLargeBuffer()
	if lBuf == nil {
		t.Fatal("expected non-nil large buffer")
	}
	lBuf.WriteString("hello large buffer")
	if lBuf.String() != "hello large buffer" {
		t.Fatalf("unexpected buffer content: %s", lBuf.String())
	}
	PutLargeBuffer(lBuf)

	lBuf2 := GetLargeBuffer()
	if lBuf2.Len() != 0 {
		t.Fatalf("expected reset buffer with len 0, got %d", lBuf2.Len())
	}
	PutLargeBuffer(lBuf2)

	// Oversized large buffer dropped
	oversizedLBuf := GetLargeBuffer()
	oversizedLBuf.Grow(300 * 1024)
	PutLargeBuffer(oversizedLBuf) // Should be dropped
}

func TestGzipReaderPool(t *testing.T) {
	// Compress a test string
	original := []byte("hello gzip pool compression test data 1234567890")
	var compBuf bytes.Buffer
	gw := gzip.NewWriter(&compBuf)
	if _, err := gw.Write(original); err != nil {
		t.Fatalf("gzip write error: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("gzip close error: %v", err)
	}

	// 1st round: acquire new reader
	gr1, err := GetGzipReader(bytes.NewReader(compBuf.Bytes()))
	if err != nil {
		t.Fatalf("GetGzipReader failed: %v", err)
	}
	decomp1, err := io.ReadAll(gr1)
	if err != nil {
		t.Fatalf("io.ReadAll decomp1 failed: %v", err)
	}
	if !bytes.Equal(decomp1, original) {
		t.Fatalf("decomp1 mismatch: got %q, want %q", decomp1, original)
	}
	PutGzipReader(gr1)

	// 2nd round: acquire recycled reader from pool
	gr2, err := GetGzipReader(bytes.NewReader(compBuf.Bytes()))
	if err != nil {
		t.Fatalf("GetGzipReader from pool failed: %v", err)
	}
	decomp2, err := io.ReadAll(gr2)
	if err != nil {
		t.Fatalf("io.ReadAll decomp2 failed: %v", err)
	}
	if !bytes.Equal(decomp2, original) {
		t.Fatalf("decomp2 mismatch: got %q, want %q", decomp2, original)
	}
	PutGzipReader(gr2)
}

func TestBufferPoolConcurrency(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				sb := GetSmallBuffer()
				sb.WriteString("concurrent test")
				PutSmallBuffer(sb)

				lb := GetLargeBuffer()
				lb.WriteString("large concurrent test")
				PutLargeBuffer(lb)
			}
		}(i)
	}
}

func BenchmarkUnpooledRequestPayload(b *testing.B) {
	cascadeID := "692d971f-84f7-4180-be58-51292dbf7629"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = bytes.NewReader([]byte(`{"cascadeId":"` + cascadeID + `"}`))
	}
}

func BenchmarkPooledRequestPayload(b *testing.B) {
	cascadeID := "692d971f-84f7-4180-be58-51292dbf7629"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf := GetSmallBuffer()
		buf.WriteString(`{"cascadeId":"`)
		buf.WriteString(cascadeID)
		buf.WriteString(`"}`)
		PutSmallBuffer(buf)
	}
}

