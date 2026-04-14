package watcher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestVideoRegex(t *testing.T) {
	tests := []struct {
		path      string
		shouldMatch bool
		eventID   string
	}{
		{"/zm/1/1705-video.mp4", true, "1705"},
		{"/zm/1/1705-video.h264.mp4", true, "1705"},
		{"/zm/1/1705-video.h265.mp4", true, "1705"},
		{"/zm/1/1705-video.vp9.mp4", true, "1705"},
		{"/zm/events/1/1705-video.mp4", true, "1705"},
		{"/zm/events/2/2345-video.h264.mp4", true, "2345"},
		{"/zm/1/1705-capture.mp4", false, ""},
		{"/zm/1/video.mp4", false, ""},
		{"/zm/1/1705-video.txt", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			matches := videoRegex.FindStringSubmatch(tt.path)
			if tt.shouldMatch {
				if matches == nil {
					t.Errorf("Expected path %s to match video regex, but it didn't", tt.path)
					return
				}
				if matches[1] != tt.eventID {
					t.Errorf("Expected eventID %s, got %s", tt.eventID, matches[1])
				}
			} else {
				if matches != nil {
					t.Errorf("Expected path %s to NOT match video regex, but it did", tt.path)
				}
			}
		})
	}
}

func TestCaptureRegex(t *testing.T) {
	tests := []struct {
		path      string
		shouldMatch bool
		eventID   string
	}{
		{"/zm/1/1705/00001-capture.jpg", true, "1705"},
		{"/zm/events/1/1705/00001-capture.jpg", true, "1705"},
		{"/zm/1/2345/00001-capture.jpg", true, "2345"},
		{"/zm/1/1705/00002-capture.jpg", false, ""},
		{"/zm/1/1705/capture.jpg", false, ""},
		{"/zm/1/1705-video.mp4", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			matches := captureRegex.FindStringSubmatch(tt.path)
			if tt.shouldMatch {
				if matches == nil {
					t.Errorf("Expected path %s to match capture regex, but it didn't", tt.path)
					return
				}
				if matches[1] != tt.eventID {
					t.Errorf("Expected eventID %s, got %s", tt.eventID, matches[1])
				}
			} else {
				if matches != nil {
					t.Errorf("Expected path %s to NOT match capture regex, but it did", tt.path)
				}
			}
		})
	}
}

// newMockBotAPI creates a BotAPI backed by a mock Telegram server.
// The returned requestLog collects all API method calls received by the mock.
func newMockBotAPI(t *testing.T) (*tgbotapi.BotAPI, *[]string, *httptest.Server) {
	t.Helper()

	var mu sync.Mutex
	var requests []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract the API method from URL path (e.g. /botTOKEN/sendMessage)
		parts := strings.Split(r.URL.Path, "/")
		method := parts[len(parts)-1]

		mu.Lock()
		requests = append(requests, method)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")

		if method == "getMe" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"result": map[string]interface{}{
					"id":         12345,
					"is_bot":     true,
					"first_name": "TestBot",
					"username":   "test_bot",
				},
			})
			return
		}

		// Default: return a successful message response
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok": true,
			"result": map[string]interface{}{
				"message_id": 1,
				"chat":       map[string]interface{}{"id": 999},
				"date":       0,
				"text":       "ok",
			},
		})
	}))

	apiEndpoint := fmt.Sprintf("%s/bot%%s/%%s", server.URL)
	bot, err := tgbotapi.NewBotAPIWithAPIEndpoint("TEST_TOKEN", apiEndpoint)
	if err != nil {
		t.Fatalf("Failed to create mock bot: %v", err)
	}

	return bot, &requests, server
}

func TestWatcherIntegration_CaptureTriggersNotification(t *testing.T) {
	bot, requests, server := newMockBotAPI(t)
	defer server.Close()

	chatIDs := []int64{111, 222}
	zmURL := "http://zm.example.com"

	handler, err := New(bot, zmURL, chatIDs)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}

	// Create temp ZM-like folder structure
	tmpDir := t.TempDir()
	monitorDir := filepath.Join(tmpDir, "1")
	if err := os.MkdirAll(monitorDir, 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start watcher in background
	watcherErr := make(chan error, 1)
	go func() {
		watcherErr <- handler.Start(ctx, tmpDir)
	}()

	// Give watcher time to set up watches
	time.Sleep(500 * time.Millisecond)

	// Simulate ZoneMinder creating an event: first create event dir, then capture
	eventDir := filepath.Join(monitorDir, "9001")
	if err := os.MkdirAll(eventDir, 0o755); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)

	captureFile := filepath.Join(eventDir, "00001-capture.jpg")
	if err := os.WriteFile(captureFile, []byte("fake jpg"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Wait for notification to be sent
	time.Sleep(2 * time.Second)

	cancel()

	// Check that sendMessage was called (once per chat ID)
	var sendCount int
	for _, req := range *requests {
		if req == "sendMessage" {
			sendCount++
		}
	}

	if sendCount != len(chatIDs) {
		t.Errorf("Expected %d sendMessage calls, got %d (requests: %v)", len(chatIDs), sendCount, *requests)
	}

	t.Logf("✅ Capture notification sent to %d chats successfully", sendCount)
}

func TestWatcherIntegration_VideoTriggersNotification(t *testing.T) {
	bot, requests, server := newMockBotAPI(t)
	defer server.Close()

	chatIDs := []int64{111}
	handler, err := New(bot, "http://zm.example.com", chatIDs)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}

	tmpDir := t.TempDir()
	monitorDir := filepath.Join(tmpDir, "1")
	if err := os.MkdirAll(monitorDir, 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = handler.Start(ctx, tmpDir)
	}()
	time.Sleep(500 * time.Millisecond)

	// Create a video file — the watcher needs both a Create and Write event
	videoFile := filepath.Join(monitorDir, "9002-video.mp4")
	f, err := os.Create(videoFile)
	if err != nil {
		t.Fatal(err)
	}

	// Give watcher time to see the Create event and start waiting for video completion
	time.Sleep(500 * time.Millisecond)

	// Write data to trigger Write event (which marks the video as complete)
	if _, err := f.Write([]byte("fake mp4 data")); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	// Wait for the video polling goroutine to detect completion and send
	time.Sleep(5 * time.Second)

	cancel()

	var sendVideoCount int
	for _, req := range *requests {
		if req == "sendVideo" {
			sendVideoCount++
		}
	}

	if sendVideoCount != len(chatIDs) {
		t.Errorf("Expected %d sendVideo calls, got %d (requests: %v)", len(chatIDs), sendVideoCount, *requests)
	}

	t.Logf("✅ Video notification sent to %d chats successfully", sendVideoCount)
}

func TestWatcherIntegration_NoMatchIgnored(t *testing.T) {
	bot, requests, server := newMockBotAPI(t)
	defer server.Close()

	handler, err := New(bot, "", []int64{111})
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}

	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "1"), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = handler.Start(ctx, tmpDir)
	}()
	time.Sleep(500 * time.Millisecond)

	// Create files that should NOT trigger notifications
	if err := os.WriteFile(filepath.Join(tmpDir, "1", "random.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "1", "00002-capture.jpg"), []byte("wrong"), 0o644); err != nil {
		t.Fatal(err)
	}

	time.Sleep(2 * time.Second)
	cancel()

	var apiCalls int
	for _, req := range *requests {
		if req == "sendMessage" || req == "sendVideo" {
			apiCalls++
		}
	}

	if apiCalls != 0 {
		t.Errorf("Expected 0 notification API calls for non-matching files, got %d", apiCalls)
	}

	t.Logf("✅ Non-matching files correctly ignored")
}
