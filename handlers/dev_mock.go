package handlers

import (
	"context"
	"net/http"
	"time"
	"strings"

	"github.com/gin-gonic/gin"
	"payment-service/models"
)

// POST /api/dev/mock-event
// { "merchantAddress": "...", "comment": "...", "amountTon": "0.100000000", "sender":"EQ_TEST" }
func (h *PaymentHandler) DevMockAddEvent(c *gin.Context) {
	var req struct {
		MerchantAddress string `json:"merchantAddress" binding:"required"`
		Comment         string `json:"comment" binding:"required"`
		AmountTon       string `json:"amountTon" binding:"required"`
		Sender          string `json:"sender"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success:false, Message:"Invalid: "+err.Error()}); return
	}
	if err := h.tonService.DevAddMockEvent(req.MerchantAddress, req.Sender, req.AmountTon, req.Comment); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success:false, Message:err.Error()}); return
	}
	c.JSON(http.StatusOK, models.APIResponse{Success:true, Message:"Mock event added"})
}

// POST /api/check-payment/wait (ждёт до 60с)
func (h *PaymentHandler) CheckPaymentWait(c *gin.Context) {
	var req models.CheckPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success:false, Message:"Invalid: "+err.Error()}); return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	ok, err := h.tonService.WaitPayment(ctx, req, 60*time.Second, 3*time.Second)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success:false, Message:"Error: "+err.Error()}); return
	}
	c.JSON(http.StatusOK, models.APIResponse{Success: ok, Message:"Wait result", Data: ok})
}

// POST /api/dev/mock-intent-paid { "intentId":"...", "sender":"EQ_TEST", "amountTon":"0.100000000" }
func (h *PaymentHandler) DevMockIntentPaid(c *gin.Context) {
	var req struct {
		IntentId string `json:"intentId" binding:"required"`
		Sender   string `json:"sender"`
		AmountTon string `json:"amountTon,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success:false, Message:"Invalid: "+err.Error()})
		return
	}
	pi, ok := h.intent.Get(req.IntentId)
	if !ok {
		c.JSON(http.StatusGone, models.APIResponse{Success:false, Message:"intent not found or expired"})
		return
	}
	amt := req.AmountTon
	if strings.TrimSpace(amt) == "" { amt = pi.AmountTon }
	if strings.TrimSpace(amt) == "" { amt = "0.000000001" } // минимальная
	if err := h.tonService.DevAddMockEvent(pi.MerchantAddress, req.Sender, amt, pi.Comment); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success:false, Message:err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{Success:true, Message:"Intent mocked as paid"})
}

// GET /api/debug/events/:account
func (h *PaymentHandler) DebugEvents(c *gin.Context) {
	account := c.Param("account")
	ctx, cancel := context.WithTimeout(context.Background(), h.config.RequestTimeout)
	defer cancel()
	evs, err := h.tonService.DebugLastEvents(ctx, account, 10)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success:false, Message:"TonAPI error: "+err.Error()}); return
	}
	c.JSON(http.StatusOK, models.APIResponse{Success:true, Message:"ok", Data: evs})
}