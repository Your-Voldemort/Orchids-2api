package handler

import (
	"encoding/json"
	"net/http"

	apperrors "orchids-api/internal/errors"
)

// ResponseBuilder 构建 HTTP 响应
type ResponseBuilder struct {
	w          http.ResponseWriter
	statusCode int
	headers    map[string]string
}

// NewResponseBuilder 创建响应构建器
func NewResponseBuilder(w http.ResponseWriter) *ResponseBuilder {
	return &ResponseBuilder{
		w:          w,
		statusCode: http.StatusOK,
		headers:    make(map[string]string),
	}
}

// Status 设置状态码
func (rb *ResponseBuilder) Status(code int) *ResponseBuilder {
	rb.statusCode = code
	return rb
}

// Header 添加响应头
func (rb *ResponseBuilder) Header(key, value string) *ResponseBuilder {
	rb.headers[key] = value
	return rb
}

// JSON 发送 JSON 响应
func (rb *ResponseBuilder) JSON(data interface{}) error {
	rb.w.Header().Set("Content-Type", "application/json")
	for k, v := range rb.headers {
		rb.w.Header().Set(k, v)
	}
	rb.w.WriteHeader(rb.statusCode)
	return json.NewEncoder(rb.w).Encode(data)
}

// Error 发送错误响应
func (rb *ResponseBuilder) Error(err *apperrors.AppError) {
	err.WriteResponse(rb.w)
}

// SSE 设置 SSE 响应头
func (rb *ResponseBuilder) SSE() *ResponseBuilder {
	rb.headers["Content-Type"] = "text/event-stream"
	rb.headers["Cache-Control"] = "no-cache"
	rb.headers["Connection"] = "keep-alive"
	return rb
}

// WriteSSEHeaders 写入 SSE 响应头
func (rb *ResponseBuilder) WriteSSEHeaders() {
	for k, v := range rb.headers {
		rb.w.Header().Set(k, v)
	}
	rb.w.WriteHeader(rb.statusCode)
	if f, ok := rb.w.(http.Flusher); ok {
		f.Flush()
	}
}

// ClaudeErrorResponse Claude API 错误响应格式
type ClaudeErrorResponse struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// WriteClaudeError 写入 Claude 格式的错误响应
func WriteClaudeError(w http.ResponseWriter, errType string, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(ClaudeErrorResponse{
		Type: "error",
		Error: struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		}{
			Type:    errType,
			Message: message,
		},
	})
}

// WriteClaudeErrorFromAppError 从 AppError 写入 Claude 格式的错误响应
func WriteClaudeErrorFromAppError(w http.ResponseWriter, err *apperrors.AppError) {
	WriteClaudeError(w, err.Code, err.Message, err.HTTPStatus)
}

// ClaudeMessageResponse Claude API 消息响应格式
type ClaudeMessageResponse struct {
	ID           string                   `json:"id"`
	Type         string                   `json:"type"`
	Role         string                   `json:"role"`
	Content      []map[string]interface{} `json:"content"`
	Model        string                   `json:"model"`
	StopReason   string                   `json:"stop_reason"`
	StopSequence interface{}              `json:"stop_sequence"`
	Usage        map[string]int           `json:"usage"`
}

// BuildMessageResponse 构建消息响应
func BuildMessageResponse(msgID, model, stopReason string, content []map[string]interface{}, inputTokens, outputTokens int) ClaudeMessageResponse {
	if stopReason == "" {
		stopReason = "end_turn"
	}
	return ClaudeMessageResponse{
		ID:           msgID,
		Type:         "message",
		Role:         "assistant",
		Content:      content,
		Model:        model,
		StopReason:   stopReason,
		StopSequence: nil,
		Usage: map[string]int{
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
		},
	}
}

// SSEEvent SSE 事件
type SSEEvent struct {
	Event string
	Data  string
}

// BuildMessageStartEvent 构建 message_start 事件
func BuildMessageStartEvent(msgID, model string, inputTokens int) (string, error) {
	data := map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id":      msgID,
			"type":    "message",
			"role":    "assistant",
			"content": []interface{}{},
			"model":   model,
			"usage":   map[string]int{"input_tokens": inputTokens, "output_tokens": 0},
		},
	}
	bytes, err := json.Marshal(data)
	return string(bytes), err
}

// BuildMessageDeltaEvent 构建 message_delta 事件
func BuildMessageDeltaEvent(stopReason string, outputTokens int) (string, error) {
	data := map[string]interface{}{
		"type": "message_delta",
		"delta": map[string]interface{}{
			"stop_reason": stopReason,
		},
		"usage": map[string]interface{}{
			"output_tokens": outputTokens,
		},
	}
	bytes, err := json.Marshal(data)
	return string(bytes), err
}

// BuildMessageStopEvent 构建 message_stop 事件
func BuildMessageStopEvent() (string, error) {
	data := map[string]interface{}{
		"type": "message_stop",
	}
	bytes, err := json.Marshal(data)
	return string(bytes), err
}

// BuildContentBlockStartEvent 构建 content_block_start 事件
func BuildContentBlockStartEvent(index int, blockType string, extra map[string]interface{}) (string, error) {
	contentBlock := map[string]interface{}{
		"type": blockType,
	}
	if blockType == "text" {
		contentBlock["text"] = ""
	} else if blockType == "thinking" {
		contentBlock["thinking"] = ""
	}
	for k, v := range extra {
		contentBlock[k] = v
	}

	data := map[string]interface{}{
		"type":          "content_block_start",
		"index":         index,
		"content_block": contentBlock,
	}
	bytes, err := json.Marshal(data)
	return string(bytes), err
}

// BuildContentBlockDeltaEvent 构建 content_block_delta 事件
func BuildContentBlockDeltaEvent(index int, deltaType string, delta interface{}) (string, error) {
	deltaMap := map[string]interface{}{
		"type": deltaType,
	}

	switch deltaType {
	case "text_delta":
		deltaMap["text"] = delta
	case "thinking_delta":
		deltaMap["thinking"] = delta
	case "input_json_delta":
		deltaMap["partial_json"] = delta
	}

	data := map[string]interface{}{
		"type":  "content_block_delta",
		"index": index,
		"delta": deltaMap,
	}
	bytes, err := json.Marshal(data)
	return string(bytes), err
}

// BuildContentBlockStopEvent 构建 content_block_stop 事件
func BuildContentBlockStopEvent(index int) (string, error) {
	data := map[string]interface{}{
		"type":  "content_block_stop",
		"index": index,
	}
	bytes, err := json.Marshal(data)
	return string(bytes), err
}
