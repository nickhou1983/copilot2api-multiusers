package accounts

import (
	"strings"
	"testing"
)

func TestGenerateAPIKey_Format(t *testing.T) {
	key := GenerateAPIKey()
	if !strings.HasPrefix(key, "sk-") {
		t.Errorf("expected sk- prefix, got %q", key)
	}
	// sk- (3) + 32 chars = 35 total
	if len(key) != 35 {
		t.Errorf("expected length 35, got %d (%q)", len(key), key)
	}
}

func TestGenerateAPIKey_Charset(t *testing.T) {
	key := GenerateAPIKey()
	body := key[3:] // strip "sk-"
	for _, c := range body {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			t.Errorf("unexpected character %q in key body", c)
		}
	}
}

func TestGenerateAPIKey_Uniqueness(t *testing.T) {
	seen := make(map[string]struct{}, 100)
	for i := 0; i < 100; i++ {
		key := GenerateAPIKey()
		if _, dup := seen[key]; dup {
			t.Fatalf("duplicate key generated on iteration %d: %q", i, key)
		}
		seen[key] = struct{}{}
	}
}
