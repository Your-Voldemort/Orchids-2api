package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"orchids-api/internal/accountpolicy"
	"orchids-api/internal/channel"
	"orchids-api/internal/cline"
	"orchids-api/internal/config"
	apperrors "orchids-api/internal/errors"
	"orchids-api/internal/grok"
	"orchids-api/internal/modelcatalog"
	"orchids-api/internal/qoder"
	"orchids-api/internal/refreshqueue"
	"orchids-api/internal/store"
	"orchids-api/internal/util"
)

func verifyGrokAccount(ctx context.Context, acc *store.Account, cfg *config.Config, accountStore *store.Store) error {
	if acc == nil {
		return fmt.Errorf("missing grok account")
	}
	// Build CLI OAuth accounts verify against the CLI proxy with a Bearer token.
	if grokAccountIsOAuth(acc) {
		if !grokAccountHasOAuthCredentials(acc) {
			return fmt.Errorf("missing oauth token")
		}
		cliClient := grok.NewCLIClient(cfg)
		cliClient.SetAccountStore(accountStore)
		result := grok.RefreshBuildAccount(ctx, cliClient, acc, grok.BuildRefreshOptions{
			Verify: true, VerifyTimeout: 20 * time.Second,
			Billing: true,
			Models:  true, ModelsTimeout: 15 * time.Second,
		})
		if result.VerifyErr != nil {
			if result.VerifyStatus != "" {
				return fmt.Errorf("%s: %w", result.VerifyStatus, result.VerifyErr)
			}
			return result.VerifyErr
		}
		if result.BillingErr != nil {
			slog.Warn("Grok CLI billing sync failed; leaving quota unavailable", "account_id", acc.ID, "error", result.BillingErr)
		}
		if result.ModelsErr != nil {
			slog.Warn("Grok CLI model catalog sync failed; keeping last catalog", "account_id", acc.ID, "error", result.ModelsErr)
		}
		return nil
	}

	// The website cookie plane was retired: only Build OAuth
	// credentials can be verified now.
	return fmt.Errorf("only Grok Build OAuth accounts are supported")
}

func httpStatusFromAccountStatus(status string) int {
	switch strings.TrimSpace(status) {
	case "401":
		return http.StatusUnauthorized
	case "402", store.AccountStatusQoderQuotaExhausted, store.AccountStatusWorkBuddyQuotaExhausted:
		return http.StatusPaymentRequired
	case "403":
		return http.StatusForbidden
	case "404":
		return http.StatusNotFound
	case "429":
		return http.StatusTooManyRequests
	default:
		return http.StatusBadGateway
	}
}

func normalizeGrokTokenInput(acc *store.Account) {
	if acc == nil || !strings.EqualFold(acc.AccountType, "grok") {
		return
	}
	acc.CredentialType = "oauth"
	acc.GrokProvider = grok.ProviderBuild
	acc.OAuthAccessToken = strings.TrimSpace(acc.OAuthAccessToken)
	acc.OAuthRefreshToken = strings.TrimSpace(acc.OAuthRefreshToken)
	// Non-Build Grok credentials must never survive an account write.
	acc.Token = ""
	acc.ClientCookie = ""
	acc.RefreshToken = ""
}

// grokAccountIsOAuth reports whether a Grok account is a Build CLI OAuth account.
func grokAccountIsOAuth(acc *store.Account) bool {
	return acc != nil && strings.EqualFold(strings.TrimSpace(acc.CredentialType), "oauth")
}

// grokAccountHasOAuthCredentials reports whether an OAuth account carries at
// least one usable token after normalization.
func grokAccountHasOAuthCredentials(acc *store.Account) bool {
	if !grokAccountIsOAuth(acc) {
		return false
	}
	return strings.TrimSpace(acc.OAuthAccessToken) != "" || strings.TrimSpace(acc.OAuthRefreshToken) != ""
}

// preserveGrokOAuthCredentials keeps existing OAuth secrets when the admin UI
// submits empty fields (secrets are redacted on read and therefore absent on
// ordinary edit/save).
func preserveGrokOAuthCredentials(acc, existing *store.Account) {
	if acc == nil || existing == nil || !grokAccountIsOAuth(acc) {
		return
	}
	if strings.TrimSpace(acc.OAuthAccessToken) == "" {
		acc.OAuthAccessToken = existing.OAuthAccessToken
	}
	if strings.TrimSpace(acc.OAuthRefreshToken) == "" {
		acc.OAuthRefreshToken = existing.OAuthRefreshToken
	}
	if acc.OAuthExpiresAt.IsZero() && !existing.OAuthExpiresAt.IsZero() {
		acc.OAuthExpiresAt = existing.OAuthExpiresAt
	}
	if strings.TrimSpace(acc.TeamID) == "" {
		acc.TeamID = existing.TeamID
	}
}

