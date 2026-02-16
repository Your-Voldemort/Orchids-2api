package grok

import (
	"net/http"
	"strings"
	"time"
)

// replyChatTextAndImages returns an OpenAI-compatible chat completion that contains:
// text first, then Markdown images. In stream mode, it emits two chunks (role, then content).
func (h *Handler) replyChatTextAndImages(w http.ResponseWriter, model string, text string, imgs []string, stream bool) {
	text = strings.TrimSpace(text)
	cleanImgs := make([]string, 0, len(imgs))
	for _, u := range imgs {
		u = strings.TrimSpace(u)
		if u != "" {
			cleanImgs = append(cleanImgs, u)
		}
	}

	var out strings.Builder
	if text != "" {
		out.WriteString(text)
	}
	if len(cleanImgs) > 0 {
		if out.Len() > 0 {
			out.WriteString("\n\n")
		}
		if text == "" {
			out.WriteString("图片链接：\n")
		}
		// Always emit raw URLs first for clients that do not render Markdown images.
		for _, u := range cleanImgs {
			out.WriteString(u)
			out.WriteString("\n")
		}
		out.WriteString("\n")
		for _, u := range cleanImgs {
			out.WriteString("![](")
			out.WriteString(u)
			out.WriteString(")\n")
		}
	}
	content := out.String()

	id := "chatcmpl_" + randomHex(8)
	created := time.Now().Unix()

	if stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, _ := w.(http.Flusher)

		chunk1 := map[string]interface{}{
			"id":      id,
			"object":  "chat.completion.chunk",
			"created": created,
			"model":   model,
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"delta":         map[string]interface{}{"role": "assistant"},
					"logprobs":      nil,
					"finish_reason": nil,
				},
			},
		}
		writeSSE(w, "", encodeJSON(chunk1))
		if flusher != nil {
			flusher.Flush()
		}

		chunk2 := map[string]interface{}{
			"id":      id,
			"object":  "chat.completion.chunk",
			"created": created,
			"model":   model,
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"delta":         map[string]interface{}{"content": content},
					"logprobs":      nil,
					"finish_reason": nil,
				},
			},
		}
		writeSSE(w, "", encodeJSON(chunk2))
		writeSSE(w, "", "[DONE]")
		if flusher != nil {
			flusher.Flush()
		}
		return
	}

	resp := map[string]interface{}{
		"id":      id,
		"object":  "chat.completion",
		"created": created,
		"model":   model,
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": content,
				},
				"logprobs":      nil,
				"finish_reason": "stop",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(encodeJSON(resp)))
}
