package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

type Config struct {
	Token        string
	ZMFolder     string
	ZMURL        string
	LogLevel     string
	FileChatPath string
	ChatIDsEnv   string
}

// LoadWithFlags loads configuration from flags (if provided) or environment variables
// Flags take precedence over environment variables
func LoadWithFlags(token, zmFolder, zmURL, logLevel, fileChatPath, chatIDs *string) *Config {
	return &Config{
		Token:        getConfigValue(token, "TOKEN", ""),
		ZMFolder:     getConfigValue(zmFolder, "ZM_FOLDER", "/zm"),
		ZMURL:        getConfigValue(zmURL, "ZM_URL", ""),
		LogLevel:     getConfigValue(logLevel, "LOG_LEVEL", "INFO"),
		FileChatPath: getConfigValue(fileChatPath, "FILE_CHAT_PATH", "chat_id"),
		ChatIDsEnv:   getConfigValue(chatIDs, "CHAT_IDS", ""),
	}
}

// Deprecated: Use LoadWithFlags instead
func Load() *Config {
	return &Config{
		Token:        getEnv("TOKEN", ""),
		ZMFolder:     getEnv("ZM_FOLDER", "/zm"),
		ZMURL:        getEnv("ZM_URL", ""),
		LogLevel:     getEnv("LOG_LEVEL", "INFO"),
		FileChatPath: getEnv("FILE_CHAT_PATH", "chat_id"),
		ChatIDsEnv:   getEnv("CHAT_IDS", ""),
	}
}

// getConfigValue returns the flag value if set, otherwise falls back to environment variable
func getConfigValue(flagValue *string, envKey, defaultValue string) string {
	// If flag is provided and not empty, use it
	if flagValue != nil && *flagValue != "" {
		return *flagValue
	}
	// Otherwise use environment variable or default
	return getEnv(envKey, defaultValue)
}

func (c *Config) GetChatIDs() []int64 {
	var chatIDs []int64

	// Try environment variable first
	if c.ChatIDsEnv != "" {
		for _, idStr := range strings.Split(c.ChatIDsEnv, ",") {
			idStr = strings.TrimSpace(idStr)
			if idStr == "" {
				continue
			}
			var id int64
			if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
				slog.Warn("invalid chat ID", "value", idStr)
				continue
			}
			chatIDs = append(chatIDs, id)
		}
		return chatIDs
	}

	// Try file
	if c.FileChatPath != "" {
		data, err := os.ReadFile(c.FileChatPath)
		if err != nil {
			slog.Warn("failed to read chat IDs from file", "path", c.FileChatPath, "error", err)
			return chatIDs
		}

		content := strings.TrimSpace(string(data))
		for _, idStr := range strings.Split(content, ",") {
			idStr = strings.TrimSpace(idStr)
			if idStr == "" {
				continue
			}
			var id int64
			if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
				slog.Warn("invalid chat ID in file", "value", idStr)
				continue
			}
			chatIDs = append(chatIDs, id)
		}
	}

	return chatIDs
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
