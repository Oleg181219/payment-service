package handlers

import (
	"net/http"
	"payment-service/models"
	"payment-service/services"

	"github.com/gin-gonic/gin"
)

type DevEvents struct { Bus *services.EventBus }

func (h *PaymentHandler) DevPublishIntentCreate(c *gin.Context) {
	var in struct{
		OrderId   string `json:"orderId" binding:"required"`
		AmountTon string `json:"amountTon" binding:"required"`
		TtlSec    int    `json:"ttlSec"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success:false, Message:"Invalid: "+err.Error()})
		return
	}
	key := in.OrderId
	if err := h.Events.Publish(c, "pay.intent.create", in, &key); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success:false, Message:err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{Success:true, Message:"published"})
}