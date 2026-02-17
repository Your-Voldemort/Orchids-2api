package grok

import "strings"

// ModelSpec defines one public model and how it maps to Grok upstream fields.
type ModelSpec struct {
	ID            string
	Name          string
	UpstreamModel string
	ModelMode     string
	IsImage       bool
	IsVideo       bool
}

// SupportedModels is the Go-native model table ported from grok2api behavior.
var SupportedModels = []ModelSpec{
	{ID: "grok-3", Name: "Grok 3", UpstreamModel: "grok-3", ModelMode: "MODEL_MODE_GROK_3"},
	{ID: "grok-3-mini", Name: "Grok 3 Mini", UpstreamModel: "grok-3", ModelMode: "MODEL_MODE_GROK_3_MINI_THINKING"},
	{ID: "grok-3-thinking", Name: "Grok 3 Thinking", UpstreamModel: "grok-3", ModelMode: "MODEL_MODE_GROK_3_THINKING"},
	{ID: "grok-3-fast", Name: "Grok 3 Fast", UpstreamModel: "grok-3", ModelMode: "MODEL_MODE_FAST"}, // compat alias
	{ID: "grok-4", Name: "Grok 4", UpstreamModel: "grok-4", ModelMode: "MODEL_MODE_GROK_4"},
	{ID: "grok-4-mini", Name: "Grok 4 Mini", UpstreamModel: "grok-4-mini", ModelMode: "MODEL_MODE_GROK_4_MINI_THINKING"},
	{ID: "grok-4-thinking", Name: "Grok 4 Thinking", UpstreamModel: "grok-4", ModelMode: "MODEL_MODE_GROK_4_THINKING"},
	{ID: "grok-4-fast", Name: "Grok 4 Fast", UpstreamModel: "grok-4", ModelMode: "MODEL_MODE_FAST"},
	{ID: "grok-4-heavy", Name: "Grok 4 Heavy", UpstreamModel: "grok-4", ModelMode: "MODEL_MODE_HEAVY"},
	{ID: "grok-4.1-mini", Name: "Grok 4.1 Mini", UpstreamModel: "grok-4-1-thinking-1129", ModelMode: "MODEL_MODE_GROK_4_1_MINI_THINKING"},
	{ID: "grok-4.1-fast", Name: "Grok 4.1 Fast", UpstreamModel: "grok-4-1-thinking-1129", ModelMode: "MODEL_MODE_FAST"},
	{ID: "grok-4.1-expert", Name: "Grok 4.1 Expert", UpstreamModel: "grok-4-1-thinking-1129", ModelMode: "MODEL_MODE_EXPERT"},
	{ID: "grok-4.1-thinking", Name: "Grok 4.1 Thinking", UpstreamModel: "grok-4-1-thinking-1129", ModelMode: "MODEL_MODE_GROK_4_1_THINKING"},
	{ID: "grok-4.1", Name: "Grok 4.1", UpstreamModel: "grok-4-1-thinking-1129", ModelMode: "MODEL_MODE_GROK_4_1_MINI_THINKING"}, // compat alias
	{ID: "grok-imagine-1.0", Name: "Grok Imagine 1.0", UpstreamModel: "grok-3", ModelMode: "MODEL_MODE_FAST", IsImage: true},
	{ID: "grok-imagine-1.0-edit", Name: "Grok Imagine 1.0 Edit", UpstreamModel: "imagine-image-edit", ModelMode: "MODEL_MODE_FAST", IsImage: true},
	{ID: "grok-imagine-1.0-video", Name: "Grok Imagine 1.0 Video", UpstreamModel: "grok-3", ModelMode: "MODEL_MODE_FAST", IsVideo: true},
}

var modelByID = func() map[string]ModelSpec {
	out := make(map[string]ModelSpec, len(SupportedModels))
	for _, m := range SupportedModels {
		out[strings.ToLower(strings.TrimSpace(m.ID))] = m
	}
	return out
}()

func normalizeModelID(modelID string) string {
	m := strings.ToLower(strings.TrimSpace(modelID))
	// Common typo compatibility: gork-* -> grok-*
	if strings.HasPrefix(m, "gork-") {
		return "grok-" + strings.TrimPrefix(m, "gork-")
	}
	return m
}

func ResolveModel(modelID string) (ModelSpec, bool) {
	m, ok := modelByID[normalizeModelID(modelID)]
	return m, ok
}
