package handler

import (
	"context"
	"time"

	"orchids-api/internal/constants"
	"orchids-api/internal/tiktoken"
	"orchids-api/internal/tokencache"
)

func (h *Handler) estimateInputTokens(ctx context.Context, model, prompt string) int {
	if prompt == "" {
		return 0
	}
	if h.tokenCache == nil || h.config == nil || !h.config.CacheTokenCount {
		return tiktoken.EstimateTextTokens(prompt)
	}

	ttl := time.Duration(h.config.CacheTTL) * time.Minute
	if ttl <= 0 {
		ttl = constants.TokenCacheTTL
	}
	h.tokenCache.SetTTL(ttl)

	key := tokencache.CacheKey(h.config.CacheStrategy, model, prompt)
	if tokens, ok := h.tokenCache.Get(ctx, key); ok {
		return tokens
	}

	tokens := tiktoken.EstimateTextTokens(prompt)
	h.tokenCache.Put(ctx, key, tokens)
	return tokens
}
