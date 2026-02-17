package middleware

import (
	"net/http"
	"strings"

	"orchids-api/internal/auth"
)

func SessionAuth(adminPass, adminToken string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session_token")
		if err == nil && auth.ValidateSessionToken(cookie.Value) {
			next(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if adminToken != "" {
			if authHeader == "Bearer "+adminToken || authHeader == adminToken {
				next(w, r)
				return
			}
			if r.Header.Get("X-Admin-Token") == adminToken {
				next(w, r)
				return
			}
		}
		if adminPass != "" {
			if authHeader == "Bearer "+adminPass || authHeader == adminPass {
				next(w, r)
				return
			}
			if r.Header.Get("X-Admin-Token") == adminPass {
				next(w, r)
				return
			}
		}

		queryKeys := []string{
			strings.TrimSpace(r.URL.Query().Get("app_key")),
			strings.TrimSpace(r.URL.Query().Get("public_key")),
		}
		for _, queryKey := range queryKeys {
			if queryKey == "" {
				continue
			}
			if adminToken != "" && queryKey == adminToken {
				next(w, r)
				return
			}
			if adminPass != "" && queryKey == adminPass {
				next(w, r)
				return
			}
		}

		_, pass, ok := r.BasicAuth()
		if ok && pass == adminPass {
			next(w, r)
			return
		}

		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	}
}
