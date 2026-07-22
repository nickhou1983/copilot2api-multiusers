package auth

import "fmt"

// Mode selects how the proxy authenticates against the Copilot backend.
type Mode string

const (
	// ModeExchange mints short-lived Copilot tokens from the GitHub token
	// via copilot_internal/v2/token. This is the default.
	ModeExchange Mode = "exchange"
	// ModeDirect uses the GitHub OAuth token directly as the Copilot bearer,
	// with a static base URL and no token refresh.
	ModeDirect Mode = "direct"
)

// ParseMode validates an auth mode string. An empty string defaults to
// ModeExchange.
func ParseMode(s string) (Mode, error) {
	switch Mode(s) {
	case "", ModeExchange:
		return ModeExchange, nil
	case ModeDirect:
		return ModeDirect, nil
	default:
		return "", fmt.Errorf("invalid auth mode %q (expected %q or %q)", s, ModeExchange, ModeDirect)
	}
}
