package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/redis/go-redis/v9"

	"ai-provider/internal/api/handlers"
	"ai-provider/internal/config"
	"ai-provider/internal/models"
	"ai-provider/internal/storage"
)

// Version information (set via ldflags during build)
var (
	Version   = "1.0.0"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

func main() {
	// Parse command-line flags
	showVersion := flag.Bool("version", false, "Show version information")
	configFile := flag.String("config", "", "Path to configuration file")
	flag.Parse()

	// Show version and exit
	if *showVersion {
		fmt.Printf("AI Provider Version: %s\n", Version)
		fmt.Printf("Build Time: %s\n", BuildTime)
		fmt.Printf("Git Commit: %s\n", GitCommit)
		os.Exit(0)
	}

	log.Printf("Starting AI Provider v%s", Version)

	// Load configuration
	cfg, err := loadConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize database
	db, err := initializeDatabase(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Initialize Redis cache
	cache, err := initializeCache(cfg)
	if err != nil {
		log.Printf("Warning: Failed to initialize cache: %v", err)
		// Continue without cache
	}

	// Initialize model registry
	registry := models.NewRegistry(db, cache)

	// Initialize download manager
	downloadConfig := models.DownloadConfig{
		MaxThreads:       4,
		ChunkSize:        10 * 1024 * 1024, // 10MB
		Timeout:          30 * time.Minute,
		RetryAttempts:    3,
		RetryDelay:       5 * time.Second,
		ResumeEnabled:    true,
		ProgressInterval: 1 * time.Second,
	}
	downloadMgr := models.NewDownloadManager(registry, downloadConfig)

	// Initialize model manager
	managerConfig := &models.ManagerConfig{
		MaxConcurrentDownloads: 3,
		AutoValidate:           true,
		AutoActivate:           false,
		DownloadTimeout:        30 * time.Minute,
		ValidationTimeout:      5 * time.Minute,
		ModelStoragePath:       cfg.Storage.ModelsPath,
		TempPath:               "/tmp/ai-provider",
	}
	manager := models.NewModelManager(registry, downloadMgr, managerConfig)

	// Initialize API handlers
	handlerConfig := &handlers.Config{
		Manager: manager,
	}
	modelHandlers := handlers.NewModelHandlers(handlerConfig)

	// Setup HTTP server
	router := mux.NewRouter()

	// Register routes
	setupRoutes(router, modelHandlers)

	// Apply middleware
	router.Use(loggingMiddleware)
	router.Use(corsMiddleware)
	router.Use(recoveryMiddleware)

	// Create HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.System.Host, cfg.System.Port),
		Handler:      router,
		ReadTimeout:  cfg.System.ReadTimeout,
		WriteTimeout: cfg.System.WriteTimeout,
		IdleTimeout:  cfg.System.IdleTimeout,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Server starting on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Give outstanding requests 30 seconds to complete
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}

func loadConfig(configPath string) (*config.Config, error) {
	configManager := config.NewManager()

	if err := configManager.Load(configPath); err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	return configManager.Get(), nil
}

func initializeDatabase(cfg *config.Config) (*sql.DB, error) {
	dbConfig := &storage.DatabaseConfig{
		Host:           cfg.Storage.Database.Host,
		Port:           cfg.Storage.Database.Port,
		Name:           cfg.Storage.Database.Name,
		User:           cfg.Storage.Database.User,
		Password:       cfg.Storage.Database.Password,
		SSLMode:        cfg.Storage.Database.SSLMode,
		MaxConnections: cfg.Storage.Database.MaxConnections,
	}

	db := storage.NewDatabase(dbConfig)
	if err := db.Connect(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := db.InitializeSchema(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to initialize database schema: %w", err)
	}

	return db.GetDB(), nil
}

func initializeCache(cfg *config.Config) (*redis.Client, error) {
	cacheConfig := &storage.CacheConfig{
		Host:     cfg.Storage.Cache.Host,
		Port:     cfg.Storage.Cache.Port,
		Password: cfg.Storage.Cache.Password,
		DB:       cfg.Storage.Cache.DB,
		PoolSize: cfg.Storage.Cache.PoolSize,
	}

	cache := storage.NewCache(cacheConfig)
	if err := cache.Connect(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to connect to cache: %w", err)
	}

	return cache.GetClient(), nil
}

func setupRoutes(router *mux.Router, handlers *handlers.ModelHandlers) {
	// Health endpoints
	router.HandleFunc("/health", handlers.HealthCheck).Methods("GET")
	router.HandleFunc("/ready", handlers.ReadinessCheck).Methods("GET")
	router.HandleFunc("/version", VersionHandler).Methods("GET")

	// API v1 routes
	api := router.PathPrefix("/api/v1").Subrouter()

	// Model management
	api.HandleFunc("/models", handlers.ListModels).Methods("GET")
	api.HandleFunc("/models", handlers.RegisterModel).Methods("POST")
	api.HandleFunc("/models/{id}", handlers.GetModel).Methods("GET")
	api.HandleFunc("/models/{id}", handlers.UpdateModel).Methods("PUT")
	api.HandleFunc("/models/{id}", handlers.DeleteModel).Methods("DELETE")

	// Model operations
	api.HandleFunc("/models/{id}/download", handlers.StartDownload).Methods("POST")
	api.HandleFunc("/models/{id}/download", handlers.CancelDownload).Methods("DELETE")
	api.HandleFunc("/models/{id}/progress", handlers.GetDownloadProgress).Methods("GET")
	api.HandleFunc("/models/{id}/validate", handlers.ValidateModel).Methods("POST")
	api.HandleFunc("/models/{id}/activate", handlers.ActivateModel).Methods("POST")
	api.HandleFunc("/models/{id}/deactivate", handlers.DeactivateModel).Methods("POST")

	// Model configuration
	api.HandleFunc("/models/{id}/config", handlers.GetModelConfig).Methods("GET")
	api.HandleFunc("/models/{id}/config", handlers.UpdateModelConfig).Methods("PUT")

	// Model versions
	api.HandleFunc("/models/{id}/versions", handlers.ListVersions).Methods("GET")
	api.HandleFunc("/models/{id}/versions", handlers.CreateVersion).Methods("POST")

	// Model statistics
	api.HandleFunc("/models/stats", handlers.GetModelStats).Methods("GET")
}

func VersionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"version":"%s","build_time":"%s","git_commit":"%s"}`, Version, BuildTime, GitCommit)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create a custom response writer to capture status code
		lrw := &loggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(lrw, r)

		duration := time.Since(start)
		log.Printf("[%s] %s %s %d %v", r.Method, r.RequestURI, r.RemoteAddr, lrw.statusCode, duration)
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("Panic recovered: %v", err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, `{"error":"internal server error"}`)
			}
		}()

		next.ServeHTTP(w, r)
	})
}

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}
