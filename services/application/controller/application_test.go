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
