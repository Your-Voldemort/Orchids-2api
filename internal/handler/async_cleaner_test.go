package handler

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestAsyncCleaner_StartStop(t *testing.T) {
	var counter int32
	cleanFn := func() {
		atomic.AddInt32(&counter, 1)
	}

	cleaner := NewAsyncCleaner(50 * time.Millisecond)
	cleaner.Start(cleanFn)

	// 等待至少执行 2 次清理
	time.Sleep(150 * time.Millisecond)

	cleaner.Stop()

	count := atomic.LoadInt32(&counter)
	if count < 2 {
		t.Errorf("Expected at least 2 cleanups, got %d", count)
	}

	// 验证停止后不再执行
	finalCount := count
	time.Sleep(100 * time.Millisecond)
	afterStopCount := atomic.LoadInt32(&counter)
	if afterStopCount != finalCount {
		t.Errorf("Cleanup continued after Stop: before=%d, after=%d", finalCount, afterStopCount)
	}
}

func TestAsyncCleaner_MultipleStops(t *testing.T) {
	cleaner := NewAsyncCleaner(100 * time.Millisecond)
	cleaner.Start(func() {})

	// 第一次停止
	cleaner.Stop()

	// 第二次停止不应 panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Second Stop() caused panic: %v", r)
		}
	}()
	cleaner.Stop()
}

func TestAsyncCleaner_CleanFnPanic(t *testing.T) {
	cleaner := NewAsyncCleaner(50 * time.Millisecond)

	var executed int32
	cleanFn := func() {
		count := atomic.AddInt32(&executed, 1)
		if count == 1 {
			panic("test panic")
		}
	}

	// 启动清理器，即使 cleanFn panic 也应继续执行
	cleaner.Start(cleanFn)

	time.Sleep(150 * time.Millisecond)
	cleaner.Stop()

	// 有 panic 恢复机制，executed 应该 > 1
	count := atomic.LoadInt32(&executed)
	if count < 2 {
		t.Errorf("Expected at least 2 executions (with panic recovery), got %d", count)
	}
}
