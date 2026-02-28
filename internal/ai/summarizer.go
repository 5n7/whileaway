package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/5n7/whileaway/internal/slack"
)

// Completer defines the interface for AI completion.
type Completer interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// SummaryConfig holds user context for summarization.
type SummaryConfig struct {
	UserName string
	UserID   string
	FromDate string // YYYY-MM-DD
	ToDate   string // YYYY-MM-DD
}

// Topic represents a single extracted topic from AI analysis.
type Topic struct {
	Title             string   `json:"title"`
	Summary           string   `json:"summary"`
	Channel           string   `json:"channel"`
	Permalink         string   `json:"permalink"`
	Participants      []string `json:"participants"`
	ActionRequired    bool     `json:"action_required"`
	ActionDescription string   `json:"action_description"`
	Priority          string   `json:"priority"`
	MatchReasons      []string `json:"match_reasons"`
}

type extractionResponse struct {
	Topics []Topic `json:"topics"`
}

type threadData struct {
	Channel      string        `json:"channel"`
	Permalink    string        `json:"permalink"`
	MatchReasons []string      `json:"match_reasons"`
	Messages     []messageData `json:"messages"`
}

type messageData struct {
	User string `json:"user"`
	Text string `json:"text"`
	Time string `json:"time"`
}

// Summarizer implements the two-phase summarization pipeline.
type Summarizer struct {
	client         Completer
	maxChunkTokens int
}

// NewSummarizer creates a new Summarizer.
func NewSummarizer(client Completer, maxChunkTokens int) *Summarizer {
	if maxChunkTokens <= 0 {
		maxChunkTokens = 80000
	}
	return &Summarizer{client: client, maxChunkTokens: maxChunkTokens}
}

// Summarize generates a report from collected threads.
func (s *Summarizer) Summarize(ctx context.Context, threads []slack.CollectedThread, cfg SummaryConfig) (string, error) {
	threadDataList := threadsToJSON(threads)
	chunks := s.chunkThreads(threadDataList)

	allTopics, err := s.extractAllTopics(ctx, chunks, cfg)
	if err != nil {
		return "", err
	}

	fmt.Fprintf(os.Stderr, "Generating final report...\n")

	topicsJSON, err := json.MarshalIndent(allTopics, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling topics: %w", err)
	}

	integrationPrompt, err := RenderIntegrationPrompt(IntegrationPromptData{
		UserName:     cfg.UserName,
		FromDate:     cfg.FromDate,
		ToDate:       cfg.ToDate,
		TotalThreads: len(threads),
		TopicsJSON:   string(topicsJSON),
	})
	if err != nil {
		return "", fmt.Errorf("rendering integration prompt: %w", err)
	}

	report, err := s.client.Complete(ctx, integrationPrompt)
	if err != nil {
		return "", fmt.Errorf("generating final report: %w", err)
	}
	return report, nil
}

func (s *Summarizer) extractAllTopics(ctx context.Context, chunks [][]threadData, cfg SummaryConfig) ([]Topic, error) {
	if len(chunks) == 1 {
		fmt.Fprintf(os.Stderr, "Processing %d threads in single batch...\n", len(chunks[0]))
	} else {
		fmt.Fprintf(os.Stderr, "Summarizing %d chunks in parallel...\n", len(chunks))
	}

	results := make([][]Topic, len(chunks))
	g, ctx := errgroup.WithContext(ctx)

	for i, chunk := range chunks {
		g.Go(func() error {
			chunkJSON, err := json.MarshalIndent(chunk, "", "  ")
			if err != nil {
				return fmt.Errorf("marshaling chunk %d: %w", i+1, err)
			}

			prompt, err := RenderExtractionPrompt(ExtractionPromptData{
				UserName:    cfg.UserName,
				UserID:      cfg.UserID,
				FromDate:    cfg.FromDate,
				ToDate:      cfg.ToDate,
				ThreadsJSON: string(chunkJSON),
			})
			if err != nil {
				return fmt.Errorf("rendering extraction prompt for chunk %d: %w", i+1, err)
			}

			topics, err := s.extractTopics(ctx, prompt)
			if err != nil {
				return fmt.Errorf("extracting topics from chunk %d: %w", i+1, err)
			}
			results[i] = topics
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	var allTopics []Topic
	for _, topics := range results {
		allTopics = append(allTopics, topics...)
	}
	return allTopics, nil
}

func (s *Summarizer) extractTopics(ctx context.Context, prompt string) ([]Topic, error) {
	resp, err := s.client.Complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("claude completion: %w", err)
	}

	topics, err := parseTopicsJSON(resp)
	if err != nil {
		return nil, fmt.Errorf("parsing AI response: %w", err)
	}
	return topics, nil
}

// chunkThreads splits threads into chunks that fit within the token limit.
func (s *Summarizer) chunkThreads(threads []threadData) [][]threadData {
	if len(threads) == 0 {
		return nil
	}

	var chunks [][]threadData
	var currentChunk []threadData
	currentTokens := 0

	for _, t := range threads {
		data, _ := json.Marshal(t)
		tokens := estimateTokens(string(data))

		if tokens > s.maxChunkTokens {
			if len(currentChunk) > 0 {
				chunks = append(chunks, currentChunk)
				currentChunk = nil
				currentTokens = 0
			}
			chunks = append(chunks, []threadData{t})
			continue
		}

		if currentTokens+tokens > s.maxChunkTokens && len(currentChunk) > 0 {
			chunks = append(chunks, currentChunk)
			currentChunk = nil
			currentTokens = 0
		}

		currentChunk = append(currentChunk, t)
		currentTokens += tokens
	}

	if len(currentChunk) > 0 {
		chunks = append(chunks, currentChunk)
	}

	return chunks
}

var jsonCodeBlockRe = regexp.MustCompile("(?s)```(?:json)?\\s*\\n?(.*?)\\n?```")

// parseTopicsJSON attempts to parse topics from AI response, handling markdown code blocks.
func parseTopicsJSON(response string) ([]Topic, error) {
	response = strings.TrimSpace(response)

	var result extractionResponse

	if err := json.Unmarshal([]byte(response), &result); err == nil {
		return result.Topics, nil
	}

	if matches := jsonCodeBlockRe.FindStringSubmatch(response); len(matches) >= 2 {
		if err := json.Unmarshal([]byte(strings.TrimSpace(matches[1])), &result); err == nil {
			return result.Topics, nil
		}
	}

	// Try finding JSON object in the response
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")
	if start >= 0 && end > start {
		if err := json.Unmarshal([]byte(response[start:end+1]), &result); err == nil {
			return result.Topics, nil
		}
	}

	return nil, fmt.Errorf("failed to parse JSON from AI response: %.200s", response)
}

// threadsToJSON converts collected threads into a JSON-serializable format.
func threadsToJSON(threads []slack.CollectedThread) []threadData {
	data := make([]threadData, 0, len(threads))
	for _, t := range threads {
		td := threadData{
			Channel:      "#" + t.ChannelName,
			Permalink:    t.Permalink,
			MatchReasons: t.MatchReasons,
		}
		for _, m := range t.Messages {
			td.Messages = append(td.Messages, messageData{
				User: m.User,
				Text: m.Text,
				Time: m.DateTime.Format("2006-01-02 15:04"),
			})
		}
		data = append(data, td)
	}
	return data
}

// estimateTokens estimates the token count for a string.
// For Japanese text, 1 character ~ 1-2 tokens; we use rune count as a safe estimate.
func estimateTokens(s string) int {
	return len([]rune(s))
}
