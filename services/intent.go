package services

import (
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type PaymentIntent struct {
	ID              string
	MerchantAddress string
	Comment         string // "ORD-XXXXXX"
	AmountTon       string // опц.
	ExpiresAt       time.Time
	CreatedAt       time.Time
}

type IntentStore struct {
	mu       sync.RWMutex
	intents  map[string]PaymentIntent
	merchant string
	ttl      time.Duration
}

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

// ORD-XXXXXX на базе UUID
func genOrd() string {
	s := strings.ToUpper(strings.ReplaceAll(uuid.New().String(), "-", ""))
	return "ORD-" + s[:6]
}

func (s *IntentStore) Create(amountTon string, ttl time.Duration) PaymentIntent {
	if ttl <= 0 {
		ttl = s.ttl
	}
	now := time.Now().UTC()
	pi := PaymentIntent{
		ID:              uuid.New().String(),
		MerchantAddress: s.merchant,
		Comment:         genOrd(),
		AmountTon:       strings.TrimSpace(amountTon),
		ExpiresAt:       now.Add(ttl),
		CreatedAt:       now,
	}
	s.mu.Lock()
	s.intents[pi.ID] = pi
	s.mu.Unlock()
	return pi
}

func (s *IntentStore) Get(id string) (PaymentIntent, bool) {
	s.mu.RLock()
	pi, ok := s.intents[id]
	s.mu.RUnlock()
	if !ok {
		return PaymentIntent{}, false
	}
	if time.Now().UTC().After(pi.ExpiresAt) {
		return PaymentIntent{}, false
	}
	return pi, true
}