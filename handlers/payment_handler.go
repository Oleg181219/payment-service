package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"payment-service/config"
	"payment-service/models"
	"payment-service/services"
)

// Handler для платежей
type PaymentHandler struct {
	tonService *services.TONService
	intent     *services.IntentStore
	Events     *services.EventBus
	config     *config.Config
}

func (h *PaymentHandler) TonService() *services.TONService   { return h.tonService }
func (h *PaymentHandler) IntentStore() *services.IntentStore { return h.intent }

// Конструктор
func NewPaymentHandler(cfg *config.Config, bus *services.EventBus) (*PaymentHandler, error) {
	tonService, err := services.NewTONService(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create TON service: %v", err)
	}
	if strings.TrimSpace(cfg.AppWallet) == "" {
		return nil, fmt.Errorf("merchant (AppWallet) is not configured")
	}

	intent := services.NewIntentStore(cfg.AppWallet, 20*time.Minute)

	return &PaymentHandler{
		tonService: tonService,
		intent:     intent,
		Events:     bus,
		config:     cfg,
	}, nil
}

// ========================== BASIC PAYMENT OPS =======================

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

// ========================== ACCOUNT INFO ===========================

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

// ========================== PAYMENT INTENT =========================

func (h *PaymentHandler) CreatePaymentIntent(c *gin.Context) {
	if h.intent == nil || strings.TrimSpace(h.intent.Merchant()) == "" {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Message: "merchant address is not configured",
		})
		return
	}

	var req models.PaymentIntentCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Message: "Invalid: " + err.Error(),
		})
		return
	}

	ttl := time.Duration(req.TtlSec) * time.Second
	pi := h.intent.Create(req.OrderIdUUID, req.AmountTon, ttl)

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Payment intent created",
		Data: models.PaymentIntentResponse{
			IntentId:        pi.ID,
			OrderIdUUID:     pi.OrderIdUUID,
			MerchantAddress: pi.MerchantAddress,
			TonComment:      pi.TonComment,
			AmountTon:       pi.AmountTon,
			ExpiresAt:       pi.ExpiresAt,
		},
	})
}

func (h *PaymentHandler) WaitPaymentByIntent(c *gin.Context) {
	var req models.PaymentIntentWaitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Message: "Invalid: " + err.Error(),
		})
		return
	}

	pi, ok := h.intent.Get(req.IntentId)
	if !ok {
		c.JSON(http.StatusGone, models.APIResponse{
			Success: false,
			Message: "intent not found or expired",
		})
		return
	}

	minAmt := pi.AmountTon
	if strings.TrimSpace(minAmt) == "" {
		minAmt = "0.000000000"
	}

	check := models.CheckPaymentRequest{
		MerchantAddress: pi.MerchantAddress,
		TonComment:      pi.TonComment,
		MinAmountTon:    minAmt,
		Limit:           100,
	}

	timeout := time.Duration(req.TimeoutSec) * time.Second
	if timeout <= 0 || timeout > 120*time.Second {
		timeout = 60 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	okPaid, err := h.tonService.WaitPayment(ctx, check, timeout, 3*time.Second)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Message: "Error: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: okPaid,
		Message: "Wait result",
		Data:    okPaid,
	})
}

// ========================== HEALTHCHECK ============================

// Readiness: проверяем только базу (TonAPI — мягко, только предупреждение)
func (h *PaymentHandler) HealthCheck(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	// Проверим базу (это критично для ready)
	if err := h.Events.DB.Ping(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "db down",
			"error":  err.Error(),
		})
		return
	}

	// Проверим TonAPI (по желанию, мягкий warning)
	if _, err := h.tonService.DebugLastEvents(ctx, h.config.AppWallet, 1); err != nil {
		// Логируем, но сервис остаётся Ready
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"warning": "tonapi unreachable: " + err.Error(),
		})
		return
	}

	// Всё ок
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// DebugTonEvents — отдает TonAPI события как есть
func (h *PaymentHandler) DebugTonEvents(c *gin.Context) {
	accountID := c.Param("account")
	limitStr := c.DefaultQuery("limit", "5")

	limit := 5
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
		limit = l
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), h.config.RequestTimeout)
	defer cancel()

	events, err := h.tonService.DebugLastEvents(ctx, accountID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, events)
}


// Liveness: всегда 200 OK
func (h *PaymentHandler) LivenessCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "alive"})
}