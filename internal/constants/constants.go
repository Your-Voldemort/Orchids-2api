// Package constants defines shared constants across the application.
package constants

import "time"

// Buffer sizes
const (
	DefaultBufferSize    = 4096
	LargeBufferSize      = 32768
	MaxDescriptionLength = 9216
	MaxToolInputLength   = 65536
	MaxRequestBodySize   = 32 * 1024 * 1024 // 32MB
)

// Concurrency thresholds
const (
	ParallelThreshold        = 8   // Min items for parallel processing
	LRUUpdateRatio           = 16  // 1/16 probability for LRU updates
	DefaultConcurrencyLimit  = 100 // Max concurrent requests
)

// Timeouts
const (
	DefaultTimeout         = 30 * time.Second
	UpstreamTimeout        = 120 * time.Second
	ShutdownTimeout        = 30 * time.Second
	SessionCleanupPeriod   = 1 * time.Hour
	ReadHeaderTimeout      = 10 * time.Second
	ReadTimeout            = 30 * time.Second
	IdleTimeout            = 60 * time.Second
	KeepAliveInterval      = 15 * time.Second
	DefaultRetryDelay      = 1 * time.Second
	DefaultConcurrencyWait = 120 * time.Second
)

// Token estimation
const (
	TokensPerWord         = 1.3
	TokensPerCJK          = 1.5
	DefaultMaxTokens      = 4096
	DefaultContextTokens  = 8000
	DefaultSummaryTokens  = 800
)

// Cache settings
const (
	DefaultCacheTTL       = 5 * time.Second
	SummaryCacheTTL       = 30 * time.Minute
	TokenCacheTTL         = 5 * time.Minute
	LoadBalancerCacheTTL  = 5 * time.Second
	DefaultSummaryCacheSize = 256
)

// WebSocket settings
const (
	WSConnectTimeout   = 30 * time.Second
	WSReadTimeout      = 120 * time.Second
	WSWriteTimeout     = 10 * time.Second
	WSPingInterval     = 30 * time.Second
	WSMaxMessageSize   = 10 * 1024 * 1024 // 10MB
)

// Retry settings
const (
	DefaultMaxRetries    = 3
	DefaultRetryDelayMS  = 1000
	AccountSwitchCount   = 5
)

// Context settings
const (
	DefaultKeepTurns         = 6
	MaxToolFollowups         = 1
	DefaultTokenRefreshMins  = 30
)

// Admin defaults
const (
	DefaultPort      = "3002"
	DefaultAdminUser = "admin"
	DefaultAdminPass = "admin123"
	DefaultAdminPath = "/admin"
)

// Store defaults
const (
	DefaultRedisPrefix        = "orchids:"
	DefaultSummaryCachePrefix = "orchids:summary:"
)

// Upstream defaults
const (
	DefaultOrchidsAPIBaseURL = "https://orchids-server.calmstone-6964e08a.westeurope.azurecontainerapps.io"
	DefaultOrchidsWSURL      = "wss://orchids-v2-alpha-108292236521.europe-west1.run.app/agent/ws/coding-agent"
)
