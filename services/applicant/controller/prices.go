package controller

import (
	"context"
	"fmt"
	"slices"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/qonics/qonics-appy-api/libs/shared-package/utils"
	"github.com/qonics/qonics-appy-api/services/applicant/model"
)

// ListPrices returns the admin-set prices (public), paginated and optionally
// filtered by ?department, ?specialization and ?duration_months.
// Courses and periods without a row use the default (see GetPriceQuote).
func (ac *ApplicantController) ListPrices(c *fiber.Ctx) error {
	page, limit := utils.PageParams(c)
	ctx := context.Background()
	where := ` WHERE ($1 = '' OR department = $1) AND ($2 = '' OR specialization = $2) AND ($3 = 0 OR duration_months = $3)`
	args := []any{c.Query("department"), c.Query("specialization"), c.QueryInt("duration_months", 0)}

	var total int
	if err := ac.DB.QueryRow(ctx, `SELECT COUNT(id) FROM prices`+where, args...).Scan(&total); err != nil {
		return ac.pricesDBError(c, "Database error", err)
	}

	rows, err := ac.DB.Query(ctx, `
		SELECT id, department, specialization, duration_months, price, currency
		FROM prices`+where+` ORDER BY department, specialization, duration_months
		LIMIT $4 OFFSET $5`, append(args, limit, (page-1)*limit)...)
	if err != nil {
		return ac.pricesDBError(c, "Database error", err)
	}
	defer rows.Close()

	prices := []model.Price{}
	for rows.Next() {
		var p model.Price
		var amount float64
		if err := rows.Scan(&p.ID, &p.Department, &p.Specialization, &p.DurationMonths, &amount, &p.Currency); err != nil {
			return ac.pricesDBError(c, "Database error", err)
		}
		p.Price = &amount
		prices = append(prices, p)
	}
	if err := rows.Err(); err != nil {
		return ac.pricesDBError(c, "Database error", err)
	}

	return utils.JsonPaginatedSuccessResponse(c, "Prices retrieved", prices, utils.GeneratePaginationData(total, limit, page))
}

// GetPriceQuote returns the price for ?specialization and ?duration_months
// (public): the admin-set price, or 200000 x months when none is set.
func (ac *ApplicantController) GetPriceQuote(c *fiber.Ctx) error {
	specialization, months := c.Query("specialization"), c.QueryInt("duration_months", 0)
	if months < 1 || !validSpecialization(specialization) {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "specialization and duration_months (>= 1) are required")
	}

	price, currency, err := utils.LookupPrice(context.Background(), ac.DB, specialization, months)
	if err != nil {
		return ac.pricesDBError(c, "Database error", err)
	}

	var exists bool
	if err := ac.DB.QueryRow(context.Background(),
		`SELECT EXISTS(SELECT id FROM prices WHERE specialization = $1 AND duration_months = $2)`,
		specialization, months).Scan(&exists); err != nil {
		return ac.pricesDBError(c, "Database error", err)
	}

	return utils.JsonSuccessResponse(c, "Price retrieved", model.PriceQuote{
		Specialization: specialization, DurationMonths: months, Price: price, Currency: currency, IsDefault: !exists,
	})
}

// SavePrice sets (or replaces) the price of a course for a period (admin only).
func (ac *ApplicantController) SavePrice(c *fiber.Ctx) error {
	var req model.Price
	if err := c.BodyParser(&req); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid request body")
	}
	if err := model.Validate.Struct(&req); err != nil {
		return utils.JsonValidationErrorResponse(c, err)
	}
	if !slices.Contains(utils.Specializations[req.Department], req.Specialization) {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "specialization does not belong to the department")
	}
	if req.Currency == "" {
		req.Currency = utils.DefaultCurrency
	}

	userID := utils.CurrentUserID(c)
	if err := ac.DB.QueryRow(context.Background(), `
		INSERT INTO prices (department, specialization, duration_months, price, currency, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (specialization, duration_months)
		DO UPDATE SET department = $1, price = $4, currency = $5, updated_by = $6, updated_at = NOW()
		RETURNING id
	`, req.Department, req.Specialization, req.DurationMonths, *req.Price, req.Currency, userID).Scan(&req.ID); err != nil {
		return ac.pricesDBError(c, "Failed to save price", err)
	}

	utils.RecordSuccessActivityLog(c, ac.DB, userID, "PRICE_SAVE",
		fmt.Sprintf("Set price (price_id=%d, user_id=%d) for %s, %d month(s): %.2f %s",
			req.ID, userID, req.Specialization, req.DurationMonths, *req.Price, req.Currency), nil)

	return utils.JsonSuccessResponse(c, "Price saved", req)
}

// DeletePrice removes an admin-set price so that course and period fall back
// to the default (admin only).
func (ac *ApplicantController) DeletePrice(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil || id < 1 {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid price id")
	}

	var removed int
	err = ac.DB.QueryRow(context.Background(), `DELETE FROM prices WHERE id = $1 RETURNING id`, id).Scan(&removed)
	if err == pgx.ErrNoRows {
		return utils.JsonErrorResponse(c, fiber.StatusNotFound, "Price not found")
	}
	if err != nil {
		return ac.pricesDBError(c, "Failed to delete price", err)
	}

	userID := utils.CurrentUserID(c)
	utils.RecordSuccessActivityLog(c, ac.DB, userID, "PRICE_DELETE",
		fmt.Sprintf("Deleted price (price_id=%d, user_id=%d)", removed, userID), nil)

	return utils.JsonSuccessResponse(c, "Price deleted", nil)
}

// validSpecialization reports whether s belongs to any department.
func validSpecialization(s string) bool {
	for _, list := range utils.Specializations {
		if slices.Contains(list, s) {
			return true
		}
	}
	return false
}

// pricesDBError answers a failed price query with a 500 and logs the cause.
func (ac *ApplicantController) pricesDBError(c *fiber.Ctx, message string, err error) error {
	return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, message, utils.Logger{
		LogLevel:    utils.CRITICAL,
		Message:     err.Error(),
		ServiceName: ac.Config.ServiceName,
	})
}
