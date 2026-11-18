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
	"strings"
	"testing"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qonics/qonics-appy-api/libs/shared-package/config"
	"github.com/qonics/qonics-appy-api/libs/shared-package/middleware"
	"github.com/qonics/qonics-appy-api/libs/shared-package/utils"
	"github.com/qonics/qonics-appy-api/services/applicant/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Shared harness, mirroring leazi's controller tests: one app + one DB set up
// once, then every route is exercised by a single table-driven test function
// carrying several scenario payloads.
var (
	applicantTestApp *fiber.App
	applicantTestDB  *pgxpool.Pool
	applicantTestCfg *config.Config
)

// testJWTSecret is used to sign the tokens consumed by the real auth
// middleware, so authenticated and unauthenticated requests flow through the
// same production code path.
const testJWTSecret = "applicant-test-secret"

// testIdentityEmail is attached to the identity carried by the test tokens.
const testIdentityEmail = "tester@example.com"

func init() {
	utils.IsTestMode = true

	if err := os.Chdir("../../.."); err != nil {
		panic("Failed to change directory: " + err.Error())
	}

	cfg, err := config.LoadConfig("applicant-service")
	if err != nil {
		panic("Failed to load config: " + err.Error())
	}
	applicantTestCfg = cfg

	db, err := cfg.InitDB()
	if err != nil {
		panic("Failed to connect to test database: " + err.Error())
	}
	if _, err := db.Exec(context.Background(), "SET app.encryption_key = 'secret'"); err != nil {
		panic("Failed to set encryption key: " + err.Error())
	}
	applicantTestDB = db

	// Deterministic settings row so the settings route table has a stable
	// baseline to compare "no changes" against on every run.
	if _, err := db.Exec(context.Background(), `
		INSERT INTO settings (id, applications_open, close_on, closure_message)
		VALUES (1, TRUE, NULL, '')
		ON CONFLICT (id) DO UPDATE SET applications_open = TRUE, close_on = NULL, closure_message = ''
	`); err != nil {
		panic("Failed to reset settings: " + err.Error())
	}

	controller := NewApplicantController(db, cfg)

	applicantTestApp = fiber.New(fiber.Config{AppName: "applicant-service-test"})

	// Mirror routes/api.go: the real auth middleware means a missing/invalid
	// token yields 401 and the admin-only groups yield 403 for non-admins.
	auth := middleware.AuthMiddleware(testJWTSecret)

	applicant := applicantTestApp.Group("/api/v1/applicant", auth)
	applicant.Get("/profile", controller.GetProfile)
	applicant.Put("/profile", controller.SaveProfile)
	applicant.Get("/notifications", controller.ListNotifications)
	applicant.Post("/notifications/read", controller.MarkNotificationsRead)

	admin := applicantTestApp.Group("/api/v1/applicants", auth, middleware.AdminMiddleware())
	admin.Get("/", controller.ListApplicants)

	settings := applicantTestApp.Group("/api/v1/settings", auth, middleware.AdminMiddleware())
	settings.Get("/", controller.GetSettings)
	settings.Put("/", controller.SaveSettings)
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

	resp, err := applicantTestApp.Test(req)
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

// seededApplicant is a user together with its applicant_profiles row, created
// directly so profile-dependent routes can be exercised without the auth
// service.
type seededApplicant struct {
	UserID int
	Name   string
	Email  string
	Phone  string
}

func seedApplicant(t *testing.T) seededApplicant {
	t.Helper()
	ctx := context.Background()

	name := uniqueName("Applicant")
	email := uniqueEmail("applicant")
	phone := "+250788123456"

	fullNameEnc, fullNameHash, err := utils.EncryptAndHash(ctx, applicantTestDB, name)
	require.NoError(t, err)
	emailEnc, emailHash, err := utils.EncryptAndHash(ctx, applicantTestDB, email)
	require.NoError(t, err)
	phoneEnc, phoneHash, err := utils.EncryptAndHash(ctx, applicantTestDB, phone)
	require.NoError(t, err)

	var userID int
	err = applicantTestDB.QueryRow(ctx, `
		INSERT INTO users (full_name_encrypted, full_name_hash, email_encrypted, email_hash, phone_encrypted, phone_hash, password_hash, role)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'applicant')
		RETURNING id
	`, fullNameEnc, fullNameHash, emailEnc, emailHash, phoneEnc, phoneHash, "not-a-real-hash").Scan(&userID)
	require.NoError(t, err)

	_, err = applicantTestDB.Exec(ctx, `
		INSERT INTO applicant_profiles (
			id, user_id, full_name_encrypted, full_name_hash, email_encrypted, email_hash,
			phone_encrypted, phone_hash, country, city, institution, field_of_study,
			qualification, academic_status, graduation_year, skills, bio,
			github_url, linkedin_url, portfolio_url
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'Rwanda', 'Kigali', 'University of Rwanda',
			'Computer Science', 'BSc', 'graduated', 2023, ARRAY['Go', 'React'], 'Passionate developer',
			'https://github.com/example', 'https://linkedin.com/in/example', 'https://example.com')
	`, uuid.New().String(), userID, fullNameEnc, fullNameHash, emailEnc, emailHash, phoneEnc, phoneHash)
	require.NoError(t, err)

	return seededApplicant{UserID: userID, Name: name, Email: email, Phone: phone}
}

// profilePayload returns a full profile body that exactly matches what
// seedApplicant writes, so it can be used as the "no changes" baseline.
func profilePayload(s seededApplicant) map[string]any {
	return map[string]any{
		"full_name":       s.Name,
		"email":           s.Email,
		"phone":           s.Phone,
		"country":         "Rwanda",
		"city":            "Kigali",
		"institution":     "University of Rwanda",
		"field_of_study":  "Computer Science",
		"qualification":   "BSc",
		"academic_status": "graduated",
		"graduation_year": 2023,
		"skills":          []any{"Go", "React"},
		"bio":             "Passionate developer",
		"github_url":      "https://github.com/example",
		"linkedin_url":    "https://linkedin.com/in/example",
		"portfolio_url":   "https://example.com",
	}
}

// TestGetProfileRoute covers GET /api/v1/applicant/profile.
func TestGetProfileRoute(t *testing.T) {
	seeded := seedApplicant(t)

	tests := []struct {
		name       string
		headers    map[string]string
		wantStatus int
		check      func(t *testing.T, body map[string]any)
	}{
		{
			name:       "unauthorized - missing token",
			headers:    nil,
			wantStatus: fiber.StatusUnauthorized,
		},
		{
			name:       "unauthorized - malformed token",
			headers:    map[string]string{"Authorization": "Bearer not-a-token"},
			wantStatus: fiber.StatusUnauthorized,
		},
		{
			name:       "not found - no profile for caller",
			headers:    authHeaders(888888888, "applicant"),
			wantStatus: fiber.StatusNotFound,
		},
		{
			name:       "success - returns decrypted profile",
			headers:    authHeaders(seeded.UserID, "applicant"),
			wantStatus: fiber.StatusOK,
			check: func(t *testing.T, body map[string]any) {
				data, ok := body["data"].(map[string]any)
				require.True(t, ok, "response should have data: %v", body)
				assert.Equal(t, seeded.Name, data["full_name"])
				assert.Equal(t, seeded.Email, data["email"])
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, body := doJSON(t, "GET", "/api/v1/applicant/profile", nil, test.headers)
			assert.Equal(t, test.wantStatus, status)
			if test.check != nil {
				test.check(t, body)
			}
		})
	}
}

// TestSaveProfileRoute covers PUT /api/v1/applicant/profile.
func TestSaveProfileRoute(t *testing.T) {
	seeded := seedApplicant(t)
	base := profilePayload(seeded)

	withField := func(key string, value any) map[string]any {
		payload := profilePayload(seeded)
		payload[key] = value
		return payload
	}

	tests := []struct {
		name       string
		headers    map[string]string
		payload    map[string]any
		wantStatus int
		check      func(t *testing.T, body map[string]any)
	}{
		{
			name:       "unauthorized - missing token",
			headers:    nil,
			payload:    base,
			wantStatus: fiber.StatusUnauthorized,
		},
		{
			name:       "validation - invalid email",
			headers:    authHeaders(seeded.UserID, "applicant"),
			payload:    withField("email", "not-an-email"),
			wantStatus: fiber.StatusBadRequest,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "email"), "expected email error: %v", body)
			},
		},
		{
			name:       "validation - invalid phone",
			headers:    authHeaders(seeded.UserID, "applicant"),
			payload:    withField("phone", "abc"),
			wantStatus: fiber.StatusBadRequest,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "phone"), "expected phone error: %v", body)
			},
		},
		{
			name:       "validation - invalid graduation year",
			headers:    authHeaders(seeded.UserID, "applicant"),
			payload:    withField("graduation_year", 1900),
			wantStatus: fiber.StatusBadRequest,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "graduation_year"), "expected graduation_year error: %v", body)
			},
		},
		{
			name:       "validation - name contains digits",
			headers:    authHeaders(seeded.UserID, "applicant"),
			payload:    withField("full_name", "John Doe 123"),
			wantStatus: fiber.StatusBadRequest,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "full_name"), "expected full_name error: %v", body)
			},
		},
		{
			name:       "validation - full_name too long",
			headers:    authHeaders(seeded.UserID, "applicant"),
			payload:    withField("full_name", strings.Repeat("a", 151)),
			wantStatus: fiber.StatusBadRequest,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "full_name"), "expected full_name error: %v", body)
			},
		},
		{
			name:       "validation - country contains digits",
			headers:    authHeaders(seeded.UserID, "applicant"),
			payload:    withField("country", "Rwanda1"),
			wantStatus: fiber.StatusBadRequest,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "country"), "expected country error: %v", body)
			},
		},
		{
			name:       "validation - institution contains unsupported characters",
			headers:    authHeaders(seeded.UserID, "applicant"),
			payload:    withField("institution", "Uni<script>"),
			wantStatus: fiber.StatusBadRequest,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "institution"), "expected institution error: %v", body)
			},
		},
		{
			name:       "validation - invalid github url",
			headers:    authHeaders(seeded.UserID, "applicant"),
			payload:    withField("github_url", "not-a-url"),
			wantStatus: fiber.StatusBadRequest,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "github_url"), "expected github_url error: %v", body)
			},
		},
		{
			name:       "validation - invalid linkedin url",
			headers:    authHeaders(seeded.UserID, "applicant"),
			payload:    withField("linkedin_url", "not-a-url"),
			wantStatus: fiber.StatusBadRequest,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "linkedin_url"), "expected linkedin_url error: %v", body)
			},
		},
		{
			name:       "validation - bio too long",
			headers:    authHeaders(seeded.UserID, "applicant"),
			payload:    withField("bio", strings.Repeat("a", 2001)),
			wantStatus: fiber.StatusBadRequest,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "bio"), "expected bio error: %v", body)
			},
		},
		{
			name:       "not found - no profile for caller",
			headers:    authHeaders(888888888, "applicant"),
			payload:    base,
			wantStatus: fiber.StatusNotFound,
		},
		{
			name:       "not acceptable - no changes",
			headers:    authHeaders(seeded.UserID, "applicant"),
			payload:    base,
			wantStatus: fiber.StatusNotAcceptable,
		},
		{
			name:       "success - updates a changed field",
			headers:    authHeaders(seeded.UserID, "applicant"),
			payload:    withField("city", "Musanze"),
			wantStatus: fiber.StatusOK,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, body := doJSON(t, "PUT", "/api/v1/applicant/profile", test.payload, test.headers)
			assert.Equal(t, test.wantStatus, status)
			if test.check != nil {
				test.check(t, body)
			}
		})
	}
}

