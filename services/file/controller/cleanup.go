package controller

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qonics/qonics-appy-api/services/file/helper"
)

// cleanupInterval is how often the orphan-file sweeper runs.
const cleanupInterval = 1 * time.Hour

// StartOrphanFileCleanup launches a background worker that permanently deletes
// files whose pending window (deleted_at) has expired - i.e. uploads that were
// created via presigned-url but never confirmed. It removes both the R2 object
// and the database row, mirroring leazi's orphan-file cleanup.
func StartOrphanFileCleanup(ctx context.Context, db *pgxpool.Pool, serviceName string) {
	go func() {
		ticker := time.NewTicker(cleanupInterval)
		defer ticker.Stop()

		// Run once shortly after startup, then on every tick.
		cleanupExpiredFiles(ctx, db, serviceName)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cleanupExpiredFiles(ctx, db, serviceName)
			}
		}
	}()
}

func cleanupExpiredFiles(ctx context.Context, db *pgxpool.Pool, serviceName string) {
	rows, err := db.Query(ctx, `
		SELECT id, file_path FROM files
		WHERE deleted_at IS NOT NULL AND deleted_at < NOW()
	`)
	if err != nil {
		log.Printf("[%s] orphan cleanup: failed to query expired files: %v", serviceName, err)
		return
	}

	type pending struct{ id, path string }
	var expired []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.id, &p.path); err != nil {
			log.Printf("[%s] orphan cleanup: failed to scan row: %v", serviceName, err)
			continue
		}
		expired = append(expired, p)
	}
	rows.Close()

	if len(expired) == 0 {
		return
	}

	r2Client, err := helper.R2ClientFactory()
	if err != nil {
		log.Printf("[%s] orphan cleanup: failed to initialize R2 client: %v", serviceName, err)
		return
	}

	for _, p := range expired {
		if err := r2Client.DeleteObject(p.path); err != nil {
			// Log and keep the row so a later sweep can retry.
			log.Printf("[%s] orphan cleanup: failed to delete object %s: %v", serviceName, p.path, err)
			continue
		}
		if _, err := db.Exec(ctx, `DELETE FROM files WHERE id = $1`, p.id); err != nil {
			log.Printf("[%s] orphan cleanup: failed to delete row %s: %v", serviceName, p.id, err)
			continue
		}
	}

	fmt.Printf("[%s] orphan cleanup: removed %d expired file(s)\n", serviceName, len(expired))
}