// preserveGrokRuntimeStateOnAdminEdit keeps provider-observed Build state out of
// the generic account edit surface. OAuth secrets are preserved separately.
func preserveGrokRuntimeStateOnAdminEdit(acc, existing *store.Account) {
	if acc == nil || existing == nil || !strings.EqualFold(acc.AccountType, "grok") {
		return
	}
	acc.Token = existing.Token
	acc.Subscription = existing.Subscription
	acc.UsageCurrent = existing.UsageCurrent
	acc.UsageTotal = existing.UsageTotal
	acc.UsageLimit = existing.UsageLimit
	acc.StatusCode = existing.StatusCode
	acc.StatusMessage = existing.StatusMessage
	acc.LastAttempt = existing.LastAttempt
	acc.VerifiedAt = existing.VerifiedAt
	acc.QuotaResetAt = existing.QuotaResetAt
	acc.GrokModels = append([]string(nil), existing.GrokModels...)
	acc.GrokModelCatalog = modelcatalog.CloneProfiles(existing.GrokModelCatalog)
	acc.GrokModelsSyncedAt = existing.GrokModelsSyncedAt
	acc.GrokBilling = existing.GrokBilling
	acc.GrokRateLimits = existing.GrokRateLimits
}

type accountOutput struct {
	*store.Account
	// SessionFingerprint is a short digest of the credential the account is
	// authenticated with. It lets the table tell two sessions apart on channels
	// that carry no email, without returning the secret itself.
	SessionFingerprint string `json:"session_fingerprint,omitempty"`
	// Quota holds the provider-specific quota projection. It is merged into every
	// account response so the management table can render the plan/allowance
	// columns consistently without re-deriving each channel's semantics on the
	// client.
	Quota map[string]interface{} `json:"-"`
}

// MarshalJSON flattens the quota projection into the account object itself.
func (o accountOutput) MarshalJSON() ([]byte, error) {
	merged := map[string]interface{}{}
	if o.Account != nil {
		raw, err := json.Marshal(o.Account)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &merged); err != nil {
			return nil, err
		}
	}
	// The session fingerprint identifies a login on channels that carry no email;
	// it is a digest, never the credential, so it is safe to expose to an
	// authenticated administrator.
	if o.SessionFingerprint != "" {
		merged["session_fingerprint"] = o.SessionFingerprint
	}
	for key, value := range o.Quota {
		merged[key] = value
	}
	// Credentials are write-only. The account API exposes only their presence,
	// including for create, update and refresh responses.
	merged["has_credential"] = o.SessionFingerprint != ""
	for _, field := range []string{"token", "client_cookie", "refresh_token", "oauth_access_token", "oauth_refresh_token", "workbuddy_access_token", "workbuddy_refresh_token", "qoder_access_token", "qoder_refresh_token", "qoder_runtime_info", "qoder_runtime_key", "cline_access_token", "cline_refresh_token", "session_fingerprint"} {
		delete(merged, field)
	}
	if o.Account != nil {
		merged["status_message"] = redactAccountSecrets(o.Account.StatusMessage, o.Account)
	}
	return json.Marshal(merged)
}

// redactAccountSecrets replaces every credential value an account holds with a
// placeholder, so a text field that quotes upstream output cannot publish one.
func redactAccountSecrets(message string, acc *store.Account) string {
	for _, secret := range acc.Secrets() {
		if secret = strings.TrimSpace(secret); secret != "" {
			message = strings.ReplaceAll(message, secret, "[REDACTED]")
		}
	}
	return message
}

func normalizeAccountOutput(acc *store.Account) *accountOutput {
	return normalizeAccountOutputWithUsage(acc, nil)
}

// normalizeAccountOutputWithUsage renders one account for the management API.
//
// usage carries the tokens this gateway observed per account inside the Free window;
// a nil map means "not measured", which the quota projection reports honestly instead
// of presenting zero usage as a measurement.
func normalizeAccountOutputWithUsage(acc *store.Account, usage map[int64]int64) *accountOutput {
	// The session fingerprint is derived from the live credential before the
	// redaction below clears it, so the operator can still tell two browser
	// logins apart without the session token ever leaving the server.
	sessionFingerprint := accountSessionFingerprint(acc)
	if acc == nil {
		return nil
	}
	// The projection renders a copy: the channel redaction below clears slots on
	// the way out, and the stored record must keep the credential it was given.
	out := *acc
	// The message is redacted with the same list the final render uses. The two
	// used to differ: this one omitted Qoder's access token and runtime pair, and
	// it ran before the channel projection cleared them, so an upstream error that
	// echoed a Qoder token published it in status_message.
	out.StatusMessage = redactAccountSecrets(acc.StatusMessage, acc)
	if strings.EqualFold(out.AccountType, "grok") {
		grok.NormalizeProvider(&out)
		// The tier column must agree with the quota column. A Build Free account
		// has no plan name from the identity endpoint (recorded as "unknown"), yet
		// the same Free inference that produces its quota window already proves it
		// is Free — and only Free. Reporting "unknown" there told an operator nothing
		// about an account the gateway had already characterised.
		if verdict := grok.InferFreeProfile(&out); verdict.Inferred {
			switch strings.ToLower(strings.TrimSpace(out.Subscription)) {
			case "", "unknown", "free":
				out.Subscription = "free"
			}
		}
		out.RefreshToken = ""
		// The administrator explicitly opted in to seeing the short-lived OAuth
		// access token in the authenticated management UI. Never return the
		// durable refresh token through normal account endpoints.
		out.OAuthRefreshToken = ""
	}
	if strings.EqualFold(out.AccountType, "workbuddy") {
		// The durable refresh token never leaves the server; the access token
		// stays visible so the account table can prove a credential exists.
		out = *RedactWorkBuddyOutput(&out)
	}
	if strings.EqualFold(out.AccountType, "qoder") {
		// The durable refresh token and the derived runtime material never leave
		// the server; the access token stays visible so the account table can
		// prove a credential exists.
		out = *RedactQoderOutput(&out)
	}
	if strings.EqualFold(out.AccountType, "cline") {
		// The durable refresh token never leaves the server; the access token
		// stays visible so the account table can prove a credential exists.
		out = *RedactClineOutput(&out)
	}
	return &accountOutput{
		Account:            &out,
		SessionFingerprint: sessionFingerprint,
		Quota:              buildQuotaResponseFieldsWithUsage(&out, usage[out.ID], usage != nil),
	}
}

