package services

import "context"

type EventPayload struct {
	Type string // "comment"
	Text string
}
type EventAction struct {
	Type      string // "TonTransfer"
	Amount    string // "3000000000" (нанотоны)
	Recipient string
	Sender    string
	Payload   *EventPayload
}
type Event struct {
	EventID   string
	Timestamp *int64
	Actions   []EventAction
}
type Events struct {
	Events []Event
}

type TonAPI interface {
	GetAccountEvents(ctx context.Context, accountID string, limit int) (Events, error)
	GetAccount(ctx context.Context, accountID string) (balanceNanos int64, status string, err error)
	GetAccountJettonsBalances(ctx context.Context, accountID string) (any, error)
	GetAccountNftItems(ctx context.Context, accountID string) ([]map[string]any, error)
}