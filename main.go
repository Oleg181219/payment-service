package main

import (
	"log"
	"payment-service/config"
	"payment-service/handlers"
	"payment-service/middleware"

	"github.com/gin-gonic/gin"
)

func main() {
	// Загрузка конфигурации
	cfg := config.LoadConfig()

	// Инициализация обработчиков
	paymentHandler, err := handlers.NewPaymentHandler(cfg)
	if err != nil {
		log.Fatalf("Failed to create payment handler: %v", err)
	}

	// Настройка роутера
	router := gin.Default()

	// Middleware
	router.Use(middleware.CORS())
	router.Use(middleware.Logger())

	// Маршруты
	api := router.Group("/api")
	{
		api.POST("/check-payment", paymentHandler.CheckPayment)
		api.POST("/validate-payment", paymentHandler.ValidatePayment)
		api.GET("/account-info/:account", paymentHandler.GetAccountInfo)
		api.GET("/transactions/:account", paymentHandler.GetTransactionHistory)
		api.GET("/balance/:account", paymentHandler.GetBalance)
		api.GET("/health", paymentHandler.HealthCheck)

		// DEV
		api.POST("/dev/mock-event", paymentHandler.DevMockAddEvent)
		api.POST("/check-payment/wait", paymentHandler.CheckPaymentWait)
		api.GET("/debug/events/:account", paymentHandler.DebugEvents)
	}
	for _, r := range router.Routes() {
		log.Printf("ROUTE %-6s %s", r.Method, r.Path)
	}

	// Запуск сервера
	log.Printf("Server starting on port %s", cfg.ServerPort)
	if err := router.Run(":" + cfg.ServerPort); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
