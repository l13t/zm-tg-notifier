// Package bot provides Telegram bot functionality for the ZoneMinder notification system.
package bot

import (
	"context"
	"fmt"
	"log/slog"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/l13t/zm-tg-notifier/internal/config"
)

// Bot represents a Telegram bot instance with its configuration and chat subscriptions.
type Bot struct {
	api     *tgbotapi.BotAPI
	config  *config.Config
	chatIDs []int64
}

// New creates a new Bot instance with the provided configuration.
// It initializes the Telegram Bot API and returns an error if authentication fails.
func New(cfg *config.Config) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot: %w", err)
	}

	slog.Info("authorized on telegram account", "username", api.Self.UserName)

	return &Bot{
		api:     api,
		config:  cfg,
		chatIDs: cfg.GetChatIDs(),
	}, nil
}

// API returns the underlying Telegram Bot API instance.
func (b *Bot) API() *tgbotapi.BotAPI {
	return b.api
}

// ChatIDs returns the list of authorized chat IDs that can receive notifications.
func (b *Bot) ChatIDs() []int64 {
	return b.chatIDs
}

// Start begins the bot's update polling loop and handles incoming commands.
// It listens for updates and processes them until the context is cancelled.
func (b *Bot) Start(ctx context.Context) error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	slog.Info("bot started, listening for commands")

	for {
		select {
		case <-ctx.Done():
			b.api.StopReceivingUpdates()
			return nil
		case update := <-updates:
			if update.Message == nil {
				continue
			}

			b.handleMessage(update.Message)
		}
	}
}

// handleMessage processes incoming messages and routes commands to their handlers.
func (b *Bot) handleMessage(message *tgbotapi.Message) {
	if !message.IsCommand() {
		return
	}

	switch message.Command() {
	case "start":
		b.handleStart(message)
	case "chat_ids":
		if b.isInChatIDs(message.Chat.ID) {
			b.handleChatIDs(message)
		}
	}
}

// handleStart responds to the /start command by sending the user their chat ID.
func (b *Bot) handleStart(message *tgbotapi.Message) {
	text := fmt.Sprintf("Your chat_id is %d", message.Chat.ID)
	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ReplyToMessageID = message.MessageID

	if _, err := b.api.Send(msg); err != nil {
		slog.Error("failed to send start message", "error", err)
	}
}

// handleChatIDs responds to the /chat_ids command by listing all authorized chat IDs.
// This command is only available to users whose chat ID is in the authorized list.
func (b *Bot) handleChatIDs(message *tgbotapi.Message) {
	text := fmt.Sprintf("Subscribed chat_ids: %v", b.chatIDs)
	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ReplyToMessageID = message.MessageID

	if _, err := b.api.Send(msg); err != nil {
		slog.Error("failed to send chat_ids message", "error", err)
	}
}

// isInChatIDs checks if the given chat ID is in the list of authorized chat IDs.
func (b *Bot) isInChatIDs(chatID int64) bool {
	for _, id := range b.chatIDs {
		if id == chatID {
			return true
		}
	}
	return false
}
