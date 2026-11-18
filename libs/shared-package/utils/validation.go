package utils

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
)

// General field patterns, modelled on leazi's regex rules. They live in Go
// regexp literals rather than struct tags because a tag value is run through
// strconv.Unquote: escapes like `\-` are invalid there and silently drop the
// whole tag, and a literal comma would split the tag into pieces.
var (
	// nameRegex accepts human names: letters (including accented ones) plus
	// inner spaces, hyphens and apostrophes; it must start with a letter, so
	// digits and symbols are rejected.
	nameRegex = regexp.MustCompile(`^[A-Za-zÀ-ÿ][A-Za-zÀ-ÿ '\-]*$`)
	// textRegex accepts letters, digits and common punctuation found in
	// institution, field-of-study and qualification names.
	textRegex = regexp.MustCompile(`^[A-Za-zÀ-ÿ0-9 .,&'()/\-]+$`)
	// phoneRegex matches an international phone number once separators are
	// stripped (see PhoneValidation).
	phoneRegex = regexp.MustCompile(`^\+?[0-9]{7,15}$`)
)

// NewValidator returns a validator pre-loaded with the general field rules
// shared across services (leazi-style): `name`, `text`, `regex`, `phone` and
// `strong_password`.
func NewValidator() *validator.Validate {
	v := validator.New()
	_ = v.RegisterValidation("name", NameValidation)
	_ = v.RegisterValidation("text", TextValidation)
	_ = v.RegisterValidation("regex", RegexValidation)
	_ = v.RegisterValidation("phone", PhoneValidation)
	_ = v.RegisterValidation("strong_password", IsStrongPassword)
	return v
}

// NameValidation checks a value against nameRegex (letters, spaces, hyphens
// and apostrophes only).
func NameValidation(fl validator.FieldLevel) bool {
	return nameRegex.MatchString(fl.Field().String())
}

// TextValidation checks a value against textRegex (letters, digits and common
// punctuation).
func TextValidation(fl validator.FieldLevel) bool {
	return textRegex.MatchString(fl.Field().String())
}

// RegexValidation compiles the regular expression supplied in the struct tag
// (e.g. `regex=^[0-9]+$`) and matches it against the field value. The pattern
// must not contain a comma and must use only valid Go escape sequences, since
// struct tags are unquoted before the validator sees them.
func RegexValidation(fl validator.FieldLevel) bool {
	re, err := regexp.Compile(fl.Param())
	if err != nil {
		return false
	}
	return re.MatchString(fl.Field().String())
}

// PhoneValidation accepts empty values (for optional fields) and otherwise
// checks the number against an international pattern once separators are
// removed, so formats like "+250 788-000-000" pass.
func PhoneValidation(fl validator.FieldLevel) bool {
	phone := fl.Field().String()
	if phone == "" {
		return true
	}
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '-', '(', ')', '.':
			return -1
		}
		return r
	}, phone)
	return phoneRegex.MatchString(cleaned)
}

// IsStrongPassword requires at least 8 characters with upper, lower, digit and
// special characters, and rejects three successive identical characters.
func IsStrongPassword(fl validator.FieldLevel) bool {
	password := fl.Field().String()
	if len(password) < 8 {
		return false
	}
	var hasDigit, hasUpper, hasLower, hasSpecial bool
	for i, char := range password {
		switch {
		case unicode.IsDigit(char):
			hasDigit = true
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
		if i >= 2 && password[i] == password[i-1] && password[i-1] == password[i-2] {
			return false
		}
	}
	return hasDigit && hasUpper && hasLower && hasSpecial
}

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
	case "name", "text", "regex":
		return "contains unsupported characters"
	case "phone":
		return "must be a valid phone number"
	case "strong_password":
		return "must be at least 8 characters and include an uppercase letter, a lowercase letter, a digit and a symbol"
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
