package main

import (
	"context"
	"fmt"
	"os"

	"orchids-api/internal/config"
	"orchids-api/internal/model"
	"orchids-api/internal/store"
)

func main() {
	cfg, _, err := config.Load("")
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	s, err := store.New(store.Options{
		StoreMode:     cfg.StoreMode,
		RedisAddr:     cfg.RedisAddr,
		RedisPassword: cfg.RedisPassword,
		RedisDB:       cfg.RedisDB,
		RedisPrefix:   cfg.RedisPrefix,
	})
	if err != nil {
		fmt.Printf("Failed to init store: %v\n", err)
		os.Exit(1)
	}
	defer s.Close()

	ctx := context.Background()
	models := []model.Model{
		{ID: "6", Channel: "Orchids", ModelID: "claude-sonnet-4-5", Name: "Claude Sonnet 4.5", Status: true, IsDefault: true, SortOrder: 0},
		{ID: "7", Channel: "Orchids", ModelID: "claude-opus-4-5", Name: "Claude Opus 4.5", Status: true, IsDefault: false, SortOrder: 1},
		{ID: "42", Channel: "Orchids", ModelID: "claude-sonnet-4-5-thinking", Name: "Claude Sonnet 4.5 Thinking", Status: true, IsDefault: false, SortOrder: 1},
		{ID: "8", Channel: "Orchids", ModelID: "claude-haiku-4-5", Name: "Claude Haiku 4.5", Status: true, IsDefault: false, SortOrder: 2},
		{ID: "9", Channel: "Orchids", ModelID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4", Status: true, IsDefault: false, SortOrder: 3},
		{ID: "43", Channel: "Orchids", ModelID: "claude-opus-4-5-thinking", Name: "Claude Opus 4.5 Thinking", Status: true, IsDefault: false, SortOrder: 3},
		{ID: "10", Channel: "Orchids", ModelID: "claude-3-7-sonnet-20250219", Name: "Claude 3.7 Sonnet", Status: true, IsDefault: false, SortOrder: 4},
	}

	for _, m := range models {
		existing, err := s.GetModelByModelID(ctx, m.ModelID)
		if err == nil {
			// Update existing
			m.ID = existing.ID
			if err := s.UpdateModel(ctx, &m); err != nil {
				fmt.Printf("Failed to update model %s: %v\n", m.ModelID, err)
			} else {
				fmt.Printf("Updated model %s\n", m.ModelID)
			}
		} else {
			// Create new
			if err := s.CreateModel(ctx, &m); err != nil {
				fmt.Printf("Failed to create model %s: %v\n", m.ModelID, err)
			} else {
				fmt.Printf("Created model %s\n", m.ModelID)
			}
		}
	}
	fmt.Println("Seeding complete.")
}
