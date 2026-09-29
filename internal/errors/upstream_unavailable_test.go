package errors

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestUpstreamUnavailableAnswerIsHonest pins the answer a caller gets when the
// upstream says its own service is down for the model.
//
// Production measured 636 such refusals in one day, every one carrying
// serviceAvailable:false and queueCount:0, and every one was answered as 429
// "the available upstream accounts are rate-limited" after a p50 of 104s of
// waiting: a lie about the cause, with no hint about when to come back.
func TestUpstreamUnavailableAnswerIsHonest(t *testing.T) {
	category := ClassifyUpstreamError(`qoder gateway is busy: {"code":"10605","serviceAvailable":false,"queueCount":0,"retryAfterSeconds":30}`).Category
	if category != "upstream_unavailable" {
		t.Fatalf("category = %q, want upstream_unavailable", category)
	}
	if got := StatusForCategory(category); got != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", got, http.StatusServiceUnavailable)
	}
	message := PublicMessage(`qoder gateway is busy: {"code":"10605","serviceAvailable":false,"queueCount":0}`)
	if message != "The upstream service for this model is temporarily unavailable. Retry after the indicated delay." {
		t.Fatalf("message = %q", message)
	}
	if got := StatusForCategory("rate_limit"); got != http.StatusTooManyRequests {
		t.Fatalf("rate_limit status = %d, want 429: a real throttle keeps its own answer", got)
	}
}

// TestAppErrorPublishesRetryAfter keeps the hint on the wire. A capacity answer
// that took a minute of upstream retries to produce has to tell the caller when
// to come back; without the header the client only learns that something failed
// and retries on its own schedule, adding load to the condition.
func TestAppErrorPublishesRetryAfter(t *testing.T) {
	rec := httptest.NewRecorder()
	NewWithRetryAfter("upstream_unavailable", "down", StatusForCategory("upstream_unavailable"), 30*time.Second).WriteResponse(rec)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "30" {
		t.Fatalf("Retry-After = %q, want \"30\"", got)
	}

	// No hint means no header: an invented one would be a promise nobody made.
	rec = httptest.NewRecorder()
	New("rate_limit", "limited", StatusForCategory("rate_limit")).WriteResponse(rec)
	if got := rec.Header().Get("Retry-After"); got != "" {
		t.Fatalf("Retry-After = %q, want it unset when no hint is known", got)
	}
}

// TestPoolAnswerDistinguishesEntitlementFromThrottle pins the pool's answer for
// a model the account plan does not cover. Qoder reports it as business code 112
// and the gateway cached it as a model cooldown, so asking for that model again
// was answered with "the requested model is temporarily rate-limited" — inviting
// a retry for a condition only a plan change fixes.
func TestPoolAnswerDistinguishesEntitlementFromThrottle(t *testing.T) {
	entitlement := ClassifyPoolExhaustion(nil, "qoder account has no usable plan or allowance; the model requires a subscription (upstream code=112)")
	if entitlement.Category != "model_unavailable" {
		t.Fatalf("category = %q, want model_unavailable", entitlement.Category)
	}
	if got := StatusForCategory(entitlement.Category); got != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", got)
	}

	unavailable := ClassifyPoolExhaustion(nil, `qoder gateway is busy: {"code":"10605","serviceAvailable":false,"queueCount":0}`)
	if unavailable.Category != "upstream_unavailable" {
		t.Fatalf("category = %q, want upstream_unavailable", unavailable.Category)
	}
	if got := StatusForCategory(unavailable.Category); got != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", got)
	}
}
