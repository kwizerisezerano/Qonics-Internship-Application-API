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
