package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qonics/qonics-appy-api/libs/shared-package/config"
	"github.com/qonics/qonics-appy-api/libs/shared-package/utils"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testDB *pgxpool.Pool
)

// testPassword satisfies the strong_password rule enforced on registration.
const testPassword = "Str0ng@Pass"

// uniqueFullName returns a letters-only name (the `name` rule rejects digits)
// with a random suffix so the UNIQUE full_name_hash constraint is never hit
// across repeated test runs.
func uniqueFullName(prefix string) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, 12)
	for i := range b {
		b[i] = alphabet[rand.Intn(len(alphabet))]
	}
	return prefix + " " + string(b)
}

func init() {
	utils.IsTestMode = true
}

func setupTestDB(t *testing.T) (*pgxpool.Pool, *config.Config, *redis.Client) {
	// Load .env from project root
	err := os.Chdir("../../..")
	require.NoError(t, err)

	cfg, err := config.LoadConfig("auth-service")
	require.NoError(t, err)

	db, err := cfg.InitDB()
	require.NoError(t, err)

	// Set encryption key for the session
	_, err = db.Exec(context.Background(), "SET app.encryption_key = 'secret'")
	require.NoError(t, err)

	// Initialize Redis
	rdb := cfg.InitRedis()

	return db, cfg, rdb
}

func teardownTestDB(db *pgxpool.Pool, rdb *redis.Client) {
	if rdb != nil {
		rdb.Close()
	}
	if db != nil {
		db.Close()
	}
}

func TestRegister(t *testing.T) {
	db, cfg, rdb := setupTestDB(t)
	defer teardownTestDB(db, rdb)

	// Generate unique email
	ctx := context.Background()
	timestamp := fmt.Sprintf("%d", time.Now().UnixNano())
	email := fmt.Sprintf("register%s@example.com", timestamp)
	fullName := uniqueFullName("Test User")

	app := fiber.New(fiber.Config{
		AppName: "auth-service-test",
	})

	controller := NewAuthController(db, cfg, rdb)
	app.Post("/api/v1/auth/register", controller.Register)

	registerPayload := map[string]interface{}{
		"full_name": fullName,
		"email":     email,
		"phone":     "+250788123456",
		"password":  testPassword,
	}

	payloadBytes, _ := json.Marshal(registerPayload)
	req := httptest.NewRequest("POST", "/api/v1/auth/register", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)

	var response map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&response)

	if resp.StatusCode != 200 {
		t.Logf("Response: %+v", response)
	}

	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, float64(200), response["status"])

	// Clean up after test
	db.Exec(ctx, "DELETE FROM users WHERE email_hash = pgp_sym_decrypt(encrypt_data($1), current_setting('app.encryption_key', true))", email)
}

func TestRegisterDuplicateEmail(t *testing.T) {
	db, cfg, rdb := setupTestDB(t)
	defer teardownTestDB(db, rdb)

	ctx := context.Background()
	timestamp := fmt.Sprintf("%d", time.Now().UnixNano())
	email := fmt.Sprintf("duplicate%s@example.com", timestamp)
	fullName := uniqueFullName("Test User")

	app := fiber.New(fiber.Config{
		AppName: "auth-service-test",
	})

	controller := NewAuthController(db, cfg, rdb)
	app.Post("/api/v1/auth/register", controller.Register)

	registerPayload := map[string]interface{}{
		"full_name": fullName,
		"email":     email,
		"phone":     "+250788123456",
		"password":  testPassword,
	}

	// First registration
	payloadBytes, _ := json.Marshal(registerPayload)
	req := httptest.NewRequest("POST", "/api/v1/auth/register", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)

	// Duplicate registration
	resp2, err := app.Test(req)
	require.NoError(t, err)

	var response map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&response)

	// Should return 409 for duplicate
	assert.Equal(t, 409, resp2.StatusCode)

	// Clean up
	db.Exec(ctx, "DELETE FROM users WHERE email_hash = pgp_sym_decrypt(encrypt_data($1), current_setting('app.encryption_key', true))", email)
}

