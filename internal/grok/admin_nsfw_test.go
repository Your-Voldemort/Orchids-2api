package grok

import (
	"testing"

	"orchids-api/internal/store"
)

func TestCollectNSFWTargets_DefaultAllGrok(t *testing.T) {
	accounts := []*store.Account{
		{ID: 1, Name: "g1", AccountType: "grok", ClientCookie: "sso=token-a; Path=/"},
		{ID: 2, Name: "w1", AccountType: "warp", ClientCookie: "token-w"},
		{ID: 3, Name: "g2", AccountType: "grok", RefreshToken: "token-b"},
		{ID: 4, Name: "g3", AgentMode: "grok", ClientCookie: "token-a"},
	}

	targets := collectNSFWTargets(adminNSFWEnableRequest{}, accounts)
	if len(targets) != 2 {
		t.Fatalf("targets len=%d want=2", len(targets))
	}
	if targets[0].Token != "token-a" || targets[1].Token != "token-b" {
		t.Fatalf("unexpected tokens: %+v", targets)
	}
}

func TestCollectNSFWTargets_ByAccountIDs(t *testing.T) {
	accounts := []*store.Account{
		{ID: 10, Name: "g10", AccountType: "grok", ClientCookie: "sso=t10"},
		{ID: 11, Name: "g11", AccountType: "grok", ClientCookie: "sso=t11"},
		{ID: 12, Name: "w12", AccountType: "warp", ClientCookie: "sso=t12"},
	}
	req := adminNSFWEnableRequest{AccountIDs: []int64{11, 12}}
	targets := collectNSFWTargets(req, accounts)
	if len(targets) != 1 {
		t.Fatalf("targets len=%d want=1", len(targets))
	}
	if targets[0].Token != "t11" || targets[0].AccountID != 11 {
		t.Fatalf("unexpected target: %+v", targets[0])
	}
}

func TestCollectNSFWTargets_ExplicitTokens(t *testing.T) {
	accounts := []*store.Account{
		{ID: 20, Name: "g20", AccountType: "grok", ClientCookie: "sso=t20"},
	}
	req := adminNSFWEnableRequest{
		Token:  "sso=t0; Path=/",
		Tokens: []string{"t1", "sso=t1", "  t2  "},
	}
	targets := collectNSFWTargets(req, accounts)
	if len(targets) != 3 {
		t.Fatalf("targets len=%d want=3", len(targets))
	}
	want := []string{"t0", "t1", "t2"}
	for i, token := range want {
		if targets[i].Token != token {
			t.Fatalf("targets[%d]=%q want=%q", i, targets[i].Token, token)
		}
	}
}

func TestMaskToken(t *testing.T) {
	if got := maskToken(""); got != "" {
		t.Fatalf("maskToken empty=%q", got)
	}
	if got := maskToken("short"); got != "short" {
		t.Fatalf("maskToken short=%q", got)
	}
	long := "abcdefghijklmnopqrstuvwxyz0123456789"
	got := maskToken(long)
	if got != "abcdefgh...23456789" {
		t.Fatalf("maskToken long=%q", got)
	}
}

func TestNormalizeNSFWConcurrency(t *testing.T) {
	cases := []struct {
		in   int
		want int
	}{
		{in: -1, want: 5},
		{in: 0, want: 5},
		{in: 1, want: 1},
		{in: 6, want: 6},
		{in: 20, want: 20},
		{in: 21, want: 20},
	}
	for _, tc := range cases {
		if got := normalizeNSFWConcurrency(tc.in); got != tc.want {
			t.Fatalf("normalizeNSFWConcurrency(%d)=%d want=%d", tc.in, got, tc.want)
		}
	}
}

func TestParseBatchTaskPath(t *testing.T) {
	taskID, action, ok := parseBatchTaskPath("/api/v1/admin/batch/task123/stream")
	if !ok || taskID != "task123" || action != "stream" {
		t.Fatalf("parse stream got ok=%v taskID=%q action=%q", ok, taskID, action)
	}

	taskID, action, ok = parseBatchTaskPath("/v1/admin/batch/task123/stream")
	if !ok || taskID != "task123" || action != "stream" {
		t.Fatalf("parse v1 stream got ok=%v taskID=%q action=%q", ok, taskID, action)
	}

	taskID, action, ok = parseBatchTaskPath("/api/v1/admin/batch/task123/cancel")
	if !ok || taskID != "task123" || action != "cancel" {
		t.Fatalf("parse cancel got ok=%v taskID=%q action=%q", ok, taskID, action)
	}

	bad := []string{
		"/api/v1/admin/batch/",
		"/api/v1/admin/batch/task123",
		"/api/v1/admin/batch/task123/unknown",
		"/api/v1/admin/other/task123/stream",
	}
	for _, path := range bad {
		if _, _, ok := parseBatchTaskPath(path); ok {
			t.Fatalf("parseBatchTaskPath(%q) expected invalid", path)
		}
	}
}
