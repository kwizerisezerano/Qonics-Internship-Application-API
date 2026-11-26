package controller

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/qonics/qonics-appy-api/libs/shared-package/utils"
	"github.com/qonics/qonics-appy-api/services/applicant/model"
	"github.com/redis/go-redis/v9"
)

// ListPackages returns all available packages (public), paginated
// If ?id= query param is provided, returns that single package instead
func (ac *ApplicantController) ListPackages(c *fiber.Ctx) error {
	ctx := context.Background()

	// Check if ?id= query param is provided for single package
	idStr := c.Query("id")
	if idStr != "" {
		id, err := strconv.Atoi(idStr)
		if err != nil || id < 1 {
			return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid package id")
		}

		var pkg model.Package
		err = ac.DB.QueryRow(ctx, `
			SELECT id, name, description, price, skills, period, currency, is_active
			FROM packages
			WHERE id = $1 AND is_active = true AND deleted_at IS NULL
		`, id).Scan(&pkg.ID, &pkg.Name, &pkg.Description, &pkg.Price, &pkg.Skills, &pkg.Period, &pkg.Currency, &pkg.IsActive)
		if err == pgx.ErrNoRows {
			return utils.JsonErrorResponse(c, fiber.StatusNotFound, "Package not found")
		}
		if err != nil {
			return ac.packagesDBError(c, "Database error", err)
		}

		return utils.JsonSuccessResponse(c, "Package retrieved", pkg)
	}

	// No ?id= parameter, list all packages with pagination
	page, limit := utils.PageParams(c)

	var total int
	if err := ac.DB.QueryRow(ctx, `SELECT COUNT(id) FROM packages WHERE is_active = true`).Scan(&total); err != nil {
		return ac.packagesDBError(c, "Database error", err)
	}

	rows, err := ac.DB.Query(ctx, `
		SELECT id, name, description, price, skills, period, currency, is_active
		FROM packages
		WHERE is_active = true AND deleted_at IS NULL
		ORDER BY id
		LIMIT $1 OFFSET $2`, limit, (page-1)*limit)
	if err != nil {
		return ac.packagesDBError(c, "Database error", err)
	}
	defer rows.Close()

	packages := []model.Package{}
	for rows.Next() {
		var pkg model.Package
		if err := rows.Scan(&pkg.ID, &pkg.Name, &pkg.Description, &pkg.Price, &pkg.Skills, &pkg.Period, &pkg.Currency, &pkg.IsActive); err != nil {
			return ac.packagesDBError(c, "Database error", err)
		}
		packages = append(packages, pkg)
	}
	if err := rows.Err(); err != nil {
		return ac.packagesDBError(c, "Database error", err)
	}

	return utils.JsonPaginatedSuccessResponse(c, "Packages retrieved", packages, utils.GeneratePaginationData(total, limit, page))
}

// SavePackage creates a new package (admin only)
func (ac *ApplicantController) SavePackage(c *fiber.Ctx) error {
	var req model.Package
	if err := c.BodyParser(&req); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid request body")
	}
	if req.Currency == "" {
		req.Currency = utils.DefaultCurrency
	}
	if req.Price < 0 {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Price cannot be negative")
	}

	ctx := context.Background()
	userID := utils.CurrentUserID(c)

	err := ac.DB.QueryRow(ctx, `
		INSERT INTO packages (name, description, price, skills, period, currency, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`, req.Name, req.Description, req.Price, req.Skills, req.Period, req.Currency, userID).Scan(&req.ID)
	if err != nil {
		// Check for unique constraint violation (duplicate name)
		if isDuplicateKeyError(err) {
			return utils.JsonErrorResponse(c, fiber.StatusConflict, "Package with this name already exists")
		}
		return ac.packagesDBError(c, "Failed to save package", err)
	}

	utils.RecordSuccessActivityLog(c, ac.DB, userID, "PACKAGE_SAVE",
		fmt.Sprintf("Created package (package_id=%d, user_id=%d): %s", req.ID, userID, req.Name), nil)

	return utils.JsonSuccessResponse(c, "Package created", req)
}

// UpdatePackage updates an existing package by ID (admin only)
func (ac *ApplicantController) UpdatePackage(c *fiber.Ctx) error {
	idStr := c.Query("id")
	if idStr == "" {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Package id is required as query parameter")
	}

	id, err := strconv.Atoi(idStr)
	if err != nil || id < 1 {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid package id")
	}

	var req model.Package
	if err := c.BodyParser(&req); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid request body")
	}
	if req.Currency == "" {
		req.Currency = utils.DefaultCurrency
	}
	if req.Price < 0 {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Price cannot be negative")
	}

	userID := utils.CurrentUserID(c)
	result, err := ac.DB.Exec(context.Background(), `
		UPDATE packages
		SET name = $2, description = $3, price = $4, skills = $5, period = $6, currency = $7, updated_by = $8, updated_at = NOW()
		WHERE id = $1
	`, id, req.Name, req.Description, req.Price, req.Skills, req.Period, req.Currency, userID)
	if err != nil {
		return ac.packagesDBError(c, "Failed to update package", err)
	}
	if result.RowsAffected() == 0 {
		return utils.JsonErrorResponse(c, fiber.StatusNotFound, "Package not found")
	}

	utils.RecordSuccessActivityLog(c, ac.DB, userID, "PACKAGE_UPDATE",
		fmt.Sprintf("Updated package (package_id=%d, user_id=%d): %s", id, userID, req.Name), nil)

	req.ID = id
	return utils.JsonSuccessResponse(c, "Package updated", req)
}

