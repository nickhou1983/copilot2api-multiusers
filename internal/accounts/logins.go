package accounts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// DefaultLoginsFileName is the name of the GitHub login credential file looked
// up inside the base token directory.
const DefaultLoginsFileName = "github_logins.json"

// GitHubLogin holds the GitHub account credentials used by the login agent to
// drive an automated Device Flow authorization.
//
// The password is stored in plaintext on disk (0600, inside the token
// directory). It is never included in any admin API response and must never be
// logged.
type GitHubLogin struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginView is the redacted projection of a GitHubLogin safe to return over the
// admin API.
type LoginView struct {
	Username    string `json:"username"`
	HasPassword bool   `json:"has_password"`
}

// loginsFile is the on-disk representation of the credential store.
type loginsFile struct {
	Logins map[string]GitHubLogin `json:"logins"`
}

// LoginStore persists per-account GitHub login credentials so the login agent
// can replay them during automated device-flow authorization.
type LoginStore struct {
	path   string
	mu     sync.RWMutex
	logins map[string]GitHubLogin
}

// ResolveLoginsPath returns the GitHub logins file path. It honors the
// COPILOT2API_LOGINS_FILE environment variable, otherwise falls back to
// <baseTokenDir>/github_logins.json.
func ResolveLoginsPath(baseTokenDir string) string {
	if v := os.Getenv("COPILOT2API_LOGINS_FILE"); v != "" {
		return v
	}
	return filepath.Join(baseTokenDir, DefaultLoginsFileName)
}

// NewLoginStore creates a credential store backed by path and loads any
// existing entries. A missing file is treated as an empty store.
func NewLoginStore(path string) (*LoginStore, error) {
	s := &LoginStore{path: path, logins: make(map[string]GitHubLogin)}
	if err := s.Load(); err != nil {
		return nil, err
	}
	return s, nil
}

// Load reads the credential file from disk, replacing the in-memory state.
func (s *LoginStore) Load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.mu.Lock()
			s.logins = make(map[string]GitHubLogin)
			s.mu.Unlock()
			return nil
		}
		return fmt.Errorf("failed to read github logins file: %w", err)
	}

	var f loginsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("failed to parse github logins file: %w", err)
	}
	if f.Logins == nil {
		f.Logins = make(map[string]GitHubLogin)
	}

	s.mu.Lock()
	s.logins = f.Logins
	s.mu.Unlock()
	return nil
}

// Get returns the stored credentials for an account.
func (s *LoginStore) Get(id string) (GitHubLogin, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	l, ok := s.logins[id]
	return l, ok
}

// Has reports whether usable credentials (username and password) are stored for
// an account.
func (s *LoginStore) Has(id string) bool {
	l, ok := s.Get(id)
	return ok && l.Username != "" && l.Password != ""
}

// View returns the redacted credentials for an account.
func (s *LoginStore) View(id string) LoginView {
	l, ok := s.Get(id)
	if !ok {
		return LoginView{}
	}
	return LoginView{Username: l.Username, HasPassword: l.Password != ""}
}

// Set stores credentials for an account and persists the store to disk.
func (s *LoginStore) Set(id string, login GitHubLogin) error {
	if id == "" {
		return fmt.Errorf("account id is required")
	}
	if login.Username == "" {
		return fmt.Errorf("github username is required")
	}
	if login.Password == "" {
		return fmt.Errorf("github password is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.logins[id] = login
	return s.saveLocked()
}

// Delete removes an account's credentials and persists the store. Deleting a
// missing account is a no-op.
func (s *LoginStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.logins[id]; !ok {
		return nil
	}
	delete(s.logins, id)
	return s.saveLocked()
}

// IDs returns the sorted account ids that have stored credentials.
func (s *LoginStore) IDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.logins))
	for id := range s.logins {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// saveLocked writes the store to disk atomically (temp file + rename). Caller
// must hold s.mu.
func (s *LoginStore) saveLocked() error {
	data, err := json.MarshalIndent(loginsFile{Logins: s.logins}, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal github logins: %w", err)
	}

	if dir := filepath.Dir(s.path); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("failed to create github logins directory: %w", err)
		}
	}

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write github logins temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0600); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to set github logins temp file permissions: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename github logins file: %w", err)
	}
	return nil
}
