package utils

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// GetEncryptionKey returns the encryption key (hardcoded to 'secret' for development)
func GetEncryptionKey() string {
	return "secret"
}

// EncryptData encrypts data using pgcrypto pgp_sym_encrypt
func EncryptData(ctx context.Context, db *pgxpool.Pool, data string) ([]byte, error) {
	var encrypted []byte
	err := db.QueryRow(ctx, "SELECT pgp_sym_encrypt($1, 'secret')", data).Scan(&encrypted)
	return encrypted, err
}

// DecryptData decrypts data using pgcrypto pgp_sym_decrypt
func DecryptData(ctx context.Context, db *pgxpool.Pool, data []byte) (string, error) {
	var decrypted string
	err := db.QueryRow(ctx, "SELECT pgp_sym_decrypt($1::bytea, 'secret')", data).Scan(&decrypted)
	return decrypted, err
}

// HashData creates a SHA256 hash of the data (hex-encoded to match PostgreSQL)
func HashData(data string) string {
	h := sha256.New()
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

// EncryptAndHash encrypts data and returns both encrypted value and hash
func EncryptAndHash(ctx context.Context, db *pgxpool.Pool, data string) ([]byte, string, error) {
	encrypted, err := EncryptData(ctx, db, data)
	if err != nil {
		return nil, "", err
	}
	hash := HashData(data)
	return encrypted, hash, nil
}

// NormalizeEmail trims and lowercases an email address. Every place that
// stores or looks up an email (and its hash) must use it, so "User@X.com" and
// "user@x.com" are the same account.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// DecryptSQL returns the SQL expression that decrypts an encrypted column in a
// query, so PII can be read in the same SELECT (or JOIN) instead of one extra
// round trip per value through DecryptData.
func DecryptSQL(column string) string {
	return "pgp_sym_decrypt(" + column + ", '" + GetEncryptionKey() + "')"
}
