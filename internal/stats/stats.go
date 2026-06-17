// Package stats records per-account, per-model token usage (including cached
// tokens) for every upstream request, and persists it to disk. Usage is
// captured at the single per-account chokepoint (upstream.Client.Do) via a
// Recorder carried on the request context, so handlers stay untouched.
package stats

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const defaultAccountID = "default"

// Usage is the normalized token usage extracted from a single response, with a
// uniform cross-protocol meaning: Input excludes Cached and CacheCreation, so
// total input = Input + Cached + CacheCreation.
type Usage struct {
	Input         int
	Output        int
	Cached        int // cache-read / prompt cache hits
	CacheCreation int // cache-write (Anthropic only); 0 elsewhere
}

// Empty reports whether the usage carries no token counts.
func (u Usage) Empty() bool {
	return u.Input == 0 && u.Output == 0 && u.Cached == 0 && u.CacheCreation == 0
}

// Counters accumulates usage for one (account, model) pair.
type Counters struct {
	Requests      int64     `json:"requests"`
	Input         int64     `json:"input"`
	Output        int64     `json:"output"`
	Cached        int64     `json:"cached"`
	CacheCreation int64     `json:"cache_creation"`
	LastUsed      time.Time `json:"last_used"`
}

// Store holds usage counters for all accounts and persists them atomically.
// It is safe for concurrent use.
type Store struct {
	mu    sync.Mutex
	path  string
	data  map[string]map[string]*Counters // accountID -> model -> counters
	dirty bool

	stop chan struct{}
	once sync.Once
}

// persistFile is the on-disk JSON shape.
type persistFile struct {
	Accounts map[string]map[string]*Counters `json:"accounts"`
}

// NewStore creates a store backed by the given file path. Call Load to populate
// it from disk and StartFlusher to persist periodically.
func NewStore(path string) *Store {
	return &Store{
		path: path,
		data: make(map[string]map[string]*Counters),
		stop: make(chan struct{}),
	}
}

// Load reads persisted counters from disk. A missing file is not an error.
func (s *Store) Load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read stats file: %w", err)
	}

	var pf persistFile
	if err := json.Unmarshal(data, &pf); err != nil {
		return fmt.Errorf("failed to parse stats file: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if pf.Accounts != nil {
		s.data = pf.Accounts
	}
	return nil
}

// Recorder returns a Recorder bound to accountID. An empty id maps to "default"
// so legacy single-account mode still accumulates usage.
func (s *Store) Recorder(accountID string) *Recorder {
	if accountID == "" {
		accountID = defaultAccountID
	}
	return &Recorder{store: s, accountID: accountID}
}

// record merges usage into the counters for (accountID, model). A request with
// empty model is bucketed under "unknown".
func (s *Store) record(accountID, model string, u Usage) {
	if model == "" {
		model = "unknown"
	}
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	models := s.data[accountID]
	if models == nil {
		models = make(map[string]*Counters)
		s.data[accountID] = models
	}
	c := models[model]
	if c == nil {
		c = &Counters{}
		models[model] = c
	}
	c.Requests++
	c.Input += int64(u.Input)
	c.Output += int64(u.Output)
	c.Cached += int64(u.Cached)
	c.CacheCreation += int64(u.CacheCreation)
	c.LastUsed = now
	s.dirty = true
}

// AccountStats is a snapshot of one account's usage.
type AccountStats struct {
	ID     string       `json:"id"`
	Totals Counters     `json:"totals"`
	Models []ModelStats `json:"models"`
}

// ModelStats is a snapshot of one (account, model) bucket.
type ModelStats struct {
	Model         string    `json:"model"`
	Requests      int64     `json:"requests"`
	Input         int64     `json:"input"`
	Output        int64     `json:"output"`
	Cached        int64     `json:"cached"`
	CacheCreation int64     `json:"cache_creation"`
	Total         int64     `json:"total"`
	LastUsed      time.Time `json:"last_used"`
}

// Snapshot returns a deep copy of all accounts' usage, sorted by account ID and
// by descending total tokens within each account.
func (s *Store) Snapshot() []AccountStats {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]AccountStats, 0, len(s.data))
	for id, models := range s.data {
		acct := AccountStats{ID: id, Models: make([]ModelStats, 0, len(models))}
		for model, c := range models {
			total := c.Input + c.Output + c.Cached + c.CacheCreation
			acct.Models = append(acct.Models, ModelStats{
				Model:         model,
				Requests:      c.Requests,
				Input:         c.Input,
				Output:        c.Output,
				Cached:        c.Cached,
				CacheCreation: c.CacheCreation,
				Total:         total,
				LastUsed:      c.LastUsed,
			})
			acct.Totals.Requests += c.Requests
			acct.Totals.Input += c.Input
			acct.Totals.Output += c.Output
			acct.Totals.Cached += c.Cached
			acct.Totals.CacheCreation += c.CacheCreation
			if c.LastUsed.After(acct.Totals.LastUsed) {
				acct.Totals.LastUsed = c.LastUsed
			}
		}
		sort.Slice(acct.Models, func(i, j int) bool {
			ti := acct.Models[i].Total
			tj := acct.Models[j].Total
			if ti != tj {
				return ti > tj
			}
			return acct.Models[i].Model < acct.Models[j].Model
		})
		out = append(out, acct)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Reset clears all counters for one account and persists immediately.
func (s *Store) Reset(accountID string) error {
	if accountID == "" {
		accountID = defaultAccountID
	}
	s.mu.Lock()
	delete(s.data, accountID)
	s.dirty = true
	s.mu.Unlock()
	return s.Flush()
}

// ResetAll clears every account's counters and persists immediately.
func (s *Store) ResetAll() error {
	s.mu.Lock()
	s.data = make(map[string]map[string]*Counters)
	s.dirty = true
	s.mu.Unlock()
	return s.Flush()
}

// Flush writes the current counters to disk atomically (temp file + rename).
// It is a no-op when nothing changed since the last flush.
func (s *Store) Flush() error {
	s.mu.Lock()
	if !s.dirty {
		s.mu.Unlock()
		return nil
	}
	pf := persistFile{Accounts: s.data}
	data, err := json.MarshalIndent(pf, "", "  ")
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("failed to marshal stats: %w", err)
	}
	s.dirty = false
	s.mu.Unlock()

	if dir := filepath.Dir(s.path); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("failed to create stats directory: %w", err)
		}
	}

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write stats temp file: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename stats file: %w", err)
	}
	return nil
}

// StartFlusher periodically flushes dirty counters to disk until Close is
// called. Pass a non-zero interval.
func (s *Store) StartFlusher(interval time.Duration) {
	if interval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = s.Flush()
			case <-s.stop:
				return
			}
		}
	}()
}

// Close stops the background flusher and performs a final flush.
func (s *Store) Close() error {
	s.once.Do(func() { close(s.stop) })
	return s.Flush()
}

// Recorder records usage for a single account. A nil Recorder is safe to call
// (it is a no-op), so capture sites need not nil-check.
type Recorder struct {
	store     *Store
	accountID string
}

// Record merges one response's usage into the account's counters. Records the
// request even when usage is empty, so request counts stay accurate.
func (r *Recorder) Record(model string, u Usage) {
	if r == nil || r.store == nil {
		return
	}
	r.store.record(r.accountID, model, u)
}
