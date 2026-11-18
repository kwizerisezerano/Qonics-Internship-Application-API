package utils

import (
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// TestNameValidation covers the `name` rule: letters (including accented),
// inner spaces, hyphens and apostrophes only.
func TestNameValidation(t *testing.T) {
	v := NewValidator()
	cases := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"simple", "John", false},
		{"two words", "John Doe", false},
		{"accented", "Amélie N'Dour", false},
		{"hyphenated", "Jean-Paul", false},
		{"apostrophe", "O'Brien", false},
		{"contains digits", "John123", true},
		{"contains emoji", "John \U0001F600", true},
		{"leading space", " John", true},
		{"symbol", "John@", true},
		{"empty", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := v.Var(tc.value, "name")
			if tc.wantErr {
				assert.Error(t, err, "expected %q to be rejected", tc.value)
			} else {
				assert.NoError(t, err, "expected %q to pass", tc.value)
			}
		})
	}
}

// TestTextValidation covers the `text` rule: letters, digits and common
// punctuation used in institution / course names (comma and hyphen included).
func TestTextValidation(t *testing.T) {
	v := NewValidator()
	cases := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"plain", "University of Rwanda", false},
		{"with comma", "University of Rwanda, Kigali", false},
		{"with ampersand", "Rwanda & Co.", false},
		{"with parentheses", "Institute (Main Campus)", false},
		{"with digits", "Level 3 College", false},
		{"with slash", "BSc/MSc", false},
		{"emoji", "Uni \U0001F600", true},
		{"semicolon", "Uni;", true},
		{"empty", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := v.Var(tc.value, "text")
			if tc.wantErr {
				assert.Error(t, err, "expected %q to be rejected", tc.value)
			} else {
				assert.NoError(t, err, "expected %q to pass", tc.value)
			}
		})
	}
}

// TestPhoneValidation covers the `phone` rule, which accepts any of the leazi
// supported formats (E164, NANP, European, Asian, generic international).
// Empty values are allowed so the tag can be used on optional fields alongside
// `omitempty`.
func TestPhoneValidation(t *testing.T) {
	v := NewValidator()
	cases := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"e164 rwanda", "+250788000000", false},
		{"international with separators", "+250 788-000-000", false},
		{"local ten digits", "0788000000", false},
		{"nanp parens and dots", "(078) 800.0000", false},
		{"nanp us format", "+1 (555) 123-4567", false},
		{"european format", "+44 20 7946 0958", false},
		{"asian format", "+81 90 1234 5678", false},
		{"empty allowed", "", false},
		{"too short", "12345", true},
		{"too long", "+1234567890123456", true},
		{"letters", "abc", true},
		{"symbols", "+250-ABC", true},
		{"only a plus", "+", true},
		{"whitespace only", "   ", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := v.Var(tc.value, "phone")
			if tc.wantErr {
				assert.Error(t, err, "expected %q to be rejected", tc.value)
			} else {
				assert.NoError(t, err, "expected %q to pass", tc.value)
			}
		})
	}
}

// TestPhoneValidatorNormalization covers the PhoneValidator used behind the
// `phone` rule: it reports the detected format and normalises the number to
// E.164 (10-digit numbers with no country code become NANP +1).
func TestPhoneValidatorNormalization(t *testing.T) {
	v, err := NewPhoneValidator()
	require.NoError(t, err)

	result := v.Validate("+250788000000", nil)
	assert.True(t, result.IsValid)
	assert.Equal(t, "+250788000000", result.E164)

	result = v.Validate("+250 788-000-000", nil)
	assert.True(t, result.IsValid)
	assert.Equal(t, FormatInternational, result.Format)
	assert.Equal(t, "+250788000000", result.E164)

	result = v.Validate("(555) 123-4567", nil)
	assert.True(t, result.IsValid)
	assert.Equal(t, FormatNANP, result.Format)
	assert.Equal(t, "+15551234567", result.E164)
	assert.Equal(t, "+1 (555) 123-4567", result.Normalized)

	invalid := v.Validate("not-a-phone", nil)
	assert.False(t, invalid.IsValid)
	assert.NotEmpty(t, invalid.ErrorMessage)
}

// TestStrongPasswordValidation covers the `strong_password` rule: at least 8
// characters with upper, lower, digit and symbol, and no three identical
// characters in a row.
func TestStrongPasswordValidation(t *testing.T) {
	v := NewValidator()
	cases := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"strong", "Str0ng@Pass", false},
		{"no symbol", "Str0ngPass1", true},
		{"no uppercase", "str0ng@pass", true},
		{"no digit", "Strong@Pass", true},
		{"no lowercase", "STR0NG@PASS", true},
		{"too short", "St0@g", true},
		{"three repeated", "Abbb1@cd", true},
		{"empty", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := v.Var(tc.value, "strong_password")
			if tc.wantErr {
				assert.Error(t, err, "expected %q to be rejected", tc.value)
			} else {
				assert.NoError(t, err, "expected %q to pass", tc.value)
			}
		})
	}
}
