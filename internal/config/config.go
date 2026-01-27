package config

import (
	"os"
)

type Config struct {
	Port         string
	DebugEnabled bool
	SessionID    string
	ClientCookie string
	ClientUat    string
	ProjectID    string
	UserID       string
	AgentMode    string
	Email        string
	AdminUser    string
	AdminPass    string
	AdminPath    string
}

func Load() *Config {
	return &Config{
		Port:         getEnv("PORT", "3002"),
		DebugEnabled: getEnv("DEBUG_ENABLED", "true") == "true",
		SessionID:    getEnv("SESSION_ID", "***REMOVED-ENV-DEFAULT-SESSION_ID***"),
		ClientCookie: getEnv("CLIENT_COOKIE", "***REMOVED-ENV-DEFAULT-CLIENT_COOKIE***"),
		ClientUat:    getEnv("CLIENT_UAT", "1768272707"),
		ProjectID:    getEnv("PROJECT_ID", "***REMOVED-ENV-DEFAULT-PROJECT_ID***"),
		UserID:       getEnv("USER_ID", "***REMOVED-ENV-DEFAULT-USER_ID***"),
		AgentMode:    getEnv("AGENT_MODE", "claude-opus-4.5"),
		Email:        getEnv("EMAIL", "***REMOVED-ENV-DEFAULT-EMAIL***"),
		AdminUser:    getEnv("ADMIN_USER", "admin"),
		AdminPass:    getEnv("ADMIN_PASS", "admin123"),
		AdminPath:    getEnv("ADMIN_PATH", "/admin"),
	}
}

func (c *Config) GetCookies() string {
	return "__client=" + c.ClientCookie + "; __client_uat=" + c.ClientUat
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
