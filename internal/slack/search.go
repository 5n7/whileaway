package slack

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

const maxSearchPages = 50

// SearchResult holds a search hit with its source information.
type SearchResult struct {
	ChannelID   string
	ChannelName string
	ThreadTS    string // Parent message timestamp (best effort)
	MessageTS   string
	MatchReason string
	Permalink   string
}

// SearchMentions searches for messages mentioning the given user ID.
func (c *Client) SearchMentions(ctx context.Context, userID, fromDate, toDate string) ([]SearchResult, error) {
	query := fmt.Sprintf("<@%s> after:%s before:%s", userID, fromDate, toDate)
	return c.searchAll(ctx, query, "mention")
}

// SearchKeyword searches for messages matching a keyword.
func (c *Client) SearchKeyword(ctx context.Context, keyword, fromDate, toDate string) ([]SearchResult, error) {
	query := fmt.Sprintf("%s after:%s before:%s", keyword, fromDate, toDate)
	return c.searchAll(ctx, query, "keyword:"+keyword)
}

// SearchChannel searches for all messages in a channel within a date range.
func (c *Client) SearchChannel(ctx context.Context, channelName, fromDate, toDate string) ([]SearchResult, error) {
	query := fmt.Sprintf("in:#%s after:%s before:%s", channelName, fromDate, toDate)
	return c.searchAll(ctx, query, "channel:"+channelName)
}

// searchAll paginates through all search results for the given query.
func (c *Client) searchAll(ctx context.Context, query, matchReason string) ([]SearchResult, error) {
	var results []SearchResult
	page := 1

	for {
		resp, err := c.SearchMessages(ctx, query, page)
		if err != nil {
			return nil, err
		}

		for _, match := range resp.Matches {
			if match.Channel.ID == "" {
				continue
			}

			threadTS := match.Timestamp
			if plTS := extractTSFromPermalink(match.Permalink); plTS != "" {
				threadTS = plTS
			}

			results = append(results, SearchResult{
				ChannelID:   match.Channel.ID,
				ChannelName: strings.TrimPrefix(match.Channel.Name, "#"),
				ThreadTS:    threadTS,
				MessageTS:   match.Timestamp,
				MatchReason: matchReason,
				Permalink:   match.Permalink,
			})
		}

		if page >= resp.Paging.Pages || page >= maxSearchPages || len(resp.Matches) == 0 {
			break
		}
		page++
	}

	return results, nil
}

// permalinkTSRe extracts the timestamp from a Slack permalink.
// Permalink format: https://workspace.slack.com/archives/CXXXX/p1234567890123456
var permalinkTSRe = regexp.MustCompile(`/p(\d{10})(\d{6})(?:\?.*)?$`)

// extractTSFromPermalink extracts a Slack timestamp from a permalink URL.
func extractTSFromPermalink(permalink string) string {
	if matches := permalinkTSRe.FindStringSubmatch(permalink); len(matches) >= 3 {
		return matches[1] + "." + matches[2]
	}
	return ""
}
