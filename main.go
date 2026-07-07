package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/whtsky/copilot2api/internal/accounts"
	"github.com/whtsky/copilot2api/internal/upstream"
)

var version = "dev"

func main() {
	var (
		port        = flag.Int("port", 0, "Server port (env: COPILOT2API_PORT, default: 7777)")
		host        = flag.String("host", "", "Server host (env: COPILOT2API_HOST, default: 0.0.0.0)")
		adminPort   = flag.Int("admin-port", 0, "Admin server port (env: COPILOT2API_ADMIN_PORT, default: 7778)")
		adminHost   = flag.String("admin-host", "", "Admin server host (env: COPILOT2API_ADMIN_HOST, default: 0.0.0.0)")
		tokenDir    = flag.String("token-dir", "", "Token storage directory (env: COPILOT2API_TOKEN_DIR, default: ~/.config/copilot2api)")
		showVersion = flag.Bool("version", false, "Show version and exit")
		debug       = flag.Bool("debug", false, "Enable debug logging (env: COPILOT2API_DEBUG)")
	)
	flag.Parse()

	// Apply debug env var
	if !*debug {
		if v := os.Getenv("COPILOT2API_DEBUG"); v != "" {
			if enabled, err := strconv.ParseBool(v); err == nil {
				*debug = enabled
			}
		}
	}

	// Apply env var defaults
	if *host == "" {
		if v := os.Getenv("COPILOT2API_HOST"); v != "" {
			*host = v
		} else {
			*host = "0.0.0.0"
		}
	}
	if *port == 0 {
		if v := os.Getenv("COPILOT2API_PORT"); v != "" {
			if p, err := strconv.Atoi(v); err == nil {
				*port = p
			}
		}
		if *port == 0 {
			*port = 7777
		}
	}
	if *adminHost == "" {
		if v := os.Getenv("COPILOT2API_ADMIN_HOST"); v != "" {
			*adminHost = v
		} else {
			*adminHost = "0.0.0.0"
		}
	}
	if *adminPort == 0 {
		if v := os.Getenv("COPILOT2API_ADMIN_PORT"); v != "" {
			if p, err := strconv.Atoi(v); err == nil {
				*adminPort = p
			}
		}
		if *adminPort == 0 {
			*adminPort = 7778
		}
	}
	if *tokenDir == "" {
		if v := os.Getenv("COPILOT2API_TOKEN_DIR"); v != "" {
			*tokenDir = v
		}
	}

	if *showVersion {
		fmt.Printf("copilot2api version %s\n", version)
		os.Exit(0)
	}

	// Set up logging
	logLevel := slog.LevelInfo
	if *debug {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	// Determine token directory
	if *tokenDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			slog.Error("failed to get home directory", "error", err)
			os.Exit(1)
		}
		*tokenDir = filepath.Join(homeDir, ".config", "copilot2api")
	}

	// Shared HTTP transport for all upstream requests
	transport := upstream.NewTransport()

	// Build the account registry. In multi-account mode each API key maps to a
	// GitHub account with its own isolated auth/token and models caches; in
	// legacy single-account mode a single account serves all requests. This
	// runs the interactive device flow per account as needed.
	ctx := context.Background()
	registry, adminManager, statsStore, err := buildRegistry(ctx, *tokenDir, transport)
	if err != nil {
		slog.Error("failed to initialize accounts", "error", err)
		os.Exit(1)
	}
	defer statsStore.Close()

	// Per-protocol dispatchers resolve the account from the request's API key
	// and delegate to that account's handler.
	openaiHandler := registry.Handler(accounts.ProtoOpenAI)
	anthropicHandler := registry.Handler(accounts.ProtoAnthropic)
	geminiHandler := registry.Handler(accounts.ProtoGemini)
	usageHandler := registry.Handler(accounts.ProtoUsage)

	// Set up routes
	mux := http.NewServeMux()

	// Core routes
	mux.HandleFunc("GET /health", handleHealth)
	mux.Handle("/v1/chat/completions", openaiHandler)
	mux.Handle("/v1/models", openaiHandler)
	mux.Handle("/v1/embeddings", openaiHandler)
	mux.Handle("/v1/responses", openaiHandler)
	mux.Handle("/v1/messages", anthropicHandler)
	mux.Handle("/v1/messages/count_tokens", anthropicHandler)
	mux.Handle("/v1beta/models", geminiHandler)
	mux.Handle("/v1beta/models/", geminiHandler)
	mux.Handle("/usage", usageHandler)

	// AmpCode routes — strip /amp prefix so existing handlers see /v1/...
	mux.Handle("/amp/v1/chat/completions", http.StripPrefix("/amp", openaiHandler))
	mux.Handle("/amp/v1/models", http.StripPrefix("/amp", openaiHandler))
	mux.Handle("/amp/v1/responses", http.StripPrefix("/amp", openaiHandler))
	mux.Handle("/amp/v1/embeddings", http.StripPrefix("/amp", openaiHandler))

	// AmpCode provider-specific routes
	mux.Handle("/api/provider/openai/v1/chat/completions", http.StripPrefix("/api/provider/openai", openaiHandler))
	mux.Handle("/api/provider/openai/v1/responses", http.StripPrefix("/api/provider/openai", openaiHandler))
	mux.Handle("/api/provider/openai/v1/models", http.StripPrefix("/api/provider/openai", openaiHandler))
	mux.Handle("/api/provider/anthropic/v1/messages", http.StripPrefix("/api/provider/anthropic", anthropicHandler))
	mux.Handle("/api/provider/google/v1beta/models", http.StripPrefix("/api/provider/google", geminiHandler))
	mux.Handle("/api/provider/google/v1beta/models/", http.StripPrefix("/api/provider/google", geminiHandler))

	// AmpCode management — reverse proxy to ampcode.com for auth, threads, etc.
	// AI inference stays on Copilot API (routes above); only metadata hits ampcode.com.
	ampBackend, _ := url.Parse("https://ampcode.com")
	ampReverseProxy := newAmpReverseProxy(ampBackend)
	mux.Handle("/api/", ampReverseProxy)
	mux.HandleFunc("/amp/v1/login", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://ampcode.com/login", http.StatusFound)
	})
	mux.HandleFunc("/amp/auth/cli-login", func(w http.ResponseWriter, r *http.Request) {
		target := "https://ampcode.com/auth/cli-login"
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, target, http.StatusFound)
	})

	// Create server
	server := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", *host, *port),
		ReadHeaderTimeout: 10 * time.Second,
		// No ReadTimeout — ReadHeaderTimeout protects against slowloris.
		// ReadTimeout would kill long-lived SSE streaming connections.
		IdleTimeout: 120 * time.Second,
		Handler:     logAllRequests(mux),
	}

	var adminServer *http.Server
	if adminManager != nil && adminEnabled() {
		if os.Getenv("COPILOT2API_ADMIN_USERNAME") == "" || os.Getenv("COPILOT2API_ADMIN_PASSWORD") == "" {
			slog.Error("admin server requires COPILOT2API_ADMIN_USERNAME and COPILOT2API_ADMIN_PASSWORD; set COPILOT2API_ADMIN_ENABLED=false to disable it")
			os.Exit(1)
		}
		adminMux := http.NewServeMux()
		adminMux.HandleFunc("GET /health", handleAdminHealth)
		adminMux.Handle("/admin/", adminManager.Handler())
		adminMux.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/", http.StatusFound)
		})
		adminServer = &http.Server{
			Addr:              fmt.Sprintf("%s:%d", *adminHost, *adminPort),
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       120 * time.Second,
			Handler:           logAllRequests(adminMux),
		}
		slog.Info("admin UI enabled on separate listener", "host", *adminHost, "port", *adminPort, "path", "/admin/", "legacy_token_enabled", os.Getenv("COPILOT2API_ADMIN_TOKEN") != "")
	}

	// Start server in goroutine
	serverErr := make(chan error, 1)
	go func() {
		slog.Info("starting server", "host", *host, "port", *port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()
	if adminServer != nil {
		go func() {
			slog.Info("starting admin server", "host", *adminHost, "port", *adminPort)
			if err := adminServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				serverErr <- err
			}
		}()
	}

	// Wait for interrupt signal or server error
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-quit:
	case err := <-serverErr:
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}

	slog.Info("shutting down server")

	// Give the server 30 seconds to finish handling existing requests
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
		os.Exit(1)
	}
	if adminServer != nil {
		if err := adminServer.Shutdown(ctx); err != nil {
			slog.Error("admin server forced to shutdown", "error", err)
			os.Exit(1)
		}
	}

	slog.Info("server stopped")
}

func adminEnabled() bool {
	v := os.Getenv("COPILOT2API_ADMIN_ENABLED")
	if v == "" {
		return true
	}
	enabled, err := strconv.ParseBool(v)
	if err != nil {
		return true
	}
	return enabled
}

// newAmpReverseProxy creates a reverse proxy to ampcode.com that forwards the
// client's auth headers. Used for amp CLI management calls (getUserInfo,
// threads, telemetry, etc.) — no AI credits are consumed.
func newAmpReverseProxy(target *url.URL) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			slog.Debug("amp proxy", "method", req.Method, "path", req.URL.Path, "query", req.URL.RawQuery)
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host
		},
	}
}

func logAllRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Debug("incoming request", "method", r.Method, "path", r.URL.Path, "query", r.URL.RawQuery)
		next.ServeHTTP(w, r)
	})
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeHealthJSON(w, map[string]string{"status": "ok", "service": "copilot2api"})
}

func handleAdminHealth(w http.ResponseWriter, _ *http.Request) {
	writeHealthJSON(w, map[string]string{"status": "ok", "service": "copilot2api-admin"})
}

func writeHealthJSON(w http.ResponseWriter, payload map[string]string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(payload)
}
