package services

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// In‑memory реализация TonAPI для dev/test.
type MockTonAPIAdapter struct {
	mu sync.RWMutex
	events map[string][]Event // accountID -> events (последние сверху)
}

func NewMockTonAPIAdapter() *MockTonAPIAdapter {
	return &MockTonAPIAdapter{events: make(map[string][]Event)}
}

// Добавить входящее событие TonTransfer на адрес accountID (merchant).
// amountNanos — строка в нанотонах, comment — текстовый комментарий.
func (m *MockTonAPIAdapter) AddEvent(accountID, sender, amountNanos, comment string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().Unix()
	ev := Event{
		EventID: uuid.New().String(),
		Timestamp: &now,
		Actions: []EventAction{
			{
				Type: "TonTransfer",
				Amount: amountNanos,
				Recipient: accountID,
				Sender: sender,
				Payload: &EventPayload{Type: "comment", Text: comment},
			},
		},
	}
	m.events[accountID] = append([]Event{ev}, m.events[accountID]...)
}

func (m *MockTonAPIAdapter) GetAccountEvents(ctx context.Context, accountID string, limit int) (Events, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := m.events[accountID]
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if len(list) > limit {
		list = list[:limit]
	}
	// копия слайса, чтобы не гонять внутренний
	out := make([]Event, len(list))
	copy(out, list)
	return Events{Events: out}, nil
}

func (m *MockTonAPIAdapter) GetAccount(ctx context.Context, accountID string) (int64, string, error) {
	return 0, "active", nil
}
func (m *MockTonAPIAdapter) GetAccountJettonsBalances(context.Context, string) (any, error) {
	return nil, nil
}
func (m *MockTonAPIAdapter) GetAccountNftItems(context.Context, string) ([]map[string]any, error) {
	return nil, nil
}