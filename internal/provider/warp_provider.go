package provider

import (
	"orchids-api/internal/config"
	"orchids-api/internal/store"
	"orchids-api/internal/warp"
)

type warpProvider struct{}

func NewWarpProvider() Provider { return warpProvider{} }

func (warpProvider) Name() string { return "warp" }

func (warpProvider) NewClient(acc *store.Account, cfg *config.Config) interface{} {
	return warp.NewFromAccount(acc, cfg)
}