// normalizeAccountOutputObserved renders one account together with the Free-window
// usage this gateway measured for it, so a single-account response carries the same
// estimate as the list. A failed measurement falls back to the plain projection
// rather than reporting zero usage as if it had been counted.
func (a *API) normalizeAccountOutputObserved(ctx context.Context, acc *store.Account) *accountOutput {
	observed, ok := a.observedTokensByAccount(ctx, time.Now().Add(-grok.FreeBuildUsageWindow))
	if !ok {
		return normalizeAccountOutput(acc)
	}
	return normalizeAccountOutputWithUsage(acc, observed)
}

// observedTokensByAccount sums the tokens the journal recorded for each account
// inside the Free window.
//
// A Build Free allowance is a rolling token window that the upstream only reveals
// once it is exhausted, so the only honest usage figure available to the admin UI is
// what this gateway itself saw. The scan is bounded (auditScanCap newest entries),
// which makes the sum a floor rather than a total: it travels with quota_observed so
// an estimate is never mistaken for a complete count. A failed scan returns ok=false,
// and callers must then leave the usage unmeasured.
func (a *API) observedTokensByAccount(ctx context.Context, since time.Time) (map[int64]int64, bool) {
	if a == nil || a.store == nil || a.store.RedisClient() == nil {
		return nil, false
	}
	entries, err := a.store.RedisClient().XRevRangeN(ctx, a.store.RedisPrefix()+"audit:log", "+", "-", auditScanCap).Result()
	if err != nil {
		return nil, false
	}
	usage := make(map[int64]int64, len(entries))
	for _, entry := range entries {
		event, ok := decodeAuditEvent(entry)
		if !ok || event.AccountID == 0 {
			continue
		}
		if !since.IsZero() && event.Timestamp.Before(since) {
			continue
		}
		tokens := event.TotalTokens
		if tokens <= 0 {
			tokens = event.InputTokens + event.OutputTokens
		}
		if tokens > 0 {
			usage[event.AccountID] += int64(tokens)
		}
	}
	return usage, true
}

// accountSessionFingerprint returns a short, non-reversible identifier of the
// credential an account is authenticated with.
//
// It exists because some channels authenticate with a session token that carries
// no identity at all (there is no email or username to show). The account table
// then had nothing to display but "login session configured", which made two different
// logins look identical. The fingerprint distinguishes them without ever
// exposing the secret: 12 hex characters of a SHA-256 digest, the same shape
// already used for upstream diagnostics.
func accountSessionFingerprint(acc *store.Account) string {
	if acc == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(acc.AccountType)) {
	case "grok":
		if strings.EqualFold(strings.TrimSpace(acc.CredentialType), "oauth") {
			return util.Fingerprint(util.FirstNonEmpty(acc.OAuthAccessToken, acc.OAuthRefreshToken))
		}
		return util.Fingerprint(util.FirstNonEmpty(acc.ClientCookie, acc.RefreshToken, acc.Token))
	case "workbuddy":
		creds := resolveWorkBuddyCredentials(acc)
		return util.Fingerprint(util.FirstNonEmpty(creds.AccessToken, creds.RefreshToken))
	case "qoder":
		creds := qoder.ResolveCredentials(acc)
		return util.Fingerprint(util.FirstNonEmpty(creds.RefreshToken, creds.AccessToken))
	case "cline":
		creds := cline.ResolveCredentials(acc)
		return util.Fingerprint(util.FirstNonEmpty(creds.RefreshToken, creds.AccessToken))
	default:
		return ""
	}
}

