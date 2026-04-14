# Telegram Event Bot for ZoneMinder events notifications

Telegram bot that watches [ZoneMinder](https://zoneminder.com/) event folder and sends
notifications with snapshots and videos to Telegram when motion is detected.

## Features

- Real-time notifications when ZoneMinder detects events
- Sends image captures from events
- Sends video files (supports both legacy and codec formats)
- Telegram commands: `/start` and `/chat_ids`
- Filter notifications by chat ID
- Lightweight Go binary (~9MB) or Docker image (~20MB)
- Multi-architecture support: amd64, arm64

## Quick Start

### Option 1: Docker (Recommended)

```bash
docker run -d \
  --name zm-tg-notifier \
  -e TOKEN=your_telegram_bot_token \
  -e ZM_URL=https://zm.example.com \
  -v /var/cache/zoneminder/events:/zm \
  ghcr.io/l13t/zm-tg-notifier:latest
```

### Option 2: Docker Compose

```yaml
version: "3.8"

services:
    zm-tg-notifier:
        image: ghcr.io/l13t/zm-tg-notifier:latest
        container_name: zm-tg-notifier
        restart: unless-stopped
        environment:
            TOKEN: your_telegram_bot_token
            ZM_URL: https://zm.example.com
            ZM_FOLDER: /zm
            LOG_LEVEL: INFO
            CHAT_IDS: "123456789,987654321" # Optional: restrict to specific chats
        volumes:
            - /var/cache/zoneminder/events:/zm:ro
```

### Option 3: Binary

Download the latest binary from [releases](https://github.com/l13t/zm-tg-notifier/releases):

```bash
# Linux amd64
wget https://github.com/l13t/zm-tg-notifier/releases/latest/download/zm-tg-notifier_linux_amd64.tar.gz
tar -xzf zm-tg-notifier_linux_amd64.tar.gz

# Run
./zm-tg-notifier run -token YOUR_TOKEN -zm-url https://zm.example.com -zm-folder /var/cache/zoneminder/events
```

## Configuration

### Environment Variables

| Variable         | Default    | Description                                                  |
| ---------------- | ---------- | ------------------------------------------------------------ |
| `TOKEN`          | _required_ | Telegram bot token from [@BotFather](https://t.me/botfather) |
| `ZM_FOLDER`      | `/zm`      | Path to ZoneMinder events folder                             |
| `ZM_URL`         |            | Public URL of your ZoneMinder instance (for event links)     |
| `LOG_LEVEL`      | `INFO`     | Log level (`INFO` or `DEBUG`)                                |
| `FILE_CHAT_PATH` | `chat_id`  | Path to file with allowed chat IDs (one per line)            |
| `CHAT_IDS`       |            | Comma-separated list of allowed Telegram chat IDs            |

### Command-Line Flags

Flags override environment variables:

```bash
zm-tg-notifier run [options]

Options:
  -token string           Telegram bot token (env: TOKEN)
  -zm-folder string       ZoneMinder events folder (env: ZM_FOLDER, default: /zm)
  -zm-url string          Public ZoneMinder URL (env: ZM_URL)
  -log-level string       Log level: INFO or DEBUG (env: LOG_LEVEL, default: INFO)
  -file-chat-path string  Path to chat IDs file (env: FILE_CHAT_PATH, default: chat_id)
  -chat-ids string        Comma-separated chat IDs (env: CHAT_IDS)

Global:
  --version               Show version
  --help                  Show help
```

## Setup Guide

### 1. Create Telegram Bot

1. Message [@BotFather](https://t.me/botfather) on Telegram
2. Send `/newbot` and follow instructions
3. Save the bot token

### 2. Get Your Chat ID

Start the bot and send it `/chat_ids`:

```text
You: /start
Bot: Bot started! Watching for ZoneMinder events...

You: /chat_ids
Bot: Your chat ID: 123456789
```

### 3. Configure Bot

#### Without Chat Filtering (sends to everyone)

```bash
docker run -d \
  -e TOKEN=your_bot_token \
  -e ZM_URL=https://zm.example.com \
  -v /var/cache/zoneminder/events:/zm:ro \
  ghcr.io/l13t/zm-tg-notifier:latest
```

#### With Chat Filtering (recommended)

#### Option A: Environment variable

```bash
docker run -d \
  -e TOKEN=your_bot_token \
  -e ZM_URL=https://zm.example.com \
  -e CHAT_IDS=123456789,987654321 \
  -v /var/cache/zoneminder/events:/zm:ro \
  ghcr.io/l13t/zm-tg-notifier:latest
```

#### Option B: File

Create `chat_id` file:

```text
123456789
987654321
```

Mount it:

```bash
docker run -d \
  -e TOKEN=your_bot_token \
  -e ZM_URL=https://zm.example.com \
  -v /var/cache/zoneminder/events:/zm:ro \
  -v $(pwd)/chat_id:/app/chat_id:ro \
  ghcr.io/l13t/zm-tg-notifier:latest
```

### 4. Point to ZoneMinder Events

Find your ZoneMinder events folder:

```bash
# Common locations:
/var/cache/zoneminder/events
/var/lib/zoneminder/events
/usr/share/zoneminder/events
```

Mount it as `/zm` (read-only recommended):

```bash
-v /var/cache/zoneminder/events:/zm:ro
```

## Usage

Once running, the bot will automatically:

1. Watch the ZoneMinder events folder
2. Detect new captures and videos
3. Send notifications to allowed chat IDs
4. Include links to ZoneMinder (if `ZM_URL` is set)

### Telegram Commands

- `/start` - Start the bot and see your chat ID
- `/chat_ids` - Display your chat ID

### Event Format

When motion is detected:

```text
Monitor 1 - Event 1234
[Image or Video]
View in ZoneMinder: https://zm.example.com/zm/index.php?view=event&eid=1234
```

## Supported Video Formats

Both ZoneMinder video formats are supported:

- Legacy: `1234-video.mp4`
- Codec: `1234-video.h264.mp4`, `1234-video.h265.mp4`

## Platform Support

Pre-built binaries and Docker images for:

- **Linux**: amd64, arm64
- **macOS**: amd64 (Intel), arm64 (Apple Silicon)
- **Docker**: Multi-arch (auto-detects)

## Troubleshooting

### Bot doesn't send notifications

**Check permissions:**

```bash
# Bot needs read access to ZoneMinder events
ls -la /var/cache/zoneminder/events
```

**Check chat filtering:**

```bash
# Send /chat_ids to get your ID
# Verify it's in CHAT_IDS or chat_id file
```

**Check logs:**

```bash
docker logs zm-tg-notifier
# or
docker logs -f zm-tg-notifier  # follow
```

### Videos not sending

**Check video file format:**

```bash
# Supported: *-video.mp4 or *-video.*.mp4
ls /var/cache/zoneminder/events/*/*/
```

**Check file size:**

- Telegram has a 50MB limit for bot uploads
- Large videos may fail to send

### Wrong ZoneMinder link

**Set ZM_URL correctly:**

```bash
# Should be your public ZoneMinder URL
ZM_URL=https://zm.example.com
# NOT the internal/Docker URL
```

## License

MIT

## Credits

Original Python version by [DanielBorgesOliveira](https://gist.github.com/DanielBorgesOliveira/d3e578e2b677245cec550e965eae1755)
