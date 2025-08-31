package services

import (
	"context"
	"fmt"
	"strings"
	"time"
	"payment-service/models"
	"github.com/shopspring/decimal"
)

type TONService struct{ client TonAPI }

func NewTONServiceWithClient(client TonAPI) *TONService { return &TONService{client: client} }

func equalsFold(a, b string) bool { return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b)) }

func nanosStrToTon(nanoStr string) (decimal.Decimal, error) {
	d, err := decimal.NewFromString(nanoStr); if err != nil { return decimal.Zero, err }
	return d.Div(decimal.NewFromInt(1_000_000_000)).Truncate(9), nil
}
func nanosIntToTonString(n int64) string {
	return decimal.NewFromInt(n).Div(decimal.NewFromInt(1_000_000_000)).Truncate(9).StringFixed(9)
}

func (s *TONService) CheckPayment(ctx context.Context, req models.CheckPaymentRequest) (bool, error) {
	limit := req.Limit; if limit <= 0 || limit > 200 { limit = 50 }
	minTon, err := decimal.NewFromString(req.MinAmountTon); if err != nil { return false, fmt.Errorf("bad MinAmountTon: %w", err) }

	evs, err := s.client.GetAccountEvents(ctx, req.MerchantAddress, limit)
	if err != nil { return false, fmt.Errorf("GetAccountEvents: %w", err) }

	for _, ev := range evs.Events {
		for _, a := range ev.Actions {
			if !equalsFold(a.Type, "TonTransfer") { continue }
			if !equalsFold(a.Recipient, req.MerchantAddress) { continue }
			comment := ""
			if a.Payload != nil && equalsFold(a.Payload.Type, "comment") { comment = a.Payload.Text }
			if comment != req.Comment { continue }
			ton, err := nanosStrToTon(a.Amount); if err != nil { continue }
			if ton.Cmp(minTon) >= 0 { return true, nil }
		}
	}
	return false, nil
}

func (s *TONService) WaitPayment(ctx context.Context, req models.CheckPaymentRequest, timeout, tick time.Duration) (bool, error) {
	if tick <= 0 { tick = 3 * time.Second }
	if timeout <= 0 { timeout = 30 * time.Second }
	deadline := time.Now().Add(timeout)
	for {
		ok, err := s.CheckPayment(ctx, req)
		if err != nil { return false, err }
		if ok { return true, nil }
		if time.Now().After(deadline) { return false, nil }
		select {
		case <-ctx.Done(): return false, ctx.Err()
		case <-time.After(tick):
		}
	}
}

func (s *TONService) GetAccountInfo(ctx context.Context, accountID string) (*models.AccountInfo, error) {
	balNanos, status, err := s.client.GetAccount(ctx, accountID)
	if err != nil { return nil, fmt.Errorf("failed to get account: %w", err) }
	return &models.AccountInfo{ Address: accountID, Balance: nanosIntToTonString(balNanos), Status: status }, nil
}

// Проверяем, что у адреса есть событие с таким txHash (event_id)
// и среди действий есть TonTransfer, где участвует walletAddress.
func (s *TONService) ValidateTransaction(ctx context.Context, txHash, walletAddress string) (bool, error) {
	evs, err := s.client.GetAccountEvents(ctx, walletAddress, 100)
	if err != nil {
		return false, fmt.Errorf("GetAccountEvents: %w", err)
	}
	for _, ev := range evs.Events {
		if ev.EventID != "" && ev.EventID == txHash {
			// Ищем участие адреса в переводе
			for _, a := range ev.Actions {
				if equalsFold(a.Type, "TonTransfer") &&
					(equalsFold(a.Recipient, walletAddress) || equalsFold(a.Sender, walletAddress)) {
					return true, nil
				}
			}
			// Если действий нет/не распарсили — на твое усмотрение.
			// Здесь считаем валидным по совпадению event_id.
			return true, nil
		}
	}
	return false, nil
}

// История TON-переводов (входящие/исходящие) по аккаунту.
// Маппим к models.TransactionInfo, как ждёт handler (Hash, From, To, Amount, Status, Timestamp, Comment, Currency).
func (s *TONService) GetTransactionHistory(ctx context.Context, accountID string, limit int) ([]models.TransactionInfo, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	evs, err := s.client.GetAccountEvents(ctx, accountID, limit)
	if err != nil {
		return nil, fmt.Errorf("GetAccountEvents: %w", err)
	}
	out := make([]models.TransactionInfo, 0, limit)
	for _, ev := range evs.Events {
		var ts time.Time
		if ev.Timestamp != nil && *ev.Timestamp > 0 {
			ts = time.Unix(*ev.Timestamp, 0).UTC()
		}
		for _, a := range ev.Actions {
			if !equalsFold(a.Type, "TonTransfer") {
				continue
			}
			amt, err := nanosStrToTon(a.Amount)
			if err != nil {
				continue
			}
			comment := ""
			if a.Payload != nil && equalsFold(a.Payload.Type, "comment") {
				comment = a.Payload.Text
			}
			out = append(out, models.TransactionInfo{
				Hash:      ev.EventID,
				From:      a.Sender,
				To:        a.Recipient,
				Amount:    amt.StringFixed(9),
				Status:    "ok",            // TonAPI обычно не даёт статус для action — ставим "ok"
				Timestamp: ts,
				Comment:   comment,
				Currency:  "TON",
			})
			if len(out) >= limit {
				return out, nil
			}
		}
	}
	return out, nil
}

// Баланс кошелька строкой "X.YYYYYYYYY"
func (s *TONService) GetWalletBalance(ctx context.Context, accountID string) (string, error) {
	nanos, _, err := s.client.GetAccount(ctx, accountID)
	if err != nil {
		return "", fmt.Errorf("GetAccount: %w", err)
	}
	return nanosIntToTonString(nanos), nil
}

// DevAddMockEvent — доступно только когда client == *MockTonAPIAdapter
func (s *TONService) DevAddMockEvent(accountID, sender, amountTon, comment string) error {
	m, ok := s.client.(*MockTonAPIAdapter)
	if !ok { return fmt.Errorf("mock mode is not enabled") }
	d, err := decimal.NewFromString(amountTon)
	if err != nil { return fmt.Errorf("bad amountTon: %w", err) }
	nanos := d.Mul(decimal.NewFromInt(1_000_000_000)).Truncate(0).String()
	if sender == "" { sender = "EQ_MOCK_SENDER" }
	m.AddEvent(accountID, sender, nanos, comment)
	return nil
}

// DebugLastEvents — пригодится для /api/debug/events
func (s *TONService) DebugLastEvents(ctx context.Context, accountID string, limit int) (Events, error) {
	if limit <= 0 || limit > 200 { limit = 10 }
	return s.client.GetAccountEvents(ctx, accountID, limit)
}