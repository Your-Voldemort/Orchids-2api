package warp

import (
	"strings"
	"testing"

	"orchids-api/internal/prompt"
)

func TestEstimateInputTokens_TracksWarpToolCost(t *testing.T) {
	t.Parallel()

	messages := []prompt.Message{
		{
			Role: "user",
			Content: prompt.MessageContent{
				Text: "当前目录",
			},
		},
	}

	noToolEstimate, err := EstimateInputTokens("", "auto-efficient", messages, nil, true)
	if err != nil {
		t.Fatalf("EstimateInputTokens without tools: %v", err)
	}
	if noToolEstimate.Total <= 0 {
		t.Fatalf("expected positive total without tools, got %+v", noToolEstimate)
	}
	if noToolEstimate.Profile != "warp-no-tools" {
		t.Fatalf("unexpected no-tool profile: %s", noToolEstimate.Profile)
	}

	withToolEstimate, err := EstimateInputTokens("", "auto-efficient", messages, []interface{}{
		map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "Bash",
				"description": strings.Repeat("Run shell command. ", 20),
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"command":     map[string]interface{}{"type": "string", "description": "command"},
						"description": map[string]interface{}{"type": "string", "description": "reason"},
						"ignored":     map[string]interface{}{"type": "string", "description": "should be dropped"},
					},
					"required": []interface{}{"command", "ignored"},
				},
			},
		},
	}, false)
	if err != nil {
		t.Fatalf("EstimateInputTokens with tools: %v", err)
	}
	if withToolEstimate.Profile != "warp-tools" {
		t.Fatalf("unexpected tool profile: %s", withToolEstimate.Profile)
	}
	if withToolEstimate.ToolSchemaTokens <= 0 {
		t.Fatalf("expected tool schema tokens > 0, got %+v", withToolEstimate)
	}
	if withToolEstimate.Total <= noToolEstimate.Total {
		t.Fatalf("expected tools to increase total tokens: no_tools=%d with_tools=%d", noToolEstimate.Total, withToolEstimate.Total)
	}
}

func TestConvertTools_FiltersUnsupportedAndMinimizesSchema(t *testing.T) {
	t.Parallel()

	defs := convertTools([]interface{}{
		map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "Bash",
				"description": strings.Repeat("desc ", 100),
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"command":     map[string]interface{}{"type": "string"},
						"description": map[string]interface{}{"type": "string"},
						"ignored":     map[string]interface{}{"type": "string"},
					},
					"required": []interface{}{"command", "ignored"},
				},
			},
		},
		map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "Agent",
				"description": "unsupported",
				"parameters": map[string]interface{}{
					"type": "object",
				},
			},
		},
	})

	if len(defs) != 1 {
		t.Fatalf("expected only supported tool to remain, got %d defs", len(defs))
	}
	if defs[0].Name != "Bash" {
		t.Fatalf("expected Bash, got %s", defs[0].Name)
	}
	if len([]rune(defs[0].Description)) > maxWarpToolDescLen {
		t.Fatalf("description not compacted: len=%d", len([]rune(defs[0].Description)))
	}
	props, ok := defs[0].Schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected schema properties, got %#v", defs[0].Schema["properties"])
	}
	if _, ok := props["command"]; !ok {
		t.Fatalf("expected command property to remain")
	}
	if _, ok := props["ignored"]; ok {
		t.Fatalf("expected ignored property to be dropped")
	}
	req, ok := defs[0].Schema["required"].([]interface{})
	if !ok || len(req) != 1 || req[0] != "command" {
		t.Fatalf("expected required to keep only command, got %#v", defs[0].Schema["required"])
	}
}
