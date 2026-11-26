package controller

import (
	"context"
	"fmt"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/qonics/qonics-appy-api/libs/shared-package/utils"
	"github.com/qonics/qonics-appy-api/services/applicant/model"
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
			WHERE id = $1 AND is_active = true
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
		WHERE is_active = true
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

	userID := utils.CurrentUserID(c)
	err := ac.DB.QueryRow(context.Background(), `
		INSERT INTO packages (name, description, price, skills, period, currency, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`, req.Name, req.Description, req.Price, req.Skills, req.Period, req.Currency, userID).Scan(&req.ID)
	if err != nil {
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

// DeletePackage removes a package (admin only)
func (ac *ApplicantController) DeletePackage(c *fiber.Ctx) error {
	idStr := c.Query("id")
	if idStr == "" {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Package id is required as query parameter")
	}

	id, err := strconv.Atoi(idStr)
	if err != nil || id < 1 {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid package id")
	}

	var removed int
	err = ac.DB.QueryRow(context.Background(), `DELETE FROM packages WHERE id = $1 RETURNING id`, id).Scan(&removed)
	if err == pgx.ErrNoRows {
		return utils.JsonErrorResponse(c, fiber.StatusNotFound, "Package not found")
	}
	if err != nil {
		return ac.packagesDBError(c, "Failed to delete package", err)
	}

	userID := utils.CurrentUserID(c)
	utils.RecordSuccessActivityLog(c, ac.DB, userID, "PACKAGE_DELETE",
		fmt.Sprintf("Deleted package (package_id=%d, user_id=%d)", removed, userID), nil)

	return utils.JsonSuccessResponse(c, "Package deleted", nil)
}

// packagesDBError answers a failed package query with a 500 and logs the cause.
func (ac *ApplicantController) packagesDBError(c *fiber.Ctx, message string, err error) error {
	return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, message, utils.Logger{
		LogLevel:    utils.CRITICAL,
		Message:     err.Error(),
		ServiceName: ac.Config.ServiceName,
	})
}
