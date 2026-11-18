package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qonics/qonics-appy-api/libs/shared-package/config"
	"github.com/qonics/qonics-appy-api/libs/shared-package/middleware"
	"github.com/qonics/qonics-appy-api/libs/shared-package/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Shared harness, mirroring leazi's controller tests: one app + one DB set up
// once, then every route is exercised by a single table-driven test function
// carrying several scenario payloads.
var (
	authTestApp *fiber.App
	authTestDB  *pgxpool.Pool
	authTestCfg *config.Config
)

// testPassword satisfies the strong_password rule enforced on registration.
const testPassword = "Str0ng@Pass"

// testJWTSecret is used to sign the tokens consumed by the real auth
// middleware, so authenticated and unauthenticated requests flow through the
// same production code path.
const testJWTSecret = "auth-test-secret"

// testIdentityEmail is attached to the identity carried by the test tokens.
const testIdentityEmail = "tester@example.com"

func init() {
	utils.IsTestMode = true

	if err := os.Chdir("../../.."); err != nil {
		panic("Failed to change directory: " + err.Error())
	}

	cfg, err := config.LoadConfig("auth-service")
	if err != nil {
		panic("Failed to load config: " + err.Error())
	}
	authTestCfg = cfg

	db, err := cfg.InitDB()
	if err != nil {
		panic("Failed to connect to test database: " + err.Error())
	}
	if _, err := db.Exec(context.Background(), "SET app.encryption_key = 'secret'"); err != nil {
		panic("Failed to set encryption key: " + err.Error())
	}
	authTestDB = db

	controller := NewAuthController(db, cfg, cfg.InitRedis())

	authTestApp = fiber.New(fiber.Config{AppName: "auth-service-test"})

	// Mirror routes/api.go: the public routes stay open while the authenticated
	// group (change-password) runs through the real middleware.
	api := authTestApp.Group("/api/v1/auth")
	api.Post("/register", controller.Register)
	api.Post("/login", controller.Login)
	api.Post("/logout", controller.Logout)
	api.Post("/forgot-password", controller.ForgotPassword)
	api.Post("/reset-password", controller.ResetPassword)

	api.Use(middleware.AuthMiddleware(testJWTSecret))
	api.Post("/change-password", controller.ChangePassword)
}

// authHeaders builds an Authorization header carrying a signed token for the
// given user id and role (the test equivalent of leazi's createTestAccessToken).
func authHeaders(userID int, role string) map[string]string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": float64(userID),
		"email":   testIdentityEmail,
		"role":    role,
	})
	signed, err := token.SignedString([]byte(testJWTSecret))
	if err != nil {
		panic("Failed to sign test token: " + err.Error())
	}
	return map[string]string{"Authorization": "Bearer " + signed}
}

// doJSON issues a request against the shared test app and returns the status
// code together with the decoded JSON body.
func doJSON(t *testing.T, method, path string, payload any, headers map[string]string) (int, map[string]any) {
	t.Helper()

	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		require.NoError(t, err)
		body = bytes.NewReader(raw)
	}

	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := authTestApp.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	var parsed map[string]any
	_ = json.Unmarshal(raw, &parsed)
	return resp.StatusCode, parsed
}

// uniqueSuffix returns a value that is unique across repeated test runs.
func uniqueSuffix() string {
	return fmt.Sprintf("%d%d", time.Now().UnixNano(), rand.Intn(100000))
}

// uniqueEmail builds a unique, valid email address.
func uniqueEmail(prefix string) string {
	return fmt.Sprintf("%s%s@example.com", prefix, uniqueSuffix())
}

// uniqueName returns a letters-only name (the `name` rule rejects digits).
func uniqueName(prefix string) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, 10)
	for i := range b {
		b[i] = alphabet[rand.Intn(len(alphabet))]
	}
	return prefix + " " + string(b)
}

