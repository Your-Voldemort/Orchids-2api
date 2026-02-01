package warp

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type session struct {
	mu             sync.Mutex
	jwt            string
	expiresAt      time.Time
	refreshToken   string
	loggedIn       bool
	lastLogin      time.Time
	clientVersion  string
	osCategory     string
	osName         string
	osVersion      string
	experimentID   string
	experimentBuck string
}

type refreshResponse struct {
	AccessToken  string `json:"access_token"`
	IDToken      string `json:"idToken"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	ExpiresInAlt int    `json:"expiresIn"`
	RefreshAlt   string `json:"refreshToken"`
}

var sessionCache sync.Map

func sessionKey(accountID int64, refreshToken string) string {
	if accountID > 0 {
		return fmt.Sprintf("warp:%d", accountID)
	}
	if refreshToken == "" {
		return "warp:anon"
	}
	if len(refreshToken) > 16 {
		return "warp:tok:" + refreshToken[:16]
	}
	return "warp:tok:" + refreshToken
}

func getSession(accountID int64, refreshToken string) *session {
	key := sessionKey(accountID, refreshToken)
	if val, ok := sessionCache.Load(key); ok {
		sess := val.(*session)
		sess.mu.Lock()
		if refreshToken != "" && sess.refreshToken != refreshToken {
			sess.refreshToken = refreshToken
		}
		sess.mu.Unlock()
		return sess
	}
	sess := &session{
		refreshToken:  refreshToken,
		clientVersion: clientVersion,
		osCategory:    osCategory,
		osName:        osName,
		osVersion:     osVersion,
	}
	sessionCache.Store(key, sess)
	return sess
}

func (s *session) tokenValid() bool {
	if s.jwt == "" || s.expiresAt.IsZero() {
		return false
	}
	return time.Now().Add(10 * time.Minute).Before(s.expiresAt)
}

func (s *session) ensureToken(ctx context.Context, httpClient *http.Client) error {
	s.mu.Lock()
	if s.tokenValid() {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	return s.refreshTokenRequest(ctx, httpClient)
}

func (s *session) refreshTokenRequest(ctx context.Context, httpClient *http.Client) error {
	s.mu.Lock()
	refreshToken := strings.TrimSpace(s.refreshToken)
	s.mu.Unlock()

	payload := []byte{}
	if refreshToken != "" {
		payload = []byte("grant_type=refresh_token&refresh_token=" + refreshToken)
	} else {
		decoded, err := base64.StdEncoding.DecodeString(refreshTokenB64)
		if err != nil {
			return fmt.Errorf("decode built-in refresh token: %w", err)
		}
		payload = decoded
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, refreshURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("x-warp-client-version", clientVersion)
	req.Header.Set("x-warp-os-category", osCategory)
	req.Header.Set("x-warp-os-name", osName)
	req.Header.Set("x-warp-os-version", osVersion)
	req.Header.Set("content-type", "application/x-www-form-urlencoded")
	req.Header.Set("accept", "*/*")
	req.Header.Set("accept-encoding", "gzip, br")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("warp refresh token failed: HTTP %d", resp.StatusCode)
	}

	var parsed refreshResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return err
	}

	accessToken := parsed.AccessToken
	if accessToken == "" {
		accessToken = parsed.IDToken
	}
	if accessToken == "" {
		return fmt.Errorf("warp refresh token response missing access token")
	}

	expiresIn := parsed.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = parsed.ExpiresInAlt
	}
	if expiresIn <= 0 {
		expiresIn = 3600
	}

	newRefresh := parsed.RefreshToken
	if newRefresh == "" {
		newRefresh = parsed.RefreshAlt
	}

	s.mu.Lock()
	s.jwt = accessToken
	s.expiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second)
	if newRefresh != "" {
		s.refreshToken = newRefresh
	}
	s.mu.Unlock()

	return nil
}

func (s *session) ensureLogin(ctx context.Context, httpClient *http.Client) error {
	s.mu.Lock()
	if s.loggedIn && time.Since(s.lastLogin) < 30*time.Minute {
		s.mu.Unlock()
		return nil
	}
	jwt := s.jwt
	if s.experimentID == "" {
		s.experimentID = newUUID()
	}
	if s.experimentBuck == "" {
		s.experimentBuck = newExperimentBucket()
	}
	experimentID := s.experimentID
	experimentBucket := s.experimentBuck
	s.mu.Unlock()

	if jwt == "" {
		return fmt.Errorf("missing jwt")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("x-warp-client-id", clientID)
	req.Header.Set("x-warp-client-version", clientVersion)
	req.Header.Set("x-warp-os-category", osCategory)
	req.Header.Set("x-warp-os-name", osName)
	req.Header.Set("x-warp-os-version", osVersion)
	req.Header.Set("authorization", "Bearer "+jwt)
	req.Header.Set("x-warp-experiment-id", experimentID)
	req.Header.Set("x-warp-experiment-bucket", experimentBucket)
	req.Header.Set("accept", "*/*")
	req.Header.Set("accept-encoding", "gzip, br")
	req.Header.Set("content-length", "0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("warp login failed: HTTP %d", resp.StatusCode)
	}

	s.mu.Lock()
	s.loggedIn = true
	s.lastLogin = time.Now()
	s.mu.Unlock()
	return nil
}

func newUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func newExperimentBucket() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (s *session) currentJWT() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.jwt
}

func (s *session) currentRefreshToken() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.refreshToken
}
