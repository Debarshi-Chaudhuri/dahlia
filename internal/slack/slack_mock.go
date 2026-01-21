package slack

import (
	"context"
	"dahlia/internal/logger"
)

type mockClient struct {
	logger logger.Logger
}

// NewMockClient creates a mock Slack client that logs messages
func NewMockClient(log logger.Logger) Client {
	return &mockClient{
		logger: log.With(logger.String("component", "slack_mock")),
	}
}

func (m *mockClient) SendMessage(ctx context.Context, channel, message string) error {
	m.logger.Info("MOCK: Slack message",
		logger.String("channel", channel),
		logger.String("message", message),
	)
	return nil
}
