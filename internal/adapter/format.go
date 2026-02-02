package adapter

import "strings"

type ResponseFormat string

const (
	FormatAnthropic ResponseFormat = "anthropic"
	FormatOpenAI    ResponseFormat = "openai"
)

func DetectResponseFormat(path string) ResponseFormat {
	if strings.Contains(path, "/chat/completions") {
		return FormatOpenAI
	}
	return FormatAnthropic
}
