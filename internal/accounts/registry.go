package accounts

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/whtsky/copilot2api/auth"
	"github.com/whtsky/copilot2api/internal/stats"
)

// Protocol identifies which per-account handler a route should dispatch to.
type Protocol int

const (
	ProtoOpenAI Protocol = iota
	ProtoAnthropic
	ProtoGemini
	ProtoUsage
)

var (
	// ErrMissingKey is returned when a request carries no API key but one is required.
	ErrMissingKey = errors.New("missing API key")
	// ErrUnknownKey is returned when the presented API key maps to no account.
	ErrUnknownKey = errors.New("unknown API key")
)

// Account bundles a single GitHub account's auth client and per-protocol
// handlers. Each account owns isolated token and models caches via its handlers.
type Account struct {
	ID     string
	APIKey string
	// TokenDir is the configured (possibly relative) token directory for this
	// account, retained so the registry can round-trip it back to accounts.json.
	TokenDir string
	Auth     *auth.Client

	// Recorder accumulates this account's token usage. May be nil.
	Recorder *stats.Recorder

	OpenAI    http.Handler
	Anthropic http.Handler
	Gemini    http.Handler
	Usage     http.Handler
}

func (a *Account) handlerFor(p Protocol) http.Handler {
	switch p {
	case ProtoOpenAI:
		return a.OpenAI
	case ProtoAnthropic:
		return a.Anthropic
	case ProtoGemini:
		return a.Gemini
	case ProtoUsage:
		return a.Usage
	default:
		return nil
	}
}

// Registry routes requests to the right account based on the presented API key.
// It is safe for concurrent use: request-time reads (Resolve) and admin-time
// mutations (Add/Remove/UpdateKey) are guarded by an RWMutex.
type Registry struct {
	mu         sync.RWMutex
	byKey      map[string]*Account
	byID       map[string]*Account
	def        *Account
	requireKey bool
}

// NewRegistry builds a key-validating registry from the given accounts. Every
// request must present a key that maps to one of these accounts. An empty list
// is allowed (e.g. to bootstrap and add accounts later via the admin API).
func NewRegistry(accounts []*Account) (*Registry, error) {
	rg := &Registry{
		byKey:      make(map[string]*Account, len(accounts)),
		byID:       make(map[string]*Account, len(accounts)),
		requireKey: true,
	}
	for _, a := range accounts {
		if a.ID == "" {
			return nil, errors.New("account has empty id")
		}
		if a.APIKey == "" {
			return nil, errors.New("account " + a.ID + " has empty api key")
		}
		if _, dup := rg.byID[a.ID]; dup {
			return nil, errors.New("duplicate account id " + a.ID)
		}
		if _, dup := rg.byKey[a.APIKey]; dup {
			return nil, errors.New("duplicate api key for account " + a.ID)
		}
		rg.byID[a.ID] = a
		rg.byKey[a.APIKey] = a
	}
	return rg, nil
}

// NewLegacyRegistry builds a registry that always serves the single given
// account and does not validate API keys, preserving pre-multi-account behavior.
func NewLegacyRegistry(a *Account) *Registry {
	return &Registry{def: a, requireKey: false}
}

// MultiAccount reports whether the registry is in key-validating multi-account
// mode (true) or legacy single-account mode (false).
func (rg *Registry) MultiAccount() bool {
	return rg.requireKey
}

// Accounts returns all registered accounts (legacy mode returns the single one).
func (rg *Registry) Accounts() []*Account {
	rg.mu.RLock()
	defer rg.mu.RUnlock()
	if !rg.requireKey {
		if rg.def == nil {
			return nil
		}
		return []*Account{rg.def}
	}
	out := make([]*Account, 0, len(rg.byID))
	for _, a := range rg.byID {
		out = append(out, a)
	}
	return out
}

// Get returns the account with the given id, or nil if absent.
func (rg *Registry) Get(id string) *Account {
	rg.mu.RLock()
	defer rg.mu.RUnlock()
	return rg.byID[id]
}

// Add registers a new account. It fails if the id or api key is already taken,
// or when called on a legacy single-account registry.
func (rg *Registry) Add(a *Account) error {
	if a == nil || a.ID == "" {
		return errors.New("account id is required")
	}
	if a.APIKey == "" {
		return errors.New("account api key is required")
	}
	rg.mu.Lock()
	defer rg.mu.Unlock()
	if !rg.requireKey {
		return errors.New("cannot add accounts in single-account mode")
	}
	if _, dup := rg.byID[a.ID]; dup {
		return errors.New("account id already exists: " + a.ID)
	}
	if _, dup := rg.byKey[a.APIKey]; dup {
		return errors.New("api key already in use")
	}
	rg.byID[a.ID] = a
	rg.byKey[a.APIKey] = a
	return nil
}

