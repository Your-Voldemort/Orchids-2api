package grok

import "testing"

func TestParseTokenValue(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "abc", want: "abc"},
		{in: "sso=abc123", want: "abc123"},
		{in: "foo=1; sso=abc123; bar=2", want: "abc123"},
	}
	for _, tt := range tests {
		got := parseTokenValue(tt.in)
		if got != tt.want {
			t.Fatalf("parseTokenValue(%q)=%q want=%q", tt.in, got, tt.want)
		}
	}
}

func TestParseDataURI(t *testing.T) {
	name, content, mime, err := parseDataURI("data:image/png;base64,QUJD")
	if err != nil {
		t.Fatalf("parseDataURI error: %v", err)
	}
	if name != "file.png" {
		t.Fatalf("name=%q want=file.png", name)
	}
	if content != "QUJD" {
		t.Fatalf("content=%q want=QUJD", content)
	}
	if mime != "image/png" {
		t.Fatalf("mime=%q want=image/png", mime)
	}
}

func TestExtractMessageAndAttachments(t *testing.T) {
	messages := []ChatMessage{
		{
			Role: "user",
			Content: []interface{}{
				map[string]interface{}{"type": "text", "text": "hello"},
				map[string]interface{}{"type": "image_url", "image_url": map[string]interface{}{"url": "https://a/b.png"}},
			},
		},
	}

	text, attachments, err := extractMessageAndAttachments(messages, false)
	if err != nil {
		t.Fatalf("extractMessageAndAttachments error: %v", err)
	}
	if text != "hello" {
		t.Fatalf("text=%q want=hello", text)
	}
	if len(attachments) != 1 {
		t.Fatalf("attachments=%d want=1", len(attachments))
	}
	if attachments[0].Data != "https://a/b.png" {
		t.Fatalf("attachment=%q want=https://a/b.png", attachments[0].Data)
	}
}

func TestResolveAspectRatio(t *testing.T) {
	if got := resolveAspectRatio("1024x1024"); got != "1:1" {
		t.Fatalf("resolveAspectRatio(1024x1024)=%q want=1:1", got)
	}
	if got := resolveAspectRatio("unknown"); got != "2:3" {
		t.Fatalf("resolveAspectRatio(unknown)=%q want=2:3", got)
	}
}

