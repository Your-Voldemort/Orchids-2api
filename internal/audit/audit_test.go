package audit

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func setupRedisLogger(t *testing.T) (*RedisLogger, *miniredis.Miniredis) {
	t.Helper()
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: s.Addr()})
	logger := NewRedisLogger(client, "test:", 1000)
	return logger, s
}

func TestRedisLoggerLogAndQuery(t *testing.T) {
	logger, _ := setupRedisLogger(t)
	defer logger.Close()
	ctx := context.Background()

	logger.Log(ctx, Event{
		Action:    "chat_request",
		AccountID: 1,
		Model:     "claude-sonnet-4-5",
		Status:    "success",
		Duration:  150,
	})

	logger.Log(ctx, Event{
		Action:    "image_generate",
		AccountID: 2,
		Status:    "error",
		Error:     "timeout",
	})

	// Give async writer time to flush
	time.Sleep(100 * time.Millisecond)

	events, err := logger.Query(ctx, QueryOpts{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	// Results are reverse-chronological
	if events[0].Action != "image_generate" {
		t.Fatalf("expected image_generate first (newest), got %s", events[0].Action)
	}
	if events[1].Action != "chat_request" {
		t.Fatalf("expected chat_request second (oldest), got %s", events[1].Action)
	}
}

func TestRedisLoggerTimestamp(t *testing.T) {
	logger, _ := setupRedisLogger(t)
	defer logger.Close()
	ctx := context.Background()

	before := time.Now()
	logger.Log(ctx, Event{Action: "test", Status: "success"})
	time.Sleep(100 * time.Millisecond)

	events, _ := logger.Query(ctx, QueryOpts{Limit: 1})
	if len(events) != 1 {
		t.Fatal("expected 1 event")
	}
	if events[0].Timestamp.Before(before) {
		t.Fatal("timestamp should be after log call")
	}
}

func TestNopLogger(t *testing.T) {
	logger := NewNopLogger()
	ctx := context.Background()

	// Should not panic
	logger.Log(ctx, Event{Action: "test", Status: "success"})
	events, err := logger.Query(ctx, QueryOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if events != nil {
		t.Fatal("nop logger should return nil events")
	}
	logger.Close()
}
