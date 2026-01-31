package client

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	orchidsClerkClientURL = "https://clerk.orchids.app/v1/client"
	orchidsClerkTokenURL  = "https://clerk.orchids.app/v1/client/sessions/%s/tokens"
	orchidsClerkJSVersion = "5.114.0"
	orchidsOrigin         = "https://www.orchids.app"
	orchidsUserAgent      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

// OrchidsCredentials represents the stored credentials file
type OrchidsCredentials struct {
	ClientJWT     string `json:"clientJwt"`
	RotatingToken string `json:"rotatingToken,omitempty"`
	ExpiresAt     string `json:"expiresAt,omitempty"`
}

// ClerkSession represents a session from Clerk API
type ClerkSession struct {
	ID              string      `json:"id"`
	UserID          string      `json:"user_id"`
	LastActiveToken *ClerkToken `json:"last_active_token"`
}

type ClerkToken struct {
	JWT string `json:"jwt"`
}

// OrchidsAuthHandle manages authentication state
type OrchidsAuthHandle struct {
	credPath       string
	clientJWT      string
	clerkSessionID string
	userID         string
	wsToken        string
	tokenExpiresAt time.Time
	mu             sync.Mutex
	httpClient     *http.Client
}

// NewOrchidsAuth creates specific auth handler
func NewOrchidsAuth(credPath string) *OrchidsAuthHandle {
	return &OrchidsAuthHandle{
		credPath: credPath,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// LoadCredentials reads credentials from file
func (a *OrchidsAuthHandle) LoadCredentials() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	data, err := os.ReadFile(a.credPath)
	if err != nil {
		return fmt.Errorf("failed to read credentials file: %w", err)
	}

	var creds OrchidsCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return fmt.Errorf("failed to parse credentials: %w", err)
	}

	if creds.ClientJWT == "" {
		return errors.New("missing clientJwt in credentials")
	}

	a.clientJWT = creds.ClientJWT
	return nil
}

// GetSessionFromClerk fetches session info using client jwt
func (a *OrchidsAuthHandle) GetSessionFromClerk() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	req, err := http.NewRequest("GET", orchidsClerkClientURL, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Cookie", fmt.Sprintf("__client=%s", a.clientJWT))
	req.Header.Set("Origin", orchidsOrigin)
	req.Header.Set("User-Agent", orchidsUserAgent)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("clerk api returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Response struct {
			Sessions []struct {
				ID   string `json:"id"`
				User struct {
					ID string `json:"id"`
				} `json:"user"`
				LastActiveToken struct {
					JWT string `json:"jwt"`
				} `json:"last_active_token"`
			} `json:"sessions"`
		} `json:"response"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if len(result.Response.Sessions) == 0 {
		return errors.New("no active sessions found")
	}

	session := result.Response.Sessions[0]
	a.clerkSessionID = session.ID
	a.userID = session.User.ID
	a.wsToken = session.LastActiveToken.JWT

	a.parseJWTExpiry(a.wsToken)

	return nil
}

// RefreshToken gets a fresh token using session id
func (a *OrchidsAuthHandle) RefreshToken() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.clerkSessionID == "" {
		return errors.New("no session id available for refresh")
	}

	url := fmt.Sprintf(orchidsClerkTokenURL, a.clerkSessionID) + "?_clerk_js_version=" + orchidsClerkJSVersion
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return err
	}

	// Use just __client cookie for auth
	req.Header.Set("Cookie", fmt.Sprintf("__client=%s", a.clientJWT))
	req.Header.Set("Origin", orchidsOrigin)
	req.Header.Set("User-Agent", orchidsUserAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token refresh failed %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		JWT string `json:"jwt"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return err
	}

	if tokenResp.JWT == "" {
		return errors.New("empty jwt in refresh response")
	}

	a.wsToken = tokenResp.JWT
	a.parseJWTExpiry(a.wsToken)

	// Update file asynchronously to avoid blocking
	go a.updateCredentialsFile(a.tokenExpiresAt)

	return nil
}

func (a *OrchidsAuthHandle) parseJWTExpiry(token string) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// Try standard encoding if raw fails
		payload, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return
		}
	}

	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return
	}

	if claims.Exp > 0 {
		a.tokenExpiresAt = time.Unix(claims.Exp, 0)
	}
}

func (a *OrchidsAuthHandle) updateCredentialsFile(expiresAt time.Time) {
	// Simple file lock logic could be added here if needed, but for now just atomic write
	// Read existing to preserve other fields
	data, err := os.ReadFile(a.credPath)
	if err != nil {
		return
	}

	var creds map[string]interface{}
	if err := json.Unmarshal(data, &creds); err != nil {
		return
	}

	creds["expiresAt"] = expiresAt.Format(time.RFC3339)

	newData, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return
	}

	// Write to temp file then rename for atomicity
	tmpPath := a.credPath + ".tmp"
	if err := os.WriteFile(tmpPath, newData, 0644); err != nil {
		return
	}

	os.Rename(tmpPath, a.credPath)
}

func (a *OrchidsAuthHandle) GetWSToken() (string, error) {
	a.mu.Lock()
	// Check expiry with 1 minute buffer
	if time.Now().Add(1 * time.Minute).After(a.tokenExpiresAt) {
		a.mu.Unlock() // Refresh needs lock
		if err := a.RefreshToken(); err != nil {
			return "", err
		}
		a.mu.Lock()
	}
	defer a.mu.Unlock()
	return a.wsToken, nil
}
