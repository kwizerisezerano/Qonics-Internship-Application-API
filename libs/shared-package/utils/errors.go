package utils

import (
	"strings"
)

// IsErrDuplicate checks if the error is a duplicate key violation
// Returns (isDuplicate, keyName)
func IsErrDuplicate(err error) (bool, string) {
	if err == nil {
		return false, ""
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "duplicate key value violates unique constraint") {
		return false, ""
	}

	// Extract the constraint name from the error message
	keyName := ""
	parts := strings.Split(errStr, "\"")
	if len(parts) > 1 {
		key := parts[1]
		// Map constraint names to user-friendly field names
		switch key {
		case "users_email_hash_key":
			keyName = "email"
		case "users_phone_hash_key":
			keyName = "phone"
		case "users_full_name_hash_key":
			keyName = "full_name"
		case "applicant_profiles_email_hash_key":
			keyName = "email"
		case "applicant_profiles_phone_hash_key":
			keyName = "phone"
		case "applications_reference_key":
			keyName = "reference"
		default:
			keyName = key
		}
	}

	return true, keyName
}

// IsForeignKeyErr checks if the error is a foreign key violation
// Returns (isForeignKey, tableName)
func IsForeignKeyErr(err error) (bool, string) {
	if err == nil {
		return false, ""
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "violates foreign key constraint") {
		return false, ""
	}

	// Extract the table name from the error message
	tableName := ""
	parts := strings.Split(errStr, "\"")
	if len(parts) > 1 {
		tableName = parts[1]
	}

	return true, tableName
}

// GetDuplicateKeyMessage returns a user-friendly message for duplicate key errors
func GetDuplicateKeyMessage(keyName string) string {
	switch keyName {
	case "email":
		return "An account with this email already exists"
	case "phone":
		return "An account with this phone number already exists"
	case "full_name":
		return "An account with this name already exists"
	case "reference":
		return "An application with this reference already exists"
	default:
		return "A record with this value already exists"
	}
}

// GetForeignKeyMessage returns a user-friendly message for foreign key errors
func GetForeignKeyMessage(tableName string) string {
	switch tableName {
	case "applicant_profiles":
		return "Referenced applicant profile does not exist"
	case "applications":
		return "Referenced application does not exist"
	case "users":
		return "Referenced user does not exist"
	default:
		return "Referenced record does not exist"
	}
}
