package grok

import (
	"context"
	"io"
	"strings"
)

// doTextOnlyChat calls Grok chat for text ONLY (no image tool overrides) and returns the best-effort message text.
func (h *Handler) doTextOnlyChat(ctx context.Context, spec ModelSpec, prompt string) (string, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", nil
	}

	acc, token, err := h.selectAccount(ctx)
	if err != nil {
		return "", err
	}
	release := h.trackAccount(acc)
	defer release()

	payload := h.client.chatPayload(spec, prompt, true, 0)
	// force-disable any image-related overrides
	payload["toolOverrides"] = map[string]interface{}{"imageGen": false}
	payload["enableImageGeneration"] = false
	payload["enableImageStreaming"] = false

	resp, err := h.client.doChat(ctx, token, payload)
	if err != nil {
		h.markAccountStatus(ctx, acc, err)
		return "", err
	}
	defer resp.Body.Close()
	h.syncGrokQuota(acc, resp.Header)

	// parse upstream lines, keep last modelResponse.message
	last := ""
	_ = parseUpstreamLines(resp.Body, func(line map[string]interface{}) error {
		if mr, ok := line["modelResponse"].(map[string]interface{}); ok {
			if msg, ok2 := mr["message"].(string); ok2 {
				if strings.TrimSpace(msg) != "" {
					last = msg
				}
			}
		}
		if ur, ok := line["userResponse"].(map[string]interface{}); ok {
			if msg, ok2 := ur["message"].(string); ok2 {
				if strings.TrimSpace(msg) != "" {
					last = msg
				}
			}
		}
		return nil
	})
	_ = io.EOF
	return strings.TrimSpace(last), nil
}
