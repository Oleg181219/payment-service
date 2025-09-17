package models

import "time"

// Универсальный API респонс
type Response = APIResponse
type PaymentValidationRequest = ValidateTxRequest

// ----------------------------------------------------------
// Транзакции / платежи
// ----------------------------------------------------------

type TransactionInfo struct {
	Hash      string    `json:"hash"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Amount    string    `json:"amount"`
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Comment   string    `json:"comment,omitempty"` // свободный текст-поле (если был в транзакции)
	Currency  string    `json:"currency,omitempty"`
}

// Вспомогательная структура (не для нашего сервиса напрямую)
type PaymentCheckByTxRequest struct {
	WalletAddress string `json:"wallet_address" binding:"required"`
	TxHash        string `json:"tx_hash" binding:"required"`
	SenderAddress string `json:"sender_address" binding:"required"`
	Amount        string `json:"amount" binding:"required"`
	Currency      string `json:"currency,omitempty"`
}

type TransferRequest struct {
	TargetWallet string `json:"target_wallet" binding:"required"`
	Amount       string `json:"amount" binding:"required"` // "X.YYYYYYYYY"
	Comment      string `json:"comment,omitempty"`         // здесь именно текстовый, NOT tonComment
}

type ValidateTxRequest struct {
	WalletAddress string `json:"wallet_address" binding:"required"`
	TxHash        string `json:"tx_hash" binding:"required"`
}

// ----------------------------------------------------------
// Универсальный ответ API
// ----------------------------------------------------------

type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// ----------------------------------------------------------
// История кошелька
// ----------------------------------------------------------

type WalletTxInfo struct {
	Hash      string    `json:"hash"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Amount    string    `json:"amount"`
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Comment   string    `json:"comment,omitempty"`
	Currency  string    `json:"currency,omitempty"`
}

// ----------------------------------------------------------
// Информация об аккаунте TON
// ----------------------------------------------------------

type AccountInfo struct {
	Address      string          `json:"address"`
	Balance      string          `json:"balance"` // TON "X.YYYYYYYYY"
	Status       string          `json:"status"`
	LastActivity time.Time       `json:"last_activity,omitempty"`
	Jettons      []JettonBalance `json:"jettons,omitempty"`
	NFTs         []NFTItem       `json:"nfts,omitempty"`
}

type JettonBalance struct {
	Name    string `json:"name"`
	Symbol  string `json:"symbol,omitempty"`
	Balance string `json:"balance"`
	Address string `json:"address,omitempty"`
}

type NFTItem struct {
	Address     string      `json:"address"`
	Name        string      `json:"name,omitempty"`
	Description string      `json:"description,omitempty"`
	Image       string      `json:"image,omitempty"`
	Metadata    interface{} `json:"metadata,omitempty"`
}

// ----------------------------------------------------------
// Работа с Intent’ами
// ----------------------------------------------------------

// Это то, что использует TONService для поиска платежа:
type CheckPaymentRequest struct {
	MerchantAddress string
	TonComment      string // ищем по комментарию в TON, напр. "ORD-39A700"
	MinAmountTon    string // "3.000000000"
	Limit           int
}

// Запрос на создание PaymentIntent
type PaymentIntentCreateRequest struct {
	OrderIdUUID string `json:"orderId"`              // UUID заказа (строкой)
	AmountTon   string `json:"amountTon,omitempty"`  // опционально, "0.100000000"
	TtlSec      int    `json:"ttlSec,omitempty"`     // опционально, дефолт 1200 (20 мин)
}

// Ответ / событие о создании Intent
type PaymentIntentResponse struct {
	IntentId        string    `json:"intentId"`
	OrderIdUUID     string    `json:"orderId"`       // UUID заказа
	MerchantAddress string    `json:"merchantAddress"`
	TonComment      string    `json:"tonComment"`    // строка вида "ORD-xxxxxx"
	AmountTon       string    `json:"amountTon,omitempty"`
	ExpiresAt       time.Time `json:"expiresAt"`
}

// Запрос на ожидание платежа по Intent
type PaymentIntentWaitRequest struct {
	IntentId   string `json:"intentId" binding:"required"`
	TimeoutSec int    `json:"timeoutSec,omitempty"` // дефолт 60
}

// ----------------------------------------------------------
// Подтверждение платежа (событие "pay.payment.confirmed")
// ----------------------------------------------------------

type PaymentConfirmedPayload struct {
	OrderIdUUID string    `json:"orderId"`   // UUID заказа
	IntentId    string    `json:"intentId"`
	TxHash      string    `json:"txHash"`
	AmountTon   string    `json:"amountTon"`
	TonComment  string    `json:"tonComment"` // тот самый "ORD-xxxxxx"
	ConfirmedAt time.Time `json:"confirmedAt"`
}