package orchids

import (
	"context"
	"fmt"
	"sync"

	"orchids-api/internal/upstream"
)

func orchidsFinishReason(state *requestState) string {
	if state.sawToolCall {
		return "tool-calls"
	}
	return "stop"
}

func finalizeOrchidsTransport(
	ctx context.Context,
	transport string,
	state *requestState,
	onMessage func(upstream.SSEMessage),
	fsWG *sync.WaitGroup,
) error {
	if state.errorMsg != "" {
		return fmt.Errorf("orchids upstream error: %s", state.errorMsg)
	}

	if !state.finishSent {
		onMessage(upstream.SSEMessage{Type: "model", Event: map[string]interface{}{"type": "finish", "finishReason": orchidsFinishReason(state)}})
	}

	return waitOrchidsFSOperations(ctx, state, fsWG, transport)
}
