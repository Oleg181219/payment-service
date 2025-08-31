package services

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"payment-service/models"
)

type mockTonAPI struct {
	eventsFn   func(ctx context.Context, accountID string, limit int) (Events, error)
	accountFn  func(ctx context.Context, accountID string) (int64, string, error)
	jettonsFn  func(ctx context.Context, accountID string) (any, error)
	nftItemsFn func(ctx context.Context, accountID string) ([]map[string]any, error)
}
func (m *mockTonAPI) GetAccountEvents(ctx context.Context, accountID string, limit int) (Events, error) {
	return m.eventsFn(ctx, accountID, limit)
}
func (m *mockTonAPI) GetAccount(ctx context.Context, accountID string) (int64, string, error) {
	if m.accountFn == nil { return 0, "", nil }
	return m.accountFn(ctx, accountID)
}
func (m *mockTonAPI) GetAccountJettonsBalances(ctx context.Context, accountID string) (any, error) {
	if m.jettonsFn == nil { return nil, nil }
	return m.jettonsFn(ctx, accountID)
}
func (m *mockTonAPI) GetAccountNftItems(ctx context.Context, accountID string) ([]map[string]any, error) {
	if m.nftItemsFn == nil { return nil, nil }
	return m.nftItemsFn(ctx, accountID)
}

func TestCheckPayment_MatchTrue(t *testing.T) {
	mock := &mockTonAPI{
		eventsFn: func(ctx context.Context, accountID string, limit int) (Events, error) {
			return Events{Events: []Event{
				{ EventID: "E1", Actions: []EventAction{
					{ Type: "TonTransfer", Amount: "3000000000", Recipient: "EQ_MERCHANT",
						Payload: &EventPayload{Type: "comment", Text: "ORD-AB12CD34"} },
				}},
			}}, nil
		},
	}
	svc := NewTONServiceWithClient(mock)
	ok, err := svc.CheckPayment(context.Background(), models.CheckPaymentRequest{
		MerchantAddress: "EQ_MERCHANT",
		Comment:         "ORD-AB12CD34",
		MinAmountTon:    "3.000000000",
		Limit:           50,
	})
	if err != nil { t.Fatalf("err: %v", err) }
	if !ok { t.Fatalf("expected true") }
}

func TestCheckPayment_WrongComment(t *testing.T) {
	mock := &mockTonAPI{
		eventsFn: func(ctx context.Context, accountID string, limit int) (Events, error) {
			return Events{Events: []Event{
				{ EventID: "E1", Actions: []EventAction{
					{ Type: "TonTransfer", Amount: "3000000000", Recipient: "EQ_MERCHANT",
						Payload: &EventPayload{Type: "comment", Text: "ORD-OTHER"} },
				}},
			}}, nil
		},
	}
	svc := NewTONServiceWithClient(mock)
	ok, err := svc.CheckPayment(context.Background(), models.CheckPaymentRequest{
		MerchantAddress: "EQ_MERCHANT", Comment: "ORD-AB12CD34", MinAmountTon: "3.000000000",
	})
	if err != nil { t.Fatalf("err: %v", err) }
	if ok { t.Fatalf("expected false for wrong comment") }
}

func TestWaitPayment_TwoPolls(t *testing.T) {
	call := 0
	mock := &mockTonAPI{
		eventsFn: func(ctx context.Context, accountID string, limit int) (Events, error) {
			call++
			if call == 1 {
				return Events{Events: []Event{{ EventID: "E1", Actions: []EventAction{
					{ Type: "TonTransfer", Amount: "3000000000", Recipient: "EQ_MERCHANT",
						Payload: &EventPayload{Type: "comment", Text: "ORD-OTHER"} },
				}}}}, nil
			}
			return Events{Events: []Event{{ EventID: "E2", Actions: []EventAction{
				{ Type: "TonTransfer", Amount: "3000000000", Recipient: "EQ_MERCHANT",
					Payload: &EventPayload{Type: "comment", Text: "ORD-AB12CD34"} },
			}}}}, nil
		},
	}
	svc := NewTONServiceWithClient(mock)
	ok, err := svc.WaitPayment(context.Background(), models.CheckPaymentRequest{
		MerchantAddress: "EQ_MERCHANT", Comment: "ORD-AB12CD34", MinAmountTon: "3.000000000",
	}, 200*time.Millisecond, 50*time.Millisecond)
	if err != nil { t.Fatalf("err: %v", err) }
	if !ok { t.Fatalf("expected true on second poll") }
}

func TestNanosStrToTon(t *testing.T) {
	got, err := nanosStrToTon("3000000000")
	if err != nil { t.Fatalf("err: %v", err) }
	want := decimal.RequireFromString("3.000000000")
	if !got.Equal(want) { t.Fatalf("want %s got %s", want, got) }
}