package utils

import (
	"context"

	"github.com/jackc/pgx/v5"
)

const (
	// DefaultPricePerMonth applies to any course and period an admin has not priced.
	DefaultPricePerMonth = 200000.0
	DefaultCurrency      = "RWF"
)

// Specializations is the canonical taxonomy: each department mapped to the
// specialization values it accepts. Kept in sync with the frontend DEPARTMENTS config.
var Specializations = map[string][]string{
	"software": {"frontend", "backend", "fullstack", "mobile", "database"},
	"iot":      {"embedded", "arduino-esp32", "sensors", "iot-apps", "smart-systems"},
}

// rowQuerier is satisfied by both *pgxpool.Pool and pgx.Tx.
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// LookupPrice returns the price for a course (specialization) and period
// (months). When no admin price exists it is DefaultPricePerMonth x months.
func LookupPrice(ctx context.Context, db rowQuerier, specialization string, months int) (price float64, currency string, err error) {
	err = db.QueryRow(ctx, `
		SELECT price, currency FROM prices WHERE specialization = $1 AND duration_months = $2
	`, specialization, months).Scan(&price, &currency)
	if err == pgx.ErrNoRows {
		return DefaultPricePerMonth * float64(months), DefaultCurrency, nil
	}
	return price, currency, err
}

// PageParams reads ?page and ?limit with the shared defaults (page 1, limit
// 10, limit at most 100).
func PageParams(c interface {
	QueryInt(key string, defaultValue ...int) int
}) (page, limit int) {
	page, limit = c.QueryInt("page", 1), c.QueryInt("limit", 10)
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	return page, limit
}
