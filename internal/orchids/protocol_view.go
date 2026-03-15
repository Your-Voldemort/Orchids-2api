package orchids

import (
	"strings"

	"orchids-api/internal/prompt"
)

const promptProfileOrchidsProtocol = "orchids-protocol"

func BuildCodeFreeMaxPromptAndHistoryWithMeta(
	messages []prompt.Message,
	system []prompt.SystemItem,
	noThinking bool,
) (string, []map[string]string, PromptBuildMeta) {
	conversation := buildOrchidsConversationMessages(messages)
	currentUserIdx := findCurrentOrchidsUserMessageIndex(conversation)

	promptText := buildCodeFreeMaxPrompt(messages, system, conversation)
	history := buildOrchidsConversationHistory(conversation, currentUserIdx)

	return promptText, history, PromptBuildMeta{
		Profile:    promptProfileOrchidsProtocol,
		NoThinking: noThinking,
	}
}

func buildCodeFreeMaxPrompt(
	messages []prompt.Message,
	system []prompt.SystemItem,
	conversation []OrchidsConversationMessage,
) string {
	systemText := extractCodeFreeMaxSystemText(messages, system)

	userText, toolResultOnly := extractOrchidsUserMessage(conversation)
	userText = resolveCodeFreeMaxCurrentTurnText(messages, userText, toolResultOnly)

	var b strings.Builder
	if systemText != "" {
		b.WriteString("<sys>\n")
		b.WriteString(systemText)
		b.WriteString("\n</sys>\n\n")
	}
	if userText != "" {
		b.WriteString("<user>\n")
		b.WriteString(userText)
		b.WriteString("\n</user>\n")
	}
	return strings.TrimSpace(b.String())
}

func extractCodeFreeMaxSystemText(messages []prompt.Message, system []prompt.SystemItem) string {
	var parts []string
	for _, msg := range messages {
		if !strings.EqualFold(strings.TrimSpace(msg.Role), "system") {
			continue
		}
		if msg.Content.IsString() {
			text := strings.TrimSpace(stripSystemReminders(msg.Content.GetText()))
			if text != "" {
				parts = append(parts, text)
			}
			continue
		}
		for _, block := range msg.Content.GetBlocks() {
			if strings.TrimSpace(block.Type) != "text" {
				continue
			}
			text := strings.TrimSpace(stripSystemReminders(block.Text))
			if text != "" {
				parts = append(parts, text)
			}
		}
	}
	if len(parts) == 0 && len(system) > 0 {
		for _, item := range system {
			text := strings.TrimSpace(stripSystemReminders(item.Text))
			if text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func resolveCodeFreeMaxCurrentTurnText(messages []prompt.Message, userText string, toolResultOnly bool) string {
	userText = strings.TrimSpace(stripSystemReminders(userText))
	currentUserIdx := findCurrentUserMessageIndex(messages)
	if currentUserIdx < 0 || currentUserIdx >= len(messages) {
		return userText
	}
	if !toolResultOnly {
		return userText
	}

	userText = buildAttributedCurrentToolResultText(messages, currentUserIdx, userText)
	userText = strings.TrimSpace(stripSystemReminders(userText))
	previousText := strings.TrimSpace(findLatestUserText(messages[:currentUserIdx]))
	if previousText == "" {
		return userText
	}
	if userText == "" {
		return previousText
	}
	if userText == previousText {
		return userText
	}

	followUp := buildToolResultFollowUpUserText(previousText, userText)
	if guidance := buildOptimizationToolFollowUpGuidance(messages[:currentUserIdx+1], previousText); guidance != "" {
		followUp = strings.TrimSpace(followUp + "\n\n" + guidance)
	}
	return strings.TrimSpace(followUp)
}