// registerUser registers a fresh user and returns its email and id.
func registerUser(t *testing.T, prefix string) (email string, userID int) {
	t.Helper()

	email = uniqueEmail(prefix)
	status, body := doJSON(t, "POST", "/api/v1/auth/register", map[string]any{
		"full_name": uniqueName("Test User"),
		"email":     email,
		"phone":     "+250788123456",
		"password":  testPassword,
	}, nil)
	require.Equal(t, 200, status, "register failed: %v", body)

	data, ok := body["data"].(map[string]any)
	require.True(t, ok, "response should have data: %v", body)
	user, ok := data["user"].(map[string]any)
	require.True(t, ok, "response should have user: %v", data)

	return email, int(user["id"].(float64))
}

// hasValidationError reports whether the response carries at least one field
// level error, optionally matching a specific snake_case key.
func hasValidationError(body map[string]any, key string) bool {
	errs, ok := body["errors"].([]any)
	if !ok {
		return false
	}
	for _, e := range errs {
		entry, ok := e.(map[string]any)
		if !ok {
			continue
		}
		if key == "" || entry["key"] == key {
			return true
		}
	}
	return false
}

// TestRegisterRoute covers POST /api/v1/auth/register with multiple payloads.
func TestRegisterRoute(t *testing.T) {
	// Seed one user so the duplicate scenario has something to collide with.
	dupEmail, _ := registerUser(t, "duplicate")

	tests := []struct {
		name       string
		payload    map[string]any
		wantStatus int
		check      func(t *testing.T, body map[string]any)
	}{
		{
			name: "success - valid registration",
			payload: map[string]any{
				"full_name": uniqueName("John Doe"),
				"email":     uniqueEmail("register"),
				"phone":     "+250788123456",
				"password":  testPassword,
			},
			wantStatus: 200,
			check: func(t *testing.T, body map[string]any) {
				data, ok := body["data"].(map[string]any)
				require.True(t, ok, "response should have data")
				assert.NotEmpty(t, data["token"])
			},
		},
		{
			name: "conflict - duplicate email",
			payload: map[string]any{
				"full_name": uniqueName("Dupe User"),
				"email":     dupEmail,
				"phone":     "+250788123456",
				"password":  testPassword,
			},
			wantStatus: 409,
		},
		{
			name: "validation - invalid email",
			payload: map[string]any{
				"full_name": uniqueName("Jane Doe"),
				"email":     "not-an-email",
				"phone":     "+250788123456",
				"password":  testPassword,
			},
			wantStatus: 400,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "email"))
			},
		},
		{
			name: "validation - weak password",
			payload: map[string]any{
				"full_name": uniqueName("Weak Pass"),
				"email":     uniqueEmail("weakpass"),
				"phone":     "+250788123456",
				"password":  "password123",
			},
			wantStatus: 400,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "password"))
			},
		},
		{
			name: "validation - name contains digits",
			payload: map[string]any{
				"full_name": "John Doe 123",
				"email":     uniqueEmail("digitname"),
				"phone":     "+250788123456",
				"password":  testPassword,
			},
			wantStatus: 400,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "full_name"))
			},
		},
		{
			name: "validation - invalid phone",
			payload: map[string]any{
				"full_name": uniqueName("Bad Phone"),
				"email":     uniqueEmail("badphone"),
				"phone":     "not-a-phone",
				"password":  testPassword,
			},
			wantStatus: 400,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "phone"))
			},
		},
		{
			name: "validation - missing full_name",
			payload: map[string]any{
				"email":    uniqueEmail("nofullname"),
				"phone":    "+250788123456",
				"password": testPassword,
			},
			wantStatus: 400,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "full_name"))
			},
		},
		{
			name: "validation - missing phone",
			payload: map[string]any{
				"full_name": uniqueName("No Phone"),
				"email":     uniqueEmail("nophone"),
				"password":  testPassword,
			},
			wantStatus: 400,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "phone"))
			},
		},
		{
			name: "validation - missing password",
			payload: map[string]any{
				"full_name": uniqueName("No Password"),
				"email":     uniqueEmail("nopassword"),
				"phone":     "+250788123456",
			},
			wantStatus: 400,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "password"))
			},
		},
		{
			name:       "validation - empty payload",
			payload:    map[string]any{},
			wantStatus: 400,
			check: func(t *testing.T, body map[string]any) {
				msg, _ := body["message"].(string)
				assert.Contains(t, msg, "full_name is required")
				assert.Contains(t, msg, "password is required")
				assert.True(t, hasValidationError(body, ""))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := doJSON(t, "POST", "/api/v1/auth/register", tt.payload, nil)
			require.Equal(t, tt.wantStatus, status, "body: %v", body)
			assert.Equal(t, float64(tt.wantStatus), body["status"])
			if tt.check != nil {
				tt.check(t, body)
			}
		})
	}
}

