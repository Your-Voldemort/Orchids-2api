package handler

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"
)

func TestComputeRequestHash_ChangesWithAuthPathBody(t *testing.T) {
	h := &Handler{}
	mkReq := func(path, auth string) *http.Request {
		r, _ := http.NewRequest(http.MethodPost, "http://example.com"+path, bytes.NewReader([]byte("{}")))
		if auth != "" {
			r.Header.Set("Authorization", auth)
		}
		return r
	}
	bodyA := []byte(`{"a":1}`)
	bodyB := []byte(`{"a":2}`)

	h1 := h.computeRequestHash(mkReq("/v1/messages", "Bearer x"), bodyA)
	h2 := h.computeRequestHash(mkReq("/v1/messages", "Bearer x"), bodyA)
	if h1 != h2 {
		t.Fatalf("expected stable hash, got %q vs %q", h1, h2)
	}

	if h1 == h.computeRequestHash(mkReq("/v1/messages", "Bearer y"), bodyA) {
		t.Fatalf("expected auth to affect hash")
	}
	if h1 == h.computeRequestHash(mkReq("/v1/other", "Bearer x"), bodyA) {
		t.Fatalf("expected path to affect hash")
	}
	if h1 == h.computeRequestHash(mkReq("/v1/messages", "Bearer x"), bodyB) {
		t.Fatalf("expected body to affect hash")
	}
}

func TestRegisterRequest_DedupWindowAndInFlight(t *testing.T) {
	h := &Handler{
		dedupStore: NewMemoryDedupStore(duplicateWindow, duplicateCleanupWindow),
	}
	key := "k"
	ctx := context.Background()

	dup, inFlight := h.dedupStore.Register(ctx, key)
	if dup || inFlight {
		t.Fatalf("first request should not be dup/inflight, got dup=%v inflight=%v", dup, inFlight)
	}

	dup, inFlight = h.dedupStore.Register(ctx, key)
	if !dup {
		t.Fatalf("second immediate request should be treated as duplicate")
	}
	if !inFlight {
		t.Fatalf("expected inflight=true while original is in flight")
	}

	h.dedupStore.Finish(ctx, key)
	dup, inFlight = h.dedupStore.Register(ctx, key)
	if !dup {
		t.Fatalf("request within dedup window should still be treated as duplicate")
	}
	if inFlight {
		t.Fatalf("expected inflight=false after finish")
	}
}

func TestDedupStore_WindowExpiry(t *testing.T) {
	store := NewMemoryDedupStore(100*time.Millisecond, 10*time.Second)
	ctx := context.Background()

	store.Register(ctx, "hash1")
	store.Finish(ctx, "hash1")

	time.Sleep(150 * time.Millisecond)

	dup, _ := store.Register(ctx, "hash1")
	if dup {
		t.Fatal("should not be duplicate after window expiry")
	}
}
