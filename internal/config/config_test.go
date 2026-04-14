package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadWithFlags_FlagsTakePrecedence(t *testing.T) {
	t.Setenv("TOKEN", "env-token")
	t.Setenv("ZM_FOLDER", "/env/zm")

	flagToken := "flag-token"
	flagFolder := "/flag/zm"
	empty := ""

	cfg := LoadWithFlags(&flagToken, &flagFolder, &empty, &empty, &empty, &empty)

	if cfg.Token != "flag-token" {
		t.Errorf("expected flag token, got %q", cfg.Token)
	}
	if cfg.ZMFolder != "/flag/zm" {
		t.Errorf("expected flag folder, got %q", cfg.ZMFolder)
	}
}

func TestLoadWithFlags_FallbackToEnv(t *testing.T) {
	t.Setenv("TOKEN", "env-token")
	t.Setenv("ZM_URL", "http://zm.local")

	empty := ""
	cfg := LoadWithFlags(&empty, &empty, &empty, &empty, &empty, &empty)

	if cfg.Token != "env-token" {
		t.Errorf("expected env token, got %q", cfg.Token)
	}
	if cfg.ZMURL != "http://zm.local" {
		t.Errorf("expected env ZM_URL, got %q", cfg.ZMURL)
	}
}

func TestLoadWithFlags_Defaults(t *testing.T) {
	// Clear all relevant env vars
	t.Setenv("TOKEN", "")
	t.Setenv("ZM_FOLDER", "")
	t.Setenv("ZM_URL", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("FILE_CHAT_PATH", "")
	t.Setenv("CHAT_IDS", "")

	cfg := LoadWithFlags(nil, nil, nil, nil, nil, nil)

	if cfg.ZMFolder != "/zm" {
		t.Errorf("expected default ZM_FOLDER '/zm', got %q", cfg.ZMFolder)
	}
	if cfg.LogLevel != "INFO" {
		t.Errorf("expected default LogLevel 'INFO', got %q", cfg.LogLevel)
	}
	if cfg.FileChatPath != "chat_id" {
		t.Errorf("expected default FileChatPath 'chat_id', got %q", cfg.FileChatPath)
	}
}

func TestLoadWithFlags_NilFlags(t *testing.T) {
	t.Setenv("TOKEN", "from-env")
	t.Setenv("ZM_FOLDER", "")

	cfg := LoadWithFlags(nil, nil, nil, nil, nil, nil)

	if cfg.Token != "from-env" {
		t.Errorf("expected env token with nil flag, got %q", cfg.Token)
	}
}

func TestGetChatIDs_FromEnvVar(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []int64
	}{
		{"single ID", "123", []int64{123}},
		{"multiple IDs", "111,222,333", []int64{111, 222, 333}},
		{"with spaces", " 111 , 222 , 333 ", []int64{111, 222, 333}},
		{"empty string", "", nil},
		{"trailing comma", "111,222,", []int64{111, 222}},
		{"negative ID", "-100123", []int64{-100123}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{ChatIDsEnv: tt.input}
			got := cfg.GetChatIDs()
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("GetChatIDs() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetChatIDs_InvalidValues(t *testing.T) {
	cfg := &Config{ChatIDsEnv: "111,abc,333"}
	got := cfg.GetChatIDs()
	expected := []int64{111, 333}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("expected invalid IDs to be skipped, got %v, want %v", got, expected)
	}
}

func TestGetChatIDs_FromFile(t *testing.T) {
	tmpDir := t.TempDir()
	chatFile := filepath.Join(tmpDir, "chat_ids")

	if err := os.WriteFile(chatFile, []byte("100,200,300"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{FileChatPath: chatFile}
	got := cfg.GetChatIDs()
	expected := []int64{100, 200, 300}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("GetChatIDs() from file = %v, want %v", got, expected)
	}
}

func TestGetChatIDs_FromFileWithWhitespace(t *testing.T) {
	tmpDir := t.TempDir()
	chatFile := filepath.Join(tmpDir, "chat_ids")

	if err := os.WriteFile(chatFile, []byte("  100 , 200 \n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{FileChatPath: chatFile}
	got := cfg.GetChatIDs()
	expected := []int64{100, 200}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("GetChatIDs() from file = %v, want %v", got, expected)
	}
}

func TestGetChatIDs_FileMissing(t *testing.T) {
	cfg := &Config{FileChatPath: "/nonexistent/chat_ids"}
	got := cfg.GetChatIDs()
	if len(got) != 0 {
		t.Errorf("expected empty slice for missing file, got %v", got)
	}
}

func TestGetChatIDs_EnvTakesPrecedenceOverFile(t *testing.T) {
	tmpDir := t.TempDir()
	chatFile := filepath.Join(tmpDir, "chat_ids")
	if err := os.WriteFile(chatFile, []byte("999"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		ChatIDsEnv:   "111",
		FileChatPath: chatFile,
	}
	got := cfg.GetChatIDs()
	expected := []int64{111}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("env should take precedence, got %v, want %v", got, expected)
	}
}

func TestGetChatIDs_EmptyConfig(t *testing.T) {
	cfg := &Config{}
	got := cfg.GetChatIDs()
	if len(got) != 0 {
		t.Errorf("expected empty slice for empty config, got %v", got)
	}
}

func TestLoad_UsesEnvVars(t *testing.T) {
	t.Setenv("TOKEN", "test-token")
	t.Setenv("ZM_FOLDER", "/test/zm")
	t.Setenv("ZM_URL", "http://test.local")
	t.Setenv("LOG_LEVEL", "DEBUG")
	t.Setenv("FILE_CHAT_PATH", "/tmp/chats")
	t.Setenv("CHAT_IDS", "1,2,3")

	cfg := Load()

	if cfg.Token != "test-token" {
		t.Errorf("Token = %q, want %q", cfg.Token, "test-token")
	}
	if cfg.ZMFolder != "/test/zm" {
		t.Errorf("ZMFolder = %q, want %q", cfg.ZMFolder, "/test/zm")
	}
	if cfg.ZMURL != "http://test.local" {
		t.Errorf("ZMURL = %q, want %q", cfg.ZMURL, "http://test.local")
	}
	if cfg.LogLevel != "DEBUG" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "DEBUG")
	}
	if cfg.FileChatPath != "/tmp/chats" {
		t.Errorf("FileChatPath = %q, want %q", cfg.FileChatPath, "/tmp/chats")
	}
	if cfg.ChatIDsEnv != "1,2,3" {
		t.Errorf("ChatIDsEnv = %q, want %q", cfg.ChatIDsEnv, "1,2,3")
	}
}