// TestLoginRoute covers POST /api/v1/auth/login with multiple payloads.
func TestLoginRoute(t *testing.T) {
	email, _ := registerUser(t, "login")

	tests := []struct {
		name       string
		payload    map[string]any
		wantStatus int
		check      func(t *testing.T, body map[string]any)
	}{
		{
			name:       "success - valid credentials",
			payload:    map[string]any{"email": email, "password": testPassword},
			wantStatus: 200,
			check: func(t *testing.T, body map[string]any) {
				data, ok := body["data"].(map[string]any)
				require.True(t, ok, "response should have data")
				assert.NotEmpty(t, data["token"])
			},
		},
		{
			name:       "unauthorized - wrong password",
			payload:    map[string]any{"email": email, "password": "Wrong@Pass1"},
			wantStatus: 401,
		},
		{
			name:       "unauthorized - unknown email",
			payload:    map[string]any{"email": uniqueEmail("missing"), "password": testPassword},
			wantStatus: 401,
		},
		{
			name:       "validation - invalid email format",
			payload:    map[string]any{"email": "invalid-email", "password": testPassword},
			wantStatus: 400,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "email"))
			},
		},
		{
			name:       "validation - missing password",
			payload:    map[string]any{"email": email},
			wantStatus: 400,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "password"))
			},
		},
		{
			name:       "validation - empty payload",
			payload:    map[string]any{},
			wantStatus: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := doJSON(t, "POST", "/api/v1/auth/login", tt.payload, nil)
			require.Equal(t, tt.wantStatus, status, "body: %v", body)
			assert.Equal(t, float64(tt.wantStatus), body["status"])
			if tt.check != nil {
				tt.check(t, body)
			}
		})
	}
}

// TestLogoutRoute covers POST /api/v1/auth/logout. The route is public so it
// always succeeds and simply clears the auth cookies.
func TestLogoutRoute(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string]string
		wantStatus int
	}{
		{name: "success - no token", headers: nil, wantStatus: 200},
		{name: "success - with token", headers: authHeaders(1, "applicant"), wantStatus: 200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := doJSON(t, "POST", "/api/v1/auth/logout", nil, tt.headers)
			require.Equal(t, tt.wantStatus, status, "body: %v", body)
			assert.Equal(t, float64(tt.wantStatus), body["status"])
			assert.Equal(t, "Logout successful", body["message"])
		})
	}
}

// TestForgotPasswordRoute covers POST /api/v1/auth/forgot-password.
func TestForgotPasswordRoute(t *testing.T) {
	tests := []struct {
		name       string
		payload    map[string]any
		wantStatus int
	}{
		{
			name:       "success - known email",
			payload:    map[string]any{"email": uniqueEmail("forgot")},
			wantStatus: 200,
		},
		{
			name:       "validation - invalid email",
			payload:    map[string]any{"email": "invalid-email"},
			wantStatus: 400,
		},
		{
			name:       "validation - empty payload",
			payload:    map[string]any{},
			wantStatus: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := doJSON(t, "POST", "/api/v1/auth/forgot-password", tt.payload, nil)
			require.Equal(t, tt.wantStatus, status, "body: %v", body)
			assert.Equal(t, float64(tt.wantStatus), body["status"])
		})
	}
}

