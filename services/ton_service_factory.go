package services

import (
"os"
"strings"
"payment-service/config"
)

func NewTONService(cfg *config.Config) (*TONService, error) {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("TONAPI_MODE")))
	if mode == "mock" || strings.EqualFold(cfg.TonApiURL, "mock") {
		return &TONService{client: NewMockTonAPIAdapter()}, nil
	}
	// реал/REST-адаптер (как сейчас)
	client := NewRestTonAPIAdapter(cfg.TonApiURL, cfg.ApiKey)
	return &TONService{client: client}, nil
}