package qoder

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

// traceparentRE is the W3C Trace Context shape for version 00: the version, a
// 32-character lowercase hex trace id, a 16-character hex parent span id and
// the sampled flag.
var traceparentRE = regexp.MustCompile(`^00-[0-9a-f]{32}-[0-9a-f]{16}-01$`)

func validTraceparent(value string) bool {
	return traceparentRE.MatchString(strings.TrimSpace(value))
}

// TestTraceparentIsAWellFormedTraceContext pins the header the reference client
// sends on every API call. The gateway adopts the trace id and answers with it
// as sw-trace-id, so the value has to be a real trace context rather than an
// opaque string, and a request id is a UUID whose dashes must not leak into it.
func TestTraceparentIsAWellFormedTraceContext(t *testing.T) {
	t.Parallel()

	const uuid = "6ffd0df8-6611-4e20-804d-4c535da80521"
	got := buildTraceparent(uuid)
	want := "00-6ffd0df866114e20804d4c535da80521-804d4c535da80521-01"
	if got != want {
		t.Fatalf("buildTraceparent(%q) = %q, want %q", uuid, got, want)
	}
	if !validTraceparent(got) {
		t.Fatalf("traceparent %q is not a version 00 trace context", got)
	}
	if len(got) != 55 {
		t.Fatalf("traceparent length = %d, want 55", len(got))
	}

	// The same request must render the same header, so a trace can be
	// reconstructed from the request id alone.
	if again := buildTraceparent(uuid); again != got {
		t.Fatalf("traceparent is not deterministic: %q then %q", got, again)
	}

	// An uppercase id still has to render lowercase-lowercase hex.
	if upper := buildTraceparent(strings.ToUpper(uuid)); upper != got {
		t.Fatalf("uppercase request id rendered %q, want %q", upper, got)
	}

	// Short, empty or non-hex ids must still produce a well-formed header.
	for _, input := range []string{"", "abc", "not-a-uuid", "zzzz", "0"} {
		if out := buildTraceparent(input); !validTraceparent(out) {
			t.Errorf("buildTraceparent(%q) = %q, want a valid trace context", input, out)
		}
	}
}

// TestCatalogFetchCarriesTheClientHeadersToo pins the same headers on the
// catalog read. The capture carries accept-language, sec-fetch-mode and a trace
// context on the model-list GET as well as on inference, so the contract is a
// property of the client rather than of the chat path.
func TestCatalogFetchCarriesTheClientHeadersToo(t *testing.T) {
	t.Parallel()

	headersCh := make(chan http.Header, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headersCh <- r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(observedCatalogResponse))
	}))
	defer server.Close()

	acc := signedTestAccount()
	client := NewFromAccount(acc, nil)
	setTestEndpoints(client, server.URL, server.URL, server.URL)

	if _, err := client.FetchUpstreamModels(context.Background()); err != nil {
		t.Fatalf("FetchUpstreamModels() error = %v", err)
	}

	select {
	case headers := <-headersCh:
		for name, want := range map[string]string{
			"Accept-Language": "*",
			"Sec-Fetch-Mode":  "cors",
		} {
			if got := headers.Get(name); got != want {
				t.Errorf("catalog header %s = %q, want %q", name, got, want)
			}
		}
		if trace := headers.Get("Traceparent"); !validTraceparent(trace) {
			t.Errorf("catalog Traceparent = %q, want a version 00 trace context", trace)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the stub server received no catalog request")
	}
}
