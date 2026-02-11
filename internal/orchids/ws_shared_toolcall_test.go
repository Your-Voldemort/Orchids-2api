package orchids

import "testing"

func TestExtractToolCallsFromResponse_FunctionCallMissingIDGetsStableFallback(t *testing.T) {
	t.Parallel()

	msg := map[string]interface{}{
		"response": map[string]interface{}{
			"output": []interface{}{
				map[string]interface{}{
					"type":      "function_call",
					"name":      "Bash",
					"arguments": `{"command":"pwd"}`,
				},
			},
		},
	}

	calls1 := extractToolCallsFromResponse(msg)
	calls2 := extractToolCallsFromResponse(msg)
	if len(calls1) != 1 || len(calls2) != 1 {
		t.Fatalf("expected one call each, got %d and %d", len(calls1), len(calls2))
	}
	if calls1[0].id == "" || calls2[0].id == "" {
		t.Fatalf("expected non-empty fallback IDs")
	}
	if calls1[0].id != calls2[0].id {
		t.Fatalf("expected stable fallback IDs, got %q and %q", calls1[0].id, calls2[0].id)
	}
	if calls1[0].name != "Bash" {
		t.Fatalf("unexpected tool name: %q", calls1[0].name)
	}
}

func TestExtractToolCallsFromResponse_ToolUseMissingIDGetsStableFallback(t *testing.T) {
	t.Parallel()

	msg := map[string]interface{}{
		"response": map[string]interface{}{
			"output": []interface{}{
				map[string]interface{}{
					"type": "tool_use",
					"name": "Write",
					"input": map[string]interface{}{
						"file_path": "/tmp/a.txt",
						"content":   "hello",
					},
				},
			},
		},
	}

	calls1 := extractToolCallsFromResponse(msg)
	calls2 := extractToolCallsFromResponse(msg)
	if len(calls1) != 1 || len(calls2) != 1 {
		t.Fatalf("expected one call each, got %d and %d", len(calls1), len(calls2))
	}
	if calls1[0].id == "" || calls2[0].id == "" {
		t.Fatalf("expected non-empty fallback IDs")
	}
	if calls1[0].id != calls2[0].id {
		t.Fatalf("expected stable fallback IDs, got %q and %q", calls1[0].id, calls2[0].id)
	}
	if calls1[0].name != "Write" {
		t.Fatalf("unexpected tool name: %q", calls1[0].name)
	}
}
