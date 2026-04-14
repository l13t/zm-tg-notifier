package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	hcli "github.com/hashicorp/cli"
	"github.com/l13t/zm-tg-notifier/internal/bot"
	"github.com/l13t/zm-tg-notifier/internal/config"
	"github.com/l13t/zm-tg-notifier/internal/watcher"
)

var (
	// Version is set at build time via ldflags
	version = "dev"
)

func main() {
	c := hcli.NewCLI("zm-tg-notifier", version)
	c.Args = os.Args[1:]
	c.Commands = map[string]hcli.CommandFactory{
		"run": func() (hcli.Command, error) {
			return &RunCommand{}, nil
		},
	}
	c.HelpWriter = os.Stdout
	c.ErrorWriter = os.Stderr

	exitStatus, err := c.Run()
	if err != nil {
		slog.Error("failed to run CLI", "error", err)
	}
	os.Exit(exitStatus)
}

// RunCommand implements the main "run" subcommand that starts the bot and watcher.
type RunCommand struct {
	token        string
	zmFolder     string
	zmURL        string
	logLevel     string
	fileChatPath string
	chatIDs      string
}

func (c *RunCommand) Synopsis() string {
	return "Start the ZoneMinder Telegram notification bot"
}

func (c *RunCommand) Help() string {
	helpText := `
Usage: zm-tg-notifier run [options]

  Start the ZoneMinder event watcher and Telegram notification bot.
  Watches a ZoneMinder events folder for new captures and videos,
  and sends notifications to configured Telegram chats.

  Options can also be set via environment variables (flags take precedence).

Options:

  -token=TOKEN             Telegram bot token (env: TOKEN)
  -zm-folder=PATH          Path to ZoneMinder events folder (env: ZM_FOLDER, default: /zm)
  -zm-url=URL              Public URL of ZoneMinder (env: ZM_URL)
  -log-level=LEVEL         Log level: INFO or DEBUG (env: LOG_LEVEL, default: INFO)
  -file-chat-path=PATH     Path to file containing chat IDs (env: FILE_CHAT_PATH, default: chat_id)
  -chat-ids=IDS            Comma-separated Telegram chat IDs (env: CHAT_IDS)
`
	return strings.TrimSpace(helpText)
}

func (c *RunCommand) Run(args []string) int {
	f := flag.NewFlagSet("run", flag.ContinueOnError)
	f.StringVar(&c.token, "token", "", "")
	f.StringVar(&c.zmFolder, "zm-folder", "", "")
	f.StringVar(&c.zmURL, "zm-url", "", "")
	f.StringVar(&c.logLevel, "log-level", "", "")
	f.StringVar(&c.fileChatPath, "file-chat-path", "", "")
	f.StringVar(&c.chatIDs, "chat-ids", "", "")

	if err := f.Parse(args); err != nil {
		return 1
	}

	cfg := config.LoadWithFlags(&c.token, &c.zmFolder, &c.zmURL, &c.logLevel, &c.fileChatPath, &c.chatIDs)

	// Set up structured JSON logging
	slogLevel := slog.LevelInfo
	if strings.ToUpper(cfg.LogLevel) == "DEBUG" {
		slogLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     slogLevel,
		AddSource: slogLevel == slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	if cfg.Token == "" {
		slog.Error("TOKEN is required")
		return 1
	}

	slog.Info("starting", "version", version)

	b, err := bot.New(cfg)
	if err != nil {
		slog.Error("failed to create bot", "error", err)
		return 1
	}

	w, err := watcher.New(b.API(), cfg.ZMURL, b.ChatIDs())
	if err != nil {
		slog.Error("failed to create watcher", "error", err)
		return 1
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		slog.Info("shutting down")
		cancel()
	}()

	go func() {
		if err := w.Start(ctx, cfg.ZMFolder); err != nil {
			slog.Error("watcher error", "error", err)
			cancel()
		}
	}()

	if err := b.Start(ctx); err != nil {
		slog.Error("bot error", "error", err)
	}

	slog.Info("shutdown complete")
	return 0
}