// Remove unregisters the account with the given id and returns it, or an error
// if it does not exist.
func (rg *Registry) Remove(id string) (*Account, error) {
	rg.mu.Lock()
	defer rg.mu.Unlock()
	a, ok := rg.byID[id]
	if !ok {
		return nil, errors.New("account not found: " + id)
	}
	delete(rg.byID, id)
	delete(rg.byKey, a.APIKey)
	return a, nil
}

// UpdateKey changes an account's API key, keeping the key index consistent.
func (rg *Registry) UpdateKey(id, newKey string) error {
	if newKey == "" {
		return errors.New("api key is required")
	}
	rg.mu.Lock()
	defer rg.mu.Unlock()
	a, ok := rg.byID[id]
	if !ok {
		return errors.New("account not found: " + id)
	}
	if a.APIKey == newKey {
		return nil
	}
	if other, dup := rg.byKey[newKey]; dup && other.ID != id {
		return errors.New("api key already in use")
	}
	delete(rg.byKey, a.APIKey)
	a.APIKey = newKey
	rg.byKey[newKey] = a
	return nil
}

// Replace swaps the in-registry account that has the same id, preserving the
// key index. Used when an account must be rebuilt (e.g. token_dir changed).
func (rg *Registry) Replace(a *Account) error {
	if a == nil || a.ID == "" {
		return errors.New("account id is required")
	}
	rg.mu.Lock()
	defer rg.mu.Unlock()
	old, ok := rg.byID[a.ID]
	if !ok {
		return errors.New("account not found: " + a.ID)
	}
	if other, dup := rg.byKey[a.APIKey]; dup && other.ID != a.ID {
		return errors.New("api key already in use")
	}
	delete(rg.byKey, old.APIKey)
	rg.byID[a.ID] = a
	rg.byKey[a.APIKey] = a
	return nil
}

// Resolve returns the account for a request, validating the API key when required.
func (rg *Registry) Resolve(r *http.Request) (*Account, error) {
	if !rg.requireKey {
		return rg.def, nil
	}
	key := ExtractAPIKey(r)
	if key == "" {
		return nil, ErrMissingKey
	}
	rg.mu.RLock()
	a, ok := rg.byKey[key]
	rg.mu.RUnlock()
	if !ok {
		return nil, ErrUnknownKey
	}
	return a, nil
}

// Handler returns an http.Handler that resolves the account from the request's
// API key and delegates to that account's handler for the given protocol.
func (rg *Registry) Handler(p Protocol) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		acct, err := rg.Resolve(r)
		if err != nil {
			writeAuthError(w, p, err)
			return
		}
		h := acct.handlerFor(p)
		if h == nil {
			http.NotFound(w, r)
			return
		}
		if acct.Recorder != nil {
			r = r.WithContext(stats.WithRecorder(r.Context(), acct.Recorder))
		}
		h.ServeHTTP(w, r)
	})
}

// ExtractAPIKey pulls the API key from a request, supporting the auth styles
// used across OpenAI, Anthropic, and Gemini clients.
func ExtractAPIKey(r *http.Request) string {
	if h := r.Header.Get("Authorization"); h != "" {
		const prefix = "bearer "
		if len(h) >= len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
			return strings.TrimSpace(h[len(prefix):])
		}
		return strings.TrimSpace(h)
	}
	if k := r.Header.Get("x-api-key"); k != "" {
		return strings.TrimSpace(k)
	}
	if k := r.Header.Get("x-goog-api-key"); k != "" {
		return strings.TrimSpace(k)
	}
	if k := r.URL.Query().Get("key"); k != "" {
		return strings.TrimSpace(k)
	}
	return ""
}

func writeAuthError(w http.ResponseWriter, p Protocol, err error) {
	msg := "Invalid API key"
	if errors.Is(err, ErrMissingKey) {
		msg = "Missing API key"
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)

	var body any
	switch p {
	case ProtoAnthropic:
		body = map[string]any{
			"type": "error",
			"error": map[string]any{
				"type":    "authentication_error",
				"message": msg,
			},
		}
	case ProtoGemini:
		body = map[string]any{
			"error": map[string]any{
				"code":    http.StatusUnauthorized,
				"message": msg,
				"status":  "UNAUTHENTICATED",
			},
		}
	default: // OpenAI + usage
		body = map[string]any{
			"error": map[string]any{
				"message": msg,
				"type":    "invalid_request_error",
				"code":    "invalid_api_key",
			},
		}
	}
	_ = json.NewEncoder(w).Encode(body)
}
