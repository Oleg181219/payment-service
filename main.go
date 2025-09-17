package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/xssnick/tonutils-go/address"
	"payment-service/config"
	"payment-service/handlers"
	"payment-service/middleware"
	"payment-service/services"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	version   = "local"
	commit    = "dev"
	buildDate = ""
)

// Универсальная функция нормализации TON-адреса
func normalizeAddr(id string) (string, error) {
	if a, err := address.ParseAddr(id); err == nil {
		return a.StringRaw(), nil
	}
	if a, err := address.ParseRawAddr(id); err == nil {
		return a.StringRaw(), nil
	}
	return "", fmt.Errorf("invalid address: %s", id)
}

//
// Main entrypoint
//
func main() {
	log.Println("🚀 Starting Woolf Payment Service...")

	// Загружаем конфиг
	cfg := config.LoadConfig()

	// Валидируем и нормализуем кошелёк
	walletRaw, err := normalizeAddr(cfg.AppWallet)
	if err != nil {
		log.Fatalf("❌ Invalid or unparseable merchant address '%s': %v", cfg.AppWallet, err)
	}
	log.Printf("✅ AppWallet loaded: %s (RAW form: %s)", cfg.AppWallet, walletRaw)

	log.Printf("🔌 Config loaded: ServerPort=%s, AppWallet=%s", cfg.ServerPort, cfg.AppWallet)
	log.Printf("📊 Database URL: %s", cfg.DatabaseURL)

	if cfg.ServerPort == "" {
		cfg.ServerPort = "8082"
	}
	log.Printf("Starting woolf-payment-go version=%s commit=%s date=%s port=%s",
		version, commit, buildDate, cfg.ServerPort)

	// Контекст завершения
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Подключение к базе
	dbURL := cfg.DatabaseURL
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("pgxpool.New: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("Database ping failed: %v", err)
	}

	// EventBus
	bus := services.NewEventBus(pool)

	// Хэндлеры
	paymentHandler, err := handlers.NewPaymentHandler(cfg, bus)
	if err != nil {
		log.Fatalf("Failed to create payment handler: %v", err)
	}

	// IntentProcessor
	proc := services.NewIntentProcessor(
		bus,
		paymentHandler.TonService(),
		walletRaw,       // используем нормализованный кошелёк
		20*time.Minute,  // default TTL
		50,              // max watchers
	)
	proc.Start(ctx)

	// Gin router
	router := gin.New()
	router.Use(gin.Recovery(), middleware.Logger(), middleware.CORS())
	if err := router.SetTrustedProxies(nil); err != nil {
		log.Printf("SetTrustedProxies: %v", err)
	}

	api := router.Group("/api")
	{
		api.POST("/check-payment", paymentHandler.CheckPayment)
		api.POST("/validate-payment", paymentHandler.ValidatePayment)
		api.GET("/account-info/:account", paymentHandler.GetAccountInfo)
		api.GET("/transactions/:account", paymentHandler.GetTransactionHistory)
		api.GET("/balance/:account", paymentHandler.GetBalance)
		api.GET("/health", paymentHandler.HealthCheck)
		api.GET("/health/live", paymentHandler.LivenessCheck) // liveness: всегда 200
		api.GET("/health/ready", paymentHandler.HealthCheck)   // readiness: DB + TonAPI
		api.GET("/debug/events/:account", paymentHandler.DebugTonEvents)
		// Payment Intent
		api.POST("/payment-intent", paymentHandler.CreatePaymentIntent)
		api.POST("/payment-intent/wait", paymentHandler.WaitPaymentByIntent)
	}

	for _, r := range router.Routes() {
		log.Printf("ROUTE %-6s %s", r.Method, r.Path)
	}

	// HTTP server
	srv := &http.Server{
		Addr:              ":" + cfg.ServerPort,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      20 * time.Minute,
		IdleTimeout:       90 * time.Second,
	}

	go func() {
		log.Printf("HTTP server listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("Shutdown signal received, shutting down HTTP server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP shutdown error: %v", err)
	}

	log.Println("HTTP server stopped. Bye 👋")
}