package workbuddy

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/goccy/go-json"

	"orchids-api/internal/upstream"
	"orchids-api/internal/util"
)

// streamResult accumulates what the upstream produced so the caller can decide
// whether the attempt was meaningful and which stop reason to report.
type streamResult struct {
	SawMeaningfulEvent bool
	ToolCallCount      int
	Usage              map[string]interface{}
	ThinkingSignature  string
	FinishReasonValue  string
}

// FinishReason maps the accumulated stream onto an Anthropic-style stop reason.
func (r streamResult) FinishReason() string {
	if r.ToolCallCount > 0 || strings.EqualFold(r.FinishReasonValue, "tool_calls") {
		return "tool_use"
	}
	switch strings.ToLower(strings.TrimSpace(r.FinishReasonValue)) {
	case "length", "max_tokens":
		return "max_tokens"
	case "content_filter":
		return "refusal"
	default:
		return "end_turn"
	}
}

// NewToolCallID mints a local tool-call id for upstream deltas that omit one.
func NewToolCallID() string { return util.NewToolCallID() }

// streamChunk is one `data:` line of the SSE response.
type streamChunk struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Model   string `json:"model"`
	Code    int    `json:"code"`
	Msg     string `json:"msg"`
	Choices []struct {
		Index        int    `json:"index"`
		FinishReason string `json:"finish_reason"`
		Delta        struct {
			Role             string `json:"role"`
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			ToolCalls        []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
	Usage map[string]interface{} `json:"usage"`
	Error struct {
		Message string          `json:"message"`
		Code    json.RawMessage `json:"code"`
	} `json:"error"`
}

// consumeStream parses the SSE body and forwards deltas to the caller.
func consumeStream(body io.Reader, onMessage func(upstream.SSEMessage)) (streamResult, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	result := streamResult{}
	tools := util.NewToolCallAccumulator()
	sawDone := false
	sawFinish := false

	emitTools := func() {
		for _, state := range tools.CompleteAll() {
			result.SawMeaningfulEvent = true
			result.ToolCallCount++
			if onMessage == nil {
				continue
			}
			id := state.ID
			if id == "" {
				id = NewToolCallID()
			}
			onMessage(upstream.SSEMessage{Type: "model.tool-call", Event: map[string]interface{}{
				"toolCallId": id,
				"toolName":   state.Name,
				"input":      util.NormalizeToolInput(state.Arguments),
			}})
		}
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			// Some edge responses keep HTTP 200 but return the business envelope as
			// plain JSON rather than SSE. Preserve that code instead of degrading it
			// to a generic "no events" protocol error.
			if strings.HasPrefix(line, "{") {
				var env envelope
				if json.Unmarshal([]byte(line), &env) == nil && env.Code != 0 {
					return result, apiError(http.StatusOK, []byte(line))
				}
			}
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			sawDone = true
			break
		}
		if payload == "" {
			continue
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return result, fmt.Errorf("workbuddy stream protocol error: invalid JSON: %w", err)
		}
		if chunk.Code != 0 {
			return result, apiError(http.StatusOK, []byte(payload))
		}
		if msg := strings.TrimSpace(chunk.Error.Message); msg != "" {
			return result, fmt.Errorf("workbuddy stream error: %s", msg)
		}
		if chunk.Usage != nil {
			if usage := normalizeUsage(chunk.Usage); len(usage) > 0 {
				result.Usage = usage
				result.SawMeaningfulEvent = true
				if onMessage != nil {
					onMessage(upstream.SSEMessage{Type: "model.tokens-used", Event: usage})
				}
			}
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		if sawFinish {
			return result, fmt.Errorf("workbuddy stream protocol error: choice data after finish")
		}
		delta := chunk.Choices[0].Delta

		if delta.ReasoningContent != "" {
			result.SawMeaningfulEvent = true
			if onMessage != nil {
				if result.ThinkingSignature == "" {
					result.ThinkingSignature = newThinkingSignature()
				}
				onMessage(upstream.SSEMessage{Type: "model.reasoning-delta", Event: map[string]interface{}{
					"delta":     delta.ReasoningContent,
					"signature": result.ThinkingSignature,
				}})
			}
		}
		if delta.Content != "" {
			result.SawMeaningfulEvent = true
			if onMessage != nil {
				onMessage(upstream.SSEMessage{Type: "model.text-delta", Event: map[string]interface{}{
					"delta": delta.Content,
				}})
			}
		}
		for _, call := range delta.ToolCalls {
			result.SawMeaningfulEvent = true
			tools.Add(call.Index, call.ID, call.Function.Name, call.Function.Arguments)
		}
		// OpenAI-style tool arguments can span several deltas. Emitting on the
		// first delta loses every later fragment and produces invalid JSON. A
		// non-empty finish reason closes the choice; [DONE]/EOF is handled below.
		if reason := strings.TrimSpace(chunk.Choices[0].FinishReason); reason != "" {
			result.FinishReasonValue = reason
			sawFinish = true
			emitTools()
		}
	}
	if err := scanner.Err(); err != nil {
		return result, fmt.Errorf("failed to read workbuddy stream: %w", err)
	}
	if !sawDone || !sawFinish {
		return result, fmt.Errorf("workbuddy stream ended before terminal finish")
	}
	return result, nil
}

func newThinkingSignature() string { return util.NewThinkingSignature("workbuddy-v1") }

// normalizeUsage maps the upstream usage object onto the key pair the shared
// stream handler consumes.
func normalizeUsage(raw map[string]interface{}) map[string]interface{} {
	if len(raw) == 0 {
		return nil
	}
	input, hasInput := util.UsageInt(raw, "prompt_tokens", "promptTokens", "input_tokens", "inputTokens")
	output, hasOutput := util.UsageInt(raw, "completion_tokens", "completionTokens", "output_tokens", "outputTokens")
	if !hasInput && !hasOutput {
		return nil
	}
	out := make(map[string]interface{}, 6)
	if hasInput {
		out["inputTokens"] = input
		out["input_tokens"] = input
	}
	if hasOutput {
		out["outputTokens"] = output
		out["output_tokens"] = output
	}
	if cached, ok := util.UsageInt(raw, "prompt_cache_hit_tokens", "cached_tokens"); ok {
		out["cacheReadTokens"] = cached
		out["cache_read_tokens"] = cached
	}
	if reasoning, ok := util.UsageInt(raw, "completion_thinking_tokens", "reasoning_tokens"); ok {
		out["reasoningTokens"] = reasoning
	} else if details := util.UsageMap(raw, "completion_tokens_details", "completionTokensDetails"); details != nil {
		if reasoning, ok := util.UsageInt(details, "reasoning_tokens", "reasoningTokens"); ok {
			out["reasoningTokens"] = reasoning
		}
	}
	return out
}
