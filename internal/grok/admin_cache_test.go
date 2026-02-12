package grok

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHandleAdminCacheEndpoints(t *testing.T) {
	oldBase := cacheBaseDir
	cacheBaseDir = t.TempDir()
	t.Cleanup(func() { cacheBaseDir = oldBase })

	imageDir := filepath.Join(cacheBaseDir, "image")
	videoDir := filepath.Join(cacheBaseDir, "video")
	if err := os.MkdirAll(imageDir, 0o755); err != nil {
		t.Fatalf("mkdir image: %v", err)
	}
	if err := os.MkdirAll(videoDir, 0o755); err != nil {
		t.Fatalf("mkdir video: %v", err)
	}
	if err := os.WriteFile(filepath.Join(imageDir, "a.jpg"), []byte("abc"), 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}
	if err := os.WriteFile(filepath.Join(videoDir, "b.mp4"), []byte("12345"), 0o644); err != nil {
		t.Fatalf("write video: %v", err)
	}

	h := &Handler{}

	summaryReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/cache", nil)
	summaryRec := httptest.NewRecorder()
	h.HandleAdminCache(summaryRec, summaryReq)
	if summaryRec.Code != http.StatusOK {
		t.Fatalf("summary status=%d want=200", summaryRec.Code)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/cache/list", nil)
	listRec := httptest.NewRecorder()
	h.HandleAdminCacheList(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status=%d want=200", listRec.Code)
	}
	var listResp struct {
		Items []cacheEntry `json:"items"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listResp.Items) != 2 {
		t.Fatalf("items=%d want=2", len(listResp.Items))
	}

	deleteBody := map[string]interface{}{
		"media_type": "image",
		"name":       "a.jpg",
	}
	deleteRaw, _ := json.Marshal(deleteBody)
	deleteReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/cache/item/delete", bytes.NewReader(deleteRaw))
	deleteRec := httptest.NewRecorder()
	h.HandleAdminCacheItemDelete(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete status=%d want=200", deleteRec.Code)
	}
	if _, err := os.Stat(filepath.Join(imageDir, "a.jpg")); !os.IsNotExist(err) {
		t.Fatalf("image cache file should be removed, err=%v", err)
	}

	clearReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/cache/clear", bytes.NewReader([]byte("{}")))
	clearRec := httptest.NewRecorder()
	h.HandleAdminCacheClear(clearRec, clearReq)
	if clearRec.Code != http.StatusOK {
		t.Fatalf("clear status=%d want=200", clearRec.Code)
	}
	remain, err := os.ReadDir(videoDir)
	if err != nil {
		t.Fatalf("readdir video: %v", err)
	}
	if len(remain) != 0 {
		t.Fatalf("video cache should be empty, remain=%d", len(remain))
	}
}
