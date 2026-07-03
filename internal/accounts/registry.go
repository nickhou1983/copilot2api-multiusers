package accounts

import (
	"encoding/json"
	"errors"
	"log/slog"
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
	// Pool is the optional pool name this account belongs to. Accounts sharing a
	// non-empty Pool (and the same APIKey) are load-balanced together. Empty
	// means the account is standalone (a pool of one).
	Pool string
	Auth *auth.Client

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
//
// Each API key maps to a Pool of one or more accounts. A single-member pool is
// the classic one-key-one-account mapping; multi-member pools load-balance and
// fail over across their members.
type Registry struct {
	mu         sync.RWMutex
	byKey      map[string]*Pool
	byID       map[string]*Account
	def        *Account
	requireKey bool
}

// NewRegistry builds a key-validating registry from the given accounts. Every
// request must present a key that maps to one of these accounts. Accounts that
// share an API key AND a non-empty Pool name are grouped into one pool; any
// other duplicate key is rejected. An empty list is allowed (e.g. to bootstrap
// and add accounts later via the admin API).
func NewRegistry(accounts []*Account) (*Registry, error) {
	rg := &Registry{
		byKey:      make(map[string]*Pool, len(accounts)),
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
		if existing, ok := rg.byKey[a.APIKey]; ok {
			// Reusing a key is only allowed within the same non-empty pool.
			if a.Pool == "" || existing.Name == "" || existing.Name != a.Pool {
				return nil, errors.New("duplicate api key for account " + a.ID)
			}
			existing.members = append(existing.members, a)
		} else {
			rg.byKey[a.APIKey] = newPool(a.APIKey, a.Pool, []*Account{a})
		}
		rg.byID[a.ID] = a
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

// addToKeyLocked inserts a under its APIKey, creating a new pool or extending an
// existing one. It returns an error when the key belongs to an incompatible
// pool. Callers must hold the write lock.
func (rg *Registry) addToKeyLocked(a *Account) error {
	if existing, ok := rg.byKey[a.APIKey]; ok {
		if a.Pool == "" || existing.Name == "" || existing.Name != a.Pool {
			return errors.New("api key already in use")
		}
		existing.members = append(existing.members, a)
		return nil
	}
	rg.byKey[a.APIKey] = newPool(a.APIKey, a.Pool, []*Account{a})
	return nil
}

// removeFromKeyLocked drops the account with the given id from the pool under
// key, deleting the pool entirely when it becomes empty. Callers must hold the
// write lock.
func (rg *Registry) removeFromKeyLocked(key, id string) {
	pool, ok := rg.byKey[key]
	if !ok {
		return
	}
	filtered := pool.members[:0]
	for _, m := range pool.members {
		if m.ID != id {
			filtered = append(filtered, m)
		}
	}
	if len(filtered) == 0 {
		delete(rg.byKey, key)
		return
	}
	pool.members = filtered
}

// Add registers a new account. It fails if the id is already taken, if the api
// key is already in use by an incompatible pool, or when called on a legacy
// single-account registry. Adding an account whose Pool and APIKey match an
// existing pool extends that pool.
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
	if err := rg.addToKeyLocked(a); err != nil {
		return err
	}
	rg.byID[a.ID] = a
	return nil
}

// Remove unregisters the account with the given id and returns it, or an error
// if it does not exist. When the account was the last member of its pool the
// pool (and its key) is removed too.
func (rg *Registry) Remove(id string) (*Account, error) {
	rg.mu.Lock()
	defer rg.mu.Unlock()
	a, ok := rg.byID[id]
	if !ok {
		return nil, errors.New("account not found: " + id)
	}
	delete(rg.byID, id)
	rg.removeFromKeyLocked(a.APIKey, id)
	return a, nil
}

// UpdateKey changes an account's API key, keeping the pool index consistent. The
// account is detached from its current pool and attached to the pool for newKey
// (created if absent). It fails when newKey belongs to an incompatible pool.
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
	if existing, ok := rg.byKey[newKey]; ok {
		if a.Pool == "" || existing.Name == "" || existing.Name != a.Pool {
			return errors.New("api key already in use")
		}
	}
	rg.removeFromKeyLocked(a.APIKey, id)
	a.APIKey = newKey
	// Compatibility was checked above, so this cannot fail.
	_ = rg.addToKeyLocked(a)
	return nil
}

// Replace swaps the in-registry account that has the same id, preserving the
// pool index. Used when an account must be rebuilt (e.g. token_dir changed).
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
	if a.APIKey == old.APIKey {
		// Same key: swap the member pointer in-place within the pool.
		if pool, ok := rg.byKey[a.APIKey]; ok {
			for i, m := range pool.members {
				if m.ID == a.ID {
					pool.members[i] = a
					break
				}
			}
		}
		rg.byID[a.ID] = a
		return nil
	}
	if existing, ok := rg.byKey[a.APIKey]; ok {
		if a.Pool == "" || existing.Name == "" || existing.Name != a.Pool {
			return errors.New("api key already in use")
		}
	}
	rg.removeFromKeyLocked(old.APIKey, a.ID)
	if err := rg.addToKeyLocked(a); err != nil {
		return err
	}
	rg.byID[a.ID] = a
	return nil
}

