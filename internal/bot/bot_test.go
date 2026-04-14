package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/l13t/zm-tg-notifier/internal/config"
)

// mockTelegramServer creates an httptest.Server that mimics the Telegram Bot API.
// It records received API method calls and request bodies for assertions.
type mockTelegramServer struct {
	server   *httptest.Server
	mu       sync.Mutex
	methods  []string
	bodies   []map[string]interface{}
	apiURL   string
}

func newMockTelegramServer(t *testing.T) *mockTelegramServer {
	t.Helper()
	m := &mockTelegramServer{}

	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")
		method := parts[len(parts)-1]

		// Parse form body
		_ = r.ParseForm()
		body := make(map[string]interface{})
		for k, v := range r.Form {
			if len(v) == 1 {
				body[k] = v[0]
			} else {
				body[k] = v
			}
		}

		m.mu.Lock()
		m.methods = append(m.methods, method)
		m.bodies = append(m.bodies, body)
		m.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")

		switch method {
		case "getMe":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"result": map[string]interface{}{
					"id":         12345,
					"is_bot":     true,
					"first_name": "TestBot",
					"username":   "test_bot",
				},
			})
		case "getUpdates":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":     true,
				"result": []interface{}{},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"result": map[string]interface{}{
					"message_id": 1,
					"chat":       map[string]interface{}{"id": 999},
					"date":       0,
					"text":       "ok",
				},
			})
		}
	}))

	m.apiURL = fmt.Sprintf("%s/bot%%s/%%s", m.server.URL)
	return m
}

func (m *mockTelegramServer) close() {
	m.server.Close()
}

func (m *mockTelegramServer) getMethods() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.methods))
	copy(result, m.methods)
	return result
}

func (m *mockTelegramServer) getBodies() []map[string]interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]map[string]interface{}, len(m.bodies))
	copy(result, m.bodies)
	return result
}

// newTestBot creates a Bot backed by a mock Telegram server.
func newTestBot(t *testing.T, mock *mockTelegramServer, chatIDs []int64) *Bot {
	t.Helper()

	api, err := tgbotapi.NewBotAPIWithAPIEndpoint("TEST_TOKEN", mock.apiURL)
	if err != nil {
		t.Fatalf("failed to create mock bot API: %v", err)
	}

	return &Bot{
		api:     api,
		config:  &config.Config{},
		chatIDs: chatIDs,
	}
}

func TestNew_Success(t *testing.T) {
	mock := newMockTelegramServer(t)
	defer mock.close()

	cfg := &config.Config{
		Token:      "TEST_TOKEN",
		ChatIDsEnv: "111,222",
	}

	// Override the default API endpoint by creating the bot through the API directly
	api, err := tgbotapi.NewBotAPIWithAPIEndpoint(cfg.Token, mock.apiURL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if api.Self.UserName != "test_bot" {
		t.Errorf("expected username 'test_bot', got %q", api.Self.UserName)
	}
}

func TestNew_InvalidToken(t *testing.T) {
	// Use a server that returns an error for getMe
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":          false,
			"description": "Unauthorized",
			"error_code":  401,
		})
	}))
	defer server.Close()

	apiEndpoint := fmt.Sprintf("%s/bot%%s/%%s", server.URL)
	_, err := tgbotapi.NewBotAPIWithAPIEndpoint("BAD_TOKEN", apiEndpoint)
	if err == nil {
		t.Error("expected error for invalid token, got nil")
	}
}

func TestBot_API(t *testing.T) {
	mock := newMockTelegramServer(t)
	defer mock.close()

	b := newTestBot(t, mock, []int64{111})
	if b.API() == nil {
		t.Error("API() returned nil")
	}
}

func TestBot_ChatIDs(t *testing.T) {
	mock := newMockTelegramServer(t)
	defer mock.close()

	expected := []int64{111, 222, 333}
	b := newTestBot(t, mock, expected)

	got := b.ChatIDs()
	if len(got) != len(expected) {
		t.Fatalf("ChatIDs() length = %d, want %d", len(got), len(expected))
	}
	for i, id := range got {
		if id != expected[i] {
			t.Errorf("ChatIDs()[%d] = %d, want %d", i, id, expected[i])
		}
	}
}

