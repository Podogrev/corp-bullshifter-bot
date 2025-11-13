# Prompt Configuration

This directory contains the system prompt used by the bot to rewrite messages.

## Editing the Prompt

### On VPS (without rebuild)
The `prompts` directory is mounted as a volume. Edit directly on the server:

```bash
cd /home/podogrev/corp-bullshifter-bot/prompts
nano system_prompt.txt  # or vim

# Restart the bot to apply changes
cd ..
sudo docker compose restart bot
```

### Locally
Edit `prompts/system_prompt.txt` and push to GitHub. The changes will be deployed automatically.

## Prompt Structure

The prompt file should end with `User message:` (no newline after it). The bot will append the user's message directly after this line.

## Token Usage

Keep the prompt concise to reduce input token costs. Current prompt uses ~300-400 tokens per request.
