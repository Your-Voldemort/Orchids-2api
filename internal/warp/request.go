package warp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

type encoder struct {
	b []byte
}

func (e *encoder) bytes() []byte {
	return e.b
}

func (e *encoder) writeVarint(x uint64) {
	for x >= 0x80 {
		e.b = append(e.b, byte(x)|0x80)
		x >>= 7
	}
	e.b = append(e.b, byte(x))
}

func (e *encoder) writeKey(field int, wire int) {
	e.writeVarint(uint64(field<<3 | wire))
}

func (e *encoder) writeString(field int, value string) {
	if value == "" {
		return
	}
	e.writeKey(field, 2)
	e.writeVarint(uint64(len(value)))
	e.b = append(e.b, value...)
}

func (e *encoder) writeBytes(field int, value []byte) {
	if len(value) == 0 {
		return
	}
	e.writeKey(field, 2)
	e.writeVarint(uint64(len(value)))
	e.b = append(e.b, value...)
}

func (e *encoder) writeBool(field int, value bool) {
	if !value {
		return
	}
	e.writeKey(field, 0)
	if value {
		e.writeVarint(1)
	} else {
		e.writeVarint(0)
	}
}

func (e *encoder) writeMessage(field int, msg []byte) {
	if len(msg) == 0 {
		return
	}
	e.writeKey(field, 2)
	e.writeVarint(uint64(len(msg)))
	e.b = append(e.b, msg...)
}

func (e *encoder) writePackedVarints(field int, values []int) {
	if len(values) == 0 {
		return
	}
	inner := encoder{}
	for _, v := range values {
		inner.writeVarint(uint64(v))
	}
	e.writeKey(field, 2)
	e.writeVarint(uint64(len(inner.b)))
	e.b = append(e.b, inner.b...)
}

func buildRequestBytes(prompt, model string, tools []interface{}, disableWarpTools bool, hasHistory bool) ([]byte, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil, fmt.Errorf("empty prompt")
	}
	if disableWarpTools {
		prompt = noWarpToolsPrompt + "\n\n" + prompt
	}

	inputContext := buildInputContext()
	userQuery := buildUserQuery(prompt, !hasHistory)
	input := buildInput(inputContext, userQuery)
	settings := buildSettings(model, disableWarpTools)
	mcpContext, err := buildMCPContext(tools)
	if err != nil {
		return nil, err
	}

	req := encoder{}
	// task_context: empty message
	req.writeMessage(1, []byte{})
	req.writeMessage(2, input)
	req.writeMessage(3, settings)
	if len(mcpContext) > 0 {
		req.writeMessage(6, mcpContext)
	}

	return req.bytes(), nil
}

func buildInputContext() []byte {
	pwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	shellName := filepath.Base(os.Getenv("SHELL"))
	if shellName == "" {
		shellName = "zsh"
	}

	dir := encoder{}
	dir.writeString(1, pwd)
	dir.writeString(2, home)

	osCtx := encoder{}
	osCtx.writeString(1, osName)
	osCtx.writeString(2, "")

	shell := encoder{}
	shell.writeString(1, shellName)
	shell.writeString(2, "")

	ts := encoder{}
	now := time.Now()
	ts.writeKey(1, 0)
	ts.writeVarint(uint64(now.Unix()))
	ts.writeKey(2, 0)
	ts.writeVarint(uint64(now.Nanosecond()))

	ctx := encoder{}
	ctx.writeMessage(1, dir.bytes())
	ctx.writeMessage(2, osCtx.bytes())
	ctx.writeMessage(3, shell.bytes())
	ctx.writeMessage(4, ts.bytes())

	return ctx.bytes()
}

func buildUserQuery(prompt string, isNew bool) []byte {
	msg := encoder{}
	msg.writeString(1, prompt)
	msg.writeBool(4, isNew)
	return msg.bytes()
}

func buildInput(contextBytes, userQueryBytes []byte) []byte {
	input := encoder{}
	input.writeMessage(1, contextBytes)
	input.writeMessage(2, userQueryBytes)
	return input.bytes()
}

