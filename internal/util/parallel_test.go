package util

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestParallelFor(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		ParallelFor(0, func(i int) {
			t.Error("should not be called")
		})
	})

	t.Run("single item", func(t *testing.T) {
		var called int32
		ParallelFor(1, func(i int) {
			atomic.AddInt32(&called, 1)
		})
		if called != 1 {
			t.Errorf("called = %d, want 1", called)
		}
	})

	t.Run("small batch (serial)", func(t *testing.T) {
		n := 5
		results := make([]int, n)
		ParallelFor(n, func(i int) {
			results[i] = i * 2
		})
		for i := 0; i < n; i++ {
			if results[i] != i*2 {
				t.Errorf("results[%d] = %d, want %d", i, results[i], i*2)
			}
		}
	})

	t.Run("large batch (parallel)", func(t *testing.T) {
		n := 100
		var counter int64
		ParallelFor(n, func(i int) {
			atomic.AddInt64(&counter, 1)
		})
		if counter != int64(n) {
			t.Errorf("counter = %d, want %d", counter, n)
		}
	})
}

func TestParallelForWithContext(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		err := ParallelForWithContext(context.Background(), 0, func(ctx context.Context, i int) error {
			t.Error("should not be called")
			return nil
		})
		if err != nil {
			t.Errorf("error = %v, want nil", err)
		}
	})

	t.Run("success", func(t *testing.T) {
		n := 10
		results := make([]int, n)
		err := ParallelForWithContext(context.Background(), n, func(ctx context.Context, i int) error {
			results[i] = i
			return nil
		})
		if err != nil {
			t.Errorf("error = %v, want nil", err)
		}
		for i := 0; i < n; i++ {
			if results[i] != i {
				t.Errorf("results[%d] = %d, want %d", i, results[i], i)
			}
		}
	})

	t.Run("error", func(t *testing.T) {
		expectedErr := errors.New("test error")
		err := ParallelForWithContext(context.Background(), 10, func(ctx context.Context, i int) error {
			if i == 5 {
				return expectedErr
			}
			return nil
		})
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err := ParallelForWithContext(ctx, 10, func(ctx context.Context, i int) error {
			return nil
		})
		if err == nil {
			t.Error("expected error, got nil")
		}
	})
}

func TestParallelMap(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		result := ParallelMap([]int{}, func(i int) int { return i * 2 })
		if result != nil {
			t.Errorf("result = %v, want nil", result)
		}
	})

	t.Run("transform", func(t *testing.T) {
		input := []int{1, 2, 3, 4, 5}
		result := ParallelMap(input, func(i int) int { return i * 2 })
		expected := []int{2, 4, 6, 8, 10}

		if len(result) != len(expected) {
			t.Errorf("len = %d, want %d", len(result), len(expected))
		}
		for i, v := range result {
			if v != expected[i] {
				t.Errorf("result[%d] = %d, want %d", i, v, expected[i])
			}
		}
	})

	t.Run("type conversion", func(t *testing.T) {
		input := []int{1, 2, 3}
		result := ParallelMap(input, func(i int) string {
			return string(rune('A' + i - 1))
		})
		expected := []string{"A", "B", "C"}

		for i, v := range result {
			if v != expected[i] {
				t.Errorf("result[%d] = %q, want %q", i, v, expected[i])
			}
		}
	})
}

func TestSleepWithContext(t *testing.T) {
	t.Run("zero duration", func(t *testing.T) {
		if !SleepWithContext(context.Background(), 0) {
			t.Error("SleepWithContext(0) = false, want true")
		}
	})

	t.Run("negative duration", func(t *testing.T) {
		if !SleepWithContext(context.Background(), -time.Second) {
			t.Error("SleepWithContext(-1s) = false, want true")
		}
	})

	t.Run("normal sleep", func(t *testing.T) {
		start := time.Now()
		if !SleepWithContext(context.Background(), 50*time.Millisecond) {
			t.Error("SleepWithContext() = false, want true")
		}
		if elapsed := time.Since(start); elapsed < 40*time.Millisecond {
			t.Errorf("elapsed = %v, want >= 40ms", elapsed)
		}
	})

	t.Run("canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			time.Sleep(20 * time.Millisecond)
			cancel()
		}()

		start := time.Now()
		if SleepWithContext(ctx, 1*time.Second) {
			t.Error("SleepWithContext() = true, want false for canceled context")
		}
		if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
			t.Errorf("elapsed = %v, want < 100ms", elapsed)
		}
	})
}

func TestRetry(t *testing.T) {
	t.Run("immediate success", func(t *testing.T) {
		var attempts int
		err := Retry(context.Background(), 3, 10*time.Millisecond, func() error {
			attempts++
			return nil
		})
		if err != nil {
			t.Errorf("error = %v, want nil", err)
		}
		if attempts != 1 {
			t.Errorf("attempts = %d, want 1", attempts)
		}
	})

	t.Run("eventual success", func(t *testing.T) {
		var attempts int
		err := Retry(context.Background(), 3, 10*time.Millisecond, func() error {
			attempts++
			if attempts < 3 {
				return errors.New("temporary error")
			}
			return nil
		})
		if err != nil {
			t.Errorf("error = %v, want nil", err)
		}
		if attempts != 3 {
			t.Errorf("attempts = %d, want 3", attempts)
		}
	})

	t.Run("max retries exceeded", func(t *testing.T) {
		var attempts int
		err := Retry(context.Background(), 2, 10*time.Millisecond, func() error {
			attempts++
			return errors.New("persistent error")
		})
		if err == nil {
			t.Error("expected error, got nil")
		}
		if attempts != 3 { // initial + 2 retries
			t.Errorf("attempts = %d, want 3", attempts)
		}
	})

	t.Run("canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := Retry(ctx, 3, 10*time.Millisecond, func() error {
			return errors.New("error")
		})
		if err == nil {
			t.Error("expected error, got nil")
		}
	})
}

func TestRetryWithBackoff(t *testing.T) {
	t.Run("backoff timing", func(t *testing.T) {
		var attempts int
		var timestamps []time.Time

		start := time.Now()
		_ = RetryWithBackoff(context.Background(), 3, 10*time.Millisecond, 100*time.Millisecond, func() error {
			attempts++
			timestamps = append(timestamps, time.Now())
			if attempts < 3 {
				return errors.New("error")
			}
			return nil
		})

		// Check that delays increase
		if len(timestamps) < 3 {
			t.Fatalf("timestamps = %d, want >= 3", len(timestamps))
		}

		delay1 := timestamps[1].Sub(timestamps[0])
		delay2 := timestamps[2].Sub(timestamps[1])

		// Second delay should be approximately 2x the first (with some tolerance)
		if delay2 < delay1 {
			t.Errorf("delay2 (%v) should be >= delay1 (%v)", delay2, delay1)
		}

		t.Logf("Total time: %v, delay1: %v, delay2: %v", time.Since(start), delay1, delay2)
	})

	t.Run("max delay cap", func(t *testing.T) {
		var attempts int
		err := RetryWithBackoff(context.Background(), 5, 50*time.Millisecond, 60*time.Millisecond, func() error {
			attempts++
			if attempts < 4 {
				return errors.New("error")
			}
			return nil
		})
		if err != nil {
			t.Errorf("error = %v, want nil", err)
		}
	})
}
