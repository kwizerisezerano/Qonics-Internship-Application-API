package utils

import (
	"strings"
	"unicode"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
)

// FieldError is a single field-level validation failure, keyed by the
// snake_case name of the offending field (leazi-style).
type FieldError struct {
	Key     string `json:"key"`
	Message string `json:"message"`
}

// FormatValidationErrors turns the raw validator error into a field-level list.
// Each entry carries the field (snake_case) and a human-readable message, so
// clients can highlight the exact inputs that failed.
func FormatValidationErrors(err error) []FieldError {
	verrs, ok := err.(validator.ValidationErrors)
	if !ok {
		return nil
	}
	fieldErrors := make([]FieldError, 0, len(verrs))
	for _, e := range verrs {
		fieldErrors = append(fieldErrors, FieldError{
			Key:     toSnakeCase(e.Field()),
			Message: validationMessage(e),
		})
	}
	return fieldErrors
}

// JsonValidationErrorResponse returns a 400 carrying the field-level validation
// errors. It is the standard response for request-body validation failures on
// POST/PUT/PATCH handlers.
func JsonValidationErrorResponse(c *fiber.Ctx, err error) error {
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
		"status":  fiber.StatusBadRequest,
		"message": "Validation failed",
		"errors":  FormatValidationErrors(err),
	})
}

// validationMessage maps a validator tag to a human-readable message.
func validationMessage(e validator.FieldError) string {
	switch e.Tag() {
	case "required":
		return "is required"
	case "email":
		return "must be a valid email address"
	case "url":
		return "must be a valid URL"
	case "uuid":
		return "must be a valid UUID"
	case "datetime":
		return "must be a valid date/time"
	case "min":
		return "must be at least " + e.Param()
	case "max":
		return "must be at most " + e.Param()
	case "len":
		return "must be exactly " + e.Param() + " characters"
	case "gt":
		return "must be greater than " + e.Param()
	case "gte":
		return "must be greater than or equal to " + e.Param()
	case "lt":
		return "must be less than " + e.Param()
	case "lte":
		return "must be less than or equal to " + e.Param()
	case "oneof":
		return "must be one of: " + strings.ReplaceAll(e.Param(), " ", ", ")
	case "specialization":
		return "must be a valid specialization for the selected department"
	default:
		return "is invalid"
	}
}

// toSnakeCase converts a Go struct field name (e.g. "FullName") to snake_case
// ("full_name") so the error keys line up with the JSON payload.
func toSnakeCase(s string) string {
	var b strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if unicode.IsUpper(r) {
			if i > 0 {
				prev := runes[i-1]
				nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
				if unicode.IsLower(prev) || unicode.IsDigit(prev) || nextLower {
					b.WriteRune('_')
				}
			}
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