// TestListNotificationsRoute covers GET /api/v1/applicant/notifications.
func TestListNotificationsRoute(t *testing.T) {
	seeded := seedApplicant(t)
	_, err := applicantTestDB.Exec(context.Background(), `
		INSERT INTO notifications (id, user_id, title, message)
		VALUES ($1, $2, 'Welcome', 'Your account is ready')
	`, uuid.New().String(), seeded.UserID)
	require.NoError(t, err)

	tests := []struct {
		name       string
		headers    map[string]string
		path       string
		wantStatus int
		check      func(t *testing.T, body map[string]any)
	}{
		{
			name:       "unauthorized - missing token",
			headers:    nil,
			path:       "/api/v1/applicant/notifications",
			wantStatus: fiber.StatusUnauthorized,
		},
		{
			name:       "success - lists caller notifications",
			headers:    authHeaders(seeded.UserID, "applicant"),
			path:       "/api/v1/applicant/notifications",
			wantStatus: fiber.StatusOK,
			check: func(t *testing.T, body map[string]any) {
				arr, ok := body["data"].([]any)
				require.True(t, ok, "response should have data array: %v", body)
				assert.NotEmpty(t, arr)
			},
		},
		{
			name:       "success - read filter applied",
			headers:    authHeaders(seeded.UserID, "applicant"),
			path:       "/api/v1/applicant/notifications?read=false",
			wantStatus: fiber.StatusOK,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, body := doJSON(t, "GET", test.path, nil, test.headers)
			assert.Equal(t, test.wantStatus, status)
			if test.check != nil {
				test.check(t, body)
			}
		})
	}
}

