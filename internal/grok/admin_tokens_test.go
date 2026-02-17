package grok

import "testing"

func TestCollectAdminTokenEntries(t *testing.T) {
	payload := map[string]interface{}{
		"ssoBasic": []interface{}{
			"t0",
			map[string]interface{}{
				"token":     "sso=t1",
				"status":    "cooling",
				"quota":     float64(80),
				"use_count": float64(2),
				"note":      "note-1",
			},
		},
		"ssoSuper": []interface{}{
			map[string]interface{}{
				"token":  "t2",
				"status": "invalid",
			},
		},
	}
	entries := collectAdminTokenEntries(payload)
	if len(entries) != 3 {
		t.Fatalf("entries len=%d want=3", len(entries))
	}
	if entries[0].Token != "t0" || entries[1].Token != "t1" || entries[2].Token != "t2" {
		t.Fatalf("unexpected tokens: %+v", entries)
	}
	if entries[1].Status != "cooling" || entries[1].Quota != 80 || entries[1].UseCount != 2 || entries[1].Note != "note-1" {
		t.Fatalf("unexpected entry[1]: %+v", entries[1])
	}
	if entries[2].Status != "expired" {
		t.Fatalf("invalid status should normalize to expired, got=%q", entries[2].Status)
	}
}

func TestNormalizeAdminTokenStatus(t *testing.T) {
	if got := normalizeAdminTokenStatus("active"); got != "active" {
		t.Fatalf("status active=%q", got)
	}
	if got := normalizeAdminTokenStatus("cooling"); got != "cooling" {
		t.Fatalf("status cooling=%q", got)
	}
	if got := normalizeAdminTokenStatus("invalid"); got != "expired" {
		t.Fatalf("status invalid=%q want expired", got)
	}
	if got := normalizeAdminTokenStatus("anything"); got != "active" {
		t.Fatalf("unknown status=%q want active", got)
	}
}