func TestBot_IsInChatIDs(t *testing.T) {
	mock := newMockTelegramServer(t)
	defer mock.close()

	b := newTestBot(t, mock, []int64{111, 222})

	if !b.isInChatIDs(111) {
		t.Error("expected 111 to be in chat IDs")
	}
	if !b.isInChatIDs(222) {
		t.Error("expected 222 to be in chat IDs")
	}
	if b.isInChatIDs(999) {
		t.Error("expected 999 to NOT be in chat IDs")
	}
}

func TestBot_HandleStart(t *testing.T) {
	mock := newMockTelegramServer(t)
	defer mock.close()

	b := newTestBot(t, mock, []int64{111})

	msg := &tgbotapi.Message{
		MessageID: 42,
		Chat:      &tgbotapi.Chat{ID: 111},
		Text:      "/start",
		Entities: []tgbotapi.MessageEntity{
			{Type: "bot_command", Offset: 0, Length: 6},
		},
	}

	b.handleStart(msg)

	methods := mock.getMethods()
	found := false
	for _, m := range methods {
		if m == "sendMessage" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected sendMessage call, got methods: %v", methods)
	}

	bodies := mock.getBodies()
	for _, body := range bodies {
		if text, ok := body["text"].(string); ok {
			if strings.Contains(text, "111") {
				return // success - found chat ID in response
			}
		}
	}
	t.Error("expected response to contain the chat ID")
}

func TestBot_HandleChatIDs(t *testing.T) {
	mock := newMockTelegramServer(t)
	defer mock.close()

	b := newTestBot(t, mock, []int64{111, 222})

	msg := &tgbotapi.Message{
		MessageID: 43,
		Chat:      &tgbotapi.Chat{ID: 111},
		Text:      "/chat_ids",
		Entities: []tgbotapi.MessageEntity{
			{Type: "bot_command", Offset: 0, Length: 9},
		},
	}

	b.handleChatIDs(msg)

	bodies := mock.getBodies()
	for _, body := range bodies {
		if text, ok := body["text"].(string); ok {
			if strings.Contains(text, "111") && strings.Contains(text, "222") {
				return // success
			}
		}
	}
	t.Error("expected response to contain all chat IDs")
}

func TestBot_HandleMessage_RoutesCommands(t *testing.T) {
	mock := newMockTelegramServer(t)
	defer mock.close()

	b := newTestBot(t, mock, []int64{111})

	// Non-command message should be ignored
	nonCmd := &tgbotapi.Message{
		MessageID: 1,
		Chat:      &tgbotapi.Chat{ID: 111},
		Text:      "hello",
	}
	b.handleMessage(nonCmd)

	// Only getMe should have been called (from setup)
	methods := mock.getMethods()
	for _, m := range methods {
		if m == "sendMessage" {
			t.Error("non-command message should not trigger sendMessage")
		}
	}
}

func TestBot_HandleMessage_ChatIDsUnauthorized(t *testing.T) {
	mock := newMockTelegramServer(t)
	defer mock.close()

	b := newTestBot(t, mock, []int64{111})

	// /chat_ids from unauthorized user should be ignored
	msg := &tgbotapi.Message{
		MessageID: 1,
		Chat:      &tgbotapi.Chat{ID: 999},
		Text:      "/chat_ids",
		Entities: []tgbotapi.MessageEntity{
			{Type: "bot_command", Offset: 0, Length: 9},
		},
	}

	initialMethods := len(mock.getMethods())
	b.handleMessage(msg)

	if len(mock.getMethods()) != initialMethods {
		t.Error("unauthorized /chat_ids should not trigger any API call")
	}
}

func TestBot_Start_ContextCancellation(t *testing.T) {
	mock := newMockTelegramServer(t)
	defer mock.close()

	b := newTestBot(t, mock, []int64{111})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- b.Start(ctx)
	}()

	// Give Start time to begin polling
	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Start() returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start() did not return after context cancellation")
	}
}
