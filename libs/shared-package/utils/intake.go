package utils

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DefaultClosureMessage is shown when intake is closed and the admin left the
// closure message empty.
const DefaultClosureMessage = "Applications are currently closed."

// IntakeStatus is whether new applications (and their document uploads) are
// being accepted.
type IntakeStatus struct {
	// Open is the effective state: false when switched off by an admin or
	// once CloseOn has passed.
	Open bool
	// CloseOn is the configured closing date, if any.
	CloseOn *time.Time
	// Message explains a closed intake; never empty when Open is false.
	Message string
}

// GetIntakeStatus reads the admin-controlled settings row (id = 1) in one
// query. A missing row means intake is open.
func GetIntakeStatus(ctx context.Context, db *pgxpool.Pool) (IntakeStatus, error) {
	status := IntakeStatus{}
	err := db.QueryRow(ctx, `
		SELECT applications_open, close_on, closure_message
		FROM settings WHERE id = 1
	`).Scan(&status.Open, &status.CloseOn, &status.Message)
	if err == pgx.ErrNoRows {
		return IntakeStatus{Open: true}, nil
	}
	if err != nil {
		return IntakeStatus{}, err
	}
	if status.CloseOn != nil && !status.CloseOn.After(time.Now()) {
		status.Open = false
	}
	if !status.Open && status.Message == "" {
		status.Message = DefaultClosureMessage
	}
	return status, nil
}
