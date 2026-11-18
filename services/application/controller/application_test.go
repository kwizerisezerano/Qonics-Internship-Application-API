package controller

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qonics/qonics-appy-api/libs/shared-package/config"
	"github.com/qonics/qonics-appy-api/libs/shared-package/utils"
	"github.com/qonics/qonics-appy-api/services/application/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var applicationTestDB *pgxpool.Pool
var applicationApp *fiber.App
var applicationController *ApplicationController

func init() {
	utils.IsTestMode = true

	// Load .env from project root
	err := os.Chdir("../../..")
	if err != nil {
		panic("Failed to change directory: " + err.Error())
	}

	cfg, err := config.LoadConfig("application-service")
	if err != nil {
		panic("Failed to load config: " + err.Error())
	}

	applicationTestDB, err = cfg.InitDB()
	if err != nil {
		panic("Failed to connect to test database: " + err.Error())
	}

	applicationApp = fiber.New(fiber.Config{
		AppName: "application-service-test",
	})

	applicationController = NewApplicationController(applicationTestDB, cfg, nil)

	applicationApp.Get("/api/v1/applications/mine", applicationController.ListMine)
	applicationApp.Post("/api/v1/applications", applicationController.Create)
}

func TestListMine(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/applications/mine", nil)
	req.Header.Set("Authorization", "Bearer test-token")

	resp, err := applicationApp.Test(req)
	require.NoError(t, err)

	// Should return 401 for unauthorized (no user_id in context)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestCreateApplication(t *testing.T) {
	reqBody := model.NewApplicationPayload{
		Department:     "Engineering",
		Specialization: "Software Development",
		Motivation:     "I want to learn",
		Experience:     "2 years",
		StartDate:      "2025-01-15T00:00:00Z",
		EducationLevel: "bachelor",
		Session:        "january-june",
		DurationMonths: 6,
		WorkMode:       "remote",
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/v1/applications", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")

	resp, err := applicationApp.Test(req)
	require.NoError(t, err)

	// Should return 401 for unauthorized (no user_id in context)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestCreateApplicationInvalidData(t *testing.T) {
	// Test with invalid education level
	reqBody := model.NewApplicationPayload{
		Department:     "Engineering",
		Specialization: "Software Development",
		Motivation:     "I want to learn",
		Experience:     "2 years",
		StartDate:      "2025-01-15T00:00:00Z",
		EducationLevel: "invalid_level",
		Session:        "january-june",
		DurationMonths: 6,
		WorkMode:       "remote",
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/v1/applications", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")

	resp, err := applicationApp.Test(req)
	require.NoError(t, err)

	// Should return 401 without auth, but if it passes auth it would return 400 for validation
	assert.Equal(t, 401, resp.StatusCode)
}

func TestCreateApplicationMissingFields(t *testing.T) {
	// Test with missing required fields
	reqBody := map[string]interface{}{
		"department": "Engineering",
		// Missing required fields
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/v1/applications", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")

	resp, err := applicationApp.Test(req)
	require.NoError(t, err)

	// Should return 401 without auth
	assert.Equal(t, 401, resp.StatusCode)
}

func TestHealthCheck(t *testing.T) {
	applicationApp.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/health", nil)
	resp, err := applicationApp.Test(req)
	require.NoError(t, err)

	assert.Equal(t, 200, resp.StatusCode)
}

// TestParseFilterBuildsWhereClause exercises the shared filter pipeline used by
// ListMine/ListAll: structured filters via BuildWhereClause, the caller scope
// and the cross-column search (plain ILIKE columns + encrypted-PII hash columns).
func TestParseFilterBuildsWhereClause(t *testing.T) {
	var (
		gotFilter model.ApplicationFilter
		gotWhere  string
		gotParams []any
		gotErr    error
	)

	app := fiber.New()
	app.Get("/parse", func(c *fiber.Ctx) error {
		gotFilter, gotWhere, gotParams, gotErr = applicationController.parseFilter(c, "profile-1")
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET",
		"/parse?page=2&limit=5&search=eng&status=accepted&department=Engineering&submitted_from=2026-01-01",
		nil)
	_, err := app.Test(req)
	require.NoError(t, err)
	require.NoError(t, gotErr)

	// Pagination is carried on the filter.
	assert.Equal(t, 2, gotFilter.Page)
	assert.Equal(t, 5, gotFilter.Limit)

	// Structured filters, in declaration order.
	assert.Contains(t, gotWhere, "a.department = $1")
	assert.Contains(t, gotWhere, "a.status = $2")
	assert.Contains(t, gotWhere, "a.submitted_at >= $3")

	// Caller scope appended after the structured filters.
	assert.Contains(t, gotWhere, "a.applicant_id = $4")

	// Cross-column search group.
	assert.Contains(t, gotWhere, "lower(a.reference) LIKE lower($5)")
	assert.Contains(t, gotWhere, "lower(a.department) LIKE lower($5)")
	assert.Contains(t, gotWhere, "lower(a.specialization) LIKE lower($5)")
	assert.Contains(t, gotWhere, "ap.full_name_hash = encode(digest($6, 'sha256'), 'hex')")
	assert.Contains(t, gotWhere, "ap.email_hash = encode(digest($6, 'sha256'), 'hex')")

	// Params line up with the placeholders above.
	require.Len(t, gotParams, 6)
	assert.Equal(t, "Engineering", gotParams[0])
	assert.Equal(t, "accepted", gotParams[1])
	assert.IsType(t, time.Time{}, gotParams[2])
	assert.Equal(t, "profile-1", gotParams[3])
	assert.Equal(t, "%eng%", gotParams[4])
	assert.Equal(t, "eng", gotParams[5])
}

// TestParseFilterDefaults verifies the page/limit clamping and that an empty
// query yields no WHERE clause.
func TestParseFilterDefaults(t *testing.T) {
	var (
		gotFilter model.ApplicationFilter
		gotWhere  string
		gotParams []any
		gotErr    error
	)

	app := fiber.New()
	app.Get("/parse", func(c *fiber.Ctx) error {
		gotFilter, gotWhere, gotParams, gotErr = applicationController.parseFilter(c, "")
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/parse?page=0&limit=999", nil)
	_, err := app.Test(req)
	require.NoError(t, err)
	require.NoError(t, gotErr)

	assert.Equal(t, 1, gotFilter.Page)     // clamped up to the minimum
	assert.Equal(t, 100, gotFilter.Limit)  // clamped down to the maximum
	assert.Empty(t, gotWhere)
	assert.Empty(t, gotParams)
}

// TestParseFilterInvalidDate rejects an unparseable submission date.
func TestParseFilterInvalidDate(t *testing.T) {
	var gotErr error

	app := fiber.New()
	app.Get("/parse", func(c *fiber.Ctx) error {
		_, _, _, gotErr = applicationController.parseFilter(c, "")
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/parse?submitted_from=not-a-date", nil)
	_, err := app.Test(req)
	require.NoError(t, err)
	assert.Error(t, gotErr)
}
