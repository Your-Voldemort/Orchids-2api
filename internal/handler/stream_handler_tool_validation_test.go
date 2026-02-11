package handler

import (
	"net/http/httptest"
	"testing"

	"orchids-api/internal/adapter"
	"orchids-api/internal/config"
	"orchids-api/internal/debug"
	"orchids-api/internal/upstream"
)

func TestHasRequiredToolInput(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		tool     string
		input    string
		expected bool
	}{
		{name: "edit empty json", tool: "Edit", input: `{}`, expected: false},
		{name: "edit missing old/new", tool: "Edit", input: `{"file_path":"/tmp/a"}`, expected: false},
		{name: "edit valid", tool: "Edit", input: `{"file_path":"/tmp/a","old_string":"a","new_string":"b"}`, expected: true},
		{name: "write empty json", tool: "Write", input: `{}`, expected: false},
		{name: "write valid", tool: "Write", input: `{"file_path":"/tmp/a","content":"x"}`, expected: true},
		{name: "bash empty", tool: "Bash", input: `{}`, expected: false},
		{name: "bash valid", tool: "Bash", input: `{"command":"ls"}`, expected: true},
		{name: "unknown tool malformed json", tool: "Unknown", input: `{`, expected: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := hasRequiredToolInput(tc.tool, tc.input)
			if got != tc.expected {
				t.Fatalf("hasRequiredToolInput(%q, %q) = %v, want %v", tc.tool, tc.input, got, tc.expected)
			}
		})
	}
}

func TestToolCallSameIDInvalidThenValid_UsesValidOne(t *testing.T) {
	t.Parallel()

	h := newStreamHandler(
		&config.Config{OutputTokenMode: "final"},
		httptest.NewRecorder(),
		debug.New(false, false),
		false,
		false, // non-stream mode for easier assertions
		adapter.FormatAnthropic,
		"",
	)
	defer h.release()

	// First frame is incomplete and should be rejected.
	h.handleMessage(upstream.SSEMessage{
		Type: "model.tool-call",
		Event: map[string]interface{}{
			"toolCallId": "tool_same_id",
			"toolName":   "Edit",
			"input":      "{}",
		},
	})

	// Second frame (same toolCallId) is valid and should be accepted.
	h.handleMessage(upstream.SSEMessage{
		Type: "model.tool-call",
		Event: map[string]interface{}{
			"toolCallId": "tool_same_id",
			"toolName":   "Write",
			"input":      `{"file_path":"/tmp/a.txt","content":"x"}`,
		},
	})

	h.handleMessage(upstream.SSEMessage{
		Type:  "model.finish",
		Event: map[string]interface{}{"finishReason": "tool_use"},
	})

	if len(h.contentBlocks) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(h.contentBlocks))
	}

	block := h.contentBlocks[0]
	if got, _ := block["type"].(string); got != "tool_use" {
		t.Fatalf("expected tool_use block, got %q", got)
	}
	if got, _ := block["name"].(string); got != "Write" {
		t.Fatalf("expected Write tool call, got %q", got)
	}
}

func TestToolCallDifferentIDsSameInput_BothAccepted(t *testing.T) {
	t.Parallel()

	h := newStreamHandler(
		&config.Config{OutputTokenMode: "final"},
		httptest.NewRecorder(),
		debug.New(false, false),
		false,
		false, // non-stream mode for easier assertions
		adapter.FormatAnthropic,
		"",
	)
	defer h.release()

	input := `{"file_path":"/tmp/a.txt","content":"x"}`
	h.handleMessage(upstream.SSEMessage{
		Type: "model.tool-call",
		Event: map[string]interface{}{
			"toolCallId": "tool_id_1",
			"toolName":   "Write",
			"input":      input,
		},
	})
	h.handleMessage(upstream.SSEMessage{
		Type: "model.tool-call",
		Event: map[string]interface{}{
			"toolCallId": "tool_id_2",
			"toolName":   "Write",
			"input":      input,
		},
	})

	h.handleMessage(upstream.SSEMessage{
		Type:  "model.finish",
		Event: map[string]interface{}{"finishReason": "tool_use"},
	})

	if len(h.contentBlocks) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(h.contentBlocks))
	}
	for i := range h.contentBlocks {
		block := h.contentBlocks[i]
		if got, _ := block["type"].(string); got != "tool_use" {
			t.Fatalf("block %d type = %q, want tool_use", i, got)
		}
		if got, _ := block["name"].(string); got != "Write" {
			t.Fatalf("block %d tool = %q, want Write", i, got)
		}
	}
}

