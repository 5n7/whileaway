package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// Config holds all resolved configuration.
type Config struct {
	Slack   SlackConfig
	AI      AIConfig
	Search  SearchConfig
	Output  OutputConfig
	Verbose bool
}

// SlackConfig holds Slack-related settings.
type SlackConfig struct {
	UserToken string
	UserID    string
}

// AIConfig holds AI-related settings.
type AIConfig struct {
	Model          string
	MaxChunkTokens int
}

// SearchConfig holds search parameters.
type SearchConfig struct {
	From     time.Time
	To       time.Time
	Channels []string
	Keywords []string
	Mentions bool
}

// OutputConfig holds output settings.
type OutputConfig struct {
	FilePath string
}

// RegisterFlags defines all CLI flags.
func RegisterFlags(cmd *cobra.Command) {
	cmd.Flags().StringP("from", "f", "", "Start date (YYYY-MM-DD)")
	cmd.Flags().StringP("to", "t", "", "End date (YYYY-MM-DD), defaults to today")
	cmd.Flags().IntP("days", "d", 0, "Number of recent days to fetch (alternative to --from)")
	cmd.Flags().StringSliceP("channels", "c", nil, "Target channel names (comma-separated, e.g. #team-backend,#general)")
	cmd.Flags().StringSliceP("keywords", "k", nil, "Search keywords (comma-separated)")
	cmd.Flags().BoolP("mentions", "m", false, "Search for mentions of self")
	cmd.Flags().String("user-id", "", "Your Slack User ID (required for --mentions)")
	cmd.Flags().StringP("output", "o", "", "Markdown output file path")
	cmd.Flags().String("model", "opus", "Claude model name")
	cmd.Flags().Int("max-chunk-tokens", 80000, "Max tokens per chunk for AI summarization")
	cmd.Flags().BoolP("verbose", "v", false, "Verbose logging")
}

// Load resolves configuration from CLI flags and environment variables.
func Load(cmd *cobra.Command) (*Config, error) {
	userToken := os.Getenv("SLACK_USER_TOKEN")
	if userToken == "" {
		return nil, fmt.Errorf("environment variable SLACK_USER_TOKEN is required")
	}
	if !strings.HasPrefix(userToken, "xoxp-") {
		return nil, fmt.Errorf("SLACK_USER_TOKEN must be a User OAuth Token (xoxp-...)")
	}

	from, to, err := resolveDates(cmd)
	if err != nil {
		return nil, err
	}

	channels := resolveChannels(cmd)
	keywords, _ := cmd.Flags().GetStringSlice("keywords")
	mentions, _ := cmd.Flags().GetBool("mentions")

	if len(channels) == 0 && len(keywords) == 0 && !mentions {
		return nil, fmt.Errorf("at least one search condition is required: --channels, --keywords, or --mentions")
	}

	userID, _ := cmd.Flags().GetString("user-id")
	if mentions && userID == "" {
		return nil, fmt.Errorf("--user-id is required when --mentions is enabled")
	}

	model, _ := cmd.Flags().GetString("model")
	maxChunkTokens, _ := cmd.Flags().GetInt("max-chunk-tokens")
	outputPath, _ := cmd.Flags().GetString("output")
	verbose, _ := cmd.Flags().GetBool("verbose")

	return &Config{
		Slack: SlackConfig{
			UserToken: userToken,
			UserID:    userID,
		},
		AI: AIConfig{
			Model:          model,
			MaxChunkTokens: maxChunkTokens,
		},
		Search: SearchConfig{
			From:     from,
			To:       to,
			Channels: channels,
			Keywords: keywords,
			Mentions: mentions,
		},
		Output: OutputConfig{
			FilePath: outputPath,
		},
		Verbose: verbose,
	}, nil
}

func resolveDates(cmd *cobra.Command) (time.Time, time.Time, error) {
	fromStr, _ := cmd.Flags().GetString("from")
	toStr, _ := cmd.Flags().GetString("to")
	days, _ := cmd.Flags().GetInt("days")

	var from, to time.Time

	if toStr != "" {
		parsed, err := time.Parse("2006-01-02", toStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid --to date format (expected YYYY-MM-DD): %w", err)
		}
		to = parsed
	} else {
		to = time.Now().Truncate(24 * time.Hour)
	}

	if fromStr != "" {
		parsed, err := time.Parse("2006-01-02", fromStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid --from date format (expected YYYY-MM-DD): %w", err)
		}
		from = parsed
	} else if days > 0 {
		from = to.AddDate(0, 0, -days)
	} else {
		return time.Time{}, time.Time{}, fmt.Errorf("either --from or --days is required")
	}

	return from, to, nil
}

func resolveChannels(cmd *cobra.Command) []string {
	channels, _ := cmd.Flags().GetStringSlice("channels")
	var result []string
	for _, ch := range channels {
		ch = strings.TrimPrefix(ch, "#")
		if ch != "" {
			result = append(result, ch)
		}
	}
	return result
}
