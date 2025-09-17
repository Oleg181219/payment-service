package services

import (
	"context"
	"fmt"
	"log"
	"payment-service/models"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// TONService инкапсулирует работу с источником событий (TonAPI или Mock).
type TONService struct{ client TonAPI }

// Удобно для DI/тестов
func NewTONServiceWithClient(client TonAPI) *TONService {
	return &TONService{client: client}
}

// Безрегистровое сравнение (адреса/типы действий)
func equalsFold(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// "3000000000" -> 3.000000000 (Decimal, scale=9)
func nanosStrToTon(nanoStr string) (decimal.Decimal, error) {
	d, err := decimal.NewFromString(nanoStr)
	if err != nil {
		return decimal.Zero, err
	}
	return d.Div(decimal.NewFromInt(1_000_000_000)).Truncate(9), nil
}

// int64 nanos -> "X.YYYYYYYYY"
func nanosIntToTonString(n int64) string {
	return decimal.NewFromInt(n).Div(decimal.NewFromInt(1_000_000_000)).Truncate(9).StringFixed(9)
}

// Нормализация комментария (обрезаем пробелы)
func normalizeComment(s string) string {
	return strings.TrimSpace(s)
}

// Санитизация строки суммы TON (обрежем {{…}} и пробелы)
func sanitizeAmountTon(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "{{") && strings.HasSuffix(s, "}}") {
		s = strings.TrimSuffix(strings.TrimPrefix(s, "{{"), "}}")
	}
	return s
}

// normalizeAmountTon приводит сумму к каноническому виду: "X.YYYYYYYYY", scale=9.
func normalizeAmountTon(s string) (string, error) {
	s = sanitizeAmountTon(s)
	d, err := decimal.NewFromString(s)
	if err != nil {
		return "", err
	}
	return d.Truncate(9).StringFixed(9), nil
}

// ----------------------------------------------------------
// Проверка платежа (тонкий режим, возвращает bool)
// ----------------------------------------------------------
func (s *TONService) CheckPayment(ctx context.Context, req models.CheckPaymentRequest) (bool, error) {
	limit := req.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	normalizedMin, err := normalizeAmountTon(req.MinAmountTon)
	if err != nil {
		return false, fmt.Errorf("bad MinAmountTon: %w", err)
	}
	minTon, err := decimal.NewFromString(normalizedMin)
	if err != nil {
		return false, fmt.Errorf("bad MinAmountTon after normalization: %w", err)
	}

	evs, err := s.client.GetAccountEvents(ctx, req.MerchantAddress, limit)
	if err != nil {
		return false, fmt.Errorf("GetAccountEvents: %w", err)
	}

	wantComment := normalizeComment(req.TonComment)
	for _, ev := range evs.Events {
		for _, a := range ev.Actions {
			if !equalsFold(a.Type, "TonTransfer") {
				continue
			}
			if !equalsFold(a.Recipient, req.MerchantAddress) {
				continue
			}

			gotComment := ""
			if a.Payload != nil && equalsFold(a.Payload.Type, "comment") {
				gotComment = normalizeComment(a.Payload.Text)
			}
			if !strings.EqualFold(gotComment, wantComment) {
				log.Printf("Skip: comment '%s' != expected '%s'", gotComment, wantComment)
				continue
			}

			if a.Amount == "" {
				log.Printf("Skip action: Amount is empty in event %s", ev.EventID)
				continue
			}
			ton, err := nanosStrToTon(a.Amount)
			if err != nil {
				log.Printf("Invalid amount '%s' in event %s: %v", a.Amount, ev.EventID, err)
				continue
			}
			if ton.Cmp(minTon) >= 0 {
				return true, nil
			}
		}
	}
	return false, nil
}

// ----------------------------------------------------------
// Ожидание платежа c polling
// ----------------------------------------------------------
func (s *TONService) WaitPayment(ctx context.Context, req models.CheckPaymentRequest, timeout, tick time.Duration) (bool, error) {
	if tick <= 0 {
		tick = 3 * time.Second
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	deadline := time.Now().Add(timeout)

	for {
		ok, err := s.CheckPayment(ctx, req)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
		if time.Now().After(deadline) {
			return false, nil
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(tick):
		}
	}
}

// ----------------------------------------------------------
// Всякие вспомогательные методы
// ----------------------------------------------------------

func (s *TONService) GetAccountInfo(ctx context.Context, accountID string) (*models.AccountInfo, error) {
	balNanos, status, err := s.client.GetAccount(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}
	return &models.AccountInfo{
		Address: accountID,
		Balance: nanosIntToTonString(balNanos),
		Status:  status,
	}, nil
}

func (s *TONService) ValidateTransaction(ctx context.Context, txHash, walletAddress string) (bool, error) {
	evs, err := s.client.GetAccountEvents(ctx, walletAddress, 100)
	if err != nil {
		return false, fmt.Errorf("GetAccountEvents: %w", err)
	}
	for _, ev := range evs.Events {
		if ev.EventID == txHash {
			for _, a := range ev.Actions {
				if equalsFold(a.Type, "TonTransfer") &&
					(equalsFold(a.Recipient, walletAddress) || equalsFold(a.Sender, walletAddress)) {
					return true, nil // ✅ найден настоящий TonTransfer с этим кошельком
				}
			}
			// событие с таким ID есть, но TON transfer по кошельку не найден
			return false, nil
		}
	}
	return false, nil
}

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
			if a.Amount == "" {
				continue
			}
			amt, err := nanosStrToTon(a.Amount)
			if err != nil {
				continue
			}
			comment := ""
			if a.Payload != nil && equalsFold(a.Payload.Type, "comment") {
				comment = normalizeComment(a.Payload.Text)
			}
			out = append(out, models.TransactionInfo{
				Hash:      ev.EventID,
				From:      a.Sender,
				To:        a.Recipient,
				Amount:    amt.StringFixed(9),
				Status:    "ok",
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

func (s *TONService) GetWalletBalance(ctx context.Context, accountID string) (string, error) {
	nanos, _, err := s.client.GetAccount(ctx, accountID)
	if err != nil {
		return "", fmt.Errorf("GetAccount: %w", err)
	}
	return nanosIntToTonString(nanos), nil
}

func (s *TONService) DevAddMockEvent(accountID, sender, amountTon, comment string) error {
	m, ok := s.client.(*MockTonAPIAdapter)
	if !ok {
		return fmt.Errorf("mock mode is not enabled")
	}
	amountTon = sanitizeAmountTon(amountTon)
	comment = normalizeComment(comment)

	d, err := decimal.NewFromString(amountTon)
	if err != nil {
		return fmt.Errorf("bad amountTon: %w", err)
	}
	nanos := d.Mul(decimal.NewFromInt(1_000_000_000)).Truncate(0).String()
	if sender == "" {
		sender = "EQ_MOCK_SENDER"
	}
	m.AddEvent(accountID, sender, nanos, comment)
	return nil
}

// ----------------------------------------------------------
// FindPayment — возвращает детальную информацию о совпавшем платеже
// ----------------------------------------------------------

type PaymentMatch struct {
	Ok        bool
	TxHash    string // event_id из TonAPI
	Amount    string // TON, scale=9
	TonComment string // нормализованный комментарий (ORD-xxxxx)
	FromAddress string
}

func (s *TONService) FindPayment(ctx context.Context, req models.CheckPaymentRequest) (PaymentMatch, error) {
	limit := req.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	normalizedMin, err := normalizeAmountTon(req.MinAmountTon)
	if err != nil {
		return PaymentMatch{}, fmt.Errorf("bad MinAmountTon: %w", err)
	}
	minTon, err := decimal.NewFromString(normalizedMin)
	if err != nil {
		return PaymentMatch{}, fmt.Errorf("bad MinAmountTon after normalization: %w", err)
	}

	evs, err := s.client.GetAccountEvents(ctx, req.MerchantAddress, limit)
	if err != nil {
		return PaymentMatch{}, fmt.Errorf("GetAccountEvents: %w", err)
	}

	wantComment := normalizeComment(req.TonComment)
	for _, ev := range evs.Events {
		for _, a := range ev.Actions {
			if !equalsFold(a.Type, "TonTransfer") {
				continue
			}
			if !equalsFold(a.Recipient, req.MerchantAddress) {
				continue
			}

			gotComment := ""
			if a.Payload != nil && equalsFold(a.Payload.Type, "comment") {
				gotComment = normalizeComment(a.Payload.Text)
			}
			if !strings.EqualFold(gotComment, wantComment) {
				continue
			}

			if a.Amount == "" {
				continue
			}
			ton, err := nanosStrToTon(a.Amount)
			if err != nil {
				continue
			}
			if ton.Cmp(minTon) >= 0 {
				return PaymentMatch{
					Ok:         true,
					TxHash:     ev.EventID,
					Amount:     ton.StringFixed(9),
					TonComment: gotComment,
					FromAddress: a.Sender,
				}, nil
			}
		}
	}
	return PaymentMatch{Ok: false}, nil
}

// ----------------------------------------------------------
// Debug helper
// ----------------------------------------------------------
func (s *TONService) DebugLastEvents(ctx context.Context, accountID string, limit int) (Events, error) {
	if limit <= 0 || limit > 200 {
		limit = 10
	}
	return s.client.GetAccountEvents(ctx, accountID, limit)
}