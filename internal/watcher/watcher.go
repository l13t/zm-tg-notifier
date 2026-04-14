package watcher

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var (
	captureRegex = regexp.MustCompile(`^.*/([0-9]+)/00001-capture\.jpg$`)
	videoRegex   = regexp.MustCompile(`^.*/([0-9]+)-video\.(?:[^.]+\.)?mp4$`)
)

type EventHandler struct {
	bot     *tgbotapi.BotAPI
	zmURL   string
	chatIDs []int64
	events  sync.Map // map[string]bool - tracks completed video writes
	watcher *fsnotify.Watcher
}

func New(bot *tgbotapi.BotAPI, zmURL string, chatIDs []int64) (*EventHandler, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	slog.Info("watcher created", "zm_url", zmURL, "chat_ids", chatIDs)
	if len(chatIDs) == 0 {
		slog.Warn("no chat IDs configured, notifications will not be sent")
	}

	return &EventHandler{
		bot:     bot,
		zmURL:   zmURL,
		chatIDs: chatIDs,
		watcher: watcher,
	}, nil
}

func (h *EventHandler) Start(ctx context.Context, folder string) error {
	if err := h.addRecursive(folder); err != nil {
		return err
	}

	go h.cleanupEvents(ctx)

	slog.Info("started watching folder", "folder", folder)

	for {
		select {
		case <-ctx.Done():
			return h.watcher.Close()
		case event, ok := <-h.watcher.Events:
			if !ok {
				return nil
			}
			h.handleEvent(ctx, event)
		case err, ok := <-h.watcher.Errors:
			if !ok {
				return nil
			}
			slog.Error("watcher error", "error", err)
		}
	}
}

func (h *EventHandler) addRecursive(folder string) error {
	dirCount := 0
	err := filepath.Walk(folder, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			slog.Error("error walking path", "path", path, "error", err)
			return err
		}
		if info.IsDir() {
			if err := h.watcher.Add(path); err != nil {
				slog.Error("failed to watch directory", "path", path, "error", err)
			} else {
				dirCount++
			}
		}
		return nil
	})
	slog.Info("initial directory scan complete", "dir_count", dirCount, "folder", folder)
	return err
}

func (h *EventHandler) handleEvent(ctx context.Context, event fsnotify.Event) {
	slog.Debug("fs event", "op", event.Op.String(), "path", event.Name)

	// Handle new directories
	if event.Op&fsnotify.Create == fsnotify.Create {
		if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
			if err := h.watcher.Add(event.Name); err != nil {
				slog.Error("failed to watch new directory", "path", event.Name, "error", err)
			} else {
				slog.Info("watching new directory", "path", event.Name)
			}
		}
	}

	// Handle file creation
	if event.Op&fsnotify.Create == fsnotify.Create {
		h.onCreated(ctx, event.Name)
	}

	// Handle file close (write complete)
	if event.Op&fsnotify.Write == fsnotify.Write {
		h.onClosed(ctx, event.Name)
	}
}

func (h *EventHandler) getEventID(path string) (string, bool) {
	// Check if it's a capture image
	if matches := captureRegex.FindStringSubmatch(path); matches != nil {
		slog.Debug("matched capture regex", "event_id", matches[1], "path", path)
		return matches[1], false
	}

	// Check if it's a video
	if matches := videoRegex.FindStringSubmatch(path); matches != nil {
		eventID := matches[1]
		if _, exists := h.events.Load(eventID); exists {
			slog.Debug("video already processed", "event_id", eventID, "path", path)
			return "", false
		}
		slog.Debug("matched video regex", "event_id", eventID, "path", path)
		return eventID, true
	}

	return "", false
}

func (h *EventHandler) onClosed(ctx context.Context, path string) {
	eventID, isVideo := h.getEventID(path)
	if eventID == "" || !isVideo {
		return
	}

	slog.Info("video write complete", "event_id", eventID, "path", path)
	h.events.Store(eventID, true)
}

func (h *EventHandler) onCreated(ctx context.Context, path string) {
	eventID, isVideo := h.getEventID(path)
	if eventID == "" {
		return
	}

	if !isVideo {
		slog.Info("new event detected", "event_id", eventID, "path", path)

		if len(h.chatIDs) == 0 {
			slog.Warn("skipping notification, no chat IDs configured", "event_id", eventID)
			return
		}

		message := "New event"
		if h.zmURL != "" {
			message += " " + h.zmURL + "/?view=event&eid=" + eventID
		}

		slog.Info("sending message", "event_id", eventID, "chat_count", len(h.chatIDs), "message", message)
		for _, chatID := range h.chatIDs {
			msg := tgbotapi.NewMessage(chatID, message)
			if _, err := h.bot.Send(msg); err != nil {
				slog.Error("failed to send message", "chat_id", chatID, "event_id", eventID, "error", err)
			} else {
				slog.Info("message sent", "chat_id", chatID, "event_id", eventID)
			}
		}
		return
	}

	incompletePath := filepath.Join(filepath.Dir(path), "incomplete.mp4")

	// If incomplete.mp4 is already gone, the video is ready now (e.g. renamed into place)
	if _, err := os.Stat(incompletePath); os.IsNotExist(err) {
		slog.Info("video ready immediately, no incomplete marker", "event_id", eventID, "path", path)
		h.events.Store(eventID, true)
		go h.sendVideo(ctx, path)
		return
	}

	// Wait for incomplete.mp4 to disappear (ZoneMinder removes it when recording is done)
	go func() {
		slog.Info("waiting for video write to complete", "event_id", eventID, "path", path)
		deadline := time.Now().Add(10 * time.Minute)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if _, err := os.Stat(incompletePath); os.IsNotExist(err) {
				if _, err := os.Stat(path); err == nil {
					slog.Info("incomplete marker gone, sending video", "event_id", eventID, "path", path)
					h.events.Store(eventID, true)
					h.sendVideo(ctx, path)
				} else {
					slog.Error("video file missing after incomplete marker gone", "event_id", eventID, "path", path, "error", err)
				}
				return
			}
			if time.Now().After(deadline) {
				slog.Error("timeout waiting for incomplete marker to disappear", "event_id", eventID, "incomplete_path", incompletePath)
				return
			}
			time.Sleep(2 * time.Second)
		}
	}()
}

func (h *EventHandler) sendVideo(ctx context.Context, path string) {
	slog.Info("sending video", "path", path, "chat_count", len(h.chatIDs))

	if info, err := os.Stat(path); err != nil {
		slog.Warn("video file not accessible", "path", path, "error", err)
		return
	} else {
		slog.Info("video file ready", "path", path, "size_bytes", info.Size())
	}

	var wg sync.WaitGroup
	for _, chatID := range h.chatIDs {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()

			video := tgbotapi.NewVideo(id, tgbotapi.FilePath(path))
			if _, err := h.bot.Send(video); err != nil {
				slog.Error("failed to send video", "chat_id", id, "path", path, "error", err)
			} else {
				slog.Info("video sent", "chat_id", id, "path", path)
			}
		}(chatID)
	}
	wg.Wait()
}

func (h *EventHandler) cleanupEvents(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			count := 0
			h.events.Range(func(key, value interface{}) bool {
				count++
				return true
			})

			if count > 10 {
				h.events = sync.Map{}
				slog.Info("cleared event cache", "previous_count", count)
			}
		}
	}
}
