package utils

import (
	"context"

	"github.com/jackc/pgx/v5"
)

const (
	// DefaultCurrency is the currency used for all packages
	DefaultCurrency = "RWF"
)

// rowQuerier is satisfied by both *pgxpool.Pool and pgx.Tx.
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Package represents a training package with pricing and skills
type Package struct {
	ID          int
	Name        string
	Description string
	Price       float64
	Skills      []string
	Period      string
	Currency    string
	IsActive    bool
}

// LookupPackage returns the package details for a given package_id
func LookupPackage(ctx context.Context, db rowQuerier, packageID int) (*Package, error) {
	var pkg Package
	err := db.QueryRow(ctx, `
		SELECT id, name, description, price, skills, period, currency, is_active
		FROM packages
		WHERE id = $1 AND is_active = true AND deleted_at IS NULL
	`, packageID).Scan(
		&pkg.ID,
		&pkg.Name,
		&pkg.Description,
		&pkg.Price,
		&pkg.Skills,
		&pkg.Period,
		pkg.Currency,
		&pkg.IsActive,
	)
	if err != nil {
		return nil, err
	}
	return &pkg, nil
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
