package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRegisterRequestValidation covers the general field rules applied to the
// registration payload (name letters, email, phone format, strong password).
func TestRegisterRequestValidation(t *testing.T) {
	base := func() RegisterRequest {
		return RegisterRequest{
			FullName: "John Doe",
			Email:    "john@example.com",
			Phone:    "+250788000000",
			Password: "Str0ng@Pass",
		}
	}

	t.Run("valid payload", func(t *testing.T) {
		req := base()
		assert.NoError(t, Validate.Struct(&req))
	})

	t.Run("rejects name with digits", func(t *testing.T) {
		req := base()
		req.FullName = "John123"
		assert.Error(t, Validate.Struct(&req))
	})

	t.Run("rejects invalid email", func(t *testing.T) {
		req := base()
		req.Email = "not-an-email"
		assert.Error(t, Validate.Struct(&req))
	})

	t.Run("rejects invalid phone", func(t *testing.T) {
		req := base()
		req.Phone = "abc"
		assert.Error(t, Validate.Struct(&req))
	})

	t.Run("rejects weak password", func(t *testing.T) {
		req := base()
		req.Password = "password123"
		assert.Error(t, Validate.Struct(&req))
	})

	t.Run("rejects empty payload", func(t *testing.T) {
		assert.Error(t, Validate.Struct(&RegisterRequest{}))
	})
}

// TestPasswordStrengthOnResetAndChange makes sure the strong-password rule is
// enforced on both password-writing flows.
func TestPasswordStrengthOnResetAndChange(t *testing.T) {
	t.Run("reset accepts strong password", func(t *testing.T) {
		req := ResetPasswordRequest{Token: "tok", Password: "Str0ng@Pass"}
		assert.NoError(t, Validate.Struct(&req))
	})

	t.Run("reset rejects weak password", func(t *testing.T) {
		req := ResetPasswordRequest{Token: "tok", Password: "weakpass"}
		assert.Error(t, Validate.Struct(&req))
	})

	t.Run("reset requires token", func(t *testing.T) {
		req := ResetPasswordRequest{Password: "Str0ng@Pass"}
		assert.Error(t, Validate.Struct(&req))
	})

	t.Run("change accepts strong new password", func(t *testing.T) {
		req := ChangePasswordRequest{CurrentPassword: "Old@Pass1", NewPassword: "Str0ng@Pass"}
		assert.NoError(t, Validate.Struct(&req))
	})

	t.Run("change rejects weak new password", func(t *testing.T) {
		req := ChangePasswordRequest{CurrentPassword: "Old@Pass1", NewPassword: "weakpass"}
		assert.Error(t, Validate.Struct(&req))
	})
}

// TestLoginValidation confirms login only requires presence (no strength rule),
// so existing accounts are never locked out by the password policy.
func TestLoginValidation(t *testing.T) {
	assert.NoError(t, Validate.Struct(&LoginRequest{Email: "john@example.com", Password: "password123"}))
	assert.Error(t, Validate.Struct(&LoginRequest{Email: "nope", Password: ""}))
}
