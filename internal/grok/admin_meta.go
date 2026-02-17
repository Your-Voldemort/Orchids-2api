package grok

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (h *Handler) HandleAdminVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
	})
}

func (h *Handler) HandleAdminStorage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	storageType := "redis"
	if h != nil && h.cfg != nil && strings.TrimSpace(h.cfg.StoreMode) != "" {
		storageType = strings.ToLower(strings.TrimSpace(h.cfg.StoreMode))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"type": storageType,
	})
}
