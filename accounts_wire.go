package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/whtsky/copilot2api/anthropic"
	"github.com/whtsky/copilot2api/auth"
	"github.com/whtsky/copilot2api/gemini"
	"github.com/whtsky/copilot2api/internal/accounts"
	"github.com/whtsky/copilot2api/internal/models"
	"github.com/whtsky/copilot2api/internal/stats"
	"github.com/whtsky/copilot2api/internal/upstream"
	"github.com/whtsky/copilot2api/proxy"
)

const modelsCacheTTL = 5 * time.Minute

// newAccountHandlers builds an account's isolated auth client, models cache, and
// per-protocol handlers WITHOUT authenticating. Authentication happens
// separately (interactive device flow at startup, or web-driven via the admin
// API), so accounts can be created before the user authorizes them.
func newAccountHandlers(cfg accounts.AccountConfig, baseTokenDir string, transport *http.Transport, rec *stats.Recorder) (*accounts.Account, error) {
	id := cfg.ID
	mode, err := cfg.ResolveAuthMode()
	if err != nil {
		return nil, fmt.Errorf("account %q: %w", id, err)
	}
	authClient, err := auth.NewClient(cfg.ResolveTokenDir(baseTokenDir), mode)
	if err != nil {
		return nil, fmt.Errorf("account %q: failed to initialize auth client: %w", id, err)
	}

	upstreamClient := upstream.NewClient(authClient, transport)
	modelsCache := models.NewCache(upstreamClient, modelsCacheTTL)
	proxyHandler := proxy.NewHandler(authClient, transport, modelsCache)

	// Pre-warm this account's models cache once it has a usable token. Warming
	// before auth would just fail, so skip it until authenticated.
	go func() {
		if !authClient.IsAuthenticated() {
			return
		}
		slog.Debug("warming models cache", "account", id)
		modelsCache.Warm(context.Background())
		slog.Info("models cache warmed", "account", id)
	}()

	return &accounts.Account{
		ID:        id,
		APIKey:    cfg.APIKey,
		TokenDir:  cfg.TokenDir,
		AuthMode:  cfg.AuthMode,
		Auth:      authClient,
		Recorder:  rec,
		Models:    modelsCache,
		OpenAI:    proxyHandler,
		Anthropic: anthropic.NewHandler(authClient, transport, modelsCache),
		Gemini:    gemini.NewHandler(authClient, transport, modelsCache),
		Usage:     http.HandlerFunc(proxyHandler.HandleUsage),
	}, nil
}

// buildAccount builds an account and runs the interactive device flow if it has
// no stored GitHub token. Used at startup.
func buildAccount(ctx context.Context, cfg accounts.AccountConfig, baseTokenDir string, transport *http.Transport, rec *stats.Recorder) (*accounts.Account, error) {
	acct, err := newAccountHandlers(cfg, baseTokenDir, transport, rec)
	if err != nil {
		return nil, err
	}
	if cfg.ID != "" {
		fmt.Printf("\n👤 Authenticating account %q (token dir: %s)\n", cfg.ID, cfg.ResolveTokenDir(baseTokenDir))
	}
	if err := acct.Auth.EnsureAuthenticated(ctx); err != nil {
		return nil, fmt.Errorf("account %q: authentication failed: %w", cfg.ID, err)
	}
	return acct, nil
}

// buildRegistry loads the accounts config and builds an account registry. The
// proxy always runs in multi-account mode: when no config file exists it is
// auto-created as an empty config so the admin UI is available out of the box.
// It returns an admin Manager for maintaining the mapping at runtime. The
// returned stats.Store accumulates token usage and must be closed by the caller
// to flush counters to disk.
func buildRegistry(ctx context.Context, baseTokenDir string, transport *http.Transport) (*accounts.Registry, *accounts.Manager, *stats.Store, error) {
	statsStore := stats.NewStore(filepath.Join(baseTokenDir, "stats.json"))
	if err := statsStore.Load(); err != nil {
		slog.Warn("failed to load stats", "error", err)
	}
	statsStore.StartFlusher(30 * time.Second)

	cfgPath := accounts.ResolveConfigPath(baseTokenDir)
	cfg, err := accounts.LoadConfig(cfgPath)
	if err != nil {
		return nil, nil, nil, err
	}

	// No config file yet: bootstrap an empty one and enter multi-account mode so
	// the admin UI is available out of the box to add and authenticate accounts.
	if cfg == nil {
		cfg = &accounts.Config{Accounts: []accounts.AccountConfig{}}
		if err := accounts.SaveConfig(cfgPath, cfg); err != nil {
			return nil, nil, nil, fmt.Errorf("failed to bootstrap accounts config: %w", err)
		}
		slog.Info("no accounts config found; created an empty one and enabled the admin UI", "config_path", cfgPath)
	}

	slog.Info("running in multi-account mode", "config_path", cfgPath, "accounts", len(cfg.Accounts))
	built := make([]*accounts.Account, 0, len(cfg.Accounts))
	for i := range cfg.Accounts {
		ac := cfg.Accounts[i]
		acct, err := buildAccount(ctx, ac, baseTokenDir, transport, statsStore.Recorder(ac.ID))
		if err != nil {
			return nil, nil, nil, err
		}
		built = append(built, acct)
	}

	reg, err := accounts.NewRegistry(built)
	if err != nil {
		return nil, nil, nil, err
	}

	// Factory used by the admin API to create accounts without authenticating.
	factory := func(c accounts.AccountConfig) (*accounts.Account, error) {
		return newAccountHandlers(c, baseTokenDir, transport, statsStore.Recorder(c.ID))
	}
	mgr := accounts.NewManager(reg, factory, cfgPath, os.Getenv("COPILOT2API_ADMIN_TOKEN"), statsStore)
	return reg, mgr, statsStore, nil
}
