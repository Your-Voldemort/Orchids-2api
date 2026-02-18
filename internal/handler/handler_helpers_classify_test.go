package handler

import "testing"

func TestClassifyUpstreamErrorCreditsExhausted(t *testing.T) {
	t.Parallel()

	errClass := classifyUpstreamError("orchids upstream error: no remaining quota: You have run out of credits.")
	if errClass.Category != "rate_limit" {
		t.Fatalf("expected rate_limit category, got %q", errClass.Category)
	}
	if !errClass.Retryable {
		t.Fatal("expected credits exhausted to be retryable")
	}
	if !errClass.SwitchAccount {
		t.Fatal("expected credits exhausted to trigger account switch")
	}
}
