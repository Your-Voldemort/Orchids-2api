// Package perf provides property-based tests for object pools.
package perf

import (
	"bytes"
	"testing"
	"testing/quick"
)

// **Validates: Requirements 1.2, 1.3, 1.5**
//
// Property 1: 对象池获取归还一致性
// For any object pool (SlicePool, MapSlicePool, LargeByteBufferPool),
// after acquiring an object, releasing it, and acquiring again,
// the object should be clean (reset to empty state).

// TestSlicePoolAcquireReleaseConsistency tests that SlicePool returns clean slices
// after release and re-acquire.
func TestSlicePoolAcquireReleaseConsistency(t *testing.T) {
	property := func(data []byte) bool {
		// Acquire a slice from the pool
		slice := AcquireSlice()
		if slice == nil {
			return false
		}

		// Add some data to the slice based on input
		for _, b := range data {
			*slice = append(*slice, b)
		}

		// Release the slice back to the pool
		ReleaseSlice(slice)

		// Acquire again - should get a clean slice
		slice2 := AcquireSlice()
		if slice2 == nil {
			return false
		}
		defer ReleaseSlice(slice2)

		// The slice should be empty (length 0) after reset
		return len(*slice2) == 0
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("SlicePool acquire/release consistency property failed: %v", err)
	}
}

// TestMapSlicePoolAcquireReleaseConsistency tests that MapSlicePool returns clean slices
// after release and re-acquire.
func TestMapSlicePoolAcquireReleaseConsistency(t *testing.T) {
	property := func(count uint8) bool {
		// Limit count to reasonable size
		n := int(count % 50)

		// Acquire a map slice from the pool
		mapSlice := AcquireMapSlice()
		if mapSlice == nil {
			return false
		}

		// Add some maps to the slice
		for i := 0; i < n; i++ {
			m := make(map[string]interface{})
			m["key"] = i
			*mapSlice = append(*mapSlice, m)
		}

		// Release the slice back to the pool
		ReleaseMapSlice(mapSlice)

		// Acquire again - should get a clean slice
		mapSlice2 := AcquireMapSlice()
		if mapSlice2 == nil {
			return false
		}
		defer ReleaseMapSlice(mapSlice2)

		// The slice should be empty (length 0) after reset
		return len(*mapSlice2) == 0
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("MapSlicePool acquire/release consistency property failed: %v", err)
	}
}

// TestLargeByteBufferPoolAcquireReleaseConsistency tests that LargeByteBufferPool
// returns clean buffers after release and re-acquire.
func TestLargeByteBufferPoolAcquireReleaseConsistency(t *testing.T) {
	property := func(data []byte) bool {
		// Acquire a buffer from the pool
		buf := AcquireLargeByteBuffer()
		if buf == nil {
			return false
		}

		// Write some data to the buffer
		buf.Write(data)

		// Release the buffer back to the pool
		ReleaseLargeByteBuffer(buf)

		// Acquire again - should get a clean buffer
		buf2 := AcquireLargeByteBuffer()
		if buf2 == nil {
			return false
		}
		defer ReleaseLargeByteBuffer(buf2)

		// The buffer should be empty (length 0) after reset
		return buf2.Len() == 0
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("LargeByteBufferPool acquire/release consistency property failed: %v", err)
	}
}

// TestSlicePoolMultipleOperationsConsistency tests that SlicePool maintains consistency
// across multiple acquire/release cycles with varying data.
func TestSlicePoolMultipleOperationsConsistency(t *testing.T) {
	property := func(operations []uint8) bool {
		for _, op := range operations {
			// Acquire a slice
			slice := AcquireSlice()
			if slice == nil {
				return false
			}

			// The acquired slice should always be clean
			if len(*slice) != 0 {
				ReleaseSlice(slice)
				return false
			}

			// Add some data based on operation value
			count := int(op % 20)
			for i := 0; i < count; i++ {
				*slice = append(*slice, i)
			}

			// Release the slice
			ReleaseSlice(slice)
		}
		return true
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 50}); err != nil {
		t.Errorf("SlicePool multiple operations consistency property failed: %v", err)
	}
}

