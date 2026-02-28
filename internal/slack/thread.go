package slack

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"

	"github.com/slack-go/slack"
	"golang.org/x/sync/errgroup"
)

// ThreadRequest represents a request to fetch a thread.
type ThreadRequest struct {
	ChannelID   string
	ChannelName string
	ThreadTS    string
	Permalink   string
}

// ThreadFetcher fetches full threads with concurrency control.
type ThreadFetcher struct {
	client     *Client
	maxWorkers int
}

// NewThreadFetcher creates a new ThreadFetcher with the given concurrency limit.
func NewThreadFetcher(client *Client, maxWorkers int) *ThreadFetcher {
	if maxWorkers <= 0 {
		maxWorkers = 5
	}
	return &ThreadFetcher{client: client, maxWorkers: maxWorkers}
}

// FetchThreads fetches multiple threads concurrently with progress reporting.
func (f *ThreadFetcher) FetchThreads(ctx context.Context, requests []ThreadRequest, onProgress func(done, total int)) ([]CollectedThread, error) {
	total := len(requests)
	results := make([]CollectedThread, total)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(f.maxWorkers)

	var done atomic.Int64

	for i, req := range requests {
		g.Go(func() error {
			thread, err := f.fetchSingleThread(ctx, req)
			if err != nil {
				if isNonFatalSlackError(err) {
					fmt.Fprintf(os.Stderr, "  Skipping thread %s/%s: %v\n", req.ChannelID, req.ThreadTS, err)
				} else {
					return err
				}
			} else {
				results[i] = thread
			}

			if onProgress != nil {
				onProgress(int(done.Add(1)), total)
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("fetching threads: %w", err)
	}

	// Filter out skipped threads
	filtered := results[:0]
	for _, t := range results {
		if len(t.Messages) > 0 {
			filtered = append(filtered, t)
		}
	}

	return filtered, nil
}

// fetchSingleThread fetches all messages in a single thread and resolves user names.
func (f *ThreadFetcher) fetchSingleThread(ctx context.Context, req ThreadRequest) (CollectedThread, error) {
	msgs, err := f.client.GetThreadReplies(ctx, req.ChannelID, req.ThreadTS)
	if err != nil {
		return CollectedThread{}, err
	}

	messages := make([]SlackMessage, 0, len(msgs))
	for _, msg := range msgs {
		userName, _ := f.client.ResolveUserName(ctx, msg.User)

		messages = append(messages, SlackMessage{
			User:      userName,
			UserID:    msg.User,
			Text:      f.client.ResolveMentions(ctx, msg.Text),
			Timestamp: msg.Timestamp,
			DateTime:  ParseSlackTimestamp(msg.Timestamp),
		})
	}

	return CollectedThread{
		ChannelID:   req.ChannelID,
		ChannelName: req.ChannelName,
		ThreadTS:    req.ThreadTS,
		Permalink:   req.Permalink,
		Messages:    messages,
	}, nil
}

var nonFatalSlackErrors = []string{
	"missing_scope", "channel_not_found", "not_in_channel", "thread_not_found",
}

// isNonFatalSlackError returns true if the error is a Slack API error that can be safely skipped.
func isNonFatalSlackError(err error) bool {
	var slackErr slack.SlackErrorResponse
	if errors.As(err, &slackErr) {
		for _, e := range nonFatalSlackErrors {
			if slackErr.Err == e {
				return true
			}
		}
	}
	// Fallback: check error string for cases where the error type doesn't match
	errStr := err.Error()
	for _, e := range nonFatalSlackErrors {
		if strings.HasSuffix(errStr, e) {
			return true
		}
	}
	return false
}
