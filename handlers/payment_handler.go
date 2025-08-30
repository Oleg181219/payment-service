package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"payment-service/config"
	"payment-service/models"
	"payment-service/services"
)

type PaymentHandler struct {
	tonService *services.TONService
	config     *config.Config
}

func NewPaymentHandler(cfg *config.Config) (*PaymentHandler, error) {
	tonService, err := services.NewTONService(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create TON service: %v", err)
	}

	return &PaymentHandler{
		tonService: tonService,
		config:     cfg,
	}, nil
}

func (h *PaymentHandler) CheckPayment(c *gin.Context) {
	var req models.CheckPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Success: false,
			Message: "Invalid request: " + err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), h.config.RequestTimeout)
	defer cancel()

	isValid, err := h.tonService.CheckPayment(ctx, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Success: false,
			Message: "Error checking payment: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Success: isValid,
		Message: "Payment checked successfully",
		Data:    isValid,
	})
}

func (h *PaymentHandler) ValidatePayment(c *gin.Context) {
	var req models.PaymentValidationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Success: false,
			Message: "Invalid request: " + err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), h.config.RequestTimeout)
	defer cancel()

	isValid, err := h.tonService.ValidateTransaction(ctx, req.TxHash, req.WalletAddress)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Success: false,
			Message: "Validation failed: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Success: isValid,
		Message: "Payment validation completed",
		Data:    isValid,
	})
}

func (h *PaymentHandler) GetAccountInfo(c *gin.Context) {
	accountID := c.Param("account")

	ctx, cancel := context.WithTimeout(context.Background(), h.config.RequestTimeout)
	defer cancel()

	info, err := h.tonService.GetAccountInfo(ctx, accountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Success: false,
			Message: "Failed to get account info: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Success: true,
		Message: "Account information retrieved",
		Data:    info,
	})
}

func (h *PaymentHandler) GetTransactionHistory(c *gin.Context) {
	accountID := c.Param("account")
	limitStr := c.DefaultQuery("limit", "10")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 || limit > 100 {
		limit = 10
	}

	ctx, cancel := context.WithTimeout(context.Background(), h.config.RequestTimeout)
	defer cancel()

	transactions, err := h.tonService.GetTransactionHistory(ctx, accountID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Success: false,
			Message: "Failed to get transaction history: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Success: true,
		Message: "Transaction history retrieved successfully",
		Data:    transactions,
	})
}

func (h *PaymentHandler) GetBalance(c *gin.Context) {
	accountID := c.Param("account")

	ctx, cancel := context.WithTimeout(context.Background(), h.config.RequestTimeout)
	defer cancel()

	balance, err := h.tonService.GetWalletBalance(ctx, accountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Success: false,
			Message: "Failed to get balance: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Success: true,
		Message: "Balance retrieved successfully",
		Data: map[string]interface{}{
			"address": accountID,
			"balance": balance,
			"unit":    "TON",
		},
	})
}

func (h *PaymentHandler) HealthCheck(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Проверяем соединение с TON API
	_, err := h.tonService.GetWalletBalance(ctx, h.config.AppWallet)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Success: false,
			Message: "Service unavailable: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Success: true,
		Message: "Service is healthy",
		Data: map[string]interface{}{
			"timestamp": time.Now(),
			"version":   "1.0.0",
		},
	})
}
