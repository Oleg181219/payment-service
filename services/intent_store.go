package services

import (
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// PaymentIntent — хранит Intent (реквизиты для оплаты), связанный с конкретным заказом
type PaymentIntent struct {
	ID              string    // = intentId (генерируем новый uuid для Intent’а)
	OrderIdUUID     string    // = UUID заказа
	MerchantAddress string    // TON-адрес мерчанта
	TonComment      string    // "ORD-XXXXXX" для TON
	AmountTon       string    // сумма TON (строкой)
	ExpiresAt       time.Time // срок жизни Intent’а
	CreatedAt       time.Time
}

// IntentStore — in‑memory storage
type IntentStore struct {
	mu       sync.RWMutex
	intents  map[string]PaymentIntent // key = intentId
	merchant string
	ttl      time.Duration
}

// NewIntentStore — создаёт новый стор с дефолтным TTL
func NewIntentStore(merchant string, defaultTTL time.Duration) *IntentStore {
	if defaultTTL <= 0 {
		defaultTTL = 20 * time.Minute
	}
	return &IntentStore{
		intents:  make(map[string]PaymentIntent),
		merchant: strings.TrimSpace(merchant),
		ttl:      defaultTTL,
	}
}

// Create — создаёт новый Intent для заказа
func (s *IntentStore) Create(orderIdUUID string, amountTon string, ttl time.Duration) PaymentIntent {
	if ttl <= 0 {
		ttl = s.ttl
	}
	now := time.Now().UTC()

	intentId := uuid.New().String()
	tonComment := CommentFromOrderId(orderIdUUID)

	pi := PaymentIntent{
		ID:              intentId,
		OrderIdUUID:     orderIdUUID,
		MerchantAddress: s.merchant,
		TonComment:      tonComment,
		AmountTon:       strings.TrimSpace(amountTon),
		ExpiresAt:       now.Add(ttl),
		CreatedAt:       now,
	}

	s.mu.Lock()
	s.intents[intentId] = pi
	s.mu.Unlock()

	return pi
}

// Merchant — возвращает адрес мерчанта
func (s *IntentStore) Merchant() string { return s.merchant }

// Get — достаёт Intent по его ID, если он не просрочен
func (s *IntentStore) Get(intentId string) (PaymentIntent, bool) {
	s.mu.RLock()
	pi, ok := s.intents[intentId]
	s.mu.RUnlock()
	if !ok {
		return PaymentIntent{}, false
	}
	if time.Now().UTC().After(pi.ExpiresAt) {
		return PaymentIntent{}, false
	}
	return pi, true
}

// CleanupExpired — удаляет все просроченные интенты
func (s *IntentStore) CleanupExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	for id, pi := range s.intents {
		if now.After(pi.ExpiresAt) {
			delete(s.intents, id)
		}
	}
}