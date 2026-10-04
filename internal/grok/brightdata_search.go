package grok

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"orchids-api/internal/config"
	"os"
	"strings"
	"time"
)

type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

type BridgeSearch interface {
	Search(context.Context, string) ([]SearchResult, error)
}

// Credentials stay outside public runtime configuration and diagnostic payloads.
type BrightDataSearch struct {
	key, zone string
	client    *http.Client
}

// SeedBrightDataConfig imports legacy private/environment settings only when
// no explicit runtime search setting exists. Saved settings win on restart.
func SeedBrightDataConfig(cfg *config.Config) error {
	if cfg == nil || cfg.BrightDataEnabled != nil {
		return nil
	}
	if cfg.BrightDataAPIKey != "" || cfg.BrightDataZone != "" {
		enabled := cfg.BrightDataAPIKey != "" && cfg.BrightDataZone != ""
		cfg.BrightDataEnabled = &enabled
		return nil
	}
	b, err := LoadBrightDataSearch()
	if err != nil {
		return err
	}
	if b != nil {
		enabled := true
		cfg.BrightDataEnabled = &enabled
		cfg.BrightDataAPIKey = b.key
		cfg.BrightDataZone = b.zone
	}
	return nil
}

func BrightDataSearchFromConfig(cfg *config.Config) BridgeSearch {
	if cfg == nil || cfg.BrightDataEnabled == nil || !*cfg.BrightDataEnabled || cfg.BrightDataAPIKey == "" || cfg.BrightDataZone == "" {
		return nil
	}
	return &BrightDataSearch{key: cfg.BrightDataAPIKey, zone: cfg.BrightDataZone, client: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func LoadBrightDataSearch() (*BrightDataSearch, error) {
	key, zone := strings.TrimSpace(os.Getenv("BRIGHTDATA_API_KEY")), strings.TrimSpace(os.Getenv("BRIGHTDATA_ZONE"))
	if key == "" && zone == "" {
		path := os.Getenv("BRIGHTDATA_CONFIG_FILE")
		explicit := path != ""
		if !explicit {
			path = ".tools/brightdata.local.json"
		}
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) && !explicit {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("cannot read Bright Data private configuration")
		}
		var cfg struct {
			Key  string `json:"api_key"`
			Zone string `json:"zone"`
		}
		if json.Unmarshal(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf}), &cfg) != nil {
			return nil, fmt.Errorf("invalid Bright Data private configuration")
		}
		key, zone = strings.TrimSpace(cfg.Key), strings.TrimSpace(cfg.Zone)
	}
	if key == "" || zone == "" {
		return nil, fmt.Errorf("Bright Data requires both API key and SERP zone")
	}
	return &BrightDataSearch{key: key, zone: zone, client: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (b *BrightDataSearch) Search(ctx context.Context, query string) ([]SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" || len(query) > 1000 {
		return nil, fmt.Errorf("search keywords must contain 1 to 1000 bytes")
	}
	values := url.Values{"q": {query}, "brd_json": {"1"}}
	data, _ := json.Marshal(map[string]string{"zone": b.zone, "url": "https://www.google.com/search?" + values.Encode(), "format": "raw"})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.brightdata.com/request", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("cannot build search request")
	}
	req.Header.Set("Authorization", "Bearer "+b.key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Bright Data search transport failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Bright Data search returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if err != nil || len(body) > 2<<20 {
		return nil, fmt.Errorf("Bright Data search response is unreadable or too large")
	}
	var parsed struct {
		Organic []struct {
			Title       string `json:"title"`
			Link        string `json:"link"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"organic"`
	}
	if json.Unmarshal(body, &parsed) != nil {
		return nil, fmt.Errorf("Bright Data returned invalid search JSON")
	}
	if parsed.Organic == nil {
		return nil, fmt.Errorf("Bright Data response is missing organic results")
	}
	results := make([]SearchResult, 0, 8)
	for _, item := range parsed.Organic {
		link := item.Link
		if link == "" {
			link = item.URL
		}
		u, err := url.Parse(link)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
			continue
		}
		results = append(results, SearchResult{Title: limitSearchText(item.Title, 512), URL: link, Snippet: limitSearchText(item.Description, 2000)})
		if len(results) == 8 {
			break
		}
	}
	return results, nil
}

func limitSearchText(value string, size int) string {
	r := []rune(value)
	if len(r) > size {
		return string(r[:size])
	}
	return value
}
