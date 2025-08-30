package models

import (
	"time"
)

type CheckPaymentRequest struct {
	WalletAddress string `json:"wallet_address" binding:"required"`
	TxHash        string `json:"tx_hash" binding:"required"`
	SenderAddress string `json:"sender_address" binding:"required"`
	Amount        string `json:"amount" binding:"required"`
	Currency      string `json:"currency,omitempty"`
}

type TransferRequest struct {
	TargetWallet string `json:"target_wallet" binding:"required"`
	Amount       string `json:"amount" binding:"required"`
	Comment      string `json:"comment,omitempty"`
}

type PaymentValidationRequest struct {
	WalletAddress string `json:"wallet_address" binding:"required"`
	TxHash        string `json:"tx_hash" binding:"required"`
}

type Response struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

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

type AccountInfo struct {
	Address      string          `json:"address"`
	Balance      string          `json:"balance"`
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
