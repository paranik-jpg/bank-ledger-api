package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/paranik-jpg/bank-ledger-api/internal/database"
	"github.com/paranik-jpg/bank-ledger-api/internal/handler"
	"github.com/paranik-jpg/bank-ledger-api/internal/middleware"
	"github.com/paranik-jpg/bank-ledger-api/internal/service"
)

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func main() {
	serverPort := getEnv("PORT", ":8080")
	if serverPort != "" && serverPort[0] != ':' {
		serverPort = ":" + serverPort
	}
	dbURL := getEnv("DATABASE_URL", getEnv("DB_URL", "postgresql://nikhil:Mahi1234@localhost:5432/bank_ledger?sslmode=disable&TimeZone=Asia/Kolkata"))
    jwtSecret := getEnv("JWT_SECRET", "0123456789abcdef0123456789abcdef")

	log.Printf("Connecting to database at: %s", dbURL)

	// 1. Open DB pool & 2. Verify database connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db, err := database.New(dbURL)
	if err != nil {
		log.Fatalf("Failed to open database connection pool: %v", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("Database connectivity check failed: %v", err)
	}
	log.Println("Database connection pool established and verified successfully.")

	// 3. Initialize repositories/services
	userService := service.NewUserService(db)
	authService := service.NewAuthService(db, jwtSecret, 24*time.Hour)
	transferService := service.NewTransferService(db)

	// 4. Initialize handlers
	userHandler := handler.NewUserHandler(userService)
	sessionHandler := handler.NewSessionHandler(authService)
	transferHandler := handler.NewTransferHandler(transferService)

	// 5. Initialize middlewares
	loggerMiddleware := middleware.NewLoggerMiddleware()
	recoveryMiddleware := middleware.NewRecoveryMiddleware()
	authMiddleware := middleware.NewAuthMiddleware(jwtSecret)

	// 6. Register routes
	mux := http.NewServeMux()

	// Public endpoints
	mux.HandleFunc("/api/v1/users", userHandler.Register)
	mux.HandleFunc("/api/v1/sessions", sessionHandler.Login)

	// Protected endpoints (guarded by auth middleware)
	mux.HandleFunc("/api/v1/transfers", authMiddleware.RequireAuth(transferHandler.CreateTransfer))

	// Optional endpoint for testing recovery
	mux.HandleFunc("/api/v1/panic-test", func(w http.ResponseWriter, r *http.Request) {
		panic("Testing panic recovery middleware")
	})

	// 7. Bind router with global middleware pipeline: Recovery -> Logger -> Mux
	pipeline := recoveryMiddleware.RecoverPanic(loggerMiddleware.LogRequest(mux))

	// 8. Create and configure http.Server with production timeouts
	srv := &http.Server{
		Addr:         serverPort,
		Handler:      pipeline,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// 9. Start server
	log.Printf("Server listening on %s...", serverPort)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server stopped with error: %v", err)
	}
}
