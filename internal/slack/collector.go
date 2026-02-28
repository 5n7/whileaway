package slack

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
)

// Collector orchestrates message collection from multiple search sources.
type Collector struct {
	client  *Client
	fetcher *ThreadFetcher
}

// NewCollector creates a new Collector.
func NewCollector(client *Client) *Collector {
	return &Collector{
		client:  client,
		fetcher: NewThreadFetcher(client, 5),
	}
}

// Collect gathers threads based on the search parameters.
func (c *Collector) Collect(ctx context.Context, params CollectParams) ([]CollectedThread, error) {
	fromDate := params.From.Format("2006-01-02")
	toDate := params.To.Format("2006-01-02")

	var (
		mu         sync.Mutex
		allResults []SearchResult
	)

	g, gCtx := errgroup.WithContext(ctx)

	if params.Mentions {
		g.Go(func() error {
			fmt.Fprintf(os.Stderr, "Searching mentions...\n")
			results, err := c.client.SearchMentions(gCtx, params.UserID, fromDate, toDate)
			if err != nil {
				return fmt.Errorf("searching mentions: %w", err)
			}
			fmt.Fprintf(os.Stderr, "  found %d messages\n", len(results))
			mu.Lock()
			allResults = append(allResults, results...)
			mu.Unlock()
			return nil
		})
	}

	for _, kw := range params.Keywords {
		g.Go(func() error {
			fmt.Fprintf(os.Stderr, "Searching keyword %q...\n", kw)
			results, err := c.client.SearchKeyword(gCtx, kw, fromDate, toDate)
			if err != nil {
				return fmt.Errorf("searching keyword %q: %w", kw, err)
			}
			fmt.Fprintf(os.Stderr, "  found %d messages\n", len(results))
			mu.Lock()
			allResults = append(allResults, results...)
			mu.Unlock()
			return nil
		})
	}

	for _, ch := range params.Channels {
		g.Go(func() error {
			fmt.Fprintf(os.Stderr, "Searching channel #%s...\n", ch)
			results, err := c.client.SearchChannel(gCtx, ch, fromDate, toDate)
			if err != nil {
				return fmt.Errorf("searching channel #%s: %w", ch, err)
			}
			fmt.Fprintf(os.Stderr, "  found %d messages\n", len(results))
			mu.Lock()
			allResults = append(allResults, results...)
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	if len(allResults) == 0 {
		return nil, nil
	}

	type threadInfo struct {
		ChannelID    string
		ChannelName  string
		ThreadTS     string
		MatchReasons map[string]bool
	}

	threadMap := make(map[string]*threadInfo)
	for _, r := range allResults {
		key := r.ChannelID + ":" + r.ThreadTS
		if existing, ok := threadMap[key]; ok {
			existing.MatchReasons[r.MatchReason] = true
		} else {
			threadMap[key] = &threadInfo{
				ChannelID:   r.ChannelID,
				ChannelName: r.ChannelName,
				ThreadTS:    r.ThreadTS,
				MatchReasons: map[string]bool{
					r.MatchReason: true,
				},
			}
		}
	}

	var requests []ThreadRequest
	reasonsMap := make(map[string][]string)
	for key, info := range threadMap {
		requests = append(requests, ThreadRequest{
			ChannelID:   info.ChannelID,
			ChannelName: info.ChannelName,
			ThreadTS:    info.ThreadTS,
			Permalink:   generatePermalink(info.ChannelID, info.ThreadTS),
		})
		var reasons []string
		for reason := range info.MatchReasons {
			reasons = append(reasons, reason)
		}
		reasonsMap[key] = reasons
	}

	fmt.Fprintf(os.Stderr, "Fetching %d unique threads...\n", len(requests))

	threads, err := c.fetcher.FetchThreads(ctx, requests, func(done, total int) {
		fmt.Fprintf(os.Stderr, "  Fetching threads... %d/%d\n", done, total)
	})
	if err != nil {
		return nil, err
	}

	for i, t := range threads {
		key := t.ChannelID + ":" + t.ThreadTS
		threads[i].MatchReasons = reasonsMap[key]
	}

	return threads, nil
}

// generatePermalink constructs a Slack permalink URL from channel ID and thread timestamp.
func generatePermalink(channelID, threadTS string) string {
	tsClean := strings.ReplaceAll(threadTS, ".", "")
	return fmt.Sprintf("https://slack.com/archives/%s/p%s", channelID, tsClean)
}