// TestResetPasswordRoute covers POST /api/v1/auth/reset-password.
func TestResetPasswordRoute(t *testing.T) {
	tests := []struct {
		name       string
		payload    map[string]any
		wantStatus int
		check      func(t *testing.T, body map[string]any)
	}{
		{
			name:       "success - strong password and token",
			payload:    map[string]any{"token": "reset-token", "password": "N3w@Passw0rd"},
			wantStatus: 200,
		},
		{
			name:       "validation - weak password",
			payload:    map[string]any{"token": "reset-token", "password": "weak"},
			wantStatus: 400,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "password"))
			},
		},
		{
			name:       "validation - missing token",
			payload:    map[string]any{"password": "N3w@Passw0rd"},
			wantStatus: 400,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "token"))
			},
		},
		{
			name:       "validation - missing password",
			payload:    map[string]any{"token": "reset-token"},
			wantStatus: 400,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "password"))
			},
		},
		{
			name:       "validation - empty payload",
			payload:    map[string]any{},
			wantStatus: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := doJSON(t, "POST", "/api/v1/auth/reset-password", tt.payload, nil)
			require.Equal(t, tt.wantStatus, status, "body: %v", body)
			assert.Equal(t, float64(tt.wantStatus), body["status"])
			if tt.check != nil {
				tt.check(t, body)
			}
		})
	}
}

// TestChangePasswordRoute covers POST /api/v1/auth/change-password. Every
// scenario that operates on a real user registers its own so the sub-tests stay
// order independent even though a successful change mutates the stored password.
func TestChangePasswordRoute(t *testing.T) {
	tests := []struct {
		name       string
		userID     int // 0 = no token, -1 = register a fresh user, >0 = literal id
		current    string
		next       string
		wantStatus int
		check      func(t *testing.T, body map[string]any)
	}{
		{name: "unauthorized - no token", userID: 0, current: testPassword, next: "N3w@Passw0rd", wantStatus: 401},
		{name: "not found - unknown user", userID: 999999999, current: testPassword, next: "N3w@Passw0rd", wantStatus: 404},
		{
			name: "validation - missing new password", userID: -1, current: testPassword, next: "",
			wantStatus: 400,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "new_password"), "expected new_password error: %v", body)
			},
		},
		{
			name: "validation - weak new password", userID: -1, current: testPassword, next: "weak",
			wantStatus: 400,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "new_password"), "expected new_password error: %v", body)
			},
		},
		{name: "bad request - wrong current password", userID: -1, current: "Wr0ng@Pass", next: "N3w@Passw0rd", wantStatus: 400},
		{name: "not acceptable - same as current", userID: -1, current: testPassword, next: testPassword, wantStatus: 406},
		{name: "success - strong new password", userID: -1, current: testPassword, next: "N3w@Passw0rd", wantStatus: 200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var headers map[string]string
			switch {
			case tt.userID == 0:
				headers = nil
			case tt.userID < 0:
				_, userID := registerUser(t, "changepass")
				headers = authHeaders(userID, "applicant")
			default:
				headers = authHeaders(tt.userID, "applicant")
			}

			status, body := doJSON(t, "POST", "/api/v1/auth/change-password", map[string]any{
				"currentPassword": tt.current,
				"newPassword":     tt.next,
			}, headers)

			require.Equal(t, tt.wantStatus, status, "body: %v", body)
			assert.Equal(t, float64(tt.wantStatus), body["status"])
			if tt.check != nil {
				tt.check(t, body)
			}
		})
	}
}

// TestUnauthorizedAccess sweeps the authenticated route surface and asserts
// every endpoint rejects requests without a token, mirroring leazi's single
// unauthorized table.
func TestUnauthorizedAccess(t *testing.T) {
	endpoints := []struct {
		method string
		path   string
	}{
		{"POST", "/api/v1/auth/change-password"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			status, body := doJSON(t, ep.method, ep.path, map[string]any{}, nil)
			require.Equal(t, 401, status, "body: %v", body)
			assert.Equal(t, float64(401), body["status"])
		})
	}
}
