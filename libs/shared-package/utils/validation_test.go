package utils

import (
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
)

// TestFormatValidationErrors covers the field-level error payload: snake_case
// keys derived from the Go field names plus a human-readable message per tag.
func TestFormatValidationErrors(t *testing.T) {
	type sample struct {
		FullName string `validate:"required"`
		Email    string `validate:"required,email"`
		WorkMode string `validate:"oneof=onsite remote"`
		Age      int    `validate:"gte=18"`
	}

	err := Validate.Struct(sample{Email: "not-an-email", WorkMode: "invalid", Age: 10})
	assert.Error(t, err)

	got := FormatValidationErrors(err)

	byKey := make(map[string]string, len(got))
	for _, e := range got {
		byKey[e.Key] = e.Message
	}

	assert.Equal(t, "is required", byKey["full_name"])
	assert.Equal(t, "must be a valid email address", byKey["email"])
	assert.Equal(t, "must be one of: onsite, remote", byKey["work_mode"])
	assert.Equal(t, "must be greater than or equal to 18", byKey["age"])
}

// TestFormatValidationErrorsNonValidatorError makes sure a non-validator error
// yields no field list rather than a panic.
func TestFormatValidationErrorsNonValidatorError(t *testing.T) {
	assert.Nil(t, FormatValidationErrors(assert.AnError))
	assert.Empty(t, FormatValidationErrors(validator.ValidationErrors{}))
}

// TestGeneralFieldValidators covers the leazi-style general rules carried by
// NewValidator: name letters, free-text punctuation, phone format and
// password strength.
func TestGeneralFieldValidators(t *testing.T) {
	type sample struct {
		FullName    string `validate:"required,name"`
		Institution string `validate:"omitempty,text"`
		Phone       string `validate:"required,phone"`
		Password    string `validate:"required,strong_password"`
	}

	v := NewValidator()

	assert.NoError(t, v.Struct(sample{
		FullName:    "Jean-Paul O'Brien",
		Institution: "University of Rwanda, Kigali", // comma is allowed for text
		Phone:       "+250 788-000-000",
		Password:    "Str0ng@Pass",
	}))

	err := v.Struct(sample{
		FullName:    "John123",
		Institution: "Uni \U0001F600", // emoji
		Phone:       "abc",
		Password:    "weak",
	})
	assert.Error(t, err)

	byKey := make(map[string]string)
	for _, e := range FormatValidationErrors(err) {
		byKey[e.Key] = e.Message
	}

	assert.Equal(t, "contains unsupported characters", byKey["full_name"])
	assert.Equal(t, "contains unsupported characters", byKey["institution"])
	assert.Equal(t, "must be a valid phone number", byKey["phone"])
	assert.Contains(t, byKey["password"], "uppercase")
}
