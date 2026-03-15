package orchids

import (
	"strings"

	"github.com/goccy/go-json"
)

type requestState struct {
	preferCodingAgent   bool
	textStarted         bool
	reasoningStarted    bool
	nextBlockIndex      int
	textBlockIndex      int
	reasoningBlockIndex int
	lastTextDelta       string
	lastTextEvent       string
	finishSent          bool
	sawToolCall         bool
	hasFSOps            bool
	responseStarted     bool
	suppressStarts      bool
	activeWrites        map[string]*fileWriterState
	errorMsg            string
}

type fileWriterState struct {
	path string
	buf  strings.Builder
}

func cloneRawJSON(data []byte) json.RawMessage {
	if len(data) == 0 {
		return nil
	}
	return json.RawMessage(data)
}
