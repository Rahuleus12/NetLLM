package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
)

// Version information (set via ldflags during build)
var (
	Version   = "1.0.0"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

// Config holds the server configuration
type Config struct {
	Host         string
	Port         int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

// Server represents the AI Provider server
type Server struct {
	config *Config
	router *mux.Router
	server *http.Server
}

// NewServer creates a new server instance
func NewServer(cfg *Config) *Server {
	router := mux.NewRouter()

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Handler:      router,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	return &Server{
		config: cfg,
		router: router,
		server: server,
	}
}

// setupRoutes configures all the routes for the server
func (s *Server) setupRoutes() {
	// Health check endpoint
	s.router.HandleFunc("/health", s.healthHandler).Methods("GET")

	// Readiness check endpoint
	s.router.HandleFunc("/ready", s.readinessHandler).Methods("GET")

	// Version endpoint
	s.router.HandleFunc("/version", s.versionHandler).Methods("GET")

	// API v1 routes
	api := s.router.PathPrefix("/api/v1").Subrouter()

	// Model management endpoints
	api.HandleFunc("/models", s.listModelsHandler).Methods("GET")
	api.HandleFunc("/models", s.uploadModelHandler).Methods("POST")
	api.HandleFunc("/models/{id}", s.getModelHandler).Methods("GET")
	api.HandleFunc("/models/{id}", s.deleteModelHandler).Methods("DELETE")

	// Inference endpoints
	api.HandleFunc("/inference", s.inferenceHandler).Methods("POST")
	api.HandleFunc("/inference/stream", s.streamInferenceHandler).Methods("POST")

	// Configuration endpoints
	api.HandleFunc("/config", s.getConfigHandler).Methods("GET")
	api.HandleFunc("/config", s.updateConfigHandler).Methods("PUT")

	// Monitoring endpoints
	api.HandleFunc("/metrics", s.metricsHandler).Methods("GET")
	api.HandleFunc("/stats", s.statsHandler).Methods("GET")

	// Apply middleware
	s.router.Use(s.loggingMiddleware)
	s.router.Use(s.corsMiddleware)
	s.router.Use(s.recoveryMiddleware)
}

// healthHandler returns the health status of the server
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"healthy","timestamp":"%s"}`, time.Now().UTC().Format(time.RFC3339))
}

// readinessHandler returns the readiness status of the server
func (s *Server) readinessHandler(w http.ResponseWriter, r *http.Request) {
	// TODO: Check database connection
	// TODO: Check model registry
	// TODO: Check GPU availability

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"ready":true,"timestamp":"%s"}`, time.Now().UTC().Format(time.RFC3339))
}

// versionHandler returns version information
func (s *Server) versionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"version":"%s","build_time":"%s","git_commit":"%s"}`, Version, BuildTime, GitCommit)
}

// Placeholder handlers for Phase 1 - will be implemented in later phases
func (s *Server) listModelsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"models":[],"message":"Model management will be implemented in Phase 2"}`)
}

func (s *Server) uploadModelHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	fmt.Fprintf(w, `{"error":"Model upload will be implemented in Phase 2"}`)
}

func (s *Server) getModelHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	modelID := vars["id"]
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	fmt.Fprintf(w, `{"error":"Model retrieval will be implemented in Phase 2","model_id":"%s"}`, modelID)
}

func (s *Server) deleteModelHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	modelID := vars["id"]
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	fmt.Fprintf(w, `{"error":"Model deletion will be implemented in Phase 2","model_id":"%s"}`, modelID)
}

func (s *Server) inferenceHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	fmt.Fprintf(w, `{"error":"Inference will be implemented in Phase 3"}`)
}

func (s *Server) streamInferenceHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	fmt.Fprintf(w, `{"error":"Streaming inference will be implemented in Phase 3"}`)
}

func (s *Server) getConfigHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"config":{},"message":"Configuration management will be implemented in Phase 1"}`)
}

func (s *Server) updateConfigHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	fmt.Fprintf(w, `{"error":"Configuration update will be implemented in Phase 1"}`)
}

func (s *Server) metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"metrics":{},"message":"Metrics will be implemented in Phase 1"}`)
}

func (s *Server) statsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"stats":{},"message":"Statistics will be implemented in Phase 1"}`)
}

// loggingMiddleware logs all incoming requests
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create a custom response writer to capture status code
		lrw := &loggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(lrw, r)

		duration := time.Since(start)
		log.Printf(
			"[%s] %s %s %d %v",
			r.Method,
			r.RequestURI,
			r.RemoteAddr,
			lrw.statusCode,
			duration,
		)
	})
}

// corsMiddleware handles CORS headers
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
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

// recoveryMiddleware recovers from panics and returns a 500 error
func (s *Server) recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("Panic recovered: %v", err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, `{"error":"Internal server error"}`)
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// loggingResponseWriter wraps http.ResponseWriter to capture status code
type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

// Start begins listening for requests
func (s *Server) Start() error {
	log.Printf("Starting AI Provider server on %s:%d", s.config.Host, s.config.Port)
	log.Printf("Version: %s, Build: %s, Commit: %s", Version, BuildTime, GitCommit)

	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	log.Println("Shutting down server gracefully...")
	return s.server.Shutdown(ctx)
}

// loadConfig loads configuration from environment variables and defaults
func loadConfig() *Config {
	return &Config{
		Host:         getEnv("AI_PROVIDER_HOST", "0.0.0.0"),
		Port:         getEnvAsInt("AI_PROVIDER_PORT", 8080),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

// getEnv gets environment variable with default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvAsInt gets environment variable as integer with default value
func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var intValue int
		if _, err := fmt.Sscanf(value, "%d", &intValue); err == nil {
			return intValue
		}
	}
	return defaultValue
}

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

	// Load configuration
	cfg := loadConfig()

	// Log if config file was specified (will be used in future implementation)
	if *configFile != "" {
		log.Printf("Config file specified: %s (will be implemented in future)", *configFile)
	}

	// Create server instance
	server := NewServer(cfg)

	// Setup routes
	server.setupRoutes()

	// Start server in a goroutine
	go func() {
		if err := server.Start(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// Give outstanding requests 30 seconds to complete
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown server
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}
