package prompt

import (
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
				contentStr, ok := toolResultString(block.Content, maxLen)
				if ok && len(contentStr) > maxLen {
					hiddenCount := countLines(contentStr)
					snippet := truncateRunes(contentStr, 200)

					var sb strings.Builder
					sb.Grow(len(snippet) + 64)
					sb.WriteString("[Output compressed by system: ")
					sb.WriteString(itoa(hiddenCount))
					sb.WriteString(" lines hidden. Preview: ")
					sb.WriteString(snippet)
					sb.WriteByte(']')
					newContent := sb.String()

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

func toolResultString(content interface{}, maxLen int) (string, bool) {
	switch v := content.(type) {
	case string:
		return stripSystemReminders(v), true
	case []interface{}:
		if maxLen > 0 {
			var sb strings.Builder
			first := true
			for _, item := range v {
				itemMap, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				text, ok := itemMap["text"].(string)
				if !ok {
					continue
				}
				clean := stripSystemReminders(text)
				if clean == "" {
					continue
				}
				if !first {
					sb.WriteByte('\n')
				}
				sb.WriteString(clean)
				first = false
				if sb.Len() > maxLen {
					return sb.String(), true
				}
			}
			if !first {
				return sb.String(), true
			}
			return "", true
		}
		return formatToolResultContent(v), true
	default:
		return formatToolResultContent(v), true
	}
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	n := 1
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			n++
		}
	}
	return n
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "..."
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

// CollapseRepeatedErrors detects and merges consecutive identical error loops.
func CollapseRepeatedErrors(messages []Message) []Message {
	if len(messages) < 4 {
		return messages
	}

	out := make([]Message, 0, len(messages))
	hashes := make([]string, len(messages))
	hasHash := make([]bool, len(messages))
	getHash := func(i int) string {
		if !hasHash[i] {
			hashes[i] = messageHash(messages[i])
			hasHash[i] = true
		}
		return hashes[i]
	}
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
					if getHash(i) == getHash(i+2) && getHash(i+1) == getHash(i+3) {
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
