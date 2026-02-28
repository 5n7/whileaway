package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/5n7/whileaway/internal/ai"
	"github.com/5n7/whileaway/internal/config"
	"github.com/5n7/whileaway/internal/output"
	"github.com/5n7/whileaway/internal/slack"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "whileaway",
		Short: "Catch up on Slack conversations that happened while you were away",
		Long: `whileaway collects Slack messages from your time away and generates
a prioritized summary report using Claude AI.

It searches for mentions, keywords, and channel activity, then produces
a structured report with action items organized by priority.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          run,
	}

	config.RegisterFlags(rootCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load(cmd)
	if err != nil {
		return err
	}

	claudeClient := ai.NewClaudeCodeClient(cfg.AI.Model)
	if err := claudeClient.Validate(); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cfg.Verbose {
		fmt.Fprintf(os.Stderr, "Search period: %s to %s\n", cfg.Search.From.Format("2006-01-02"), cfg.Search.To.Format("2006-01-02"))
		fmt.Fprintf(os.Stderr, "Channels: %v\n", cfg.Search.Channels)
		fmt.Fprintf(os.Stderr, "Keywords: %v\n", cfg.Search.Keywords)
		fmt.Fprintf(os.Stderr, "Mentions: %v\n", cfg.Search.Mentions)
	}

	slackClient := slack.NewClient(cfg.Slack.UserToken)
	collector := slack.NewCollector(slackClient)

	threads, err := collector.Collect(ctx, slack.CollectParams{
		UserID:   cfg.Slack.UserID,
		From:     cfg.Search.From,
		To:       cfg.Search.To,
		Channels: cfg.Search.Channels,
		Keywords: cfg.Search.Keywords,
		Mentions: cfg.Search.Mentions,
	})
	if err != nil {
		return fmt.Errorf("collecting messages: %w", err)
	}

	if len(threads) == 0 {
		fmt.Fprintf(os.Stderr, "No matching messages found.\n")
		return nil
	}

	fmt.Fprintf(os.Stderr, "Collected %d threads. Starting AI summarization...\n", len(threads))

	userName := cfg.Slack.UserID
	if userName == "" {
		userName = "you"
	}

	summarizer := ai.NewSummarizer(claudeClient, cfg.AI.MaxChunkTokens)
	report, err := summarizer.Summarize(ctx, threads, ai.SummaryConfig{
		UserName: userName,
		UserID:   cfg.Slack.UserID,
		FromDate: cfg.Search.From.Format("2006-01-02"),
		ToDate:   cfg.Search.To.Format("2006-01-02"),
	})
	if err != nil {
		return fmt.Errorf("summarizing: %w", err)
	}

	if err := output.RenderTerminal(report); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: terminal rendering failed: %v\n", err)
		fmt.Println(report)
	}

	if cfg.Output.FilePath != "" {
		if err := output.WriteMarkdownFile(cfg.Output.FilePath, report); err != nil {
			return err
		}
	}

	return nil
}
