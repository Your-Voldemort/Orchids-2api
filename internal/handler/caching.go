package handler

import (
	"orchids-api/internal/prompt"
	"strings"
)

// ApplyCacheStrategy applies the selected cache strategy to the request messages and system prompt.
// Supported strategies:
// - "split": Caches the system prompt and the second-to-last user turn (standard checkpointing).
// - "mixed": Caches system prompt, tools (if applicable), and attempts to cache large context blocks in the last user message.
func applyCacheStrategy(req *ClaudeRequest, strategy string) {
	strategy = strings.ToLower(strings.TrimSpace(strategy))
	if strategy == "" || strategy == "none" || strategy == "off" {
		return
	}

	breakpoints := 0
	const maxBreakpoints = 4

	// Helper to add cache control
	addCache := func(block *prompt.ContentBlock) bool {
		if breakpoints >= maxBreakpoints {
			return false
		}
		if block.CacheControl == nil {
			block.CacheControl = &prompt.CacheControl{Type: "ephemeral"}
			breakpoints++
			return true
		}
		return false
	}

	// 1. System Prompt
	// Always cache the last system block if available
	if len(req.System) > 0 {
		// Cache the last system item
		lastSys := &req.System[len(req.System)-1]
		if lastSys.CacheControl == nil {
			lastSys.CacheControl = &prompt.CacheControl{Type: "ephemeral"}
			breakpoints++
		}
	}

	// 2. Tools
	// Cache the last tool definition if tools are present
	if len(req.Tools) > 0 && breakpoints < maxBreakpoints {
		lastToolIdx := len(req.Tools) - 1
		if toolMap, ok := req.Tools[lastToolIdx].(map[string]interface{}); ok {
			// Check if already has cache control
			if _, hasCache := toolMap["cache_control"]; !hasCache {
				toolMap["cache_control"] = map[string]string{"type": "ephemeral"}
				req.Tools[lastToolIdx] = toolMap // Update in place
				breakpoints++
			}
		}
	}

	// 2. Messages
	// Iterate backwards to find suitable breakpoints

	// Strategy "Split": Cache 2nd to last user message (history checkpoint)
	// Strategy "Mixed": Cache 2nd to last user message AND large blocks in last message

	msgLen := len(req.Messages)
	if msgLen == 0 {
		return
	}

	// Find user turns indices
	var userIndices []int
	for i, msg := range req.Messages {
		if msg.Role == "user" {
			userIndices = append(userIndices, i)
		}
	}

	// Turn Checkpoint (2nd to last user message) - Common for both strategies
	// This caches the conversation history up to that point.
	if len(userIndices) >= 2 {
		idx := userIndices[len(userIndices)-2]
		msg := &req.Messages[idx]
		if !msg.Content.IsString() {
			blocks := msg.Content.GetBlocks()
			if len(blocks) > 0 {
				// Cache the last block of this message
				if addCache(&blocks[len(blocks)-1]) {
					req.Messages[idx].Content.Blocks = blocks // Update
				}
			}
		}
	}

	// Last User Message
	// For "Mixed" or "Split" (if we treat Split as just history + static context)
	// If the last message has large content (e.g., file dump), cache it.
	if len(userIndices) > 0 {
		idx := userIndices[len(userIndices)-1]
		msg := &req.Messages[idx]
		if !msg.Content.IsString() {
			blocks := msg.Content.GetBlocks()
			// If Mixed, checking for large blocks
			// Or just cache the last block if we have budget
			if len(blocks) > 0 {
				// For Claude Code, typically the context is big.
				// Cache the last block.
				if strategy == "mixed" || strategy == "split" {
					if addCache(&blocks[len(blocks)-1]) {
						req.Messages[idx].Content.Blocks = blocks
					}
				}
			}
		}
	}

	// Tools?
	// If tools are passed as content blocks (some agents do this), cache them.
	// But req.Tools is separate.
}
