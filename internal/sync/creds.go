package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"orchids-api/internal/store"
)

type OrchidsCredentials struct {
	ClientJWT string `json:"clientJwt"`
}

// SyncCredentials exports a valid session from the database to the local credentials file
// if the file is missing or invalid.
func SyncCredentials(db *store.Store, path string) error {
	if path == "" {
		return nil
	}

	// 1. Check if file exists and acts as a valid fallback
	if valid, _ := isCredsFileValid(path); valid {
		// File exists and has content, trusting user/previous sync
		slog.Debug("Credentials file valid, skipping sync", "path", path)
		return nil
	}

	slog.Info("Credentials file missing or invalid, attempting to sync from DB", "path", path)

	// 2. Fetch enabled accounts
	ctx := context.Background()
	accounts, err := db.GetEnabledAccounts(ctx)
	if err != nil {
		return fmt.Errorf("failed to list database accounts: %w", err)
	}

	if len(accounts) == 0 {
		return fmt.Errorf("no enabled accounts in database to sync")
	}

	// 3. Pick a candidate
	// We prefer the account with the most recent usage or highest weight,
	// checking if it has a valid ClientCookie.
	var bestAccount *store.Account
	for _, acc := range accounts {
		if strings.TrimSpace(acc.ClientCookie) == "" {
			continue
		}
		// Logic: just pick the first valid one implies "default" behavior
		// Since loadbalancer logic also picks from enabled accounts, any valid one is better than none.
		bestAccount = acc
		break
	}

	if bestAccount == nil {
		return fmt.Errorf("no database accounts have valid client cookies")
	}

	// 4. Write to file
	creds := OrchidsCredentials{
		ClientJWT: bestAccount.ClientCookie,
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal creds: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write credentials file: %w", err)
	}

	slog.Info("Credentials successfully synced from database", "account", bestAccount.Name, "path", path)
	return nil
}

func isCredsFileValid(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}

	var creds OrchidsCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return false, err
	}

	if strings.TrimSpace(creds.ClientJWT) == "" {
		return false, nil
	}

	return true, nil
}