func buildSettings(model string, disableWarpTools bool) []byte {
	modelName := normalizeModel(model)
	modelCfg := encoder{}
	modelCfg.writeString(1, modelName)
	modelCfg.writeString(2, "o3")
	modelCfg.writeString(4, "auto")

	settings := encoder{}
	settings.writeMessage(1, modelCfg.bytes())
	settings.writeBool(2, true)
	settings.writeBool(3, true)
	settings.writeBool(4, true)
	settings.writeBool(6, true)
	settings.writeBool(7, true)
	settings.writeBool(8, true)
	settings.writeBool(10, true)
	settings.writeBool(11, true)
	settings.writeBool(12, true)
	settings.writeBool(13, true)
	settings.writeBool(14, true)
	settings.writeBool(15, true)
	settings.writeBool(16, true)
	settings.writeBool(17, true)
	settings.writeBool(21, true)
	settings.writeBool(23, true)

	if !disableWarpTools {
		settings.writePackedVarints(9, []int{6, 7, 12, 8, 9, 15, 14, 0, 11, 16, 10, 20, 17, 19, 18, 2, 3, 1, 13})
	}
	settings.writePackedVarints(22, []int{10, 20, 6, 7, 12, 9, 2, 1})

	return settings.bytes()
}

func buildMCPContext(tools []interface{}) ([]byte, error) {
	converted := convertTools(tools)
	if len(converted) == 0 {
		return nil, nil
	}
	ctx := encoder{}
	for _, tool := range converted {
		toolMsg := encoder{}
		toolMsg.writeString(1, tool.Name)
		toolMsg.writeString(2, tool.Description)
		if len(tool.Schema) > 0 {
			st, err := structpb.NewStruct(tool.Schema)
			if err != nil {
				return nil, err
			}
			encoded, err := proto.Marshal(st)
			if err != nil {
				return nil, err
			}
			toolMsg.writeMessage(3, encoded)
		}
		ctx.writeMessage(2, toolMsg.bytes())
	}

	return ctx.bytes(), nil
}

type toolDef struct {
	Name        string
	Description string
	Schema      map[string]interface{}
}

func convertTools(tools []interface{}) []toolDef {
	if len(tools) == 0 {
		return nil
	}
	defs := make([]toolDef, 0, len(tools))
	for _, raw := range tools {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if typ, _ := m["type"].(string); typ == "function" {
			if fn, ok := m["function"].(map[string]interface{}); ok {
				name, _ := fn["name"].(string)
				description, _ := fn["description"].(string)
				schema := schemaMap(fn["parameters"])
				if name != "" {
					defs = append(defs, toolDef{Name: name, Description: description, Schema: schema})
				}
				continue
			}
		}
		name, _ := m["name"].(string)
		description, _ := m["description"].(string)
		schema := schemaMap(m["input_schema"])
		if schema == nil {
			schema = schemaMap(m["parameters"])
		}
		if name != "" {
			defs = append(defs, toolDef{Name: name, Description: description, Schema: schema})
		}
	}
	return defs
}

func schemaMap(v interface{}) map[string]interface{} {
	if v == nil {
		return nil
	}
	if m, ok := v.(map[string]interface{}); ok {
		return m
	}
	return nil
}

func normalizeModel(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return defaultModel
	}

	known := map[string]struct{}{
		"auto":              {},
		"auto-efficient":    {},
		"auto-genius":       {},
		"warp-basic":        {},
		"gpt-5":             {},
		"gpt-4o":            {},
		"gpt-4.1":           {},
		"o3":                {},
		"o4-mini":           {},
		"gemini-2.5-pro":    {},
		"claude-4-sonnet":   {},
		"claude-4-opus":     {},
		"claude-4.1-opus":   {},
		"claude-4-5-sonnet": {},
		"claude-4-5-opus":   {},
	}
	if _, ok := known[model]; ok {
		return model
	}

	if strings.Contains(model, "sonnet-4-5") || strings.Contains(model, "sonnet 4.5") {
		return "claude-4-5-sonnet"
	}
	if strings.Contains(model, "opus-4-5") || strings.Contains(model, "opus 4.5") {
		return "claude-4-5-opus"
	}
	if strings.Contains(model, "sonnet-4") {
		return "claude-4-sonnet"
	}
	if strings.Contains(model, "opus-4") || strings.Contains(model, "opus 4") {
		return "claude-4-opus"
	}

	return defaultModel
}