// TestMarkNotificationsReadRoute covers POST /api/v1/applicant/notifications/read.
func TestMarkNotificationsReadRoute(t *testing.T) {
	seeded := seedApplicant(t)
	notificationID := uuid.New().String()
	_, err := applicantTestDB.Exec(context.Background(), `
		INSERT INTO notifications (id, user_id, title, message, read)
		VALUES ($1, $2, 'Reminder', 'Submit your documents', FALSE)
	`, notificationID, seeded.UserID)
	require.NoError(t, err)

	auth := authHeaders(seeded.UserID, "applicant")

	tests := []struct {
		name       string
		headers    map[string]string
		payload    map[string]any
		wantStatus int
		check      func(t *testing.T, body map[string]any)
	}{
		{
			name:       "unauthorized - missing token",
			headers:    nil,
			payload:    map[string]any{"ids": []any{notificationID}},
			wantStatus: fiber.StatusUnauthorized,
		},
		{
			name:       "validation - missing ids",
			headers:    auth,
			payload:    map[string]any{},
			wantStatus: fiber.StatusBadRequest,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, ""), "expected a validation error: %v", body)
			},
		},
		{
			name:       "bad request - notification not owned by caller",
			headers:    auth,
			payload:    map[string]any{"ids": []any{uuid.New().String()}},
			wantStatus: fiber.StatusBadRequest,
		},
		{
			name:       "success - marks notification read",
			headers:    auth,
			payload:    map[string]any{"ids": []any{notificationID}},
			wantStatus: fiber.StatusOK,
		},
		{
			name:       "not acceptable - already read",
			headers:    auth,
			payload:    map[string]any{"ids": []any{notificationID}},
			wantStatus: fiber.StatusNotAcceptable,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, body := doJSON(t, "POST", "/api/v1/applicant/notifications/read", test.payload, test.headers)
			assert.Equal(t, test.wantStatus, status)
			if test.check != nil {
				test.check(t, body)
			}
		})
	}
}