// TestMapSlicePoolMultipleOperationsConsistency tests that MapSlicePool maintains consistency
// across multiple acquire/release cycles.
func TestMapSlicePoolMultipleOperationsConsistency(t *testing.T) {
	property := func(operations []uint8) bool {
		for _, op := range operations {
			// Acquire a map slice
			mapSlice := AcquireMapSlice()
			if mapSlice == nil {
				return false
			}

			// The acquired slice should always be clean
			if len(*mapSlice) != 0 {
				ReleaseMapSlice(mapSlice)
				return false
			}

			// Add some maps based on operation value
			count := int(op % 10)
			for i := 0; i < count; i++ {
				m := make(map[string]interface{})
				m["index"] = i
				*mapSlice = append(*mapSlice, m)
			}

			// Release the slice
			ReleaseMapSlice(mapSlice)
		}
		return true
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 50}); err != nil {
		t.Errorf("MapSlicePool multiple operations consistency property failed: %v", err)
	}
}

// TestLargeByteBufferPoolMultipleOperationsConsistency tests that LargeByteBufferPool
// maintains consistency across multiple acquire/release cycles.
func TestLargeByteBufferPoolMultipleOperationsConsistency(t *testing.T) {
	property := func(dataChunks [][]byte) bool {
		for _, data := range dataChunks {
			// Acquire a buffer
			buf := AcquireLargeByteBuffer()
			if buf == nil {
				return false
			}

			// The acquired buffer should always be clean
			if buf.Len() != 0 {
				ReleaseLargeByteBuffer(buf)
				return false
			}

			// Write data to buffer
			buf.Write(data)

			// Release the buffer
			ReleaseLargeByteBuffer(buf)
		}
		return true
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 50}); err != nil {
		t.Errorf("LargeByteBufferPool multiple operations consistency property failed: %v", err)
	}
}

// TestLargeByteBufferPoolCapacity tests that LargeByteBufferPool provides buffers
// with at least 16KB capacity.
func TestLargeByteBufferPoolCapacity(t *testing.T) {
	property := func(_ uint8) bool {
		buf := AcquireLargeByteBuffer()
		if buf == nil {
			return false
		}
		defer ReleaseLargeByteBuffer(buf)

		// Buffer should have at least 16KB capacity
		return buf.Cap() >= 16384
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("LargeByteBufferPool capacity property failed: %v", err)
	}
}

// TestSlicePoolNilRelease tests that releasing nil to SlicePool doesn't panic.
func TestSlicePoolNilRelease(t *testing.T) {
	// This should not panic
	ReleaseSlice(nil)
}

// TestMapSlicePoolNilRelease tests that releasing nil to MapSlicePool doesn't panic.
func TestMapSlicePoolNilRelease(t *testing.T) {
	// This should not panic
	ReleaseMapSlice(nil)
}

// TestLargeByteBufferPoolNilRelease tests that releasing nil to LargeByteBufferPool doesn't panic.
func TestLargeByteBufferPoolNilRelease(t *testing.T) {
	// This should not panic
	ReleaseLargeByteBuffer(nil)
}

// TestAllPoolsDataIsolation tests that data written to pooled objects doesn't leak
// to subsequent acquires (validates requirement 1.5 - prevent data leakage).
func TestAllPoolsDataIsolation(t *testing.T) {
	property := func(sensitiveData []byte) bool {
		// Test SlicePool data isolation
		slice := AcquireSlice()
		for _, b := range sensitiveData {
			*slice = append(*slice, b)
		}
		ReleaseSlice(slice)

		slice2 := AcquireSlice()
		sliceClean := len(*slice2) == 0
		ReleaseSlice(slice2)

		// Test MapSlicePool data isolation
		mapSlice := AcquireMapSlice()
		for i, b := range sensitiveData {
			if i >= 10 {
				break
			}
			m := make(map[string]interface{})
			m["sensitive"] = b
			*mapSlice = append(*mapSlice, m)
		}
		ReleaseMapSlice(mapSlice)

		mapSlice2 := AcquireMapSlice()
		mapSliceClean := len(*mapSlice2) == 0
		ReleaseMapSlice(mapSlice2)

		// Test LargeByteBufferPool data isolation
		buf := AcquireLargeByteBuffer()
		buf.Write(sensitiveData)
		ReleaseLargeByteBuffer(buf)

		buf2 := AcquireLargeByteBuffer()
		bufClean := buf2.Len() == 0
		ReleaseLargeByteBuffer(buf2)

		return sliceClean && mapSliceClean && bufClean
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("Pool data isolation property failed: %v", err)
	}
}

