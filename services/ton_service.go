package services

import (
	"context"
	"fmt"
	"log"
	"payment-service/config"
	"payment-service/models"
	"strconv"

	"github.com/tonkeeper/tonapi-go"
)

type TONService struct {
	client *tonapi.Client
	config *config.Config
}

func NewTONService(cfg *config.Config) (*TONService, error) {
	security := &tonapi.Security{}
	if cfg.ApiKey != "" {
		security.Token = cfg.ApiKey
	}

	client, err := tonapi.NewClient(cfg.TonApiURL, security)
	if err != nil {
		return nil, fmt.Errorf("failed to create TON client: %v", err)
	}

	return &TONService{
		client: client,
		config: cfg,
	}, nil
}

func (s *TONService) CheckPayment(ctx context.Context, req models.CheckPaymentRequest) (bool, error) {
	// тут проверка транзакции
	return true, nil
}

func (s *TONService) GetAccountInfo(ctx context.Context, accountID string) (*models.AccountInfo, error) {
	account, err := s.client.GetAccount(ctx, tonapi.GetAccountParams{AccountID: accountID})
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %v", err)
	}

	info := &models.AccountInfo{
		Address: accountID,
		Balance: convertNanotonToTONString(account.Balance),
		Status:  string(account.Status),
	}

	// Получаем Jetton балансы
	balances, err := s.client.GetAccountJettonsBalances(ctx, tonapi.GetAccountJettonsBalancesParams{
		AccountID: accountID,
	})
	if err == nil {
		for _, balance := range balances.Balances {
			info.Jettons = append(info.Jettons, models.JettonBalance{
				Name:    balance.Jetton.Name,
				Symbol:  balance.Jetton.Symbol,
				Balance: balance.Balance,
				Address: balance.Jetton.Address,
			})
		}
	} else {
		log.Printf("Warning: failed to get jetton balances: %v", err)
	}

	// Получаем NFT items
	nftItems, err := s.client.GetAccountNftItems(ctx, tonapi.GetAccountNftItemsParams{
		AccountID: accountID,
	})
	if err == nil {
		for _, item := range nftItems.NftItems {
			nft := models.NFTItem{
				Address:     item.Address,
				Name:        item.Metadata["Name"].(string),
				Description: item.Metadata["description"].(string),
				Image:       item.Metadata["image"].(string),
				Metadata:    item.Metadata,
			}
			info.NFTs = append(info.NFTs, nft)
		}
	} else {
		log.Printf("Warning: failed to get NFT items: %v", err)
	}

	return info, nil
}

func (s *TONService) ValidateTransaction(ctx context.Context, txHash, walletAddress string) (bool, error) {
	// валидация транзакции
	return true, nil
}

func (s *TONService) GetTransactionHistory(ctx context.Context, accountID string, limit int) ([]models.TransactionInfo, error) {
	//истроия транзакций
	return result, nil
}

func (s *TONService) GetWalletBalance(ctx context.Context, accountID string) (string, error) {
	account, err := s.client.GetAccount(ctx, tonapi.GetAccountParams{AccountID: accountID})
	if err != nil {
		return "", fmt.Errorf("failed to get account balance: %v", err)
	}

	return convertNanotonToTONString(account.Balance), nil
}

// Вспомогательные функции для конвертации
func convertNanotonToTON(nanoton int64) float64 {
	return float64(nanoton) / 1e9
}

func convertNanotonToTONString(nanoton int64) string {
	return fmt.Sprintf("%.9f", float64(nanoton)/1e9)
}

func convertTONToNanoton(tonAmount string) (int64, error) {
	amount, err := strconv.ParseFloat(tonAmount, 64)
	if err != nil {
		return 0, err
	}
	return int64(amount * 1e9), nil
}
