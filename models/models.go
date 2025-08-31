package models

import "time"

type Response = APIResponse
type PaymentValidationRequest = ValidateTxRequest

type TransactionInfo struct {
	Hash      string    `json:"hash"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Amount    string    `json:"amount"`
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Comment   string    `json:"comment,omitempty"`
	Currency  string    `json:"currency,omitempty"`
}

// Если где-то нужна проверка по txHash (не для нашего сервиса)
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
	Comment      string `json:"comment,omitempty"`
}

type ValidateTxRequest struct {
	WalletAddress string `json:"wallet_address" binding:"required"`
	TxHash        string `json:"tx_hash" binding:"required"`
}

type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

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

// Это то, что использует TONService для поиска платежа:
type CheckPaymentRequest struct {
	MerchantAddress string
	Comment         string
	MinAmountTon    string // "3.000000000"
	Limit           int
}

type PaymentIntentCreateRequest struct {
	AmountTon string `json:"amountTon,omitempty"` // опционально, "0.100000000"
	TtlSec    int    `json:"ttlSec,omitempty"`    // опционально, дефолт 1200 (20 мин)
}

type PaymentIntentResponse struct {
	IntentId        string    `json:"intentId"`
	MerchantAddress string    `json:"merchantAddress"`
	Comment         string    `json:"comment"`
	AmountTon       string    `json:"amountTon,omitempty"`
	ExpiresAt       time.Time `json:"expiresAt"`
}

type PaymentIntentWaitRequest struct {
	IntentId string `json:"intentId" binding:"required"`
	TimeoutSec int  `json:"timeoutSec,omitempty"` // дефолт 60
}