// TestListApplicantsRoute covers GET /api/v1/applicants/ (admin only).
func TestListApplicantsRoute(t *testing.T) {
	seedApplicant(t)

	tests := []struct {
		name       string
		headers    map[string]string
		path       string
		wantStatus int
		check      func(t *testing.T, body map[string]any)
	}{
		{
			name:       "unauthorized - missing token",
			headers:    nil,
			path:       "/api/v1/applicants/",
			wantStatus: fiber.StatusUnauthorized,
		},
		{
			name:       "forbidden - applicant role",
			headers:    authHeaders(1, "applicant"),
			path:       "/api/v1/applicants/",
			wantStatus: fiber.StatusForbidden,
		},
		{
			name:       "success - admin lists applicants",
			headers:    authHeaders(1, "admin"),
			path:       "/api/v1/applicants/",
			wantStatus: fiber.StatusOK,
			check: func(t *testing.T, body map[string]any) {
				_, ok := body["data"].([]any)
				assert.True(t, ok, "response should have data array: %v", body)
				assert.NotNil(t, body["meta"], "response should carry pagination meta: %v", body)
			},
		},
		{
			name:       "success - filter by institution",
			headers:    authHeaders(1, "admin"),
			path:       "/api/v1/applicants/?institution=University",
			wantStatus: fiber.StatusOK,
		},
		{
			name:       "bad request - invalid filter value",
			headers:    authHeaders(1, "admin"),
			path:       "/api/v1/applicants/?institution=" + strings.Repeat("a", 120),
			wantStatus: fiber.StatusBadRequest,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, body := doJSON(t, "GET", test.path, nil, test.headers)
			assert.Equal(t, test.wantStatus, status)
			if test.check != nil {
				test.check(t, body)
			}
		})
	}
}

