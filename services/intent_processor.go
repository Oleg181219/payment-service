package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"payment-service/models"

	"github.com/google/uuid"
)

// ------------ Payload structs -------------

// Запрос на создание Intent
type IntentCreatePayload struct {
	OrderIdUUID string `json:"orderId"`             // UUID заказа
	AmountTon   string `json:"amountTon"`
	TtlSec      int    `json:"ttlSec"`
	TelegramId  *int64 `json:"telegramId,omitempty"`
	BloggerId   *int64 `json:"bloggerId,omitempty"`
}

// Событие "intent создан"
type IntentCreatedPayload struct {
	OrderIdUUID     string    `json:"orderId"`        // UUID заказа
	IntentId        string    `json:"intentId"`
	MerchantAddress string    `json:"merchantAddress"`
	TonComment      string    `json:"tonComment"`     // "ORD-xxxxxx"
	AmountTon       string    `json:"amountTon"`
	ExpiresAt       time.Time `json:"expiresAt"`
}

// Событие "платёж подтверждён"
type PaymentConfirmedPayload struct {
	OrderIdUUID string    `json:"orderId"`   // UUID заказа
	IntentId    string    `json:"intentId"`
	TxHash      string    `json:"txHash"`
	AmountTon   string    `json:"amountTon"`
	TonComment  string    `json:"tonComment"` // тот самый "ORD-xxxxxx"
	FromAddress string    `json:"fromAddress"`
	ConfirmedAt time.Time `json:"confirmedAt"`
}

// ------------ Processor -------------

type IntentProcessor struct {
	Bus         *EventBus
	Ton         *TONService
	Merchant    string
	DefaultTTL  time.Duration
	MaxWatchers int
	watchSem    chan struct{}
}

func NewIntentProcessor(bus *EventBus, ton *TONService, merchant string, ttl time.Duration, maxWatchers int) *IntentProcessor {
	if ttl <= 0 {
		ttl = 20 * time.Minute
	}
	if maxWatchers <= 0 {
		maxWatchers = 50
	}
	return &IntentProcessor{
		Bus: bus, Ton: ton, Merchant: strings.TrimSpace(merchant),
		DefaultTTL: ttl,
		MaxWatchers: maxWatchers,
		watchSem: make(chan struct{}, maxWatchers),
	}
}

func (p *IntentProcessor) Start(ctx context.Context) {
	go p.loop(ctx)
}

func (p *IntentProcessor) loop(ctx context.Context) {
	topics := []string{"pay.intent.create"}
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rows, err := p.Bus.Claim(ctx, topics, 100)
			if err != nil {
				log.Printf("intent claim err: %v", err)
				continue
			}
			for _, r := range rows {
				if err := p.processCreate(ctx, r); err != nil {
					_ = p.Bus.Fail(ctx, r.ID, err.Error(), 30*time.Second)
				} else {
					_ = p.Bus.Ack(ctx, r.ID)
				}
			}
		}
	}
}

func (p *IntentProcessor) processCreate(ctx context.Context, r EventRow) error {
	var in IntentCreatePayload
	if err := json.Unmarshal(r.Payload, &in); err != nil {
		return err
	}
	if p.Merchant == "" {
		return fmt.Errorf("merchant address not configured")
	}

	// OrderId должен быть строго UUID
	_, err := uuid.Parse(in.OrderIdUUID)
	if err != nil {
		return fmt.Errorf("invalid orderId '%s': must be UUID", in.OrderIdUUID)
	}

	intentId := uuid.New().String()
	tonComment := CommentFromOrderId(in.OrderIdUUID)

	ttl := p.DefaultTTL
	if in.TtlSec > 0 {
		ttl = time.Duration(in.TtlSec) * time.Second
	}

	amt := strings.TrimSpace(in.AmountTon)
	if amt == "" {
		amt = "0.000000000"
	}
	expires := time.Now().UTC().Add(ttl)

	created := IntentCreatedPayload{
		OrderIdUUID:     in.OrderIdUUID,
		IntentId:        intentId,
		MerchantAddress: p.Merchant,
		TonComment:      tonComment,
		AmountTon:       amt,
		ExpiresAt:       expires,
	}
	key := in.OrderIdUUID
	if err := p.Bus.Publish(ctx, "pay.intent.created", created, &key); err != nil {
		return err
	}
	log.Printf("intent.created orderId=%s intentId=%s tonComment=%s amount=%s expires=%s",
		in.OrderIdUUID, intentId, tonComment, amt, expires.Format(time.RFC3339))

	// watcher
	p.spawnWatcher(ctx, in.OrderIdUUID, intentId, tonComment, amt, ttl)
	return nil
}

func (p *IntentProcessor) spawnWatcher(parent context.Context, orderIdUUID, intentId, tonComment, amountTon string, ttl time.Duration) {
	select {
	case p.watchSem <- struct{}{}:
	default:
		log.Printf("watcher backlog full, skip orderId=%s", orderIdUUID)
		return
	}

	go func() {
		defer func() { <-p.watchSem }()

		wctx, cancel := context.WithTimeout(parent, ttl)
		defer cancel()

		req := models.CheckPaymentRequest{
			MerchantAddress: p.Merchant,
			TonComment:      tonComment,
			MinAmountTon:    amountTon,
			Limit:           100,
		}

		ok, err := p.Ton.WaitPayment(wctx, req, ttl, 3*time.Second)
		if err != nil {
			log.Printf("waitPayment err orderId=%s intentId=%s: %v", orderIdUUID, intentId, err)
			return
		}
		if !ok {
			return
		}

		match, err := p.Ton.FindPayment(wctx, req)
		if err != nil {
			log.Printf("findPayment err orderId=%s intentId=%s: %v", orderIdUUID, intentId, err)
			return
		}
		if !match.Ok {
			log.Printf("⚠️ Платёж не найден по комментарию %s и сумме %s", req.TonComment, req.MinAmountTon)
			return
		}

		parsedOrderID := orderIdUUID

		confirmed := PaymentConfirmedPayload{
			OrderIdUUID: parsedOrderID,
			IntentId:    intentId,
			TxHash:      match.TxHash,
			AmountTon:   match.Amount,
			TonComment:  match.TonComment,
			FromAddress: match.FromAddress,
			ConfirmedAt: time.Now().UTC(),
		}

		key := parsedOrderID
		p.Bus.Publish(parent, "pay.payment.confirmed", confirmed, &key)
		if err := p.Bus.Publish(parent, "pay.payment.confirmed", confirmed, &key); err != nil {
			log.Printf("publish confirmed err orderId=%s intentId=%s: %v", orderIdUUID, intentId, err)
			return
		}
		log.Printf("payment.confirmed OK orderId=%s intentId=%s tx=%s amount=%s",
			orderIdUUID, intentId, match.TxHash, match.Amount)
	}()
}