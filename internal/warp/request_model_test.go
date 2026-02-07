package warp

import "testing"

func TestNormalizeModel(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// 空字符串 → 默认模型
		{"", defaultModel},
		{"  ", defaultModel},

		// Known 列表直接匹配
		{"auto", "auto"},
		{"auto-efficient", "auto-efficient"},
		{"auto-genius", "auto-genius"},
		{"warp-basic", "warp-basic"},
		{"claude-4-5-sonnet", "claude-4-5-sonnet"},
		{"claude-4-5-sonnet-thinking", "claude-4-5-sonnet-thinking"},
		{"claude-4-5-opus", "claude-4-5-opus"},
		{"claude-4-5-opus-thinking", "claude-4-5-opus-thinking"},
		{"claude-4-6-opus-high", "claude-4-6-opus-high"},
		{"claude-4-6-opus-max", "claude-4-6-opus-max"},
		{"claude-4-5-haiku", "claude-4-5-haiku"},
		{"claude-4-opus", "claude-4-opus"},
		{"claude-4-sonnet", "claude-4-sonnet"},
		{"gpt-5-low", "gpt-5-low"},
		{"gpt-5-medium", "gpt-5-medium"},
		{"gpt-5-high", "gpt-5-high"},
		{"gpt-5-1-low", "gpt-5-1-low"},
		{"gpt-5-1-medium", "gpt-5-1-medium"},
		{"gpt-5-1-high", "gpt-5-1-high"},
		{"gpt-5-1-codex-low", "gpt-5-1-codex-low"},
		{"gpt-5-1-codex-medium", "gpt-5-1-codex-medium"},
		{"gpt-5-1-codex-high", "gpt-5-1-codex-high"},
		{"gpt-5-1-codex-max-low", "gpt-5-1-codex-max-low"},
		{"gemini-2-5-pro", "gemini-2-5-pro"},
		{"gemini-2.5-pro", "gemini-2.5-pro"},
		{"gemini-3-pro", "gemini-3-pro"},
		{"o3", "o3"},
		{"o4-mini", "o4-mini"},

		// 大小写归一化 → known 匹配
		{"Auto", "auto"},
		{"AUTO-GENIUS", "auto-genius"},
		{"Claude-4-5-Sonnet", "claude-4-5-sonnet"},

		// Sonnet 4.5 模糊匹配
		{"claude-sonnet-4-5", "claude-4-5-sonnet"},
		{"claude-sonnet 4.5", "claude-4-5-sonnet"},
		{"claude-sonnet-4-5-thinking", "claude-4-5-sonnet-thinking"},

		// Opus 4.6 模糊匹配
		{"claude-opus-4-6", "claude-4-6-opus-high"},
		{"claude-opus 4.6", "claude-4-6-opus-high"},
		{"claude-opus-4-6-max", "claude-4-6-opus-max"},
		{"claude-opus 4.6 max", "claude-4-6-opus-max"},

		// Opus 4.5 模糊匹配
		{"claude-opus-4-5", "claude-4-5-opus"},
		{"claude-opus 4.5", "claude-4-5-opus"},
		{"claude-opus-4-5-thinking", "claude-4-5-opus-thinking"},

		// Haiku 4.5 模糊匹配
		{"claude-haiku-4-5", "claude-4-5-haiku"},
		{"claude-haiku 4.5", "claude-4-5-haiku"},

		// Sonnet 4 模糊匹配
		{"claude-sonnet-4", "claude-4-sonnet"},

		// Opus 4 模糊匹配
		{"claude-opus-4", "claude-4-opus"},
		{"claude-opus 4", "claude-4-opus"},

		// Gemini 模糊匹配
		{"gemini-3", "gemini-3-pro"},
		{"gemini 3", "gemini-3-pro"},
		{"gemini-2-5", "gemini-2-5-pro"},
		{"gemini-2.5", "gemini-2-5-pro"},
		{"gemini 2.5", "gemini-2-5-pro"},

		// GPT-5.1 Codex Max 模糊匹配
		{"gpt-5-1-codex-max", "gpt-5-1-codex-max-low"},
		{"gpt-5.1-codex-max", "gpt-5-1-codex-max-low"},

		// 通配匹配
		{"haiku", "claude-4-5-haiku"},
		{"my-haiku-model", "claude-4-5-haiku"},
		{"sonnet", "claude-4-5-sonnet"},
		{"sonnet-thinking", "claude-4-5-sonnet-thinking"},
		{"opus", "claude-4-5-opus"},
		{"opus-thinking", "claude-4-5-opus-thinking"},

		// 未知模型 → auto
		{"unknown-model", "auto"},
		{"gpt-4-turbo", "auto"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeModel(tt.input)
			if got != tt.want {
				t.Errorf("normalizeModel(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