func TestBashToolCallDifferentIDsSameCommand_Deduped(t *testing.T) {
	t.Parallel()

	h := newStreamHandler(
		&config.Config{OutputTokenMode: "final"},
		httptest.NewRecorder(),
		debug.New(false, false),
		false,
		false, // non-stream mode for easier assertions
		adapter.FormatAnthropic,
		"",
	)
	defer h.release()

	input := `{"command":"rm /Users/dailin/Documents/GitHub/TEST/calculator.py"}`
	h.handleMessage(upstream.SSEMessage{
		Type: "model.tool-call",
		Event: map[string]interface{}{
			"toolCallId": "bash_id_1",
			"toolName":   "Bash",
			"input":      input,
		},
	})
	h.handleMessage(upstream.SSEMessage{
		Type: "model.tool-call",
		Event: map[string]interface{}{
			"toolCallId": "bash_id_2",
			"toolName":   "Bash",
			"input":      input,
		},
	})

	h.handleMessage(upstream.SSEMessage{
		Type:  "model.finish",
		Event: map[string]interface{}{"finishReason": "tool_use"},
	})

	if len(h.contentBlocks) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(h.contentBlocks))
	}
	if got, _ := h.contentBlocks[0]["name"].(string); got != "Bash" {
		t.Fatalf("expected Bash tool call, got %q", got)
	}
}

func TestBashToolCallDifferentIDsDifferentCommands_BothAccepted(t *testing.T) {
	t.Parallel()

	h := newStreamHandler(
		&config.Config{OutputTokenMode: "final"},
		httptest.NewRecorder(),
		debug.New(false, false),
		false,
		false, // non-stream mode for easier assertions
		adapter.FormatAnthropic,
		"",
	)
	defer h.release()

	h.handleMessage(upstream.SSEMessage{
		Type: "model.tool-call",
		Event: map[string]interface{}{
			"toolCallId": "bash_id_1",
			"toolName":   "Bash",
			"input":      `{"command":"pwd"}`,
		},
	})
	h.handleMessage(upstream.SSEMessage{
		Type: "model.tool-call",
		Event: map[string]interface{}{
			"toolCallId": "bash_id_2",
			"toolName":   "Bash",
			"input":      `{"command":"ls -la"}`,
		},
	})

	h.handleMessage(upstream.SSEMessage{
		Type:  "model.finish",
		Event: map[string]interface{}{"finishReason": "tool_use"},
	})

	if len(h.contentBlocks) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(h.contentBlocks))
	}
}

func TestToolCallMissingID_UsesFallbackAndIsAccepted(t *testing.T) {
	t.Parallel()

	h := newStreamHandler(
		&config.Config{OutputTokenMode: "final"},
		httptest.NewRecorder(),
		debug.New(false, false),
		false,
		false, // non-stream mode for easier assertions
		adapter.FormatAnthropic,
		"",
	)
	defer h.release()

	h.handleMessage(upstream.SSEMessage{
		Type: "model.tool-call",
		Event: map[string]interface{}{
			"toolName": "Bash",
			"input":    `{"command":"pwd"}`,
		},
	})

	h.handleMessage(upstream.SSEMessage{
		Type:  "model.finish",
		Event: map[string]interface{}{"finishReason": "tool_use"},
	})

	if len(h.contentBlocks) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(h.contentBlocks))
	}
	if got, _ := h.contentBlocks[0]["name"].(string); got != "Bash" {
		t.Fatalf("expected Bash tool call, got %q", got)
	}
}