func TestRegisterInvalidEmail(t *testing.T) {
	db, cfg, rdb := setupTestDB(t)
	defer teardownTestDB(db, rdb)

	app := fiber.New(fiber.Config{
		AppName: "auth-service-test",
	})

	controller := NewAuthController(db, cfg, rdb)
	app.Post("/api/v1/auth/register", controller.Register)

	registerPayload := map[string]interface{}{
		"full_name": "Test User",
		"email":     "invalid-email",
		"phone":     "+250788123456",
		"password":  testPassword,
	}

	payloadBytes, _ := json.Marshal(registerPayload)
	req := httptest.NewRequest("POST", "/api/v1/auth/register", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)

	// Should return 400 for validation error
	assert.Equal(t, 400, resp.StatusCode)
}

func TestRegisterShortPassword(t *testing.T) {
	db, cfg, rdb := setupTestDB(t)
	defer teardownTestDB(db, rdb)

	app := fiber.New(fiber.Config{
		AppName: "auth-service-test",
	})

	controller := NewAuthController(db, cfg, rdb)
	app.Post("/api/v1/auth/register", controller.Register)

	registerPayload := map[string]interface{}{
		"full_name": "Test User",
		"email":     "test@example.com",
		"phone":     "+250788123456",
		"password":  "short",
	}

	payloadBytes, _ := json.Marshal(registerPayload)
	req := httptest.NewRequest("POST", "/api/v1/auth/register", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)

	// Should return 400 for validation error
	assert.Equal(t, 400, resp.StatusCode)
}

func TestLogin(t *testing.T) {
	db, cfg, rdb := setupTestDB(t)
	defer teardownTestDB(db, rdb)

	// Generate unique email
	ctx := context.Background()
	timestamp := fmt.Sprintf("%d", time.Now().UnixNano())
	email := fmt.Sprintf("login%s@example.com", timestamp)
	fullName := uniqueFullName("Login Test User")

	app := fiber.New(fiber.Config{
		AppName: "auth-service-test",
	})

	controller := NewAuthController(db, cfg, rdb)
	app.Post("/api/v1/auth/register", controller.Register)
	app.Post("/api/v1/auth/login", controller.Login)

	// First register a user
	registerPayload := map[string]interface{}{
		"full_name": fullName,
		"email":     email,
		"phone":     "+250788654321",
		"password":  testPassword,
	}

	payloadBytes, _ := json.Marshal(registerPayload)
	req := httptest.NewRequest("POST", "/api/v1/auth/register", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)

	// Now login
	loginPayload := map[string]interface{}{
		"email":    email,
		"password": testPassword,
	}

	payloadBytes, _ = json.Marshal(loginPayload)
	req = httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(req)
	require.NoError(t, err)

	var response map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&response)

	if resp.StatusCode != 200 {
		t.Logf("Response: %+v", response)
	}

	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, float64(200), response["status"])

	// Clean up after test
	db.Exec(ctx, "DELETE FROM users WHERE email_hash = pgp_sym_decrypt(encrypt_data($1), current_setting('app.encryption_key', true))", email)
}

func TestLoginInvalidCredentials(t *testing.T) {
	db, cfg, rdb := setupTestDB(t)
	defer teardownTestDB(db, rdb)

	app := fiber.New(fiber.Config{
		AppName: "auth-service-test",
	})

	controller := NewAuthController(db, cfg, rdb)
	app.Post("/api/v1/auth/login", controller.Login)

	loginPayload := map[string]interface{}{
		"email":    "nonexistent@example.com",
		"password": "wrongpassword",
	}

	payloadBytes, _ := json.Marshal(loginPayload)
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)

	// Should return 401 for invalid credentials
	assert.Equal(t, 401, resp.StatusCode)
}

