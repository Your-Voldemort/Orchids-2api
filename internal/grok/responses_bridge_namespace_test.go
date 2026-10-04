package grok

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"encoding/json"

	"orchids-api/internal/testutil"
)

func TestResponsesBridgeSpecialToolsRoundTrip(t *testing.T) {
	for _, kind := range []string{"custom", "apply_patch"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%v", kind, stream), func(t *testing.T) {
				name, arguments := "patch", `{"input":"*** Begin Patch\n*** End Patch"}`
				tool := map[string]interface{}{"type": kind, "name": name}
				if kind == "apply_patch" {
					name = "apply_patch"
					arguments = `{"operation":{"type":"delete_file","path":"old.txt"}}`
				}
				chat := func(w http.ResponseWriter, r *http.Request) {
					var body ChatCompletionsRequest
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					if len(body.Tools) != 1 || body.Tools[0].Function["name"] != name {
						t.Fatalf("tool not lowered: %#v", body.Tools)
					}
					call := map[string]interface{}{"index": 0, "id": "call_special", "type": "function", "function": map[string]interface{}{"name": name, "arguments": arguments}}
					if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						chunk, _ := json.Marshal(map[string]interface{}{"choices": []interface{}{map[string]interface{}{"index": 0, "delta": map[string]interface{}{"tool_calls": []interface{}{call}}}}})
						fmt.Fprintf(w, "data: %s\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n", chunk)
					} else {
						json.NewEncoder(w).Encode(map[string]interface{}{"choices": []interface{}{map[string]interface{}{"message": map[string]interface{}{"tool_calls": []interface{}{call}}, "finish_reason": "tool_calls"}}})
					}
				}
				body, _ := json.Marshal(map[string]interface{}{"model": "test", "input": "patch", "stream": stream, "tools": []interface{}{tool}})
				rec := httptest.NewRecorder()
				ResponsesBridgeHandler(chat, ResponsesBridgeOptions{})(rec, httptest.NewRequest("POST", "/workbuddy/v1/responses", strings.NewReader(string(body))))
				if rec.Code != 200 {
					t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
				}
				wire := rec.Body.String()
				if !strings.Contains(wire, `"type":"`+kind+`_tool_call"`) && kind == "custom" {
					t.Fatal(wire)
				}
				if kind == "apply_patch" && (!strings.Contains(wire, `"type":"apply_patch_call"`) || !strings.Contains(wire, `"operation":`)) {
					t.Fatal(wire)
				}
				if stream && (strings.Contains(wire, "response.function_call_arguments.") || !strings.Contains(wire, "response.completed")) {
					t.Fatal(wire)
				}
			})
		}
	}
}

