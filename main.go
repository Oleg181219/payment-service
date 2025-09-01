package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"payment-service/config"
	"payment-service/handlers"
	"payment-service/middleware"
	"payment-service/services"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	// 1) Конфиг
	cfg := config.LoadConfig()

	// 2) Контекст с graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 3) Подключение к Postgres
	dbURL := cfg.DatabaseURL
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		log.Fatal("DATABASE_URL is not set")
	}
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("pgxpool: %v", err)
	}
	defer pool.Close()

	// 4) EventBus
	bus := services.NewEventBus(pool)

	// 5) Хендлеры (пробрасываем EventBus)
	paymentHandler, err := handlers.NewPaymentHandler(cfg, bus)
	if err != nil {
		log.Fatalf("Failed to create payment handler: %v", err)
	}

	// 6) Запускаем процессор intent'ов (Go обрабатывает pay.intent.create и публикует created/confirmed)
	proc := services.NewIntentProcessor(
		bus,
		paymentHandler.TonService(),
		cfg.AppWallet,   // адрес мерчанта из конфига
		20*time.Minute,  // TTL по умолчанию
		50,              // максимальное число watcher'ов
	)
	proc.Start(ctx)

	// 7) Роутер
	router := gin.New()
	router.Use(gin.Recovery(), middleware.Logger(), middleware.CORS())
	_ = router.SetTrustedProxies(nil) // не доверяем прокси по умолчанию

	api := router.Group("/api")
	{
		// Основные ручки
		api.POST("/check-payment",        paymentHandler.CheckPayment)
		api.POST("/validate-payment",     paymentHandler.ValidatePayment)
		api.GET ("/account-info/:account", paymentHandler.GetAccountInfo)
		api.GET ("/transactions/:account", paymentHandler.GetTransactionHistory)
		api.GET ("/balance/:account",      paymentHandler.GetBalance)
		api.GET ("/health",                paymentHandler.HealthCheck)

		// Payment Intent (REST-режим, если используете)
		api.POST("/payment-intent",       paymentHandler.CreatePaymentIntent)
		api.POST("/payment-intent/wait",  paymentHandler.WaitPaymentByIntent)

		// DEV / DEBUG
		api.POST("/dev/events/intent-create", paymentHandler.DevPublishIntentCreate)
		api.POST("/dev/mock-event",           paymentHandler.DevMockAddEvent)
		api.POST("/dev/mock-intent-paid",     paymentHandler.DevMockIntentPaid)
		api.POST("/check-payment/wait",       paymentHandler.CheckPaymentWait)
		api.GET ("/debug/events/:account",    paymentHandler.DebugEvents)
	}

	// Выведем список маршрутов разово
	for _, r := range router.Routes() {
		log.Printf("ROUTE %-6s %s", r.Method, r.Path)
	}

	// 8) Старт HTTP
	log.Printf("Server starting on port %s", cfg.ServerPort)
	if err := router.Run(":" + cfg.ServerPort); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}