func normalizedAccountCredentialKey(acc *store.Account) string {
	if acc == nil {
		return ""
	}

	accountType := strings.ToLower(strings.TrimSpace(acc.AccountType))
	var token string

	switch accountType {
	case "grok":
		token = strings.TrimSpace(util.FirstNonEmpty(acc.OAuthRefreshToken, acc.OAuthAccessToken))
	case "workbuddy":
		return WorkBuddyCredentialKey(acc)
	case "qoder":
		return QoderCredentialKey(acc)
	case "cline":
		return ClineCredentialKey(acc)
	default:
		token = strings.TrimSpace(util.FirstNonEmpty(acc.RefreshToken, acc.ClientCookie, acc.Token))
	}

	if token == "" || accountType == "" {
		return ""
	}
	return accountType + ":" + token
}

func isSupportedAccountType(accountType string) bool { return channel.IsSupported(accountType) }

// validateAccountType rejects an account whose type is missing or unknown.
//
// The create and update surfaces both take an account type from the request
// body, and both have to answer the same two questions before touching the
// store: is a type present, and is it one this gateway serves. Reporting the
// error and writing the response belongs here so the two surfaces cannot drift
// into giving different answers about the same input.
func validateAccountType(w http.ResponseWriter, accountType string) bool {
	if strings.TrimSpace(accountType) == "" {
		http.Error(w, "account_type is required", http.StatusBadRequest)
		return false
	}
	if !isSupportedAccountType(accountType) {
		http.Error(w, "unsupported account type", http.StatusBadRequest)
		return false
	}
	return true
}

func (a *API) findDuplicateAccountByCredential(ctx context.Context, acc *store.Account, excludeID int64) (*store.Account, error) {
	if a == nil || a.store == nil || acc == nil {
		return nil, nil
	}

	key := normalizedAccountCredentialKey(acc)
	identityKey := stableProviderIdentityKey(acc)
	if key == "" && identityKey == "" {
		return nil, nil
	}

	accounts, err := a.store.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	for _, existing := range accounts {
		if existing == nil || existing.ID == excludeID {
			continue
		}
		if identityKey != "" && stableProviderIdentityKey(existing) == identityKey {
			return existing, nil
		}
		if key != "" && normalizedAccountCredentialKey(existing) == key {
			return existing, nil
		}
	}
	return nil, nil
}

// saveNewAccountUnlessDuplicate stores a freshly authenticated account, or
// returns the row that already carries its credential.
//
// A completed device login and a completed browser login reach the same
// decision — an upstream may hand out a second grant for an account this
// gateway already has, and inserting it would give the scheduler two rows for
// one allowance. The duplicate check and the insert share a single deadline
// because they are one step: leaving it to the caller's context would let a
// login hold a store round-trip open for the whole poll lifetime.
func (a *API) saveNewAccountUnlessDuplicate(ctx context.Context, acc *store.Account) (*store.Account, error) {
	storeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	existing, err := a.findDuplicateAccountByCredential(storeCtx, acc, 0)
	if err == nil && existing == nil {
		err = a.store.CreateAccount(storeCtx, acc)
	}
	return existing, err
}

// stableProviderIdentityKey survives OAuth token rotation. WorkBuddy and Qoder
// issue a new durable token during a fresh login, so token-only deduplication
// would create a second row for the same upstream user and leave the old row
// holding a consumed refresh token.
func stableProviderIdentityKey(acc *store.Account) string {
	if acc == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(acc.AccountType)) {
	case "grok":
		if grokAccountIsOAuth(acc) {
			if userID := strings.TrimSpace(acc.UserID); userID != "" {
				return "grok:oauth:user:" + userID
			}
			if email := strings.ToLower(strings.TrimSpace(acc.Email)); email != "" {
				return "grok:oauth:email:" + email
			}
		}
	case "workbuddy":
		if uid := strings.TrimSpace(acc.WorkBuddyUID); uid != "" {
			return "workbuddy:uid:" + uid
		}
	case "qoder":
		if uid := strings.TrimSpace(acc.QoderUserID); uid != "" {
			return "qoder:uid:" + uid
		}
		if machineID := strings.TrimSpace(acc.QoderMachineID); machineID != "" {
			return "qoder:machine:" + machineID
		}
	case "cline":
		if email := strings.ToLower(strings.TrimSpace(acc.ClineEmail)); email != "" {
			return "cline:email:" + email
		}
	}
	return ""
}

func duplicateAccountError(existing *store.Account) error {
	if existing == nil {
		return fmt.Errorf("duplicate account token")
	}
	accountType := strings.TrimSpace(existing.AccountType)
	accountType = util.FirstNonEmptyUntrimmed(accountType, "account")
	return fmt.Errorf("duplicate %s token already exists on account #%d", accountType, existing.ID)
}

func buildQuotaResponseFields(acc *store.Account) map[string]interface{} {
	return buildQuotaResponseFieldsWithUsage(acc, 0, false)
}

