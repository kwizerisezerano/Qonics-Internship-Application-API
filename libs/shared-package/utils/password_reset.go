package utils

import (
	"fmt"
	"net/url"
	"strings"
)

// PasswordResetKeyPrefix namespaces the single-use password-reset tokens kept
// in Redis (token -> user id). Shared so any service can issue a link that the
// auth service's reset-password endpoint accepts.
const PasswordResetKeyPrefix = "password_reset:"

// FrontendResetURL builds the password-reset link consumed by the frontend
// reset page (/auth/reset-password?token=...). WebURL is configured as a full
// page URL (e.g. http://localhost:3000/auth/login), so only its origin is used
// as the base of the link.
func FrontendResetURL(webURL, token string) string {
	origin := strings.TrimRight(webURL, "/")
	if u, err := url.Parse(webURL); err == nil && u.Scheme != "" && u.Host != "" {
		origin = u.Scheme + "://" + u.Host
	}
	return fmt.Sprintf("%s/auth/reset-password?token=%s", origin, token)
}