func TestLoginWrongPassword(t *testing.T) {
	db, cfg, rdb := setupTestDB(t)
	defer teardownTestDB(db, rdb)

	ctx := context.Background()
	timestamp := fmt.Sprintf("%d", time.Now().UnixNano())
	email := fmt.Sprintf("wrongpass%s@example.com", timestamp)
	fullName := uniqueFullName("Test User")

	app := fiber.New(fiber.Config{
		AppName: "auth-service-test",
	})

	controller := NewAuthController(db, cfg, rdb)
	app.Post("/api/v1/auth/register", controller.Register)
	app.Post("/api/v1/auth/login", controller.Login)

	// Register a user
	registerPayload := map[string]interface{}{
		"full_name": fullName,
		"email":     email,
		"phone":     "+250788123456",
		"password":  testPassword,
	}

	payloadBytes, _ := json.Marshal(registerPayload)
	req := httptest.NewRequest("POST", "/api/v1/auth/register", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)

	// Try login with wrong password
	loginPayload := map[string]interface{}{
		"email":    email,
		"password": "wrongpassword",
	}

	payloadBytes, _ = json.Marshal(loginPayload)
	req = httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(req)
	require.NoError(t, err)

	// Should return 401 for wrong password
	assert.Equal(t, 401, resp.StatusCode)

	// Clean up
	db.Exec(ctx, "DELETE FROM users WHERE email_hash = pgp_sym_decrypt(encrypt_data($1), current_setting('app.encryption_key', true))", email)
}

func TestChangePasswordSamePassword(t *testing.T) {
	db, cfg, rdb := setupTestDB(t)
	defer teardownTestDB(db, rdb)

	ctx := context.Background()
	timestamp := fmt.Sprintf("%d", time.Now().UnixNano())
	email := fmt.Sprintf("samepass%s@example.com", timestamp)
	fullName := uniqueFullName("Test User")

	app := fiber.New(fiber.Config{
		AppName: "auth-service-test",
	})

	controller := NewAuthController(db, cfg, rdb)
	app.Post("/api/v1/auth/register", controller.Register)
	app.Post("/api/v1/auth/login", controller.Login)

	// Add change-password with a simple test middleware that sets user_id
	app.Post("/api/v1/auth/change-password", func(c *fiber.Ctx) error {
		// For testing, extract user_id from the registered user
		// We'll look up the user by email and set user_id in context
		var userID int
		err := db.QueryRow(context.Background(), "SELECT id FROM users WHERE email_hash = $1", utils.HashData(email)).Scan(&userID)
		if err != nil {
			return c.Status(401).JSON(fiber.Map{"status": 401, "message": "Unauthorized"})
		}
		c.Locals("user_id", userID)
		return controller.ChangePassword(c)
	})

	// Register a user
	registerPayload := map[string]interface{}{
		"full_name": fullName,
		"email":     email,
		"phone":     "+250788123456",
		"password":  testPassword,
	}

	payloadBytes, _ := json.Marshal(registerPayload)
	req := httptest.NewRequest("POST", "/api/v1/auth/register", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)

	// Login to get token
	loginPayload := map[string]interface{}{
		"email":    email,
		"password": testPassword,
	}

	payloadBytes, _ = json.Marshal(loginPayload)
	req = httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(req)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)

	var loginResponse map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&loginResponse)
	token := loginResponse["data"].(map[string]interface{})["token"].(string)

	// Try to change password to the same password
	changePasswordPayload := map[string]interface{}{
		"currentPassword": testPassword,
		"newPassword":     testPassword,
	}

	payloadBytes, _ = json.Marshal(changePasswordPayload)
	req = httptest.NewRequest("POST", "/api/v1/auth/change-password", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = app.Test(req)
	require.NoError(t, err)

	// Should return 406 (Not Acceptable) for same password
	assert.Equal(t, 406, resp.StatusCode)

	// Clean up
	db.Exec(ctx, "DELETE FROM users WHERE email_hash = pgp_sym_decrypt(encrypt_data($1), current_setting('app.encryption_key', true))", email)
}
