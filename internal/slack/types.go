package slack

import "time"

// CollectParams holds parameters for message collection.
type CollectParams struct {
	UserID   string
	From     time.Time
	To       time.Time
	Channels []string
	Keywords []string
	Mentions bool
}

// SlackMessage represents a single Slack message with resolved user info.
type SlackMessage struct {
	User      string    // Display name (resolved)
	UserID    string    // Slack User ID
	Text      string    // Message body (mentions resolved to display names)
	Timestamp string    // Slack timestamp
	DateTime  time.Time // Parsed time
}

// CollectedThread represents a full thread with metadata.
type CollectedThread struct {
	ChannelID    string
	ChannelName  string
	ThreadTS     string
	Permalink    string         // Link to the thread
	Messages     []SlackMessage // Chronological order
	MatchReasons []string       // Reasons this thread was collected
}