// applyQuotaProvenance records where a quota number came from and how far it should
// be trusted, next to the number itself.
//
// Without it a table can only say "unknown", which conflates three different facts — a
// paid account whose numeric window upstream does not publish, a Free account whose
// window has to be estimated, and an account that was never synced. quota_type is
// paid/free/unknown, quota_source names the signal, quota_confidence is
// confirmed/observed/estimated, and quota_limit_known is false whenever the limit is
// an estimate that the upstream has not confirmed.
func applyQuotaProvenance(fields map[string]interface{}, quotaType, source, confidence, note string, limitKnown, observed bool) {
	fields["quota_type"] = quotaType
	fields["quota_source"] = source
	fields["quota_confidence"] = confidence
	fields["quota_limit_known"] = limitKnown
	fields["quota_observed"] = observed
	if note != "" {
		fields["quota_note"] = note
	}
}

// buildQuotaResponseFieldsWithUsage projects an account's allowance. observedTokens
// is the usage this gateway measured inside grok.FreeBuildUsageWindow and
// usageObserved says whether that measurement actually ran; both are used only by the
// Free estimate, which must never present unmeasured usage as if it were measured.
func buildQuotaResponseFieldsWithUsage(acc *store.Account, observedTokens int64, usageObserved bool) map[string]interface{} {
	fields := map[string]interface{}{
		"quota_limit":     0.0,
		"quota_used":      0.0,
		"quota_remaining": 0.0,
		"quota_mode":      "remaining",
		"quota_unit":      "credits",
		"quota_supported": true,
	}
	applyQuotaProvenance(fields, "unknown", "unknown", "", "", false, false)
	if acc == nil {
		return fields
	}

	limit := acc.UsageLimit
	current := acc.UsageCurrent
	if limit < 0 {
		limit = 0
	}
	if current < 0 {
		current = 0
	}

	projectQuotaFields(fields, acc, limit, current, observedTokens, usageObserved)
	return fields
}

