# Corporate Bullshifter Bot

A Telegram bot that transforms casual messages into polite, professional corporate English suitable for workplace communication.

## Features

- Converts messages from any language (especially Russian) into professional English
- Maintains original meaning while adjusting tone and style
- Perfect for internal chats, emails, and workplace communication
- Fast response time with Claude AI
- Simple command interface

## Requirements

- **Go version**: 1.20 or higher
- **Telegram Bot Token**: Get one from [@BotFather](https://t.me/botfather)
- **Claude API Key**: Get one from [Anthropic Console](https://console.anthropic.com/)

## Installation

### 1. Clone or navigate to the project directory

```bash
cd corp-bullshifter
```

### 2. Initialize Go modules and download dependencies

```bash
go mod download
```

Alternatively, if starting fresh:

```bash
go mod init corp-bullshifter
go get github.com/go-telegram-bot-api/telegram-bot-api/v5
go mod tidy
```

### 3. Set up environment variables

Create a `.env` file or export the following variables:

```bash
export TELEGRAM_BOT_TOKEN="your_telegram_bot_token_here"
export CLAUDE_API_KEY="your_claude_api_key_here"

# Optional (with defaults):
export CLAUDE_MODEL="claude-3-5-sonnet-20241022"
export CLAUDE_API_URL="https://api.anthropic.com/v1/messages"
```

### 4. Run the bot

```bash
go run main.go
```

Or build and run:

```bash
go build -o corp-bullshifter
./corp-bullshifter
```

## Usage

### Commands

- `/start` - Welcome message and bot introduction
- `/help` - Usage instructions and examples

### Text Conversion

Simply send any text message to the bot, and it will respond with a professional corporate English version.

### Examples

**Input (Russian):**
```
я всё сделал по инструкции, но ничего не сработало. может я что-то не так понял?
```

**Output (Corporate English):**
```
I followed all the steps in the instructions, but unfortunately nothing seems to be working yet.
It's possible I misunderstood some part of the process. Could you please clarify the expected
flow so I can double-check?
```

---

**Input (Russian):**
```
Посмотри, пожалуйста, мой PR, там правки по логированию
```

**Output (Corporate English):**
```
Could you please take a look at my PR? It contains some updates related to logging.
```

---

**Input (Russian):**
```
Кажется, это бага на проде, у клиентов уже воспроизводится
```

**Output (Corporate English):**
```
It looks like this might be a production issue, and our clients are already able to reproduce it.
```

## Configuration Details

### Required Environment Variables

| Variable | Description |
|----------|-------------|
| `TELEGRAM_BOT_TOKEN` | Your Telegram bot token from BotFather |
| `CLAUDE_API_KEY` | Your Anthropic Claude API key |

### Optional Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `CLAUDE_MODEL` | Claude model to use | `claude-3-5-sonnet-20241022` |
| `CLAUDE_API_URL` | Claude API endpoint | `https://api.anthropic.com/v1/messages` |

## Deployment

For production deployment on a VPS with Docker, see [DEPLOYMENT.md](DEPLOYMENT.md) for detailed instructions including:
- Docker setup
- Secure token management
- VPS deployment steps
- Monitoring and troubleshooting

Quick start with Docker:
```bash
docker-compose build
docker-compose up -d
```

## Project Structure

```
corp-bullshifter/
├── main.go              # Main application code
├── go.mod               # Go module definition
├── go.sum               # Go module checksums (generated)
├── Dockerfile           # Docker image definition
├── docker-compose.yml   # Docker Compose configuration
├── .env.example         # Environment variables template
├── .gitignore           # Git ignore rules
├── README.md            # This file
└── DEPLOYMENT.md        # VPS deployment guide
```

## How It Works

1. User sends a message to the bot via Telegram
2. Bot receives the message and sends it to Claude API with a specialized prompt
3. Claude rewrites the message into polite, corporate English
4. Bot returns the rewritten text to the user

The bot uses:
- **Telegram Bot API** for receiving and sending messages
- **Claude Messages API** for AI-powered text transformation
- **Go's standard HTTP client** for API communication

## Error Handling

- If Claude API is unavailable or returns an error, the bot responds with: "Sorry, I couldn't process your request right now. Please try again later."
- All errors are logged to stdout/stderr for debugging
- The bot continues running even if individual requests fail

## Development

### Dependencies

- `github.com/go-telegram-bot-api/telegram-bot-api/v5` - Telegram Bot API wrapper

### Building

```bash
go build -o corp-bullshifter main.go
```

### Testing

Send test messages to your bot on Telegram after starting it.

## Troubleshooting

### Bot doesn't respond
- Check that `TELEGRAM_BOT_TOKEN` is correct
- Verify the bot is running without errors
- Check the logs for error messages

### "Couldn't process your request" error
- Verify `CLAUDE_API_KEY` is valid
- Check your Anthropic API quota/limits
- Ensure network connectivity to Anthropic API
- Check logs for detailed error messages

### Import errors
- Run `go mod download` to fetch dependencies
- Run `go mod tidy` to clean up module file

## License

This is a personal project. Use at your own discretion.

## Contributing

This is an MVP implementation. Feel free to fork and extend with additional features.
