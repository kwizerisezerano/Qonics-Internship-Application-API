package utils

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewUserPayload(t *testing.T) {
	tests := []struct {
		name   string
		userID any
		wantID int
	}{
		{"JWT number (float64)", float64(42), 42},
		{"int", 7, 7},
		{"int64", int64(9), 9},
		{"missing", nil, 0},
		{"wrong type", "42", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewUserPayload(tt.userID, "a@example.com", "applicant")
			assert.Equal(t, tt.wantID, p.ID)
			assert.Equal(t, "a@example.com", p.Email)
			assert.False(t, p.IsAdmin())
		})
	}

	assert.True(t, NewUserPayload(float64(1), "x@example.com", "admin").IsAdmin())
	empty := NewUserPayload(float64(1), nil, nil)
	assert.Equal(t, "", empty.Email)
	assert.False(t, empty.IsAdmin())
}

func TestGetUserPayload(t *testing.T) {
	run := func(set func(c *fiber.Ctx)) (UserPayload, bool) {
		var got UserPayload
		var ok bool
		app := fiber.New()
		app.Get("/", func(c *fiber.Ctx) error {
			set(c)
			got, ok = GetUserPayload(c)
			return nil
		})
		_, err := app.Test(httptest.NewRequest("GET", "/", nil))
		require.NoError(t, err)
		return got, ok
	}

	p, ok := run(func(c *fiber.Ctx) {
		c.Locals(UserPayloadKey, UserPayload{ID: 5, Email: "e@example.com", Role: "admin"})
	})
	assert.True(t, ok)
	assert.Equal(t, 5, p.ID)
	assert.True(t, p.IsAdmin())

	_, ok = run(func(c *fiber.Ctx) {})
	assert.False(t, ok, "no payload set")

	_, ok = run(func(c *fiber.Ctx) { c.Locals(UserPayloadKey, UserPayload{ID: 0}) })
	assert.False(t, ok, "a payload without a user id is not authenticated")
}

func TestNormalizeEmail(t *testing.T) {
	assert.Equal(t, "user@example.com", NormalizeEmail("  User@Example.COM "))
	assert.Equal(t, "", NormalizeEmail("   "))
}

func TestDecryptSQL(t *testing.T) {
	assert.Equal(t, "pgp_sym_decrypt(ap.email_encrypted, '"+GetEncryptionKey()+"')", DecryptSQL("ap.email_encrypted"))
}
