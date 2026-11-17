package utils

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/gofiber/fiber/v2"
)

const (
	SESSION_COOKIE_NAME    = "qonics_session"
	REFRESH_COOKIE_NAME    = "qonics_refresh"
	DEVICE_COOKIE_NAME     = "device_id"
	COOKIE_MAX_AGE         = 86400  // 24 hours in seconds
	REFRESH_COOKIE_MAX_AGE = 604800 // 7 days in seconds
)

// SetAuthCookie sets the authentication cookie
func SetAuthCookie(c *fiber.Ctx, token string, maxAge int) {
	c.Cookie(&fiber.Cookie{
		Name:     SESSION_COOKIE_NAME,
		Value:    token,
		MaxAge:   maxAge,
		Path:     "/",
		Secure:   false, // Set to true in production with HTTPS
		HTTPOnly: true,
		SameSite: "lax",
	})
}

// SetRefreshCookie sets the refresh token cookie
func SetRefreshCookie(c *fiber.Ctx, token string, maxAge int) {
	c.Cookie(&fiber.Cookie{
		Name:     REFRESH_COOKIE_NAME,
		Value:    token,
		MaxAge:   maxAge,
		Path:     "/",
		Secure:   false, // Set to true in production with HTTPS
		HTTPOnly: true,
		SameSite: "lax",
	})
}

// SetDeviceCookie sets a device identifier cookie
func SetDeviceCookie(c *fiber.Ctx, deviceID string) {
	c.Cookie(&fiber.Cookie{
		Name:     DEVICE_COOKIE_NAME,
		Value:    deviceID,
		MaxAge:   REFRESH_COOKIE_MAX_AGE,
		Path:     "/",
		Secure:   false,
		HTTPOnly: true,
		SameSite: "lax",
	})
}

// ClearAuthCookies clears all authentication cookies
func ClearAuthCookies(c *fiber.Ctx) {
	c.ClearCookie(SESSION_COOKIE_NAME, REFRESH_COOKIE_NAME, DEVICE_COOKIE_NAME)
}

// GetCookieValuesFromRequest retrieves all values for a given cookie name
func GetCookieValuesFromRequest(c *fiber.Ctx, cookieName string) []string {
	cookie := c.Cookies(cookieName)
	if cookie == "" {
		return nil
	}
	return []string{cookie}
}

// GenerateRefreshToken generates a random refresh token
func GenerateRefreshToken(userID int) string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// GenerateDeviceID generates a unique device identifier
func GenerateDeviceID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// GetClientIPAddress gets the client's IP address
func GetClientIPAddress(c *fiber.Ctx) string {
	// Check for forwarded headers (behind proxy)
	if ip := c.Get("X-Forwarded-For"); ip != "" {
		return ip
	}
	if ip := c.Get("X-Real-IP"); ip != "" {
		return ip
	}
	return c.IP()
}
