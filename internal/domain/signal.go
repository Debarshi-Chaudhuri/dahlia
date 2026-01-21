package domain

import (
	"time"

	"github.com/google/uuid"
)

// Signal represents an incoming signal from external systems
type Signal struct {
	SignalID   string                 `json:"signal_id" dynamodbav:"signal_id"`
	Timestamp  string                 `json:"timestamp" dynamodbav:"timestamp"` // ISO8601 format for sorting
	SignalType string                 `json:"signal_type" dynamodbav:"signal_type"`
	OrgID      string                 `json:"org_id" dynamodbav:"org_id"`
	Value      map[string]interface{} `json:"value" dynamodbav:"value"`
	Metadata   map[string]interface{} `json:"metadata" dynamodbav:"metadata"`
	CreatedAt  int64                  `json:"created_at" dynamodbav:"created_at"`

	// Composite key for GSI-1: signal_type#org_id
	SignalTypeOrgID string `json:"-" dynamodbav:"signal_type_org_id"`
}

// NewSignal creates a new signal
func NewSignal(signalType, orgID string, value, metadata map[string]interface{}, timestamp time.Time) *Signal {
	now := time.Now()

	if value == nil {
		value = make(map[string]interface{})
	}
	if metadata == nil {
		metadata = make(map[string]interface{})
	}

	return &Signal{
		SignalID:        uuid.New().String(),
		Timestamp:       timestamp.Format(time.RFC3339),
		SignalType:      signalType,
		OrgID:           orgID,
		Value:           value,
		Metadata:        metadata,
		CreatedAt:       now.UnixMilli(),
		SignalTypeOrgID: signalType + "#" + orgID,
	}
}
