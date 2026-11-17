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
