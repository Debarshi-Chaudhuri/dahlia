package slack

import "context"

// Client defines interface for Slack notifications
type Client interface {
	SendMessage(ctx context.Context, channel, message string) error
}
