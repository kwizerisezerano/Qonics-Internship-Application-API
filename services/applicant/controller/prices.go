package controller

import (
	"context"
	"errors"
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/qonics/qonics-appy-api/libs/shared-package/utils"
	"github.com/qonics/qonics-appy-api/services/applicant/model"
)

const (
	defaultPricePerMonth = 200000.0
	defaultCurrency      = "RWF"
)

// loadPrices reads the single prices row; a missing row means the defaults.
func (ac *ApplicantController) loadPrices(ctx context.Context) (price float64, currency string, err error) {
	err = ac.DB.QueryRow(ctx, `SELECT price_per_month, currency FROM prices WHERE id = 1`).Scan(&price, &currency)
	if errors.Is(err, pgx.ErrNoRows) {
		return defaultPricePerMonth, defaultCurrency, nil
	}
	return price, currency, err
}

// GetPrices returns the internship price per month (no login required) so the
// application form can show and send the amount.
func (ac *ApplicantController) GetPrices(c *fiber.Ctx) error {
	price, currency, err := ac.loadPrices(context.Background())
	if err != nil {
		return ac.pricesDBError(c, "Database error", err)
	}
	return utils.JsonSuccessResponse(c, "Prices retrieved", model.Prices{PricePerMonth: &price, Currency: currency})
}

// SavePrices sets the price per month (admin only).
func (ac *ApplicantController) SavePrices(c *fiber.Ctx) error {
	var req model.Prices
	if err := c.BodyParser(&req); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid request body")
	}
	if err := model.Validate.Struct(&req); err != nil {
		return utils.JsonValidationErrorResponse(c, err)
	}

	ctx := context.Background()
	_, currentCurrency, err := ac.loadPrices(ctx)
	if err != nil {
		return ac.pricesDBError(c, "Database error", err)
	}
	if req.Currency == "" {
		req.Currency = currentCurrency
	}

	if _, err := ac.DB.Exec(ctx, `
		INSERT INTO prices (id, price_per_month, currency, updated_by) VALUES (1, $1, $2, $3)
		ON CONFLICT (id) DO UPDATE SET price_per_month = $1, currency = $2, updated_by = $3, updated_at = NOW()
	`, *req.PricePerMonth, req.Currency, utils.CurrentUserID(c)); err != nil {
		return ac.pricesDBError(c, "Failed to update prices", err)
	}

	utils.RecordSuccessActivityLog(c, ac.DB, utils.CurrentUserID(c), "PRICES_UPDATE",
		fmt.Sprintf("Set internship price (price_id=1, user_id=%d) to %.2f %s per month", utils.CurrentUserID(c), *req.PricePerMonth, req.Currency), nil)

	return utils.JsonSuccessResponse(c, "Prices updated", req)
}

// pricesDBError answers a failed price query with a 500 and logs the cause.
func (ac *ApplicantController) pricesDBError(c *fiber.Ctx, message string, err error) error {
	return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, message, utils.Logger{
		LogLevel:    utils.CRITICAL,
		Message:     err.Error(),
		ServiceName: ac.Config.ServiceName,
	})
}