// Resolve returns a single account for a request, validating the API key when
// required. For a multi-member pool it returns the next member by round-robin.
func (rg *Registry) Resolve(r *http.Request) (*Account, error) {
	if !rg.requireKey {
		return rg.def, nil
	}
	key := ExtractAPIKey(r)
	if key == "" {
		return nil, ErrMissingKey
	}
	rg.mu.RLock()
	pool, ok := rg.byKey[key]
	var acct *Account
	if ok {
		acct = pool.pick()
	}
	rg.mu.RUnlock()
	if !ok {
		return nil, ErrUnknownKey
	}
	return acct, nil
}

// resolveOrder returns the pool and the members ordered for this request
// (round-robin primary first, then failover candidates), validating the API key
// when required. The ordering is snapshotted under the read lock so it is safe
// to iterate without holding the lock.
func (rg *Registry) resolveOrder(r *http.Request) (*Pool, []*Account, error) {
	if !rg.requireKey {
		return nil, []*Account{rg.def}, nil
	}
	key := ExtractAPIKey(r)
	if key == "" {
		return nil, nil, ErrMissingKey
	}
	rg.mu.RLock()
	pool, ok := rg.byKey[key]
	var order []*Account
	if ok {
		order = pool.order()
	}
	rg.mu.RUnlock()
	if !ok {
		return nil, nil, ErrUnknownKey
	}
	return pool, order, nil
}

// Handler returns an http.Handler that resolves the account pool from the
// request's API key and delegates to the matching per-protocol handler, with
// round-robin selection and automatic failover across pool members.
func (rg *Registry) Handler(p Protocol) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pool, order, err := rg.resolveOrder(r)
		if err != nil {
			writeAuthError(w, p, err)
			return
		}
		rg.servePool(w, r, p, pool, order)
	})
}

// serveOne dispatches a request to a single account's handler, attaching its
// usage recorder to the context.
func (rg *Registry) serveOne(w http.ResponseWriter, r *http.Request, p Protocol, acct *Account) {
	h := acct.handlerFor(p)
	if h == nil {
		http.NotFound(w, r)
		return
	}
	if acct.Recorder != nil {
		r = r.WithContext(stats.WithRecorder(r.Context(), acct.Recorder))
	}
	h.ServeHTTP(w, r)
}

// servePool serves a request across the ordered pool members. Single-member
// pools take a zero-overhead fast path identical to the classic behavior.
// Multi-member pools buffer the request body and retry the next member when an
// attempt fails with a retryable status before any response bytes are streamed.
func (rg *Registry) servePool(w http.ResponseWriter, r *http.Request, p Protocol, pool *Pool, order []*Account) {
	if len(order) == 1 {
		rg.serveOne(w, r, p, order[0])
		return
	}

	body, buffered, err := drainForRetry(r)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	if !buffered {
		// Body too large to replay for failover: serve the primary member only.
		rg.serveOne(w, r, p, order[0])
		return
	}

	poolName := ""
	if pool != nil {
		poolName = pool.Name
	}
	for i, acct := range order {
		last := i == len(order)-1
		resetBody(r, body)
		h := acct.handlerFor(p)
		if h == nil {
			http.NotFound(w, r)
			return
		}
		rr := r
		if acct.Recorder != nil {
			rr = r.WithContext(stats.WithRecorder(r.Context(), acct.Recorder))
		}
		cw := &captureWriter{rw: w, last: last}
		h.ServeHTTP(cw, rr)
		if !cw.aborted {
			// Response committed (success or non-retryable error), or this was
			// the final attempt which commits even a retryable status.
			return
		}
		slog.Warn("account pool: attempt failed, failing over to next member",
			"pool", poolName, "account", acct.ID, "status", cw.status, "attempt", i+1, "members", len(order))
	}
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
