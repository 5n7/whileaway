# whileaway

A CLI tool to catch up on Slack conversations that happened while you were away. It collects messages from Slack using the search API and generates a prioritized summary report using Claude AI.

## Prerequisites

- **Go** 1.24+
- **Claude Code CLI** (`claude` command) installed and available in PATH
- **Slack App** with a User OAuth Token

### Slack App Setup

Create a Slack App at [api.slack.com/apps](https://api.slack.com/apps) and add the following **User Token Scopes**:

| Scope              | Description                  |
| ------------------ | ---------------------------- |
| `search:read`      | Search messages              |
| `channels:history` | Read public channel history  |
| `groups:history`   | Read private channel history |
| `im:history`       | Read direct message history  |
| `channels:read`    | List channels                |
| `users:read`       | Read user information        |

After installing the app to your workspace, copy the **User OAuth Token** (`xoxp-...`).

## Installation

```bash
go install github.com/5n7/whileaway/cmd/whileaway@latest
```

## Environment Variables

| Variable           | Required | Description                         |
| ------------------ | -------- | ----------------------------------- |
| `SLACK_USER_TOKEN` | Yes      | Slack User OAuth Token (`xoxp-...`) |

## Usage

```bash
# Catch up on the last 7 days: mentions + specific channels
whileaway --days 7 --mentions --user-id U1234567890 --channels "#pj-whileaway,#pj-whileaway-dev"

# Date range + keyword search, output to file
whileaway --from 2025-02-10 --to 2025-02-18 --keywords "deploy,incident" --output report.md

# All options combined
whileaway --from 2025-02-10 --to 2025-02-18 \
  --channels "#pj-whileaway,#pj-whileaway-dev" \
  --keywords "deploy,incident" \
  --mentions --user-id U1234567890 \
  --output report.md \
  --model opus \
  --verbose
```

### Flags

| Flag                 | Short | Type                     | Default | Description                                    |
| -------------------- | ----- | ------------------------ | ------- | ---------------------------------------------- |
| `--from`             | `-f`  | string (YYYY-MM-DD)      | -       | Start date (required if `--days` not set)      |
| `--to`               | `-t`  | string (YYYY-MM-DD)      | today   | End date                                       |
| `--days`             | `-d`  | int                      | -       | Fetch last N days (alternative to `--from`)    |
| `--channels`         | `-c`  | string (comma-separated) | -       | Target channels                                |
| `--keywords`         | `-k`  | string (comma-separated) | -       | Search keywords                                |
| `--mentions`         | `-m`  | bool                     | false   | Search for mentions of self                    |
| `--user-id`          |       | string                   | -       | Your Slack User ID (required for `--mentions`) |
| `--output`           | `-o`  | string                   | -       | Markdown output file                           |
| `--model`            |       | string                   | `opus`  | Claude model name                              |
| `--max-chunk-tokens` |       | int                      | `80000` | Max tokens per chunk for AI summarization      |
| `--verbose`          | `-v`  | bool                     | false   | Verbose logging                                |

## How It Works

1. **Collect**: Searches Slack for messages matching your criteria (mentions, keywords, channels) using the `search.messages` API
2. **Fetch threads**: Retrieves full thread context for each matching message using `conversations.replies`
3. **Summarize**: Sends collected threads to Claude AI in chunks for topic extraction
4. **Report**: Generates a prioritized Markdown report with action items

The report is organized by priority:

- 🔴 **High**: Direct requests, blockers, incidents
- 🟡 **Medium**: Important project updates and decisions
- 📋 **Low**: FYI and reference information