func TestResponsesBridgeSpecialToolHistory(t *testing.T) {
	req := ResponsesCreateRequest{Model: "test", Tools: []map[string]interface{}{{"type": "apply_patch"}}, Input: []interface{}{
		map[string]interface{}{"type": "apply_patch_call", "call_id": "call_patch", "operation": map[string]interface{}{"type": "delete_file", "path": "old.txt"}},
		map[string]interface{}{"type": "apply_patch_call_output", "call_id": "call_patch", "output": "deleted"},
	}}
	if _, err := normalizeBridgedTools(&req); err != nil {
		t.Fatal(err)
	}
	chat, err := chatRequestFromResponses(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(chat.Messages) != 2 || chat.Messages[0].ToolCalls[0].ID != "call_patch" || chat.Messages[1].ToolCallID != "call_patch" {
		t.Fatalf("history=%#v", chat.Messages)
	}
}

// codexNamespaceTools is the tool list Codex sends: seven flat functions
// followed by a `namespace` grouping. The eighth entry is the one the chat
// bridge used to reject with `tools[7]: tool type "namespace" requires a native
// Responses provider`, which made every non-Grok channel unusable from Codex.
func codexNamespaceTools() string {
	return `[
		{"type":"function","name":"update_plan","description":"Update the plan","parameters":{"type":"object","properties":{"plan":{"type":"array"}},"required":["plan"],"additionalProperties":false}},
		{"type":"function","name":"apply_patch","description":"Apply a patch","parameters":{"type":"object","properties":{"input":{"type":"string"}},"required":["input"],"additionalProperties":false}},
		{"type":"function","name":"view_image","description":"View an image","parameters":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}},
		{"type":"function","name":"read_file","description":"Read a file","parameters":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}},
		{"type":"function","name":"list_dir","description":"List a directory","parameters":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}},
		{"type":"function","name":"grep_files","description":"Search files","parameters":{"type":"object","properties":{"pattern":{"type":"string"}},"required":["pattern"],"additionalProperties":false}},
		{"type":"function","name":"run_command","description":"Run a command","parameters":{"type":"object","properties":{"cmd":{"type":"string"}},"required":["cmd"],"additionalProperties":false}},
		{"type":"namespace","name":"functions","tools":[
			{"type":"function","name":"custom_tool","description":"A namespaced tool","parameters":{"type":"object","properties":{"value":{"type":"string"}},"required":["value"],"additionalProperties":false}}
		]}
	]`
}

// A Codex request carries a grouped tool, so the whole request used to be
// rejected before it reached the channel. Every chat-only channel must serve it,
// and the chat layer must receive one flat function name per grouped function.
func TestResponsesBridgeFlattensNamespacedToolsForCodex(t *testing.T) {
	for _, channel := range []string{"workbuddy", "qoder", "cline"} {
		t.Run(channel, func(t *testing.T) {
			var mu sync.Mutex
			calls := []recordedChatCall{}
			bridge := ResponsesBridgeHandler(recordingChat(t, &calls, &mu), ResponsesBridgeOptions{})

			body := `{"model":"gpt-5.6-luna","input":"do the task","tools":` + codexNamespaceTools() + `}`
			rec := httptest.NewRecorder()
			bridge(rec, httptest.NewRequest(http.MethodPost, "/"+channel+"/v1/responses", strings.NewReader(body)))

			if rec.Code != http.StatusOK {
				t.Fatalf("%s: status=%d body=%s", channel, rec.Code, rec.Body.String())
			}
			mu.Lock()
			defer mu.Unlock()
			testutil.Equal(t, len(calls), 1)
			tools, _ := calls[0].body["tools"].([]interface{})
			var names []string
			for _, raw := range tools {
				tool, _ := raw.(map[string]interface{})
				// The chat layer declares a tool as {"type":"function",
				// "function":{...}}, so the name lives inside `function`.
				function, _ := tool["function"].(map[string]interface{})
				names = append(names, parseLooseStringAny(function["name"]))
			}
			testutil.Falsef(t, len(names) != 8, "%s: upstream tools = %v, want one per declared function", channel, names)
			testutil.MustContain(t, strings.Join(names, ","), "functions__custom_tool")
			for _, tool := range tools {
				testutil.Equal(t, parseLooseStringAny(tool.(map[string]interface{})["type"]), "function")
			}
		})
	}
}

// The name the chat layer uses is internal: the response must answer with the
// short name and the namespace the caller declared, or a client that looks its
// own tool up by name cannot match the call to the declaration it sent.
func TestResponsesBridgeRestoresNamespaceOnFunctionCalls(t *testing.T) {
	chat := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"id":"chatcmpl-ns","object":"chat.completion","model":"gpt-5.6-luna","choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"functions__custom_tool","arguments":"{\"value\":\"done\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}
	bridge := ResponsesBridgeHandler(chat, ResponsesBridgeOptions{})

	body := `{"model":"gpt-5.6-luna","input":"do the task","tools":` + codexNamespaceTools() + `}`
	rec := httptest.NewRecorder()
	bridge(rec, httptest.NewRequest(http.MethodPost, "/cline/v1/responses", strings.NewReader(body)))
	testutil.Equal(t, rec.Code, http.StatusOK)

	var decoded map[string]interface{}
	testutil.CheckNoError(t, json.Unmarshal(rec.Body.Bytes(), &decoded))
	output, _ := decoded["output"].([]interface{})
	call, _ := output[0].(map[string]interface{})
	testutil.Equal(t, call["type"], "function_call")
	testutil.Equal(t, call["name"], "custom_tool")
	testutil.Equal(t, call["namespace"], "functions")
	testutil.Equal(t, call["arguments"], `{"value":"done"}`)
}

// The same restore has to hold on the streamed path: Codex streams, and an
// event that reports the flat name leaves the client with a call it cannot map
// back to the tool it declared.
func TestResponsesBridgeRestoresNamespaceOnStreamedFunctionCalls(t *testing.T) {
	chat := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for _, frame := range []string{
			`data: {"id":"chatcmpl-ns","choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
			`data: {"id":"chatcmpl-ns","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"functions__custom_tool","arguments":""}}]}}]}`,
			`data: {"id":"chatcmpl-ns","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"value\":\"done\"}"}}]}}]}`,
			`data: {"id":"chatcmpl-ns","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		} {
			_, _ = io.WriteString(w, frame+"\n\n")
		}
	}
	bridge := ResponsesBridgeHandler(chat, ResponsesBridgeOptions{})

	body := `{"model":"gpt-5.6-luna","input":"do the task","stream":true,"tools":` + codexNamespaceTools() + `}`
	rec := httptest.NewRecorder()
	bridge(rec, httptest.NewRequest(http.MethodPost, "/qoder/v1/responses", strings.NewReader(body)))
	testutil.Equal(t, rec.Code, http.StatusOK)

	wire := rec.Body.String()
	testutil.MustContain(t, wire, `"namespace":"functions"`)
	testutil.MustContain(t, wire, `"name":"custom_tool"`)
	testutil.Falsef(t, strings.Contains(wire, "functions__custom_tool"),
		"the flat name leaked to the client: %s", wire)
}

// A tool the chat layer genuinely cannot serve is still reported, with the same
// message as before: silently dropping it would answer a caller who asked for a
// code interpreter with a model that has none.
func TestResponsesBridgeStillRejectsUnservableToolTypes(t *testing.T) {
	bridge := ResponsesBridgeHandler(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("the inner chat handler must not run")
	}, ResponsesBridgeOptions{})

	for name, tool := range map[string]string{
		"mcp":              `{"type":"mcp","server_label":"docs"}`,
		"code_interpreter": `{"type":"code_interpreter","container":{"type":"auto"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			body := `{"model":"gpt-5.6-luna","input":"hi","tools":[` + tool + `]}`
			rec := httptest.NewRecorder()
			bridge(rec, httptest.NewRequest(http.MethodPost, "/workbuddy/v1/responses", strings.NewReader(body)))
			testutil.Equal(t, rec.Code, http.StatusBadRequest)
			testutil.MustContain(t, rec.Body.String(), "requires a native Responses provider")
		})
	}
}

// A client that echoes a grouped call back expects it to reach the upstream
// under the same flat name the declaration was given, or the upstream rejects a
// call to a tool it never saw.
func TestResponsesBridgeRenamesEchoedNamespacedCalls(t *testing.T) {
	var mu sync.Mutex
	calls := []recordedChatCall{}
	bridge := ResponsesBridgeHandler(recordingChat(t, &calls, &mu), ResponsesBridgeOptions{})

	body := `{"model":"gpt-5.6-luna","input":[
		{"type":"message","role":"user","content":"do the task"},
		{"type":"function_call","call_id":"call_1","name":"custom_tool","namespace":"functions","arguments":"{}"},
		{"type":"function_call_output","call_id":"call_1","output":"ok"}
	],"tools":` + codexNamespaceTools() + `}`
	rec := httptest.NewRecorder()
	bridge(rec, httptest.NewRequest(http.MethodPost, "/workbuddy/v1/responses", strings.NewReader(body)))
	testutil.Equal(t, rec.Code, http.StatusOK)

	mu.Lock()
	defer mu.Unlock()
	testutil.Equal(t, len(calls), 1)
	testutil.MustContain(t, fmt.Sprint(calls[0].body["messages"]), "functions__custom_tool")
	testutil.Falsef(t, strings.Contains(fmt.Sprint(calls[0].body["messages"]), `"name":"custom_tool"`),
		"the echoed call kept its short name: %v", calls[0].body["messages"])
}
