package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/niammuddin/wa-gateway-v2/internal/auth"
	"github.com/niammuddin/wa-gateway-v2/internal/database"
	"github.com/niammuddin/wa-gateway-v2/internal/httpapi"
	"github.com/niammuddin/wa-gateway-v2/internal/queue"
	"github.com/niammuddin/wa-gateway-v2/internal/store"
	"github.com/niammuddin/wa-gateway-v2/internal/throttle"
	"github.com/niammuddin/wa-gateway-v2/internal/webhook"
	"github.com/niammuddin/wa-gateway-v2/internal/whatsapp"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	var dataStore store.Store = store.NewMemory()
	var messageQueue queue.Queue = queue.Nop{}
	var closeDB func() error
	var closeQueue func() error
	var authService *auth.Service
	var sessionManager *whatsapp.SessionManager
	var databaseConn *sql.DB
	var messageWorker *queue.Worker
	var webhookDispatcher *webhook.Dispatcher
	var limiter *throttle.Limiter
	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		db, err := database.Open(ctx, databaseURL)
		cancel()
		if err != nil {
			logger.Error("database connection failed", "error", err)
			os.Exit(1)
		}
		migrationDir, _ := filepath.Abs("migrations")
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		if err := database.Migrate(ctx, db, migrationDir); err != nil {
			cancel()
			_ = db.Close()
			logger.Error("database migration failed", "error", err)
			os.Exit(1)
		}
		cancel()
		dataStore = store.NewPostgres(db)
		databaseConn = db
		closeDB = db.Close
		jwtSecret := env("JWT_SECRET", "development-only-change-me")
		authService = auth.New(db, jwtSecret, env("JWT_REFRESH_SECRET", jwtSecret+"-refresh"))
		adminUsername, adminPassword := os.Getenv("ADMIN_USERNAME"), os.Getenv("ADMIN_PASSWORD")
		if adminUsername == "" || adminPassword == "" {
			logger.Error("ADMIN_USERNAME and ADMIN_PASSWORD are required when DATABASE_URL is configured")
			_ = db.Close()
			os.Exit(1)
		}
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		if err := authService.EnsureAdmin(ctx, adminUsername, adminPassword); err != nil {
			cancel()
			logger.Error("admin bootstrap failed", "error", err)
			os.Exit(1)
		}
		cancel()
		ctx, cancel = context.WithTimeout(context.Background(), 20*time.Second)
		sessionManager, err = whatsapp.NewSessionManager(ctx, databaseURL, dataStore)
		cancel()
		if err != nil {
			logger.Error("whatsapp store initialization failed", "error", err)
			os.Exit(1)
		}
		if err := sessionManager.Load(context.Background()); err != nil {
			logger.Error("whatsapp session restore failed", "error", err)
			os.Exit(1)
		}
	}
	if redisURL := os.Getenv("REDIS_URL"); redisURL != "" {
		q, err := queue.NewFromURL(redisURL)
		if err != nil {
			logger.Error("redis configuration failed", "error", err)
			os.Exit(1)
		}
		messageQueue = q
		closeQueue = q.Close
		if pending, err := dataStore.ListPendingQueueJobs(context.Background()); err == nil {
			if err := q.Recover(context.Background(), pending); err != nil {
				logger.Error("queue recovery failed", "error", err)
			}
		} else {
			logger.Error("queue pending-job lookup failed", "error", err)
		}
		if sessionManager != nil {
			if databaseConn != nil {
				webhookDispatcher = webhook.New(databaseConn)
				limiter = throttle.New(databaseConn)
				sessionManager.SetDispatcher(webhookDispatcher)
			}
			messageWorker, err = queue.NewWorker(redisURL, dataStore, func(sessionID string) (queue.Sender, bool) {
				client, ok := sessionManager.Client(sessionID)
				if !ok {
					return nil, false
				}
				return whatsapp.QueueSender{Client: client}, true
			}, webhookDispatcher, limiter)
			if err != nil {
				logger.Error("message worker configuration failed", "error", err)
				os.Exit(1)
			}
			go func() {
				if err := messageWorker.Run(); err != nil {
					logger.Error("message worker stopped", "error", err)
				}
			}()
		}
	}
	app := httpapi.NewWithAll(dataStore, messageQueue, authService, sessionManager, databaseConn, webhookDispatcher, logger)
	server := &http.Server{Addr: env("PORT", ":3000"), Handler: app.Handler(), ReadHeaderTimeout: 10 * time.Second}

	go func() {
		logger.Info("gateway listening", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server stopped", "error", err)
			os.Exit(1)
		}
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	<-signals
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
	if closeDB != nil {
		_ = closeDB()
	}
	if messageWorker != nil {
		messageWorker.Shutdown()
	}
	if closeQueue != nil {
		_ = closeQueue()
	}
	if sessionManager != nil {
		sessionManager.Close()
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
