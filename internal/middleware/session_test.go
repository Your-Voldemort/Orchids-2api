package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSessionAuth_AdminPassBearer(t *testing.T) {
	called := false
	handler := SessionAuth("admin123", "", func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer admin123")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if !called {
		t.Fatalf("expected handler to be called")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d", rec.Code, http.StatusOK)
	}
}

func TestSessionAuth_QueryAppKey(t *testing.T) {
	called := false
	handler := SessionAuth("admin123", "admintoken", func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/admin/batch/task/stream?app_key=admin123", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if !called {
		t.Fatalf("expected handler to be called")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d", rec.Code, http.StatusOK)
	}
}

func TestSessionAuth_QueryPublicKey(t *testing.T) {
	called := false
	handler := SessionAuth("admin123", "admintoken", func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/public/video/sse?public_key=admin123", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if !called {
		t.Fatalf("expected handler to be called")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d", rec.Code, http.StatusOK)
	}
}

func TestSessionAuth_Unauthorized(t *testing.T) {
	called := false
	handler := SessionAuth("admin123", "admintoken", func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if called {
		t.Fatalf("expected handler not to be called")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want=%d", rec.Code, http.StatusUnauthorized)
	}
}