func (a *API) HandleAccounts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		accounts, err := a.store.ListAccounts(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if accounts == nil {
			accounts = []*store.Account{}
		}
		// The Free estimate needs the usage this gateway observed; the scan is one
		// bounded read shared by the whole page, and it is skipped entirely when there
		// is nothing to describe.
		var observed map[int64]int64
		if len(accounts) > 0 {
			if measured, ok := a.observedTokensByAccount(r.Context(), time.Now().Add(-grok.FreeBuildUsageWindow)); ok {
				observed = measured
			}
		}
		normalized := make([]*accountOutput, 0, len(accounts))
		for _, acc := range accounts {
			if acc == nil {
				continue
			}
			normalized = append(normalized, normalizeAccountOutputWithUsage(acc, observed))
		}
		util.WriteJSON(w, normalized)

	case http.MethodPost:
		var acc store.Account
		if err := json.NewDecoder(r.Body).Decode(&acc); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		acc.AccountType = strings.ToLower(strings.TrimSpace(acc.AccountType))
		if !validateAccountType(w, acc.AccountType) {
			return
		}
		if strings.EqualFold(acc.AccountType, "grok") {
			normalizeGrokTokenInput(&acc)
			if !grokAccountIsOAuth(&acc) {
				http.Error(w, "Grok accounts must be added through the Build OAuth device login (/api/grok/device-auth)", http.StatusBadRequest)
				return
			}
			if grokAccountIsOAuth(&acc) && !grokAccountHasOAuthCredentials(&acc) {
				http.Error(w, "missing oauth token", http.StatusBadRequest)
				return
			}
		} else if strings.EqualFold(acc.AccountType, "workbuddy") {
			if !NormalizeWorkBuddyCredentials(&acc) {
				http.Error(w, "missing WorkBuddy credential: paste the access token or refresh token from the WorkBuddy desktop session", http.StatusBadRequest)
				return
			}
		} else if strings.EqualFold(acc.AccountType, "qoder") {
			// Qoder is OAuth-only: an account is created by the browser device
			// flow (/api/qoder/login), never by pasting a personal access token.
			http.Error(w, "Qoder accounts must be added using the official browser login (/api/qoder/login)", http.StatusBadRequest)
			return
		} else if strings.EqualFold(acc.AccountType, "cline") {
			// Cline is OAuth-only for the same reason: the WorkOS device grant is
			// the only way to obtain the credential.
			http.Error(w, "Cline accounts must be added using the official browser login (/api/cline/login)", http.StatusBadRequest)
			return
		}
		if existing, err := a.findDuplicateAccountByCredential(r.Context(), &acc, 0); err != nil {
			slog.Error("Failed to detect duplicate account token", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		} else if existing != nil {
			http.Error(w, duplicateAccountError(existing).Error(), http.StatusConflict)
			return
		}

		if err := a.store.CreateAccount(r.Context(), &acc); err != nil {
			slog.Error("Failed to create account", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if acc.Enabled {
			if strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Account-Sync")), "async") {
				a.syncAccountAfterCreate(acc)
			} else {
				syncCtx, syncCancel := context.WithTimeout(r.Context(), 25*time.Second)
				accountStatus, _, syncErr := a.refreshAccountState(syncCtx, &acc)
				syncCancel()
				if syncErr != nil {
					slog.Warn("Initial account sync failed", "account_id", acc.ID, "type", acc.AccountType, "error", syncErr)
					if accountStatus != "" {
						acc.StatusCode = accountStatus
						acc.StatusMessage = strings.TrimSpace(syncErr.Error())
						acc.LastAttempt = time.Now()
						acc.VerifiedAt = acc.LastAttempt
					}
				} else {
					applySuccessfulAccountRefreshStatus(&acc, accountStatus)
				}
				// A credential the upstream definitively rejects must not be
				// persisted as a healthy account: it would sit in the pool looking
				// healthy while every request routed to it fails.
				if acc.StatusCode == "401" {
					if deleteErr := a.store.DeleteAccount(r.Context(), acc.ID); deleteErr != nil {
						slog.Error("Failed to roll back rejected account", "account_id", acc.ID, "type", acc.AccountType, "error", deleteErr)
					} else {
						slog.Warn("Rejected account was not saved (upstream refused the credential)",
							"account_id", acc.ID, "type", acc.AccountType, "reason", acc.StatusMessage)
					}
					apperrors.New("authentication_error",
						"account was rejected by the upstream and was not saved: "+strings.TrimSpace(acc.StatusMessage),
						http.StatusUnauthorized).WriteResponse(w)
					return
				}
				if updateErr := a.store.UpdateAccount(r.Context(), &acc); updateErr != nil {
					slog.Warn("Failed to persist initial account sync", "account_id", acc.ID, "type", acc.AccountType, "error", updateErr)
				}
			}
		}

		util.WriteJSONStatus(w, http.StatusCreated, normalizeAccountOutput(&acc))

	default:
		writeMethodNotAllowed(w)
	}
}

func (a *API) HandleAccountByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/accounts/")
	parts := strings.Split(path, "/")
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	account, err := a.store.GetAccount(r.Context(), id)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	isCheck := len(parts) > 1 && parts[1] == "check"
	isUsage := len(parts) > 1 && parts[1] == "usage"
	if len(parts) > 2 || (len(parts) > 1 && !(isCheck || isUsage)) {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		if isUsage {
			resp := map[string]interface{}{
				"account_id":     account.ID,
				"name":           account.Name,
				"account_type":   account.AccountType,
				"subscription":   account.Subscription,
				"usage_current":  account.UsageCurrent,
				"usage_limit":    account.UsageLimit,
				"usage_total":    account.UsageTotal,
				"quota_reset_at": account.QuotaResetAt,
			}
			for k, v := range buildQuotaResponseFields(account) {
				resp[k] = v
			}
			util.WriteJSON(w, resp)
			return
		}
		if isCheck {
			// Storm control / backoff: only allow a small number of concurrent checks,
			// and apply exponential backoff per account on failures.
			now := time.Now()
			a.checkMu.Lock()
			if a.checkInFlight[id] {
				a.checkMu.Unlock()
				http.Error(w, "account check already in progress", http.StatusTooManyRequests)
				return
			}
			if next, ok := a.checkNextAllowed[id]; ok && !next.IsZero() && now.Before(next) {
				retryAfter := int(next.Sub(now).Seconds())
				if retryAfter < 1 {
					retryAfter = 1
				}
				a.checkMu.Unlock()
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				http.Error(w, "account check backoff", http.StatusTooManyRequests)
				return
			}
			a.checkInFlight[id] = true
			a.checkMu.Unlock()
			defer func() {
				a.checkMu.Lock()
				delete(a.checkInFlight, id)
				a.checkMu.Unlock()
			}()

			// global concurrency limit
			a.checkSem <- struct{}{}
			defer func() { <-a.checkSem }()

			acc := account
			checkOK := false
			checkErrStatus := ""
			defer func() {
				a.checkMu.Lock()
				defer a.checkMu.Unlock()
				if checkOK {
					a.checkFailCount[id] = 0
					a.checkNextAllowed[id] = time.Now().Add(3 * time.Second)
					return
				}
				fails := a.checkFailCount[id] + 1
				a.checkFailCount[id] = fails
				d := time.Duration(1<<min(fails, 8)) * time.Second
				// For CF/rate-limit style failures, start with a bigger cooldown.
				if checkErrStatus == "403" || checkErrStatus == "429" {
					if d < 60*time.Second {
						d = 60 * time.Second
					}
				}
				if d > 10*time.Minute {
					d = 10 * time.Minute
				}
				a.checkNextAllowed[id] = time.Now().Add(d)
			}()

			// The manual check takes the same process-wide lease as the background
			// scheduler: two refreshes of one account must never run at once, or the
			// slower writer would persist an older snapshot over a newer verdict.
			var accountStatus string
			var httpStatus int
			var refreshErr error
			if !refreshqueue.WithLease(acc.ID, func() {
				accountStatus, httpStatus, refreshErr = a.refreshAccountState(r.Context(), acc)
			}) {
				slog.Info("Account check skipped: a refresh of this account is already running", "account_id", acc.ID)
				checkErrStatus = ""
				a.checkMu.Lock()
				a.checkInFlight[id] = false
				a.checkMu.Unlock()
				writeAccountCheckBusy(w)
				return
			}
			if refreshErr != nil {
				checkErrStatus = accountStatus
				if accountStatus != "" {
					acc.StatusCode = accountStatus
					// The reason matters: a bare "401" cannot tell an operator
					// whether the credential was retired upstream or the record
					// lost it.
					acc.StatusMessage = strings.TrimSpace(refreshErr.Error())
					acc.LastAttempt = time.Now()
					acc.VerifiedAt = acc.LastAttempt
					if updateErr := a.store.UpdateAccount(r.Context(), acc); updateErr != nil {
						slog.Warn("Failed to persist account refresh status", "account_id", acc.ID, "error", updateErr)
					}
				}
				if httpStatus == 0 {
					httpStatus = http.StatusBadRequest
				}
				http.Error(w, refreshErr.Error(), httpStatus)
				return
			}

			// Clear the account status after a successful refresh/verification
			applySuccessfulAccountRefreshStatus(acc, accountStatus)
			checkOK = true

			if err := a.store.UpdateAccount(r.Context(), acc); err != nil {
				http.Error(w, "Failed to save checked account: "+err.Error(), http.StatusInternalServerError)
				return
			}
			util.WriteJSON(w, a.normalizeAccountOutputObserved(r.Context(), acc))
			return
		}
		util.WriteJSON(w, a.normalizeAccountOutputObserved(r.Context(), account))

	case http.MethodPut:
		existing := account

		var acc store.Account
		if err := json.NewDecoder(r.Body).Decode(&acc); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		acc.ID = id
		if strings.TrimSpace(acc.AccountType) == "" {
			acc.AccountType = existing.AccountType
		}
		acc.AccountType = strings.ToLower(strings.TrimSpace(acc.AccountType))
		if !validateAccountType(w, acc.AccountType) {
			return
		}
		if strings.EqualFold(acc.AccountType, "grok") {
			normalizeGrokTokenInput(&acc)
			// Admin UI redacts OAuth secrets on read; empty inbound fields mean
			// "keep existing", not "clear credentials".
			preserveGrokOAuthCredentials(&acc, existing)
			preserveGrokRuntimeStateOnAdminEdit(&acc, existing)
			if grokAccountIsOAuth(&acc) && !grokAccountHasOAuthCredentials(&acc) {
				http.Error(w, "missing oauth token", http.StatusBadRequest)
				return
			}
		} else if strings.EqualFold(acc.AccountType, "workbuddy") {
			// The read path redacts the refresh token, so an ordinary edit
			// arrives without it; keep the stored credential unless a new one
			// was actually submitted.
			submitted := resolveWorkBuddyCredentials(&acc)
			PreserveWorkBuddyCredentialsOnEdit(&acc, existing)
			if resolveWorkBuddyCredentials(&acc).RefreshToken == "" && resolveWorkBuddyCredentials(&acc).AccessToken == "" {
				http.Error(w, "missing WorkBuddy credential", http.StatusBadRequest)
				return
			}
			NormalizeWorkBuddyCredentials(&acc)
			existingCreds := resolveWorkBuddyCredentials(existing)
			acc.ReplaceWorkBuddyCredentials = submitted.HasCredential() &&
				(submitted.AccessToken != existingCreds.AccessToken || submitted.RefreshToken != existingCreds.RefreshToken)
			if acc.ReplaceWorkBuddyCredentials {
				acc.ClearVerifiedAt = true
			}
		} else if strings.EqualFold(acc.AccountType, "qoder") {
			submitted := qoder.ResolveCredentials(&acc)
			submittedMachineID := strings.TrimSpace(acc.QoderMachineID)
			PreserveQoderCredentialsOnEdit(&acc, existing)
			if !NormalizeQoderCredentials(&acc) {
				http.Error(w, "missing Qoder credential: sign in again with the browser login", http.StatusBadRequest)
				return
			}
			if strings.TrimSpace(acc.QoderMachineID) == "" {
				http.Error(w, "missing Qoder device identity: sign in again", http.StatusBadRequest)
				return
			}
			existingCreds := qoder.ResolveCredentials(existing)
			acc.ReplaceQoderCredentials = submitted.HasCredential() &&
				(submitted.AccessToken != existingCreds.AccessToken ||
					submitted.RefreshToken != existingCreds.RefreshToken ||
					(submittedMachineID != "" && submittedMachineID != strings.TrimSpace(existing.QoderMachineID)))
			if acc.ReplaceQoderCredentials {
				acc.ClearVerifiedAt = true
			}
		} else if strings.EqualFold(acc.AccountType, "cline") {
			// The read path redacts the refresh token, so an ordinary edit
			// arrives without it; keep the stored credential unless a new one
			// was actually submitted.
			submitted := cline.ResolveCredentials(&acc)
			PreserveClineCredentialsOnEdit(&acc, existing)
			if !NormalizeClineCredentials(&acc) {
				http.Error(w, "missing Cline credential: sign in again with the browser login", http.StatusBadRequest)
				return
			}
			existingCreds := cline.ResolveCredentials(existing)
			acc.ReplaceClineCredentials = submitted.HasCredential() &&
				(submitted.AccessToken != existingCreds.AccessToken ||
					submitted.RefreshToken != existingCreds.RefreshToken)
			if acc.ReplaceClineCredentials {
				acc.ClearVerifiedAt = true
			}
		}

		if acc.UserID == "" {
			acc.UserID = existing.UserID
		}
		if acc.Email == "" {
			acc.Email = existing.Email
		}
		if duplicate, err := a.findDuplicateAccountByCredential(r.Context(), &acc, id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		} else if duplicate != nil {
			http.Error(w, duplicateAccountError(duplicate).Error(), http.StatusConflict)
			return
		}

		if err := a.store.UpdateAccount(r.Context(), &acc); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		util.WriteJSON(w, normalizeAccountOutput(&acc))

	case http.MethodDelete:
		if err := a.store.DeleteAccount(r.Context(), id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		writeMethodNotAllowed(w)
	}
}

func (a *API) syncAccountAfterCreate(acc store.Account) {
	if !acc.Enabled {
		return
	}

	go func(account store.Account) {
		syncCtx, syncCancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer syncCancel()

		accountStatus, _, syncErr := a.refreshAccountState(syncCtx, &account)
		if syncErr != nil {
			slog.Warn("Initial account sync failed", "account_id", account.ID, "type", account.AccountType, "error", syncErr)
			if accountStatus != "" {
				account.StatusCode = accountStatus
				account.StatusMessage = strings.TrimSpace(syncErr.Error())
				account.LastAttempt = time.Now()
			}
		} else {
			applySuccessfulAccountRefreshStatus(&account, accountStatus)
		}

		if updateErr := a.store.UpdateAccount(context.Background(), &account); updateErr != nil {
			slog.Warn("Failed to persist initial account sync", "account_id", account.ID, "type", account.AccountType, "error", updateErr)
		}
	}(acc)
}

func applySuccessfulAccountRefreshStatus(acc *store.Account, status string) {
	if acc == nil {
		return
	}
	status = strings.TrimSpace(status)
	// The credentials answered the upstream, whatever the verdict: stamp it so a
	// scheduler can tell a verified account from one that was never checked. The
	// policy package owns that pairing so every entrance behaves identically.
	if status == "" {
		// A successful check proves the *credential* works; it says nothing about the
		// allowance. Clearing a spent-allowance verdict here is what made the console
		// show a green account that failed again on the very next request — the check
		// answered for the token, while the verdict that parked the account came from
		// the upstream refusing an actual request.
		if accountHoldsAllowanceVerdict(acc, time.Now()) && allowanceStillSpent(acc) {
			acc.VerifiedAt = time.Now()
			return
		}
		accountpolicy.Success(time.Now()).Apply(acc)
		return
	}
	verdict := accountpolicy.Verdict{Status: status, At: time.Now()}
	if verdict.Scope = accountpolicy.ScopeForStatus(status); verdict.Scope == accountpolicy.ScopeCredential {
		verdict.NeedsLogin = true
	}
	// A verifier that reports only a status has no better explanation than the one
	// already on the record. The reason is the operator's only signal — the account
	// table shows a bare code without it — so an unchanged verdict keeps the
	// specific wording (the verifier returns "402" with nothing else, and a
	// manual check used to wipe the upstream's own explanation).
	//
	// The carry-over is limited to the same status: a new code means the old reason
	// described a different problem and would mislead.
	if strings.TrimSpace(verdict.Message) == "" && strings.TrimSpace(acc.StatusCode) == status {
		verdict.Message = strings.TrimSpace(acc.StatusMessage)
	}
	verdict.Apply(acc)
}

// accountHoldsAllowanceVerdict reports whether the account is parked for an
// allowance the upstream refused, with its reset time still ahead.
//
// The question is "would the selector still hold this account?", so the answer has
// to match the selector: a 402 with a future reset time is out of rotation, and a
// check that cleared the marker would only make the next request re-park it — which
// is what the console showed as an account turning green and then red again.
func accountHoldsAllowanceVerdict(acc *store.Account, now time.Time) bool {
	if acc == nil || strings.TrimSpace(acc.StatusCode) != "402" || acc.QuotaResetAt.IsZero() {
		return false
	}
	return now.Before(acc.QuotaResetAt)
}

// allowanceStillSpent reports whether the meter still says the allowance is gone.
//
// This is the half that keeps the rule from stranding an account: an operator who
// buys credits is released by the next check rather than waiting for the cycle
// boundary the reset time names. It reads the snapshot the check just refreshed,
// so a top-up is visible immediately.
//
// A failed meter read leaves the previous snapshot in place, and a stale "spent"
// answer holds the account until its reset time. That is the conservative
// direction: the alternative is re-offering an account whose allowance was last
// observed to be gone.
func allowanceStillSpent(acc *store.Account) bool {
	if acc == nil {
		return false
	}
	return acc.UsageLimit > 0 && acc.UsageCurrent <= 0
}
