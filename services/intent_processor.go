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

type IntentCreatePayload struct {
	OrderId    string `json:"orderId"`
	AmountTon  string `json:"amountTon"`
	TtlSec     int    `json:"ttlSec"`
	TelegramId *int64 `json:"telegramId,omitempty"`
	BloggerId  *int64 `json:"bloggerId,omitempty"`
}

type IntentCreatedPayload struct {
	OrderId         string    `json:"orderId"`
	IntentId        string    `json:"intentId"`
	MerchantAddress string    `json:"merchantAddress"`
	Comment         string    `json:"comment"`
	AmountTon       string    `json:"amountTon"`
	ExpiresAt       time.Time `json:"expiresAt"`
}

type PaymentConfirmedPayload struct {
	OrderId     string    `json:"orderId"`
	IntentId    string    `json:"intentId"`
	TxHash      string    `json:"txHash"`
	AmountTon   string    `json:"amountTon"`
	Comment     string    `json:"comment"`
	ConfirmedAt time.Time `json:"confirmedAt"`
}

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
		DefaultTTL: ttl, MaxWatchers: maxWatchers, watchSem: make(chan struct{}, maxWatchers),
	}
}

func genORD() string {
	s := strings.ToUpper(strings.ReplaceAll(uuid.New().String(), "-", ""))
	return "ORD-" + s[:6]
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

	intentId := uuid.New().String()
	comment := genORD()

	ttl := p.DefaultTTL
	if in.TtlSec > 0 {
		ttl = time.Duration(in.TtlSec) * time.Second
	}

	// sanitize amount
	amt := strings.TrimSpace(in.AmountTon)
	if amt == "" {
		amt = "0.000000000" // любой платёж с этим comment
	}
	expires := time.Now().UTC().Add(ttl)

	created := IntentCreatedPayload{
		OrderId:         in.OrderId,
		IntentId:        intentId,
		MerchantAddress: p.Merchant,
		Comment:         comment,
		AmountTon:       amt,
		ExpiresAt:       expires,
	}
	key := in.OrderId
	if err := p.Bus.Publish(ctx, "pay.intent.created", created, &key); err != nil {
		return err
	}
	log.Printf("intent.created orderId=%s intentId=%s comment=%s amount=%s expires=%s",
		in.OrderId, intentId, comment, amt, expires.Format(time.RFC3339))

	// watcher: ждём платёж по этому комменту
	p.spawnWatcher(ctx, in.OrderId, intentId, comment, amt, ttl)
	return nil
}

func (p *IntentProcessor) spawnWatcher(parent context.Context, orderId, intentId, comment, amountTon string, ttl time.Duration) {
	select {
	case p.watchSem <- struct{}{}:
	default:
		log.Printf("watcher backlog full, skip orderId=%s", orderId)
		return
	}
	go func() {
		defer func() { <-p.watchSem }()
		// уважим отмену приложения
		wctx, cancel := context.WithTimeout(parent, ttl)
		defer cancel()

		req := models.CheckPaymentRequest{
			MerchantAddress: p.Merchant,
			Comment:         comment,
			MinAmountTon:    amountTon,
			Limit:           100,
		}

		ok, err := p.Ton.WaitPayment(wctx, req, ttl, 3*time.Second)
		if err != nil {
			log.Printf("waitPayment err orderId=%s intentId=%s: %v", orderId, intentId, err)
		}
		if !ok {
			return
		}

		// Находим точный матч (txHash/amount/comment)
		match, err := p.Ton.FindPayment(wctx, req)
		if err != nil {
			log.Printf("findPayment err orderId=%s intentId=%s: %v", orderId, intentId, err)
			return
		}
		if !match.Ok {
			return
		}

		confirmed := PaymentConfirmedPayload{
			OrderId:     orderId,
			IntentId:    intentId,
			TxHash:      match.TxHash,
			AmountTon:   match.Amount,
			Comment:     match.Comment,
			ConfirmedAt: time.Now().UTC(),
		}
		// event_key = txHash (идемпотентность)
		key := match.TxHash
		if err := p.Bus.Publish(parent, "pay.payment.confirmed", confirmed, &key); err != nil {
			log.Printf("publish confirmed err orderId=%s intentId=%s: %v", orderId, intentId, err)
			return
		}
		log.Printf("payment.confirmed orderId=%s intentId=%s tx=%s amount=%s", orderId, intentId, match.TxHash, match.Amount)
	}()
}