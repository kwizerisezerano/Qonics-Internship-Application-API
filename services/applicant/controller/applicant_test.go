package controller

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qonics/qonics-appy-api/libs/shared-package/config"
	"github.com/qonics/qonics-appy-api/libs/shared-package/utils"
	"github.com/qonics/qonics-appy-api/services/applicant/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var applicantTestDB *pgxpool.Pool
var applicantApp *fiber.App
var applicantController *ApplicantController

func init() {
	utils.IsTestMode = true

	// Load .env from project root
	err := os.Chdir("../../..")
	if err != nil {
		panic("Failed to change directory: " + err.Error())
	}

	cfg, err := config.LoadConfig("applicant-service")
	if err != nil {
		panic("Failed to load config: " + err.Error())
	}

	applicantTestDB, err = cfg.InitDB()
	if err != nil {
		panic("Failed to connect to test database: " + err.Error())
	}

	applicantApp = fiber.New(fiber.Config{
		AppName: "applicant-service-test",
	})

	applicantController = NewApplicantController(applicantTestDB, cfg)

	applicantApp.Get("/api/v1/applicant/profile", applicantController.GetProfile)
	applicantApp.Put("/api/v1/applicant/profile", applicantController.SaveProfile)
}

func TestGetProfile(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/applicant/profile", nil)
	req.Header.Set("Authorization", "Bearer test-token")

	resp, err := applicantApp.Test(req)
	require.NoError(t, err)

	// Should return 401 for unauthorized (no user_id in context)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestSaveProfile(t *testing.T) {
	// This test verifies the endpoint returns 401 without auth
	app := fiber.New(fiber.Config{
		AppName: "applicant-service-test",
	})
	controller := NewApplicantController(applicantTestDB, nil)
	app.Put("/api/v1/applicant/profile", controller.SaveProfile)

	payloadBytes, _ := json.Marshal(fiber.Map{"country": "Rwanda"})
	req := httptest.NewRequest("PUT", "/api/v1/applicant/profile", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")

	resp, err := app.Test(req)
	require.NoError(t, err)

	// Should return 401 without proper auth middleware
	assert.Equal(t, 401, resp.StatusCode)
}

func TestHealthCheck(t *testing.T) {
	applicantApp.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/health", nil)
	resp, err := applicantApp.Test(req)
	require.NoError(t, err)

	assert.Equal(t, 200, resp.StatusCode)
}

// TestApplicantFilterQueryParsing verifies the query contract of GET /applicants:
// structured filters become ILIKE conditions and the search group matches the
// plain columns (ILIKE) plus the encrypted-PII hash columns.
func TestApplicantFilterQueryParsing(t *testing.T) {
	var (
		gotWhere  string
		gotParams []any
		gotErr    error
	)

	app := fiber.New()
	app.Get("/parse", func(c *fiber.Ctx) error {
		filter := model.ApplicantFilter{}
		if err := c.QueryParser(&filter); err != nil {
			gotErr = err
			return c.SendStatus(fiber.StatusBadRequest)
		}
		where, _, params, err := utils.BuildWhereClause(&filter)
		if err != nil {
			gotErr = err
			return c.SendStatus(fiber.StatusBadRequest)
		}
		where, params = utils.AppendSearchClause(where, params,
			[]string{"ap.institution", "ap.field_of_study"},
			[]string{"ap.full_name_hash", "ap.email_hash"},
			filter.Search,
		)
		gotWhere, gotParams = where, params
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/parse?institution=University&field_of_study=CS&search=eng", nil)
	_, err := app.Test(req)
	require.NoError(t, err)
	require.NoError(t, gotErr)

	assert.Contains(t, gotWhere, "lower(ap.institution) ILIKE lower($1)")
	assert.Contains(t, gotWhere, "lower(ap.field_of_study) ILIKE lower($2)")
	assert.Contains(t, gotWhere, "lower(ap.institution) LIKE lower($3)")
	assert.Contains(t, gotWhere, "ap.full_name_hash = encode(digest($4, 'sha256'), 'hex')")
	assert.Contains(t, gotWhere, "ap.email_hash = encode(digest($4, 'sha256'), 'hex')")

	require.Len(t, gotParams, 4)
	assert.Equal(t, "%University%", gotParams[0])
	assert.Equal(t, "%CS%", gotParams[1])
	assert.Equal(t, "%eng%", gotParams[2])
	assert.Equal(t, "eng", gotParams[3])
}

// TestNotificationFilterQueryParsing verifies the read filter and the caller
// scope used by GET /applicant/notifications.
func TestNotificationFilterQueryParsing(t *testing.T) {
	var (
		gotWhere  string
		gotParams []any
		gotErr    error
	)

	app := fiber.New()
	app.Get("/parse", func(c *fiber.Ctx) error {
		filter := model.NotificationFilter{}
		if err := c.QueryParser(&filter); err != nil {
			gotErr = err
			return c.SendStatus(fiber.StatusBadRequest)
		}
		filter.UserID = 42
		where, _, params, err := utils.BuildWhereClause(&filter)
		if err != nil {
			gotErr = err
			return c.SendStatus(fiber.StatusBadRequest)
		}
		gotWhere, gotParams = where, params
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/parse?read=true", nil)
	_, err := app.Test(req)
	require.NoError(t, err)
	require.NoError(t, gotErr)

	assert.Contains(t, gotWhere, "n.read = $1")
	assert.Contains(t, gotWhere, "n.user_id = $2")

	require.Len(t, gotParams, 2)
	assert.Equal(t, true, gotParams[0])
	assert.Equal(t, float64(42), gotParams[1])
}

// TestListNotificationsUnauthorized ensures the notifications endpoint rejects
// unauthenticated callers.
func TestListNotificationsUnauthorized(t *testing.T) {
	app := fiber.New()
	app.Get("/api/v1/applicant/notifications", applicantController.ListNotifications)

	req := httptest.NewRequest("GET", "/api/v1/applicant/notifications", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, 401, resp.StatusCode)
}
