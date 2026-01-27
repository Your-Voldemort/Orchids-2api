package loadbalancer

import (
	"errors"
	"math/rand"
	"sync"

	"orchids-api/internal/store"
)

type LoadBalancer struct {
	store *store.Store
	mu    sync.RWMutex
}

func New(s *store.Store) *LoadBalancer {
	return &LoadBalancer{
		store: s,
	}
}

func (lb *LoadBalancer) GetNextAccount() (*store.Account, error) {
	return lb.GetNextAccountExcluding(nil)
}

func (lb *LoadBalancer) GetNextAccountExcluding(excludeIDs []int64) (*store.Account, error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	accounts, err := lb.store.GetEnabledAccounts()
	if err != nil {
		return nil, err
	}

	if len(excludeIDs) > 0 {
		excludeSet := make(map[int64]bool)
		for _, id := range excludeIDs {
			excludeSet[id] = true
		}
		var filtered []*store.Account
		for _, acc := range accounts {
			if !excludeSet[acc.ID] {
				filtered = append(filtered, acc)
			}
		}
		accounts = filtered
	}

	if len(accounts) == 0 {
		return nil, errors.New("no enabled accounts available")
	}

	account := lb.selectAccount(accounts)

	if err := lb.store.IncrementRequestCount(account.ID); err != nil {
		return nil, err
	}

	return account, nil
}

func (lb *LoadBalancer) selectAccount(accounts []*store.Account) *store.Account {
	if len(accounts) == 1 {
		return accounts[0]
	}

	var totalWeight int
	for _, acc := range accounts {
		totalWeight += acc.Weight
	}

	randomWeight := rand.Intn(totalWeight)
	currentWeight := 0

	for _, acc := range accounts {
		currentWeight += acc.Weight
		if currentWeight > randomWeight {
			return acc
		}
	}

	return accounts[0]
}
