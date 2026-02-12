package api

import (
	"encoding/base64"
	"fmt"
	"testing"
)

func TestIsLikelyJWT(t *testing.T) {
	if isLikelyJWT("") {
		t.Fatalf("empty should be false")
	}
	if isLikelyJWT("abc") {
		t.Fatalf("no dots should be false")
	}
	if !isLikelyJWT("aaaaaaaaaa.bbbbbbbbbb.cccccccccc") {
		t.Fatalf("expected true")
	}
}

func TestJwtHasRotatingToken(t *testing.T) {
	// Build a minimal JWT-like string: header.payload.sig (base64url)
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"rotating_token":"rt_123"}`))
	jwt := fmt.Sprintf("%s.%s.%s", "aaaaaaaaaa", payload, "cccccccccc")
	if !jwtHasRotatingToken(jwt) {
		t.Fatalf("expected rotating_token to be detected")
	}
	payload2 := base64.RawURLEncoding.EncodeToString([]byte(`{"sid":"sess_1"}`))
	jwt2 := fmt.Sprintf("%s.%s.%s", "aaaaaaaaaa", payload2, "cccccccccc")
	if jwtHasRotatingToken(jwt2) {
		t.Fatalf("did not expect rotating_token to be detected")
	}
}
