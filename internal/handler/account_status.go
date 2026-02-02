package handler

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"orchids-api/internal/store"
)

func classifyAccountStatus(errStr string) string {
	lower := strings.ToLower(errStr)
	switch {
	case strings.Contains(lower, "429") || strings.Contains(lower, "too many requests"):
		return "429"
	case strings.Contains(lower, "quota_exceeded") || strings.Contains(lower, "quota exceeded") || strings.Contains(lower, "quota"):
		return "quota_exceeded"
	case strings.Contains(lower, "401") || strings.Contains(lower, "signed out") || strings.Contains(lower, "signed_out"):
		return "401"
	case strings.Contains(lower, "403"):
		return "403"
	case strings.Contains(lower, "404"):
		return "404"
	default:
		return ""
	}
}

func markAccountStatus(ctx context.Context, store *store.Store, acc *store.Account, status string) {
	if acc == nil || store == nil || status == "" {
		return
	}

	now := time.Now()
	acc.StatusCode = status
	acc.LastAttempt = now
	if status == "quota_exceeded" {
		acc.QuotaResetAt = nextMonthStart(now)
	}

	if err := store.UpdateAccount(ctx, acc); err != nil {
		slog.Warn("账号状态更新失败", "account_id", acc.ID, "status", status, "error", err)
		return
	}
	slog.Info("账号状态已标记", "account_id", acc.ID, "status", status)
}

func nextMonthStart(now time.Time) time.Time {
	year, month, _ := now.Date()
	loc := now.Location()
	if month == time.December {
		return time.Date(year+1, time.January, 1, 0, 0, 0, 0, loc)
	}
	return time.Date(year, month+1, 1, 0, 0, 0, 0, loc)
}
