package utils

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

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