// DeletePackage soft-deletes a package with Redis undo token (leazi pattern - admin only)
func (ac *ApplicantController) DeletePackage(c *fiber.Ctx) error {
	idStr := c.Query("id")
	if idStr == "" {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Package id is required as query parameter")
	}

	id, err := strconv.Atoi(idStr)
	if err != nil || id < 1 {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid package id")
	}

	// Generate undo token (leazi-style secure random token)
	undoToken, err := utils.SecureRandString(32)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to generate undo token")
	}

	userID := utils.CurrentUserID(c)
	ctx := context.Background()

	// Soft delete: set deleted_at (leazi pattern - same table, no separate deleted table)
	result, err := ac.DB.Exec(ctx, `
		UPDATE packages
		SET deleted_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`, id)
	if err != nil {
		return ac.packagesDBError(c, "Failed to delete package", err)
	}
	if result.RowsAffected() == 0 {
		return utils.JsonErrorResponse(c, fiber.StatusNotFound, "Package not found")
	}

	// Store undo token in Redis with TTL (leazi pattern - 7 days like password reset)
	undoKey := fmt.Sprintf("package:undo:%s", undoToken)
	if err := ac.Redis.Set(ctx, undoKey, id, 7*24*time.Hour).Err(); err != nil {
		// Log warning but don't fail - package is already deleted
		fmt.Printf("WARNING: Failed to store undo token in Redis: %v\n", err)
	}

	utils.RecordSuccessActivityLog(c, ac.DB, userID, "PACKAGE_DELETE",
		fmt.Sprintf("Soft-deleted package (package_id=%d, user_id=%d)", id, userID), nil)

	// Return undo token in response (leazi-style single-use token pattern)
	return utils.JsonSuccessResponse(c, "Package deleted", map[string]string{
		"undo_token": undoToken,
	})
}

// RestorePackage restores a soft-deleted package using undo token from Redis (leazi pattern - admin only)
func (ac *ApplicantController) RestorePackage(c *fiber.Ctx) error {
	var req struct {
		UndoToken string `json:"undo_token"`
	}
	if err := c.BodyParser(&req); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid request body")
	}
	if req.UndoToken == "" {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Undo token is required")
	}

	userID := utils.CurrentUserID(c)
	ctx := context.Background()

	// Look up package ID from Redis (leazi pattern - token in Redis with TTL)
	undoKey := fmt.Sprintf("package:undo:%s", req.UndoToken)
	packageIDStr, err := ac.Redis.Get(ctx, undoKey).Result()
	if err == redis.Nil {
		return utils.JsonErrorResponse(c, fiber.StatusNotFound, "Invalid or expired undo token")
	}
	if err != nil {
		return ac.packagesDBError(c, "Failed to verify undo token", err)
	}

	packageID, err := strconv.Atoi(packageIDStr)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid undo token")
	}

	// Restore: clear deleted_at (leazi pattern - same table, just clear the flag)
	result, err := ac.DB.Exec(ctx, `
		UPDATE packages
		SET deleted_at = NULL
		WHERE id = $1 AND deleted_at IS NOT NULL
	`, packageID)
	if err != nil {
		return ac.packagesDBError(c, "Failed to restore package", err)
	}
	if result.RowsAffected() == 0 {
		return utils.JsonErrorResponse(c, fiber.StatusNotFound, "Package not found or not deleted")
	}

	// Delete the undo token from Redis (single-use token pattern)
	if err := ac.Redis.Del(ctx, undoKey).Err(); err != nil {
		fmt.Printf("WARNING: Failed to delete undo token from Redis: %v\n", err)
	}

	// Fetch restored package
	var pkg model.Package
	err = ac.DB.QueryRow(ctx, `
		SELECT id, name, description, price, skills, period, currency, is_active
		FROM packages
		WHERE id = $1
	`, packageID).Scan(&pkg.ID, &pkg.Name, &pkg.Description, &pkg.Price, &pkg.Skills, &pkg.Period, &pkg.Currency, &pkg.IsActive)
	if err != nil {
		return ac.packagesDBError(c, "Failed to fetch restored package", err)
	}

	utils.RecordSuccessActivityLog(c, ac.DB, userID, "PACKAGE_RESTORE",
		fmt.Sprintf("Restored package via undo token (package_id=%d, user_id=%d)", pkg.ID, userID), nil)

	return utils.JsonSuccessResponse(c, "Package restored", pkg)
}

// packagesDBError answers a failed package query with a 500 and logs the cause.
func (ac *ApplicantController) packagesDBError(c *fiber.Ctx, message string, err error) error {
	return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, message, utils.Logger{
		LogLevel:    utils.CRITICAL,
		Message:     err.Error(),
		ServiceName: ac.Config.ServiceName,
	})
}

// isDuplicateKeyError checks if the error is a PostgreSQL unique constraint violation
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	// PostgreSQL unique constraint violation error code
	errStr := err.Error()
	return strings.Contains(errStr, "duplicate key") || strings.Contains(errStr, "23505")
}
