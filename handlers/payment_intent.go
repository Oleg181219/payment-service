package handlers

import (
	"context"
	"net/http"
	"time"
	"strings"

	"github.com/gin-gonic/gin"
	"payment-service/models"
)

func (h *PaymentHandler) CreatePaymentIntent(c *gin.Context) {
	var req models.PaymentIntentCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success:false, Message:"Invalid: "+err.Error()})
		return
	}
	ttl := time.Duration(req.TtlSec) * time.Second
	pi := h.intent.Create(req.AmountTon, ttl)

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Payment intent created",
		Data: models.PaymentIntentResponse{
			IntentId:        pi.ID,
			MerchantAddress: pi.MerchantAddress,
			Comment:         pi.Comment,
			AmountTon:       pi.AmountTon,
			ExpiresAt:       pi.ExpiresAt,
		},
	})
}

func (h *PaymentHandler) WaitPaymentByIntent(c *gin.Context) {
	var req models.PaymentIntentWaitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success:false, Message:"Invalid: "+err.Error()})
		return
	}
	pi, ok := h.intent.Get(req.IntentId)
	if !ok {
		c.JSON(http.StatusGone, models.APIResponse{Success:false, Message:"intent not found or expired"})
		return
	}
	// конструируем проверку из intent
	minAmt := pi.AmountTon
	if strings.TrimSpace(minAmt) == "" {
		minAmt = "0.000000000"
	}
	check := models.CheckPaymentRequest{
		MerchantAddress: pi.MerchantAddress,
		Comment:         pi.Comment,
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
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success:false, Message:"Error: "+err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{Success: okPaid, Message:"Wait result", Data: okPaid})
}