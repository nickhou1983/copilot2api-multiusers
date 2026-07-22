package auth

import "testing"

func TestParseMode(t *testing.T) {
	tests := []struct {
		in      string
		want    Mode
		wantErr bool
	}{
		{"", ModeExchange, false},
		{"exchange", ModeExchange, false},
		{"direct", ModeDirect, false},
		{"bogus", "", true},
		{"Direct", "", true},
	}
	for _, tt := range tests {
		got, err := ParseMode(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseMode(%q): expected error", tt.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseMode(%q): %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseMode(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestClientIDForMode(t *testing.T) {
	if got := clientIDForMode(ModeExchange); got != GitHubClientID {
		t.Errorf("exchange client id = %q, want %q", got, GitHubClientID)
	}
	if got := clientIDForMode(ModeDirect); got != DirectClientID {
		t.Errorf("direct client id = %q, want %q", got, DirectClientID)
	}
}