// TestGetSettingsRoute covers GET /api/v1/settings/ (admin only).
func TestGetSettingsRoute(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string]string
		wantStatus int
		check      func(t *testing.T, body map[string]any)
	}{
		{
			name:       "unauthorized - missing token",
			headers:    nil,
			wantStatus: fiber.StatusUnauthorized,
		},
		{
			name:       "forbidden - applicant role",
			headers:    authHeaders(1, "applicant"),
			wantStatus: fiber.StatusForbidden,
		},
		{
			name:       "success - returns settings",
			headers:    authHeaders(1, "admin"),
			wantStatus: fiber.StatusOK,
			check: func(t *testing.T, body map[string]any) {
				data, ok := body["data"].(map[string]any)
				require.True(t, ok, "response should have data: %v", body)
				assert.Equal(t, true, data["applications_open"])
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, body := doJSON(t, "GET", "/api/v1/settings/", nil, test.headers)
			assert.Equal(t, test.wantStatus, status)
			if test.check != nil {
				test.check(t, body)
			}
		})
	}
}

// TestSaveSettingsRoute covers PUT /api/v1/settings/ (admin only).
func TestSaveSettingsRoute(t *testing.T) {
	admin := authHeaders(1, "admin")

	tests := []struct {
		name       string
		headers    map[string]string
		payload    map[string]any
		wantStatus int
		check      func(t *testing.T, body map[string]any)
	}{
		{
			name:       "unauthorized - missing token",
			headers:    nil,
			payload:    map[string]any{"applications_open": true},
			wantStatus: fiber.StatusUnauthorized,
		},
		{
			name:       "forbidden - applicant role",
			headers:    authHeaders(1, "applicant"),
			payload:    map[string]any{"applications_open": true},
			wantStatus: fiber.StatusForbidden,
		},
		{
			name:       "validation - closure message too long",
			headers:    admin,
			payload:    map[string]any{"applications_open": true, "closure_message": strings.Repeat("a", 501)},
			wantStatus: fiber.StatusBadRequest,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "closure_message"), "expected closure_message error: %v", body)
			},
		},
		{
			name:       "bad request - invalid close_on date",
			headers:    admin,
			payload:    map[string]any{"applications_open": true, "close_on": "not-a-date"},
			wantStatus: fiber.StatusBadRequest,
		},
		{
			name:       "not acceptable - no changes",
			headers:    admin,
			payload:    map[string]any{"applications_open": true, "closure_message": ""},
			wantStatus: fiber.StatusNotAcceptable,
		},
		{
			name:       "success - updates settings",
			headers:    admin,
			payload:    map[string]any{"applications_open": false, "closure_message": "Closed for now"},
			wantStatus: fiber.StatusOK,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, body := doJSON(t, "PUT", "/api/v1/settings/", test.payload, test.headers)
			assert.Equal(t, test.wantStatus, status)
			if test.check != nil {
				test.check(t, body)
			}
		})
	}
}

// TestUnauthorizedAccess asserts every protected route rejects callers without a
// token (leazi-style single table over the whole surface).
func TestUnauthorizedAccess(t *testing.T) {
	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v1/applicant/profile"},
		{"PUT", "/api/v1/applicant/profile"},
		{"GET", "/api/v1/applicant/notifications"},
		{"POST", "/api/v1/applicant/notifications/read"},
		{"GET", "/api/v1/applicants/"},
		{"GET", "/api/v1/settings/"},
		{"PUT", "/api/v1/settings/"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			status, _ := doJSON(t, ep.method, ep.path, nil, nil)
			assert.Equal(t, fiber.StatusUnauthorized, status)
		})
	}
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
