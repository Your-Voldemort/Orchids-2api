package qoder

import (
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
)

// TestEncodeBodyMatchesPrivateAlphabet pins the wire encoding. The signature is
// computed over these bytes, so an alphabet or swap change produces a request
// the gateway rejects as unauthenticated even though every header is present.
func TestEncodeBodyMatchesPrivateAlphabet(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"a":1,"b":"hello"}`)

	// Reference implementation: standard base64, then a positional alphabet
	// substitution, then the outer-third swap.
	standard := base64.StdEncoding.EncodeToString(raw)
	var substituted strings.Builder
	for i := 0; i < len(standard); i++ {
		c := standard[i]
		if c == '=' {
			substituted.WriteByte('$')
			continue
		}
		index := strings.IndexByte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/", c)
		if index < 0 {
			t.Fatalf("standard base64 produced %q, which is outside the standard alphabet", c)
		}
		substituted.WriteByte(bodyAlphabet[index])
	}
	want := swapOuterThirds([]byte(substituted.String()))

	got := EncodeBody(raw)
	if string(got) != string(want) {
		t.Fatalf("EncodeBody() = %q, want %q", got, want)
	}
	for _, b := range got {
		if strings.ContainsRune("+/=", rune(b)) {
			t.Fatalf("EncodeBody() left a standard-alphabet byte %q in %q", b, got)
		}
	}
}

// TestBodyCodecRoundTrip covers the inverse, including payload lengths that make
// the outer thirds unequal in size.
func TestBodyCodecRoundTrip(t *testing.T) {
	t.Parallel()

	for length := 0; length < 200; length++ {
		raw := make([]byte, length)
		for i := range raw {
			raw[i] = byte(i * 7 % 251)
		}
		encoded := EncodeBody(raw)
		for name, decoder := range map[string]func([]byte) ([]byte, error){
			"production":               decodeBody,
			"independent test decoder": decodeBodyForTest,
		} {
			decoded, err := decoder(encoded)
			if err != nil {
				t.Fatalf("%s DecodeBody(len=%d) error = %v", name, length, err)
			}
			if string(decoded) != string(raw) {
				t.Fatalf("%s round trip mismatch at length %d", name, length)
			}
		}
	}
}

// TestDecodeBodyRejectsLineBreaks pins the strict framing: a wrapped body is a
// different byte sequence and so a different signature.
func TestDecodeBodyRejectsLineBreaks(t *testing.T) {
	t.Parallel()

	encoded := EncodeBody([]byte(`{"a":1}`))
	for _, newline := range []string{"\n", "\r", "\r\n"} {
		for _, offset := range []int{0, 4, len(encoded)} {
			malformed := append([]byte(nil), encoded[:offset]...)
			malformed = append(malformed, newline...)
			malformed = append(malformed, encoded[offset:]...)
			if _, err := decodeBody(malformed); err == nil {
				t.Fatalf("production decodeBody accepted %q at offset %d", newline, offset)
			}
		}
	}
	for _, malformed := range [][]byte{[]byte("+"), []byte("invalid-length"), []byte("$$$$")} {
		if _, err := decodeBody(malformed); err == nil {
			t.Fatalf("production decodeBody accepted malformed encoding %q", malformed)
		}
	}
}

// TestCOSYSignatureSeparators pins the five-field, four-newline MD5 formula with
// no trailing separator. The trailing byte is the classic mistake here: a
// trailing newline yields a signature that is 32 valid-looking hex characters
// the gateway rejects.
func TestCOSYSignatureSeparators(t *testing.T) {
	t.Parallel()

	const (
		payload = "eyJ2ZXJzaW9uIjoidjEifQ=="
		key     = "AAAABBBBCCCCDDDD"
		seconds = "1700000000"
		body    = "encoded-body"
		path    = "/api/v2/service/pro/sse/agent_chat_generation"
	)
	want := md5.Sum([]byte(payload + "\n" + key + "\n" + seconds + "\n" + body + "\n" + path))
	got := cosySignature([]byte(payload), key, seconds, []byte(body), path)
	if got != want {
		t.Fatalf("cosySignature() = %x, want %x", got, want)
	}
	if fmt.Sprintf("%x", got) != "fb256c9d505e6c4a4ae4c1525d583c9e" {
		t.Fatalf("signature differs from fixed vector: %x", got)
	}
	if got == md5.Sum([]byte(payload+"\n"+key+"\n"+seconds+"\n"+body+"\n"+path+"\n")) {
		t.Fatal("signature includes a trailing separator")
	}
}

// TestCOSYAuthorizationFixedVector pins the prefix, payload field order and
// empty ideVersion, standard base64, and hexadecimal signature together.
func TestCOSYAuthorizationFixedVector(t *testing.T) {
	t.Parallel()

	const want = "Bearer COSY.eyJ2ZXJzaW9uIjoidjEiLCJyZXF1ZXN0SWQiOiJyZXF1ZXN0IiwiaW5mbyI6ImluZm8iLCJjb3N5VmVyc2lvbiI6InZlcnNpb24iLCJpZGVWZXJzaW9uIjoiIn0=.759d3ccbc63c510f16c3e66bd4c0b230"
	got, err := buildCOSYAuthorization("request", "info", "version", "key", "123", []byte("encoded-body"), "/path")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("buildCOSYAuthorization() = %q, want %q", got, want)
	}
}

// TestSignPathStripsAlgoAndQuery pins the signed path: the covered path drops
// the /algo gateway prefix and never includes the query string.
func TestSignPathStripsAlgoAndQuery(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"https://api2.qoder.sh/algo/api/v2/service/pro/sse/agent_chat_generation?FetchKeys=llm_model_result&AgentId=agent_common&Encode=1": "/api/v2/service/pro/sse/agent_chat_generation",
		"https://gateway.qoder.com.cn/algo/api/v2/quota/usage?Encode=1":                                                                    "/api/v2/quota/usage",
		"/algo/api/v2/quota/usage?Encode=1": "/api/v2/quota/usage",
	}
	for raw, want := range cases {
		if got := signPath(raw); got != want {
			t.Errorf("signPath(%q) = %q, want %q", raw, got, want)
		}
	}
}

// TestChatURLCarriesFixedAgentQuery pins the query the gateway reads instead of
// the body's agent_id.
func TestChatURLCarriesFixedAgentQuery(t *testing.T) {
	t.Parallel()

	got := chatURL("https://api2.qoder.sh")
	want := "https://api2.qoder.sh/algo/api/v2/service/pro/sse/agent_chat_generation?FetchKeys=llm_model_result&AgentId=agent_common&Encode=1"
	if got != want {
		t.Fatalf("chatURL() = %q, want %q", got, want)
	}
}
