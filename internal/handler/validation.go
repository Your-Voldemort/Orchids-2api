package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"orchids-api/internal/constants"
	apperrors "orchids-api/internal/errors"
	"orchids-api/internal/prompt"
)

// ValidateRequest 验证请求方法和解析请求体
func ValidateRequest(w http.ResponseWriter, r *http.Request, req *ClaudeRequest) *apperrors.AppError {
	if r.Method != http.MethodPost {
		return apperrors.ErrMethodNotAllowed
	}

	r.Body = http.MaxBytesReader(w, r.Body, constants.MaxRequestBodySize)
	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			return apperrors.ErrRequestBodyTooLarge
		}
		return apperrors.ErrInvalidRequest.WithCause(err)
	}

	return nil
}

// ValidateModel 验证模型是否有效
func ValidateModel(model string) *apperrors.AppError {
	model = strings.TrimSpace(model)
	if model == "" {
		return apperrors.ErrInvalidRequest.WithMessage("模型不能为空")
	}
	return nil
}

// ValidateMessages 验证消息列表
func ValidateMessages(messages []prompt.Message) *apperrors.AppError {
	if len(messages) == 0 {
		return apperrors.ErrInvalidRequest.WithMessage("消息列表不能为空")
	}

	// 验证消息格式
	for i, msg := range messages {
		if msg.Role == "" {
			return apperrors.ErrInvalidRequest.WithMessage("消息角色不能为空")
		}
		if msg.Role != "user" && msg.Role != "assistant" && msg.Role != "system" {
			return apperrors.ErrInvalidRequest.WithMessage("无效的消息角色: " + msg.Role)
		}

		// 检查内容
		if msg.Content.IsString() {
			if strings.TrimSpace(msg.Content.GetText()) == "" && i == len(messages)-1 {
				// 最后一条用户消息不能为空
				if msg.Role == "user" {
					return apperrors.ErrInvalidRequest.WithMessage("用户消息内容不能为空")
				}
			}
		}
	}

	return nil
}

// ValidateToolCall 验证工具调用
func ValidateToolCall(call toolCall) *apperrors.AppError {
	if strings.TrimSpace(call.id) == "" {
		return apperrors.ErrInvalidRequest.WithMessage("工具调用 ID 不能为空")
	}
	if strings.TrimSpace(call.name) == "" {
		return apperrors.ErrInvalidRequest.WithMessage("工具名称不能为空")
	}
	return nil
}

// SanitizeInput 清理用户输入
func SanitizeInput(input string) string {
	// 移除潜在的危险字符
	input = strings.TrimSpace(input)
	// 限制长度
	if len(input) > constants.MaxToolInputLength {
		input = input[:constants.MaxToolInputLength]
	}
	return input
}

// IsStreamingSupported 检查是否支持流式响应
func IsStreamingSupported(w http.ResponseWriter) bool {
	_, ok := w.(http.Flusher)
	return ok
}

// ExtractConversationKey 从请求中提取会话标识
func ExtractConversationKey(r *http.Request, req ClaudeRequest) string {
	// 优先使用请求中的 conversation_id
	if req.ConversationID != "" {
		return req.ConversationID
	}

	// 从 metadata 中获取
	if req.Metadata != nil {
		if convID, ok := req.Metadata["conversation_id"].(string); ok && convID != "" {
			return convID
		}
	}

	// 从请求头获取
	if convID := r.Header.Get("X-Conversation-ID"); convID != "" {
		return convID
	}

	// 生成默认 key
	return generateDefaultConversationKey(r)
}

// generateDefaultConversationKey 生成默认的会话 key
func generateDefaultConversationKey(r *http.Request) string {
	// 使用客户端 IP + User-Agent 的组合作为默认 key
	clientIP := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		clientIP = strings.Split(forwarded, ",")[0]
	}
	return strings.TrimSpace(clientIP)
}

// ParseWorkdirFromRequest 从请求中解析工作目录
func ParseWorkdirFromRequest(r *http.Request, system SystemItems) string {
	// 按优先级检查各种来源

	// 1. 请求头
	if workdir := r.Header.Get("X-Orchids-Workdir"); workdir != "" {
		return workdir
	}
	if workdir := r.Header.Get("X-Project-Root"); workdir != "" {
		return workdir
	}
	if workdir := r.Header.Get("X-Working-Dir"); workdir != "" {
		return workdir
	}

	// 2. 系统提示词中的 env 块
	if workdir := extractWorkdirFromSystem(system); workdir != "" {
		return workdir
	}

	return ""
}

// ParseChannelFromPath 从请求路径解析渠道
func ParseChannelFromPath(path string) string {
	return channelFromPath(path)
}
