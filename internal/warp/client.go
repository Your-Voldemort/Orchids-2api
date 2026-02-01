package warp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"orchids-api/internal/client"
	"orchids-api/internal/config"
	"orchids-api/internal/debug"
	"orchids-api/internal/store"
)

type Client struct {
	config     *config.Config
	account    *store.Account
	httpClient *http.Client
	session    *session
}

func NewFromAccount(acc *store.Account, cfg *config.Config) *Client {
	refresh := ""
	if acc != nil {
		refresh = strings.TrimSpace(acc.ClientCookie)
	}
	sess := getSession(acc.ID, refresh)
	return &Client{
		config:     cfg,
		account:    acc,
		httpClient: newHTTPClient(),
		session:    sess,
	}
}

func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 120 * time.Second,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			MaxIdleConns:          50,
			MaxIdleConnsPerHost:   50,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
		},
	}
}

func (c *Client) SendRequest(ctx context.Context, prompt string, chatHistory []interface{}, model string, onMessage func(client.SSEMessage), logger *debug.Logger) error {
	req := client.UpstreamRequest{
		Prompt:      prompt,
		ChatHistory: chatHistory,
		Model:       model,
	}
	return c.SendRequestWithPayload(ctx, req, onMessage, logger)
}

func (c *Client) SendRequestWithPayload(ctx context.Context, req client.UpstreamRequest, onMessage func(client.SSEMessage), logger *debug.Logger) error {
	if c.session == nil {
		return fmt.Errorf("warp session not initialized")
	}
	if err := c.session.ensureToken(ctx, c.httpClient); err != nil {
		return err
	}
	if err := c.session.ensureLogin(ctx, c.httpClient); err != nil {
		return err
	}

	prompt := req.Prompt
	hasHistory := len(req.Messages) > 1
	disableWarpTools := true
	if c.config != nil && c.config.WarpDisableTools != nil {
		disableWarpTools = *c.config.WarpDisableTools
	}
	if req.NoTools {
		disableWarpTools = true
	}

	tools := req.Tools
	if req.NoTools {
		tools = nil
	}

	payload, err := buildRequestBytes(prompt, req.Model, tools, disableWarpTools, hasHistory)
	if err != nil {
		return err
	}

	jwt := c.session.currentJWT()
	if jwt == "" {
		return fmt.Errorf("warp jwt missing")
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, aiURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("x-warp-client-id", clientID)
	request.Header.Set("accept", "text/event-stream")
	request.Header.Set("content-type", "application/x-protobuf")
	request.Header.Set("x-warp-client-version", clientVersion)
	request.Header.Set("x-warp-os-category", osCategory)
	request.Header.Set("x-warp-os-name", osName)
	request.Header.Set("x-warp-os-version", osVersion)
	request.Header.Set("authorization", "Bearer "+jwt)
	request.Header.Set("accept-encoding", "identity")
	request.Header.Set("content-length", fmt.Sprintf("%d", len(payload)))

	if logger != nil {
		logger.LogUpstreamRequest(aiURL, map[string]string{
			"Accept":                "text/event-stream",
			"Authorization":         "Bearer [REDACTED]",
			"Content-Type":          "application/x-protobuf",
			"X-Warp-Client-Version": clientVersion,
		}, map[string]interface{}{"payload_bytes": len(payload)})
	}

	resp, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("warp api error: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	reader := bufio.NewReader(resp.Body)
	var dataLines []string
	toolCallSeen := false
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if len(dataLines) == 0 {
				continue
			}
			data := strings.Join(dataLines, "")
			dataLines = nil
			payloadBytes, err := decodeWarpPayload(data)
			if err != nil {
				if logger != nil {
					logger.LogUpstreamSSE("warp_decode_error", err.Error())
				}
				continue
			}
			parsed, err := parseResponseEvent(payloadBytes)
			if err != nil {
				if logger != nil {
					logger.LogUpstreamSSE("warp_parse_error", err.Error())
				}
				continue
			}
			for _, delta := range parsed.TextDeltas {
				onMessage(client.SSEMessage{Type: "model.text-delta", Event: map[string]interface{}{"delta": delta}})
			}
			for _, delta := range parsed.ReasoningDeltas {
				onMessage(client.SSEMessage{Type: "model.reasoning-delta", Event: map[string]interface{}{"delta": delta}})
			}
			for _, call := range parsed.ToolCalls {
				toolCallSeen = true
				onMessage(client.SSEMessage{Type: "model.tool-call", Event: map[string]interface{}{"toolCallId": call.ID, "toolName": call.Name, "input": call.Input}})
			}
			if parsed.Finish != nil {
				finish := map[string]interface{}{
					"finishReason": "end_turn",
				}
				if toolCallSeen {
					finish["finishReason"] = "tool_use"
				}
				if parsed.Finish.InputTokens > 0 || parsed.Finish.OutputTokens > 0 {
					finish["usage"] = map[string]interface{}{
						"inputTokens":  parsed.Finish.InputTokens,
						"outputTokens": parsed.Finish.OutputTokens,
					}
				}
				onMessage(client.SSEMessage{Type: "model.finish", Event: finish})
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(line[5:]))
			continue
		}
		// ignore event: or other lines
	}

	// Send finish if stream ended without explicit finish event
	if !toolCallSeen {
		onMessage(client.SSEMessage{Type: "model.finish", Event: map[string]interface{}{"finishReason": "end_turn"}})
	} else {
		onMessage(client.SSEMessage{Type: "model.finish", Event: map[string]interface{}{"finishReason": "tool_use"}})
	}

	return nil
}

func decodeWarpPayload(data string) ([]byte, error) {
	if data == "" {
		return nil, fmt.Errorf("empty payload")
	}
	if decoded, err := base64.RawURLEncoding.DecodeString(data); err == nil {
		return decoded, nil
	}
	if decoded, err := base64.URLEncoding.DecodeString(data); err == nil {
		return decoded, nil
	}
	return base64.StdEncoding.DecodeString(data)
}

func (c *Client) RefreshAccount(ctx context.Context) (string, error) {
	if c.session == nil {
		return "", fmt.Errorf("warp session not initialized")
	}
	if err := c.session.refreshTokenRequest(ctx, c.httpClient); err != nil {
		return "", err
	}
	if c.account != nil {
		newRefresh := c.session.currentRefreshToken()
		if newRefresh != "" {
			c.account.ClientCookie = newRefresh
		}
	}
	jwt := c.session.currentJWT()
	if jwt == "" {
		return "", fmt.Errorf("warp jwt missing")
	}
	return jwt, nil
}

func (c *Client) LogSessionState() {
	if c.session == nil {
		return
	}
	jwt := c.session.currentJWT()
	if jwt == "" {
		return
	}
	slog.Debug("warp session ready")
}
