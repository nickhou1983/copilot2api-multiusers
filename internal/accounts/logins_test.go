package accounts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func newTestLoginStore(t *testing.T) (*LoginStore, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "github_logins.json")
	store, err := NewLoginStore(path)
	if err != nil {
		t.Fatalf("NewLoginStore: %v", err)
	}
	return store, path
}

func TestLoginStoreMissingFileIsEmpty(t *testing.T) {
	store, path := newTestLoginStore(t)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected no file on disk before first write, got err=%v", err)
	}
	if got := store.IDs(); len(got) != 0 {
		t.Fatalf("expected empty store, got %v", got)
	}
	if store.Has("alice") {
		t.Fatal("expected Has to be false for unknown account")
	}
	if v := store.View("alice"); v.Username != "" || v.HasPassword {
		t.Fatalf("expected zero view for unknown account, got %+v", v)
	}
}

func TestLoginStoreRoundTrip(t *testing.T) {
	store, path := newTestLoginStore(t)

	if err := store.Set("alice", GitHubLogin{Username: "alice-gh", Password: "s3cret"}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := store.Set("bob", GitHubLogin{Username: "bob-gh", Password: "hunter2"}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	reloaded, err := NewLoginStore(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	got, ok := reloaded.Get("alice")
	if !ok {
		t.Fatal("expected alice after reload")
	}
	if got.Username != "alice-gh" || got.Password != "s3cret" {
		t.Fatalf("unexpected credentials after reload: %+v", got)
	}
	if ids := reloaded.IDs(); len(ids) != 2 || ids[0] != "alice" || ids[1] != "bob" {
		t.Fatalf("expected sorted ids [alice bob], got %v", ids)
	}
}

func TestLoginStoreSetValidation(t *testing.T) {
	store, _ := newTestLoginStore(t)

	cases := []struct {
		name  string
		id    string
		login GitHubLogin
	}{
		{"empty id", "", GitHubLogin{Username: "u", Password: "p"}},
		{"empty username", "alice", GitHubLogin{Password: "p"}},
		{"empty password", "alice", GitHubLogin{Username: "u"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := store.Set(tc.id, tc.login); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestLoginStoreViewRedactsPassword(t *testing.T) {
	store, _ := newTestLoginStore(t)
	if err := store.Set("alice", GitHubLogin{Username: "alice-gh", Password: "s3cret"}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	v := store.View("alice")
	if v.Username != "alice-gh" {
		t.Fatalf("expected username in view, got %q", v.Username)
	}
	if !v.HasPassword {
		t.Fatal("expected has_password true")
	}

	// The serialized view must never carry the password.
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(data) != `{"username":"alice-gh","has_password":true}` {
		t.Fatalf("unexpected view JSON: %s", data)
	}
}

func TestLoginStoreDelete(t *testing.T) {
	store, path := newTestLoginStore(t)
	if err := store.Set("alice", GitHubLogin{Username: "alice-gh", Password: "s3cret"}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if err := store.Delete("alice"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if store.Has("alice") {
		t.Fatal("expected alice to be gone")
	}
	// Deleting a missing account is a no-op.
	if err := store.Delete("nobody"); err != nil {
		t.Fatalf("Delete missing: %v", err)
	}

	reloaded, err := NewLoginStore(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(reloaded.IDs()) != 0 {
		t.Fatalf("expected empty store after delete, got %v", reloaded.IDs())
	}
}

func TestLoginStoreFilePermissions(t *testing.T) {
	store, path := newTestLoginStore(t)
	if err := store.Set("alice", GitHubLogin{Username: "alice-gh", Password: "s3cret"}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Fatalf("expected credentials file mode 0600, got %o", perm)
	}
	// The atomic write must not leave the temp file behind.
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("expected temp file to be renamed away, got err=%v", err)
	}
}

func TestLoginStoreHasRequiresBothFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "github_logins.json")
	// Hand-write a partial record; Set would reject it, but an operator could.
	raw := `{"logins":{"partial":{"username":"only-user","password":""}}}`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	store, err := NewLoginStore(path)
	if err != nil {
		t.Fatalf("NewLoginStore: %v", err)
	}
	if store.Has("partial") {
		t.Fatal("expected Has false when password is empty")
	}
	if v := store.View("partial"); v.HasPassword {
		t.Fatal("expected has_password false")
	}
}

func TestLoginStoreConcurrentAccess(t *testing.T) {
	store, _ := newTestLoginStore(t)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := store.Set("alice", GitHubLogin{Username: "alice-gh", Password: "s3cret"}); err != nil {
				t.Errorf("Set: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			store.Has("alice")
			store.View("alice")
			store.IDs()
		}()
	}
	wg.Wait()

	if !store.Has("alice") {
		t.Fatal("expected alice to be stored")
	}
}

func TestResolveLoginsPath(t *testing.T) {
	if got, want := ResolveLoginsPath("/base"), filepath.Join("/base", DefaultLoginsFileName); got != want {
		t.Fatalf("ResolveLoginsPath = %q, want %q", got, want)
	}

	t.Setenv("COPILOT2API_LOGINS_FILE", "/custom/logins.json")
	if got := ResolveLoginsPath("/base"); got != "/custom/logins.json" {
		t.Fatalf("ResolveLoginsPath with env = %q", got)
	}
}
