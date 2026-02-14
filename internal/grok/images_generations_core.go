package grok

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// generateViaImagesGenerations implements the TRUE scheme-1: reuse the /grok/v1/images/generations
// behavior (and account switching policy) to obtain stable /files URLs, then let chat embed them.
//
// It does NOT rely on the upstream imagine chat returning direct URLs, which is often empty.
func (h *Handler) generateViaImagesGenerations(ctx context.Context, prompt string, n int, responseFormat string) ([]string, string) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil, "empty-prompt"
	}
	if n <= 0 {
		n = 1
	}
	if n > 10 {
		// keep consistent with images endpoint contract
		n = 10
	}

	spec, ok := ResolveModel("grok-imagine-1.0")
	if !ok {
		return nil, "model-not-found"
	}

	acc, token, err := h.selectAccount(ctx)
	if err != nil {
		return nil, "no-token"
	}
	release := h.trackAccount(acc)
	defer release()

	switched := false
	doChatWithSwitch := func(payload map[string]interface{}) (map[string]interface{}, error) {
		resp, err := h.client.doChat(ctx, token, payload)
		if err != nil {
			status := classifyAccountStatusFromError(err.Error())
			h.markAccountStatus(ctx, acc, err)
			if h.cfg != nil && h.cfg.GrokDebugImageFallback {
				slog.Warn("images-generations core: upstream error", "status", status, "err", err.Error(), "switched", switched)
			}
			if !switched && (status == "403" || status == "429") {
				switched = true
				release()
				acc2, token2, err2 := h.selectAccount(ctx)
				if err2 != nil {
					return nil, err
				}
				acc, token = acc2, token2
				release = h.trackAccount(acc)
				resp2, err3 := h.client.doChat(ctx, token, payload)
				if err3 != nil {
					status2 := classifyAccountStatusFromError(err3.Error())
					h.markAccountStatus(ctx, acc, err3)
					if h.cfg != nil && h.cfg.GrokDebugImageFallback {
						slog.Warn("images-generations core: upstream error(after switch)", "status", status2, "err", err3.Error())
					}
					return nil, err3
				}
				defer resp2.Body.Close()
				h.syncGrokQuota(acc, resp2.Header)
				var out map[string]interface{}
				_ = parseUpstreamLines(resp2.Body, func(line map[string]interface{}) error {
					out = line
					if mr, ok := line["modelResponse"].(map[string]interface{}); ok {
						out = mr
					}
					return nil
				})
				return out, nil
			}
			return nil, err
		}
		defer resp.Body.Close()
		h.syncGrokQuota(acc, resp.Header)
		var out map[string]interface{}
		_ = parseUpstreamLines(resp.Body, func(line map[string]interface{}) error {
			out = line
			if mr, ok := line["modelResponse"].(map[string]interface{}); ok {
				out = mr
			}
			return nil
		})
		return out, nil
	}

	// mirrors HandleImagesGenerations non-stream behavior: request single images and vary prompt.
	var urls []string
	maxAttempts := n * 4
	if maxAttempts < 4 {
		maxAttempts = 4
	}
	deadline := time.Now().Add(60 * time.Second)
	variants := []string{"安福路白天街拍", "外滩夜景街拍", "南京路人潮街拍", "法租界梧桐街拍", "弄堂市井街拍", "陆家嘴现代街拍", "地铁口街拍", "雨天街拍"}

	for i := 0; i < maxAttempts; i++ {
		if time.Now().After(deadline) {
			break
		}
		cur := normalizeImageURLs(urls, 0)
		if len(cur) >= n {
			urls = cur
			break
		}
		v := variants[i%len(variants)]
		seed := randomHex(4)
		prompt2 := fmt.Sprintf("%s\n\n请生成与之前不同的一张图片：%s。要求不同人物/不同构图/不同光线。（seed %s #%d）", prompt, v, seed, i+1)
		payload := h.client.chatPayload(spec, "Image Generation: "+strings.TrimSpace(prompt2), true, 1)
		if h.cfg != nil && h.cfg.GrokDebugImageFallback {
			slog.Info("images-generations core: attempt", "i", i+1, "max", maxAttempts, "variant", v, "seed", seed)
		}
		if err := ctx.Err(); err != nil {
			break
		}
		mr, err := doChatWithSwitch(payload)
		if err != nil {
			break
		}
		before := len(urls)
		urls = append(urls, extractImageURLs(mr)...)
		urls = normalizeImageURLs(urls, 0)
		after := len(urls)
		if h.cfg != nil && h.cfg.GrokDebugImageFallback {
			slog.Info("images-generations core: attempt result", "new_urls", after-before, "total_urls", after)
		}
		if after <= before {
			// no new urls this attempt
			// keep trying variants; upstream often repeats
		}
	}

	urls = normalizeImageURLs(urls, n)
	if len(urls) == 0 {
		return nil, "no-urls"
	}

	// apply /files caching + "no part-0 if full exists" contract and response_format.
	out := make([]string, 0, len(urls))
	// In chat fallback we may not have the original request host; best effort: keep absolute if imageOutputValue returns it,
	// otherwise leave as-is.
	publicBase := ""
	for _, u := range urls {
		val, errV := h.imageOutputValue(context.Background(), token, u, responseFormat)
		if errV != nil || strings.TrimSpace(val) == "" {
			val = u
		}
		if publicBase != "" && strings.HasPrefix(val, "/") {
			val = publicBase + val
		}
		out = append(out, val)
	}
	return out, "ok"
}
