package slack

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/slack-go/slack"
	"golang.org/x/time/rate"
)

// API defines the Slack API methods used by this package.
type API interface {
	SearchMessagesContext(ctx context.Context, query string, params slack.SearchParameters) (*slack.SearchMessages, error)
	GetConversationRepliesContext(ctx context.Context, params *slack.GetConversationRepliesParameters) ([]slack.Message, bool, string, error)
	GetUserInfoContext(ctx context.Context, userID string) (*slack.User, error)
}

// Client wraps the Slack API client with rate limiting and user caching.
type Client struct {
	api           API
	searchLimiter *rate.Limiter
	replyLimiter  *rate.Limiter
	userCache     map[string]string
	userCacheMu   sync.RWMutex
	maxRetries    int
}

// NewClient creates a new Slack client with rate limiting.
func NewClient(token string) *Client {
	return &Client{
		api:           slack.New(token),
		searchLimiter: rate.NewLimiter(rate.Every(time.Second), 1),          // Tier 2: ~1 req/s
		replyLimiter:  rate.NewLimiter(rate.Every(400*time.Millisecond), 1), // Tier 3: ~2.5 req/s
		userCache:     make(map[string]string),
		maxRetries:    3,
	}
}

// newClientWithAPI creates a client with a custom API implementation (for testing).
func newClientWithAPI(api API) *Client {
	return &Client{
		api:           api,
		searchLimiter: rate.NewLimiter(rate.Inf, 1),
		replyLimiter:  rate.NewLimiter(rate.Inf, 1),
		userCache:     make(map[string]string),
		maxRetries:    3,
	}
}

// SearchMessages performs a search.messages API call with rate limiting and retry.
func (c *Client) SearchMessages(ctx context.Context, query string, page int) (*slack.SearchMessages, error) {
	if err := c.searchLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter wait: %w", err)
	}

	params := slack.SearchParameters{
		Sort:  "timestamp",
		Count: 100,
		Page:  page,
	}

	var result *slack.SearchMessages
	if err := c.withRetry(ctx, func() error {
		var searchErr error
		result, searchErr = c.api.SearchMessagesContext(ctx, query, params)
		return searchErr
	}); err != nil {
		return nil, fmt.Errorf("search messages (query=%q, page=%d): %w", query, page, err)
	}
	return result, nil
}

// GetThreadReplies fetches all replies in a thread with rate limiting and retry.
func (c *Client) GetThreadReplies(ctx context.Context, channelID, threadTS string) ([]slack.Message, error) {
	var allMessages []slack.Message
	cursor := ""

	for {
		if err := c.replyLimiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limiter wait: %w", err)
		}

		var msgs []slack.Message
		var hasMore bool
		var nextCursor string

		if err := c.withRetry(ctx, func() error {
			var replyErr error
			params := &slack.GetConversationRepliesParameters{
				ChannelID: channelID,
				Timestamp: threadTS,
				Limit:     200,
				Cursor:    cursor,
			}
			msgs, hasMore, nextCursor, replyErr = c.api.GetConversationRepliesContext(ctx, params)
			return replyErr
		}); err != nil {
			return nil, fmt.Errorf("get thread replies (channel=%s, thread=%s): %w", channelID, threadTS, err)
		}

		allMessages = append(allMessages, msgs...)

		if !hasMore || nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	return allMessages, nil
}

// ResolveUserName resolves a Slack user ID to a display name, using cache.
func (c *Client) ResolveUserName(ctx context.Context, userID string) (string, error) {
	c.userCacheMu.RLock()
	if name, ok := c.userCache[userID]; ok {
		c.userCacheMu.RUnlock()
		return name, nil
	}
	c.userCacheMu.RUnlock()

	user, err := c.api.GetUserInfoContext(ctx, userID)
	if err != nil {
		return userID, nil
	}

	displayName := resolveDisplayName(user)

	c.userCacheMu.Lock()
	c.userCache[userID] = displayName
	c.userCacheMu.Unlock()

	return displayName, nil
}

// resolveDisplayName returns the best available display name for a Slack user.
func resolveDisplayName(user *slack.User) string {
	if user.Profile.DisplayName != "" {
		return user.Profile.DisplayName
	}
	if user.RealName != "" {
		return user.RealName
	}
	return user.Name
}

var mentionRe = regexp.MustCompile(`<@(U[A-Z0-9]+)>`)

// ResolveMentions replaces <@U1234> patterns in text with display names.
func (c *Client) ResolveMentions(ctx context.Context, text string) string {
	return mentionRe.ReplaceAllStringFunc(text, func(match string) string {
		sub := mentionRe.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		name, _ := c.ResolveUserName(ctx, sub[1])
		return "@" + name
	})
}

// withRetry executes fn with retry logic, backing off on rate limit errors.
func (c *Client) withRetry(ctx context.Context, fn func() error) error {
	var lastErr error
	for attempt := range c.maxRetries {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		var rlErr *slack.RateLimitedError
		if errors.As(lastErr, &rlErr) {
			retryAfter := rlErr.RetryAfter
			if retryAfter == 0 {
				retryAfter = time.Duration(attempt+1) * time.Second
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(retryAfter):
				continue
			}
		}

		return lastErr
	}
	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

// ParseSlackTimestamp converts a Slack timestamp (e.g. "1234567890.123456") to time.Time.
func ParseSlackTimestamp(ts string) time.Time {
	parts := strings.Split(ts, ".")
	if len(parts) == 0 {
		return time.Time{}
	}
	sec, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(sec, 0)
}
