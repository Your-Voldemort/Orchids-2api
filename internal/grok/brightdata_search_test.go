package grok

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"orchids-api/internal/config"
	"os"
	"strings"
	"testing"
)

type searchTransport func(*http.Request) (*http.Response, error)

func TestBrightDataRuntimeConfig(t *testing.T) {
	enabled := true
	cfg := &config.Config{BrightDataEnabled: &enabled, BrightDataAPIKey: "first", BrightDataZone: "serp_api1"}
	first := BrightDataSearchFromConfig(cfg).(*BrightDataSearch)
	next := cfg.Clone()
	next.BrightDataAPIKey = "second"
	next.BrightDataZone = "serp_api2"
	second := BrightDataSearchFromConfig(next).(*BrightDataSearch)
	if first.key != "first" || second.key != "second" || second.zone != "serp_api2" {
		t.Fatal("search did not use immutable config snapshots")
	}
	disabled := false
	next.BrightDataEnabled = &disabled
	if BrightDataSearchFromConfig(next) != nil {
		t.Fatal("disabled search remains active")
	}
	if err := SeedBrightDataConfig(next); err != nil || *next.BrightDataEnabled {
		t.Fatal("private fallback overrode explicit disable")
	}
}

func (f searchTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestBrightDataSearchContract(t *testing.T) {
	client := &http.Client{Transport: searchTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://api.brightdata.com/request" || r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Fatal("wrong endpoint or authentication")
		}
		var payload map[string]string
		json.NewDecoder(r.Body).Decode(&payload)
		if payload["zone"] != "serp_api1" || payload["format"] != "raw" || !strings.Contains(payload["url"], "q=Go+documentation") {
			t.Fatalf("unexpected payload: %v", payload)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"organic":[{"title":"Go","link":"https://go.dev/doc/","description":"Documentation"},{"title":"bad","link":"javascript:alert(1)"}]}`))}, nil
	})}
	b := &BrightDataSearch{key: "test-secret", zone: "serp_api1", client: client}
	results, err := b.Search(context.Background(), "Go documentation")
	if err != nil || len(results) != 1 || results[0].URL != "https://go.dev/doc/" {
		t.Fatalf("results=%v err=%v", results, err)
	}
	client.Transport = searchTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader("test-secret private error"))}, nil
	})
	_, err = b.Search(context.Background(), "Go documentation")
	if err == nil || strings.Contains(err.Error(), "test-secret") {
		t.Fatalf("unsafe error: %v", err)
	}
}

type fakeBridgeSearch struct {
	calls []string
	fail  bool
}

func (f *fakeBridgeSearch) Search(_ context.Context, q string) ([]SearchResult, error) {
	f.calls = append(f.calls, q)
	if f.fail {
		return nil, io.ErrUnexpectedEOF
	}
	return []SearchResult{{Title: "Go", URL: "https://go.dev/doc/", Snippet: "Go documentation"}}, nil
}

func TestResponsesBrightDataBridge(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "json", true: "sse"}[stream], func(t *testing.T) {
			search := &fakeBridgeSearch{}
			calls := 0
			chat := func(w http.ResponseWriter, r *http.Request) {
				calls++
				var req ChatCompletionsRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Fatal(err)
				}
				if len(req.ResponsesTools) != 0 {
					t.Fatal("hosted tools reached chat upstream")
				}
				if calls == 1 {
					io.WriteString(w, `{"choices":[{"message":{"content":"{\"queries\":[\"Go documentation\"]}"},"finish_reason":"stop"}]}`)
					return
				}
				data, _ := json.Marshal(req.Messages)
				if !strings.Contains(string(data), "https://go.dev/doc/") {
					t.Fatal("search evidence missing")
				}
				if !stream {
					io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"See https://go.dev/doc/"},"finish_reason":"stop"}]}`)
					return
				}
				io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"See https://go.dev/doc/\"}}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			}
			var stored map[string]interface{}
			_ = stored
			body, _ := json.Marshal(map[string]interface{}{"model": "test", "stream": stream, "input": "find Go docs", "tool_choice": map[string]interface{}{"type": "web_search"}, "tools": []interface{}{map[string]interface{}{"type": "web_search"}}})
			rec := httptest.NewRecorder()
			ResponsesBridgeHandler(chat, ResponsesBridgeOptions{Search: search})(rec, httptest.NewRequest("POST", "/workbuddy/v1/responses", strings.NewReader(string(body))))
			if rec.Code != 200 || calls != 2 || len(search.calls) != 1 {
				t.Fatalf("status=%d calls=%d searches=%v body=%s", rec.Code, calls, search.calls, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), `"type":"web_search_call"`) || strings.Contains(rec.Body.String(), "search planner") {
				t.Fatalf("bad response: %s", rec.Body.String())
			}
			if stream && strings.Count(rec.Body.String(), "event: response.output_item.done") != 2 {
				t.Fatalf("duplicate or missing completion: %s", rec.Body.String())
			}
		})
	}
}

func TestResponsesSearchNoneDoesNotCallBackend(t *testing.T) {
	search := &fakeBridgeSearch{}
	chat := func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}]}`)
	}
	rec := httptest.NewRecorder()
	ResponsesBridgeHandler(chat, ResponsesBridgeOptions{Search: search})(rec, httptest.NewRequest("POST", "/cline/v1/responses", strings.NewReader(`{"model":"test","input":"hello","tool_choice":"none","tools":[{"type":"web_search"}]}`)))
	if rec.Code != 200 || len(search.calls) != 0 {
		t.Fatalf("none caused search: %d %v", rec.Code, search.calls)
	}
}

func TestBrightDataLiveSearch(t *testing.T) {
	if os.Getenv("BRIGHTDATA_LIVE_TEST") != "1" {
		t.Skip("explicit opt-in required")
	}
	backend, err := LoadBrightDataSearch()
	if err != nil || backend == nil {
		t.Fatal("missing private Bright Data configuration")
	}
	results, err := backend.Search(context.Background(), "Go programming language documentation")
	if err != nil || len(results) == 0 {
		t.Fatalf("live search failed: %v", err)
	}
	t.Logf("Bright Data returned %d valid search results", len(results))
}
