package utils

import (
	"fmt"
	"regexp"
	"strings"
)

// PhoneFormat represents the type of phone number format.
type PhoneFormat string

const (
	FormatE164          PhoneFormat = "E164"
	FormatNANP          PhoneFormat = "NANP"
	FormatEuropean      PhoneFormat = "European"
	FormatAsian         PhoneFormat = "Asian"
	FormatInternational PhoneFormat = "International"
)

// PhoneValidator contains the compiled regex patterns used for phone validation.
// A number is accepted when it matches any of the known formats.
type PhoneValidator struct {
	patterns map[PhoneFormat]*regexp.Regexp
}

// PhoneValidationResult carries the validation outcome plus the normalised
// representation of the number.
type PhoneValidationResult struct {
	IsValid      bool
	Format       PhoneFormat
	E164         string
	Normalized   string
	ErrorMessage string
}

// NewPhoneValidator builds a phone validator with all patterns compiled once.
func NewPhoneValidator() (*PhoneValidator, error) {
	patterns := map[PhoneFormat]string{
		FormatE164:          `^\+[1-9]\d{1,14}$`,
		FormatNANP:          `^(?:\+?1[-. ]?)?\(?([0-9]{3})\)?[-. ]?([0-9]{3})[-. ]?([0-9]{4})$`,
		FormatEuropean:      `^\+(?:[0-9]){1,3}(?: ?[0-9]{2,4}){2,4}$`,
		FormatAsian:         `^\+(?:[0-9]){1,4}(?: ?[0-9]{2,4}){2,5}$`,
		FormatInternational: `^\+?(?:[0-9][ -]?){6,14}[0-9]$`,
	}

	v := &PhoneValidator{patterns: make(map[PhoneFormat]*regexp.Regexp, len(patterns))}
	for format, pattern := range patterns {
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("failed to compile pattern for %s: %v", format, err)
		}
		v.patterns[format] = compiled
	}

	return v, nil
}

// Validate checks a phone number against all patterns or, when a format is
// supplied, only that format.
func (v *PhoneValidator) Validate(phoneNumber string, format *PhoneFormat) PhoneValidationResult {
	result := PhoneValidationResult{IsValid: false}

	phoneNumber = strings.TrimSpace(phoneNumber)
	if phoneNumber == "" {
		result.ErrorMessage = "phone number cannot be empty"
		return result
	}

	if format != nil {
		pattern, ok := v.patterns[*format]
		if !ok {
			result.ErrorMessage = "invalid format specified"
			return result
		}
		e164, err := v.ToE164(phoneNumber)
		if !pattern.MatchString(phoneNumber) || err != nil {
			result.ErrorMessage = fmt.Sprintf("does not match %s format", *format)
			return result
		}
		result.IsValid = true
		result.Format = *format
		result.E164 = e164
		result.Normalized = v.FormatForDisplay(e164)
		return result
	}

	for fmtFormat, pattern := range v.patterns {
		if !pattern.MatchString(phoneNumber) {
			continue
		}
		// A regional-format match is only accepted when the number also
		// normalises to a valid E.164 value. This enforces the E.164 cap of 15
		// digits, so over-long numbers the looser regional patterns allow are
		// still rejected.
		e164, err := v.ToE164(phoneNumber)
		if err != nil {
			continue
		}
		result.IsValid = true
		result.Format = fmtFormat
		result.E164 = e164
		result.Normalized = v.FormatForDisplay(e164)
		return result
	}

	result.ErrorMessage = "no matching format found"
	return result
}

// ToE164 keeps only a leading + and the digits, then normalises 10-digit
// numbers without a country code to NANP (+1).
func (v *PhoneValidator) ToE164(phoneNumber string) (string, error) {
	var builder strings.Builder
	for i, char := range phoneNumber {
		if i == 0 && char == '+' {
			builder.WriteRune(char)
		} else if char >= '0' && char <= '9' {
			builder.WriteRune(char)
		}
	}

	digits := builder.String()
	if !strings.HasPrefix(digits, "+") {
		if len(digits) == 10 {
			digits = "+1" + digits
		} else {
			digits = "+" + digits
		}
	}

	if v.patterns[FormatE164].MatchString(digits) {
		return digits, nil
	}
	return "", fmt.Errorf("invalid phone number format")
}

// FormatForDisplay renders a stored E.164 number in a friendly format.
func (v *PhoneValidator) FormatForDisplay(e164 string) string {
	if !strings.HasPrefix(e164, "+") || len(e164) < 2 {
		return e164
	}

	rest := e164[1:] // Skip the +

	switch {
	case strings.HasPrefix(rest, "1"): // NANP (USA, Canada)
		number := rest[1:]
		if len(number) == 10 {
			return fmt.Sprintf("+1 (%s) %s-%s", number[:3], number[3:6], number[6:])
		}
	case strings.HasPrefix(rest, "44"): // UK
		number := rest[2:]
		if len(number) >= 10 {
			return fmt.Sprintf("+44 %s %s %s", number[:2], number[2:6], number[6:])
		}
	case strings.HasPrefix(rest, "33"), strings.HasPrefix(rest, "49"): // France, Germany
		countryCode := rest[:2]
		number := rest[2:]
		if len(number) >= 9 {
			return fmt.Sprintf("+%s %s %s %s %s %s",
				countryCode, number[:1], number[1:3], number[3:5], number[5:7], number[7:])
		}
	}

	return fmt.Sprintf("+%s %s", rest[:2], rest[2:])
}

// ValidatePhone reports whether the number matches any supported format. It is
// the primitive behind the `phone` validator rule.
func ValidatePhone(phone string) bool {
	v, err := NewPhoneValidator()
	if err != nil {
		return false
	}
	return v.Validate(phone, nil).IsValid
}
