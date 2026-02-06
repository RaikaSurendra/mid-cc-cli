package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	log "github.com/sirupsen/logrus"

	"github.com/servicenow/claude-terminal-mid-service/internal/config"
	"github.com/servicenow/claude-terminal-mid-service/internal/logging"
	"github.com/servicenow/claude-terminal-mid-service/internal/middleware"
	"github.com/servicenow/claude-terminal-mid-service/internal/server"
	"github.com/servicenow/claude-terminal-mid-service/internal/session"
	"github.com/servicenow/claude-terminal-mid-service/internal/store"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Warn("No .env file found, using environment variables")
	}

	// Initialize configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// H7: Deduplicated shared logging setup
	logging.Setup(cfg)

	// C1: Validate auth token configuration
	if cfg.Security.APIAuthToken == "" {
		if cfg.Server.Mode == "release" {
			log.Fatal("API_AUTH_TOKEN must be configured in release mode")
		}
		log.Warn("API_AUTH_TOKEN is not configured; authentication is disabled (development mode only)")
	}

	log.Info("Starting Claude Terminal Service for ServiceNow MID Server")
	log.Infof("Service: %s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Infof("ServiceNow Instance: %s", cfg.ServiceNow.Instance)
	log.Infof("Workspace Base: %s", cfg.Workspace.BasePath)

	// Initialize PostgreSQL store (optional).
	var pgStore *store.PostgresStore
	if cfg.Database.Enabled() {
		log.Infof("Connecting to PostgreSQL at %s:%d/%s", cfg.Database.Host, cfg.Database.Port, cfg.Database.DBName)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		var err error
		pgStore, err = store.NewPostgresStore(ctx, cfg.Database)
		cancel()
		if err != nil {
			log.WithError(err).Warn("Failed to initialize PostgreSQL store; falling back to in-memory sessions")
			pgStore = nil
		}
	} else {
		log.Info("DB_HOST not set; running with in-memory session storage only")
	}

	// Initialize session manager
	sessionManager := session.NewManager(cfg, pgStore)

	// Recover stale sessions from previous run.
	{
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		sessionManager.RecoverSessions(ctx)
		cancel()
	}

	// Start session timeout checker
	go sessionManager.StartTimeoutChecker(context.Background())

	// Initialize HTTP server
	router := setupRouter(cfg)
	srv := server.New(cfg, sessionManager, router)
	srv.RegisterRoutes()

	// Start HTTP server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	httpServer := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	// H5: TLS support
	useTLS := cfg.Security.TLSCertPath != "" && cfg.Security.TLSKeyPath != ""
	if useTLS {
		httpServer.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	// Start server in goroutine
	go func() {
		if useTLS {
			log.Infof("HTTPS server listening on %s", addr)
			if err := httpServer.ListenAndServeTLS(cfg.Security.TLSCertPath, cfg.Security.TLSKeyPath); err != nil && err != http.ErrServerClosed {
				log.Fatalf("Failed to start TLS server: %v", err)
			}
		} else {
			log.Infof("HTTP server listening on %s", addr)
			if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("Failed to start server: %v", err)
			}
		}
	}()

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down server...")

	// Cleanup all sessions
	log.Info("Cleaning up sessions...")
	sessionManager.CleanupAll()

	// Close PostgreSQL store.
	if pgStore != nil {
		pgStore.Close()
	}

	// Shutdown HTTP server with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Errorf("Server forced to shutdown: %v", err)
	}

	log.Info("Server exited")
}

func setupRouter(cfg *config.Config) *gin.Engine {
	if cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(loggingMiddleware())
	router.Use(corsMiddleware(cfg))

	// H11: Per-IP rate limiting (10 req/s, burst of 20)
	rl := middleware.NewRateLimiter(10, 20)
	router.Use(rl.Middleware())

	return router
}

func loggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method

		c.Next()

		duration := time.Since(start)
		statusCode := c.Writer.Status()

		log.WithFields(log.Fields{
			"method":   method,
			"path":     path,
			"status":   statusCode,
			"duration": duration.Milliseconds(),
			"ip":       c.ClientIP(),
		}).Info("HTTP request")
	}
}

// C2: CORS middleware with configurable allowed origins (no more wildcard).
func corsMiddleware(cfg *config.Config) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(cfg.Security.CORSAllowedOrigins))
	for _, origin := range cfg.Security.CORSAllowedOrigins {
		allowed[origin] = struct{}{}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if _, ok := allowed[origin]; ok {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Vary", "Origin")
		}
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-User-ID")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
