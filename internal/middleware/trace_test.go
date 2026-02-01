package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGenerateTraceID(t *testing.T) {
	t.Run("generates unique IDs", func(t *testing.T) {
		ids := make(map[string]bool)
		for i := 0; i < 1000; i++ {
			id := GenerateTraceID()
			if ids[id] {
				t.Errorf("duplicate trace ID generated: %s", id)
			}
			ids[id] = true
		}
	})

	t.Run("generates valid hex string", func(t *testing.T) {
		id := GenerateTraceID()
		if len(id) != 32 { // 16 bytes = 32 hex chars
			t.Errorf("trace ID length = %d, want 32", len(id))
		}
		for _, c := range id {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Errorf("invalid character in trace ID: %c", c)
			}
		}
	})
}

func TestTraceMiddleware(t *testing.T) {
	t.Run("generates trace ID when not provided", func(t *testing.T) {
		handler := TraceMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			traceID := GetTraceID(r.Context())
			if traceID == "" {
				t.Error("trace ID not found in context")
			}
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Header().Get(TraceIDHeader) == "" {
			t.Error("trace ID not set in response header")
		}
	})

	t.Run("uses provided trace ID", func(t *testing.T) {
		expectedID := "test-trace-id-123"
		handler := TraceMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			traceID := GetTraceID(r.Context())
			if traceID != expectedID {
				t.Errorf("trace ID = %q, want %q", traceID, expectedID)
			}
		}))

		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set(TraceIDHeader, expectedID)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Header().Get(TraceIDHeader) != expectedID {
			t.Errorf("response trace ID = %q, want %q", w.Header().Get(TraceIDHeader), expectedID)
		}
	})

	t.Run("uses X-Request-ID as fallback", func(t *testing.T) {
		expectedID := "request-id-456"
		handler := TraceMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			traceID := GetTraceID(r.Context())
			if traceID != expectedID {
				t.Errorf("trace ID = %q, want %q", traceID, expectedID)
			}
		}))

		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set(RequestIDHeader, expectedID)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)
	})
}

func TestTraceFunc(t *testing.T) {
	called := false
	handler := TraceFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		traceID := GetTraceID(r.Context())
		if traceID == "" {
			t.Error("trace ID not found in context")
		}
	})

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if !called {
		t.Error("handler was not called")
	}
}

func TestGetTraceID(t *testing.T) {
	t.Run("returns empty for nil context", func(t *testing.T) {
		if got := GetTraceID(nil); got != "" {
			t.Errorf("GetTraceID(nil) = %q, want empty", got)
		}
	})

	t.Run("returns empty for context without trace ID", func(t *testing.T) {
		ctx := context.Background()
		if got := GetTraceID(ctx); got != "" {
			t.Errorf("GetTraceID(ctx) = %q, want empty", got)
		}
	})

	t.Run("returns trace ID from context", func(t *testing.T) {
		expected := "test-trace-id"
		ctx := WithTraceID(context.Background(), expected)
		if got := GetTraceID(ctx); got != expected {
			t.Errorf("GetTraceID(ctx) = %q, want %q", got, expected)
		}
	})
}

func TestWithTraceID(t *testing.T) {
	expected := "my-trace-id"
	ctx := WithTraceID(context.Background(), expected)
	got := GetTraceID(ctx)
	if got != expected {
		t.Errorf("GetTraceID after WithTraceID = %q, want %q", got, expected)
	}
}

func TestLogWithTrace(t *testing.T) {
	t.Run("returns logger without trace ID", func(t *testing.T) {
		logger := LogWithTrace(context.Background())
		if logger == nil {
			t.Error("LogWithTrace returned nil")
		}
	})

	t.Run("returns logger with trace ID", func(t *testing.T) {
		ctx := WithTraceID(context.Background(), "test-id")
		logger := LogWithTrace(ctx)
		if logger == nil {
			t.Error("LogWithTrace returned nil")
		}
	})
}

func TestTracedResponseWriter(t *testing.T) {
	t.Run("tracks status code", func(t *testing.T) {
		w := httptest.NewRecorder()
		traced := NewTracedResponseWriter(w)

		traced.WriteHeader(http.StatusNotFound)

		if traced.StatusCode != http.StatusNotFound {
			t.Errorf("StatusCode = %d, want %d", traced.StatusCode, http.StatusNotFound)
		}
	})

	t.Run("default status is 200", func(t *testing.T) {
		w := httptest.NewRecorder()
		traced := NewTracedResponseWriter(w)

		if traced.StatusCode != http.StatusOK {
			t.Errorf("default StatusCode = %d, want %d", traced.StatusCode, http.StatusOK)
		}
	})

	t.Run("tracks bytes written", func(t *testing.T) {
		w := httptest.NewRecorder()
		traced := NewTracedResponseWriter(w)

		traced.Write([]byte("hello"))
		traced.Write([]byte(" world"))

		if traced.BytesWritten != 11 {
			t.Errorf("BytesWritten = %d, want 11", traced.BytesWritten)
		}
	})

	t.Run("flush works", func(t *testing.T) {
		w := httptest.NewRecorder()
		traced := NewTracedResponseWriter(w)

		// Should not panic
		traced.Flush()
	})
}

func TestLoggingMiddleware(t *testing.T) {
	handler := LoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	// Add trace middleware first
	handler = TraceMiddleware(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestChain(t *testing.T) {
	var order []string

	m1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "m1-before")
			next.ServeHTTP(w, r)
			order = append(order, "m1-after")
		})
	}

	m2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "m2-before")
			next.ServeHTTP(w, r)
			order = append(order, "m2-after")
		})
	}

	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order = append(order, "handler")
	})

	chained := Chain(m1, m2)(final)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	chained.ServeHTTP(w, req)

	expected := []string{"m1-before", "m2-before", "handler", "m2-after", "m1-after"}
	if len(order) != len(expected) {
		t.Fatalf("order length = %d, want %d", len(order), len(expected))
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("order[%d] = %q, want %q", i, order[i], v)
		}
	}
}

func TestChainFunc(t *testing.T) {
	var order []string

	m1 := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "m1")
			next(w, r)
		}
	}

	m2 := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "m2")
			next(w, r)
		}
	}

	final := func(w http.ResponseWriter, r *http.Request) {
		order = append(order, "handler")
	}

	chained := ChainFunc(m1, m2)(final)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	chained.ServeHTTP(w, req)

	expected := []string{"m1", "m2", "handler"}
	if len(order) != len(expected) {
		t.Fatalf("order length = %d, want %d", len(order), len(expected))
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("order[%d] = %q, want %q", i, order[i], v)
		}
	}
}
