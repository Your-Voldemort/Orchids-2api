package provider

import (
	"orchids-api/internal/config"
	"orchids-api/internal/orchids"
	"orchids-api/internal/store"
)

type orchidsProvider struct{}

func NewOrchidsProvider() Provider { return orchidsProvider{} }

func (orchidsProvider) Name() string { return "orchids" }

func (orchidsProvider) NewClient(acc *store.Account, cfg *config.Config) interface{} {
	return orchids.NewFromAccount(acc, cfg)
}
