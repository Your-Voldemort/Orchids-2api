package prompt

import (
	"fmt"
	"strings"
)

// CompressToolResults limits the size of tool outputs in older messages to save tokens.
// messages: The full history.
// keepLastN: Number of most recent messages to leave untouched (e.g., 6).
// maxLen: Maximum characters allowed for a tool result before compression (e.g., 2000).
func CompressToolResults(messages []Message, keepLastN int, maxLen int) []Message {
	if len(messages) <= keepLastN {
		return messages
	}

	compressed := make([]Message, 0, len(messages))
	thresholdIndex := len(messages) - keepLastN

	for i, msg := range messages {
		// If it's a recent message, keep it exactly as is
		if i >= thresholdIndex {
			compressed = append(compressed, msg)
			continue
		}

		// Only process User messages containing tool_results
		if msg.Role != "user" || msg.Content.IsString() {
			compressed = append(compressed, msg)
			continue
		}

		blocks := msg.Content.GetBlocks()
		if len(blocks) == 0 {
			compressed = append(compressed, msg)
			continue
		}

		newBlocks := make([]ContentBlock, 0, len(blocks))
		changed := false

		for _, block := range blocks {
			if block.Type == "tool_result" {
				contentStr := formatToolResultContent(block.Content)
				if len(contentStr) > maxLen {
					// Perform compression
					lines := strings.Split(contentStr, "\n")
					hiddenCount := len(lines)
					snippet := contentStr
					if len([]rune(snippet)) > 200 {
						runes := []rune(snippet)
						snippet = string(runes[:200]) + "..."
					}

					// Create a systematic summary
					newContent := fmt.Sprintf("[Output compressed by system: %d lines hidden. Preview: %s]", hiddenCount, snippet)

					// Update block
					newBlock := block
					newBlock.Content = newContent
					newBlocks = append(newBlocks, newBlock)
					changed = true
					continue
				}
			}
			newBlocks = append(newBlocks, block)
		}

		if changed {
			newMsg := msg
			newMsg.Content.Blocks = newBlocks
			compressed = append(compressed, newMsg)
		} else {
			compressed = append(compressed, msg)
		}
	}

	return compressed
}

// CollapseRepeatedErrors detects and merges consecutive identical error loops.
func CollapseRepeatedErrors(messages []Message) []Message {
	if len(messages) < 4 {
		return messages
	}

	out := make([]Message, 0, len(messages))
	i := 0
	for i < len(messages) {
		// Look for pattern: [Assistant A, User Error A] followed by [Assistant A, User Error A]
		if i+3 < len(messages) {
			a1 := messages[i]
			u1 := messages[i+1]
			a2 := messages[i+2]
			u2 := messages[i+3]

			if a1.Role == "assistant" && u1.Role == "user" &&
				a2.Role == "assistant" && u2.Role == "user" {

				if isErrorResult(u1) && isErrorResult(u2) {
					// Check if they are effectively identical loops
					if messageHash(a1) == messageHash(a2) && messageHash(u1) == messageHash(u2) {
						// Skip the first pair (deduplicate), effectively collapsing the loop
						i += 2
						continue
					}
				}
			}
		}
		out = append(out, messages[i])
		i++
	}
	return out
}

func isErrorResult(m Message) bool {
	if m.Content.IsString() {
		return false
	}
	for _, b := range m.Content.GetBlocks() {
		if b.IsError {
			return true
		}
	}
	return false
}
