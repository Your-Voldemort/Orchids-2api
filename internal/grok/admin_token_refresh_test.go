package grok

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCollectRefreshTokens(t *testing.T) {
	req := adminTokenRefreshRequest{
		Token:  "sso=t0; Path=/",
		Tokens: []string{"t1", "sso=t1", "  t2  "},
	}
	got := collectRefreshTokens(req)
	if len(got) != 3 {
		t.Fatalf("collectRefreshTokens len=%d want=3", len(got))
	}
	want := []string{"t0", "t1", "t2"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("collectRefreshTokens[%d]=%q want=%q", i, got[i], want[i])
		}
	}
}

func TestCollectRefreshTokens_Empty(t *testing.T) {
	if got := collectRefreshTokens(adminTokenRefreshRequest{}); len(got) != 0 {
		t.Fatalf("collectRefreshTokens empty len=%d want=0", len(got))
	}
}

func TestHandleAdminTokensRefresh_MethodNotAllowed(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/tokens/refresh", strings.NewReader(""))
	rec := httptest.NewRecorder()
	h.HandleAdminTokensRefresh(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d want=%d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleAdminTokensRefreshAsync_MethodNotAllowed(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/tokens/refresh/async", strings.NewReader(""))
	rec := httptest.NewRecorder()
	h.HandleAdminTokensRefreshAsync(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d want=%d", rec.Code, http.StatusMethodNotAllowed)
	}
}