// TestByteBufferPoolComparison tests that both ByteBufferPool and LargeByteBufferPool
// properly reset their buffers.
func TestByteBufferPoolComparison(t *testing.T) {
	property := func(data []byte) bool {
		// Test regular ByteBufferPool
		buf := AcquireByteBuffer()
		buf.Write(data)
		ReleaseLargeByteBuffer(buf)

		buf2 := AcquireByteBuffer()
		regularClean := buf2.Len() == 0
		ReleaseByteBuffer(buf2)

		// Test LargeByteBufferPool
		largeBuf := AcquireLargeByteBuffer()
		largeBuf.Write(data)
		ReleaseLargeByteBuffer(largeBuf)

		largeBuf2 := AcquireLargeByteBuffer()
		largeClean := largeBuf2.Len() == 0
		ReleaseLargeByteBuffer(largeBuf2)

		return regularClean && largeClean
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("ByteBuffer pools comparison property failed: %v", err)
	}
}

// TestPooledObjectsAreReusable tests that pooled objects can be reused multiple times
// without issues.
func TestPooledObjectsAreReusable(t *testing.T) {
	property := func(iterations uint8) bool {
		n := int(iterations%20) + 1

		for i := 0; i < n; i++ {
			// Test SlicePool reusability
			slice := AcquireSlice()
			*slice = append(*slice, "test", 123, true)
			if len(*slice) != 3 {
				ReleaseSlice(slice)
				return false
			}
			ReleaseSlice(slice)

			// Test MapSlicePool reusability
			mapSlice := AcquireMapSlice()
			*mapSlice = append(*mapSlice, map[string]interface{}{"key": "value"})
			if len(*mapSlice) != 1 {
				ReleaseMapSlice(mapSlice)
				return false
			}
			ReleaseMapSlice(mapSlice)

			// Test LargeByteBufferPool reusability
			buf := AcquireLargeByteBuffer()
			buf.WriteString("test data")
			if buf.Len() != 9 {
				ReleaseLargeByteBuffer(buf)
				return false
			}
			ReleaseLargeByteBuffer(buf)
		}

		return true
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 50}); err != nil {
		t.Errorf("Pooled objects reusability property failed: %v", err)
	}
}

// BenchmarkSlicePoolAcquireRelease benchmarks SlicePool performance.
func BenchmarkSlicePoolAcquireRelease(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		slice := AcquireSlice()
		*slice = append(*slice, 1, 2, 3)
		ReleaseSlice(slice)
	}
}

// BenchmarkMapSlicePoolAcquireRelease benchmarks MapSlicePool performance.
func BenchmarkMapSlicePoolAcquireRelease(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		mapSlice := AcquireMapSlice()
		*mapSlice = append(*mapSlice, map[string]interface{}{"key": "value"})
		ReleaseMapSlice(mapSlice)
	}
}

// BenchmarkLargeByteBufferPoolAcquireRelease benchmarks LargeByteBufferPool performance.
func BenchmarkLargeByteBufferPoolAcquireRelease(b *testing.B) {
	b.ReportAllocs()
	data := bytes.Repeat([]byte("test"), 1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := AcquireLargeByteBuffer()
		buf.Write(data)
		ReleaseLargeByteBuffer(buf)
	}
}
