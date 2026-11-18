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
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qonics/qonics-appy-api/libs/shared-package/config"
	"github.com/qonics/qonics-appy-api/libs/shared-package/middleware"
	"github.com/qonics/qonics-appy-api/libs/shared-package/utils"
	"github.com/qonics/qonics-appy-api/services/application/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Shared harness, mirroring leazi's controller tests: one app + one DB set up
// once, then every route is exercised by a single table-driven test function
// carrying several scenario payloads.
var (
	applicationTestApp *fiber.App
	applicationTestDB  *pgxpool.Pool
	applicationTestCfg *config.Config
)

// testJWTSecret is used to sign the tokens consumed by the real auth
// middleware, so authenticated and unauthenticated requests flow through the
// same production code path.
const testJWTSecret = "application-test-secret"

// testIdentityEmail is attached to the identity carried by the test tokens.
const testIdentityEmail = "tester@example.com"

func init() {
	utils.IsTestMode = true

	if err := os.Chdir("../../.."); err != nil {
		panic("Failed to change directory: " + err.Error())
	}

	cfg, err := config.LoadConfig("application-service")
	if err != nil {
		panic("Failed to load config: " + err.Error())
	}
	applicationTestCfg = cfg

	db, err := cfg.InitDB()
	if err != nil {
		panic("Failed to connect to test database: " + err.Error())
	}
	if _, err := db.Exec(context.Background(), "SET app.encryption_key = 'secret'"); err != nil {
		panic("Failed to set encryption key: " + err.Error())
	}
	applicationTestDB = db

	controller := NewApplicationController(db, cfg, cfg.InitRedis())

	applicationTestApp = fiber.New(fiber.Config{AppName: "application-service-test"})

	// Mirror routes/api.go: the real auth middleware means a missing/invalid
	// token yields 401 and the admin-only route yields 403 for non-admins.
	auth := middleware.AuthMiddleware(testJWTSecret)

	applications := applicationTestApp.Group("/api/v1/applications", auth)
	applications.Get("/mine", controller.ListMine)
	applications.Post("/", controller.Create)
	applications.Get("/:id", controller.Get)
	applications.Patch("/:id/status", controller.UpdateStatus, middleware.AdminMiddleware())
	applications.Post("/:id/notes", controller.AddNote)
	applications.Get("/", controller.ListAll)

	public := applicationTestApp.Group("/api/v1/sessions")
	public.Get("/bookings", controller.Availability)
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

	resp, err := applicationTestApp.Test(req)
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

// seededProfile is a user together with its applicant_profiles row, created
// directly so application routes can be exercised without the auth service.
type seededProfile struct {
	UserID    int
	ProfileID string
	Name      string
	Email     string
	Phone     string
}

func seedProfile(t *testing.T) seededProfile {
	t.Helper()
	ctx := context.Background()

	name := uniqueName("Applicant")
	email := uniqueEmail("applicant")
	phone := "+250788123456"

	fullNameEnc, fullNameHash, err := utils.EncryptAndHash(ctx, applicationTestDB, name)
	require.NoError(t, err)
	emailEnc, emailHash, err := utils.EncryptAndHash(ctx, applicationTestDB, email)
	require.NoError(t, err)
	phoneEnc, phoneHash, err := utils.EncryptAndHash(ctx, applicationTestDB, phone)
	require.NoError(t, err)

	var userID int
	err = applicationTestDB.QueryRow(ctx, `
		INSERT INTO users (full_name_encrypted, full_name_hash, email_encrypted, email_hash, phone_encrypted, phone_hash, password_hash, role)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'applicant')
		RETURNING id
	`, fullNameEnc, fullNameHash, emailEnc, emailHash, phoneEnc, phoneHash, "not-a-real-hash").Scan(&userID)
	require.NoError(t, err)

	profileID := uuid.New().String()
	_, err = applicationTestDB.Exec(ctx, `
		INSERT INTO applicant_profiles (id, user_id, full_name_encrypted, full_name_hash, email_encrypted, email_hash, phone_encrypted, phone_hash, country, city)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'Rwanda', 'Kigali')
	`, profileID, userID, fullNameEnc, fullNameHash, emailEnc, emailHash, phoneEnc, phoneHash)
	require.NoError(t, err)

	return seededProfile{UserID: userID, ProfileID: profileID, Name: name, Email: email, Phone: phone}
}

// seedApplication inserts an application owned by the given profile.
func seedApplication(t *testing.T, profileID string) string {
	t.Helper()
	appID := uuid.New().String()
	_, err := applicationTestDB.Exec(context.Background(), `
		INSERT INTO applications (id, reference, applicant_id, department, specialization,
			motivation, experience, education_level, session, duration_months, work_mode, status)
		VALUES ($1, $2, $3, 'software', 'backend', 'Build things', '2 years',
			'university', 'morning', 6, 'remote', 'submitted')
	`, appID, fmt.Sprintf("APP-%s", uniqueSuffix()), profileID)
	require.NoError(t, err)
	return appID
}

// seedOwnedFile inserts a confirmed (non-pending) file owned by the user, so it
// can be attached to an application.
func seedOwnedFile(t *testing.T, ownerUserID int) string {
	t.Helper()
	fileID := uuid.New().String()
	_, err := applicationTestDB.Exec(context.Background(), `
		INSERT INTO files (id, owner_id, owner_type, name, file_path, file_type, file_size)
		VALUES ($1, $2, 'user', 'cv.pdf', 'uploads/cv.pdf', 'cv', 1024)
	`, fileID, fmt.Sprintf("%d", ownerUserID))
	require.NoError(t, err)
	return fileID
}

// validApplicationPayload returns a payload that passes validation.
func validApplicationPayload() map[string]any {
	return map[string]any{
		"department":      "software",
		"specialization":  "backend",
		"motivation":      "I want to learn",
		"experience":      "2 years",
		"start_date":      "2026-01-15T00:00:00Z",
		"education_level": "university",
		"session":         "morning",
		"duration_months": 6,
		"work_mode":       "remote",
	}
}

// TestCreateApplicationRoute covers POST /api/v1/applications/.
func TestCreateApplicationRoute(t *testing.T) {
	profile := seedProfile(t)
	auth := authHeaders(profile.UserID, "applicant")
	ownedFile := seedOwnedFile(t, profile.UserID)

	withField := func(key string, value any) map[string]any {
		payload := validApplicationPayload()
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
			payload:    validApplicationPayload(),
			wantStatus: fiber.StatusUnauthorized,
		},
		{
			name:       "validation - missing required fields",
			headers:    auth,
			payload:    map[string]any{},
			wantStatus: fiber.StatusBadRequest,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "department"), "expected department error: %v", body)
			},
		},
		{
			name:       "validation - invalid department",
			headers:    auth,
			payload:    withField("department", "Engineering"),
			wantStatus: fiber.StatusBadRequest,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "department"), "expected department error: %v", body)
			},
		},
		{
			name:       "validation - specialization not in department",
			headers:    auth,
			payload:    withField("specialization", "embedded"),
			wantStatus: fiber.StatusBadRequest,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "specialization"), "expected specialization error: %v", body)
			},
		},
		{
			name:       "validation - invalid education level",
			headers:    auth,
			payload:    withField("education_level", "bachelor"),
			wantStatus: fiber.StatusBadRequest,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "education_level"), "expected education_level error: %v", body)
			},
		},
		{
			name:       "validation - invalid session",
			headers:    auth,
			payload:    withField("session", "january-june"),
			wantStatus: fiber.StatusBadRequest,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "session"), "expected session error: %v", body)
			},
		},
		{
			name:       "validation - invalid work mode",
			headers:    auth,
			payload:    withField("work_mode", "on-site"),
			wantStatus: fiber.StatusBadRequest,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "work_mode"), "expected work_mode error: %v", body)
			},
		},
		{
			name:       "validation - duration must be positive",
			headers:    auth,
			payload:    withField("duration_months", 0),
			wantStatus: fiber.StatusBadRequest,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "duration_months"), "expected duration_months error: %v", body)
			},
		},
		{
			name:       "bad request - invalid start date",
			headers:    auth,
			payload:    withField("start_date", "not-a-date"),
			wantStatus: fiber.StatusBadRequest,
		},
		{
			name:       "bad request - unowned document",
			headers:    auth,
			payload:    withField("document_ids", []any{uuid.New().String()}),
			wantStatus: fiber.StatusBadRequest,
		},
		{
			name:       "not found - caller has no profile",
			headers:    authHeaders(888888888, "applicant"),
			payload:    validApplicationPayload(),
			wantStatus: fiber.StatusNotFound,
		},
		{
			name:       "success - creates application with document",
			headers:    auth,
			payload:    withField("document_ids", []any{ownedFile}),
			wantStatus: fiber.StatusOK,
			check: func(t *testing.T, body map[string]any) {
				data, ok := body["data"].(map[string]any)
				require.True(t, ok, "response should have data: %v", body)
				assert.Equal(t, "software", data["department"])
				assert.NotEmpty(t, data["reference"])
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, body := doJSON(t, "POST", "/api/v1/applications/", test.payload, test.headers)
			assert.Equal(t, test.wantStatus, status)
			if test.check != nil {
				test.check(t, body)
			}
		})
	}
}

// TestListMineRoute covers GET /api/v1/applications/mine.
func TestListMineRoute(t *testing.T) {
	profile := seedProfile(t)
	seedApplication(t, profile.ProfileID)

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
			path:       "/api/v1/applications/mine",
			wantStatus: fiber.StatusUnauthorized,
		},
		{
			name:       "not found - caller has no profile",
			headers:    authHeaders(888888888, "applicant"),
			path:       "/api/v1/applications/mine",
			wantStatus: fiber.StatusNotFound,
		},
		{
			name:       "success - lists caller applications with summary",
			headers:    authHeaders(profile.UserID, "applicant"),
			path:       "/api/v1/applications/mine",
			wantStatus: fiber.StatusOK,
			check: func(t *testing.T, body map[string]any) {
				data, ok := body["data"].(map[string]any)
				require.True(t, ok, "response should have data: %v", body)
				_, ok = data["applications"].([]any)
				assert.True(t, ok, "data should carry applications: %v", data)
				assert.NotNil(t, data["summary"], "data should carry a summary: %v", data)
			},
		},
		{
			name:       "success - status filter applied",
			headers:    authHeaders(profile.UserID, "applicant"),
			path:       "/api/v1/applications/mine?status=submitted",
			wantStatus: fiber.StatusOK,
		},
		{
			name:       "bad request - invalid status filter",
			headers:    authHeaders(profile.UserID, "applicant"),
			path:       "/api/v1/applications/mine?status=bogus",
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

// TestGetApplicationRoute covers GET /api/v1/applications/:id.
func TestGetApplicationRoute(t *testing.T) {
	profile := seedProfile(t)
	appID := seedApplication(t, profile.ProfileID)

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
			path:       "/api/v1/applications/" + appID,
			wantStatus: fiber.StatusUnauthorized,
		},
		{
			name:       "not found - unknown application",
			headers:    authHeaders(profile.UserID, "applicant"),
			path:       "/api/v1/applications/" + uuid.New().String(),
			wantStatus: fiber.StatusNotFound,
		},
		{
			name:       "success - returns application",
			headers:    authHeaders(profile.UserID, "applicant"),
			path:       "/api/v1/applications/" + appID,
			wantStatus: fiber.StatusOK,
			check: func(t *testing.T, body map[string]any) {
				data, ok := body["data"].(map[string]any)
				require.True(t, ok, "response should have data: %v", body)
				assert.Equal(t, appID, data["id"])
			},
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

// TestUpdateStatusRoute covers PATCH /api/v1/applications/:id/status (admin only).
func TestUpdateStatusRoute(t *testing.T) {
	profile := seedProfile(t)
	appID := seedApplication(t, profile.ProfileID)

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
			payload:    map[string]any{"status": "under_review"},
			wantStatus: fiber.StatusUnauthorized,
		},
		{
			name:       "forbidden - applicant role",
			headers:    authHeaders(profile.UserID, "applicant"),
			payload:    map[string]any{"status": "under_review"},
			wantStatus: fiber.StatusForbidden,
		},
		{
			name:       "validation - invalid status",
			headers:    admin,
			payload:    map[string]any{"status": "bogus"},
			wantStatus: fiber.StatusBadRequest,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "status"), "expected status error: %v", body)
			},
		},
		{
			name:       "not found - unknown application",
			headers:    admin,
			payload:    map[string]any{"status": "under_review"},
			wantStatus: fiber.StatusNotFound,
		},
		{
			name:       "not acceptable - status unchanged",
			headers:    admin,
			payload:    map[string]any{"status": "submitted"},
			wantStatus: fiber.StatusNotAcceptable,
		},
		{
			name:       "success - updates status",
			headers:    admin,
			payload:    map[string]any{"status": "under_review", "note": "Looks promising"},
			wantStatus: fiber.StatusOK,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := "/api/v1/applications/" + appID + "/status"
			if test.name == "not found - unknown application" {
				path = "/api/v1/applications/" + uuid.New().String() + "/status"
			}
			status, body := doJSON(t, "PATCH", path, test.payload, test.headers)
			assert.Equal(t, test.wantStatus, status)
			if test.check != nil {
				test.check(t, body)
			}
		})
	}
}

// TestAddNoteRoute covers POST /api/v1/applications/:id/notes.
func TestAddNoteRoute(t *testing.T) {
	profile := seedProfile(t)
	appID := seedApplication(t, profile.ProfileID)
	auth := authHeaders(profile.UserID, "applicant")

	tests := []struct {
		name       string
		headers    map[string]string
		payload    map[string]any
		path       string
		wantStatus int
		check      func(t *testing.T, body map[string]any)
	}{
		{
			name:       "unauthorized - missing token",
			headers:    nil,
			payload:    map[string]any{"text": "Nice"},
			path:       "/api/v1/applications/" + appID + "/notes",
			wantStatus: fiber.StatusUnauthorized,
		},
		{
			name:       "validation - missing text",
			headers:    auth,
			payload:    map[string]any{},
			path:       "/api/v1/applications/" + appID + "/notes",
			wantStatus: fiber.StatusBadRequest,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "text"), "expected text error: %v", body)
			},
		},
		{
			name:       "success - adds a note",
			headers:    auth,
			payload:    map[string]any{"text": "Strong portfolio"},
			path:       "/api/v1/applications/" + appID + "/notes",
			wantStatus: fiber.StatusOK,
		},
		{
			name:       "conflict - duplicate note",
			headers:    auth,
			payload:    map[string]any{"text": "Strong portfolio"},
			path:       "/api/v1/applications/" + appID + "/notes",
			wantStatus: fiber.StatusConflict,
		},
		{
			name:       "bad request - unknown application",
			headers:    auth,
			payload:    map[string]any{"text": "Orphan note"},
			path:       "/api/v1/applications/" + uuid.New().String() + "/notes",
			wantStatus: fiber.StatusBadRequest,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, body := doJSON(t, "POST", test.path, test.payload, test.headers)
			assert.Equal(t, test.wantStatus, status)
			if test.check != nil {
				test.check(t, body)
			}
		})
	}
}

// TestListAllRoute covers GET /api/v1/applications/ (admin only).
func TestListAllRoute(t *testing.T) {
	profile := seedProfile(t)
	seedApplication(t, profile.ProfileID)

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
			path:       "/api/v1/applications/",
			wantStatus: fiber.StatusUnauthorized,
		},
		{
			name:       "forbidden - applicant role",
			headers:    authHeaders(profile.UserID, "applicant"),
			path:       "/api/v1/applications/",
			wantStatus: fiber.StatusForbidden,
		},
		{
			name:       "success - admin lists applications with dashboard",
			headers:    authHeaders(1, "admin"),
			path:       "/api/v1/applications/",
			wantStatus: fiber.StatusOK,
			check: func(t *testing.T, body map[string]any) {
				data, ok := body["data"].(map[string]any)
				require.True(t, ok, "response should have data: %v", body)
				assert.NotNil(t, data["summary"], "data should carry a summary: %v", data)
				assert.NotNil(t, data["by_department"], "data should carry by_department: %v", data)
				assert.NotNil(t, body["meta"], "response should carry pagination meta: %v", body)
			},
		},
		{
			name:       "success - filters applied",
			headers:    authHeaders(1, "admin"),
			path:       "/api/v1/applications/?department=software&status=submitted",
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

// TestAvailabilityRoute covers the public GET /api/v1/sessions/bookings.
func TestAvailabilityRoute(t *testing.T) {
	status, body := doJSON(t, "GET", "/api/v1/sessions/bookings", nil, nil)
	assert.Equal(t, fiber.StatusOK, status)
	assert.Equal(t, "Availability retrieved", body["message"])
}

// TestUnauthorizedAccess asserts every protected route rejects callers without a
// token (leazi-style single table over the whole surface).
func TestUnauthorizedAccess(t *testing.T) {
	appID := uuid.New().String()
	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v1/applications/mine"},
		{"POST", "/api/v1/applications/"},
		{"GET", "/api/v1/applications/" + appID},
		{"PATCH", "/api/v1/applications/" + appID + "/status"},
		{"POST", "/api/v1/applications/" + appID + "/notes"},
		{"GET", "/api/v1/applications/"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			status, _ := doJSON(t, ep.method, ep.path, nil, nil)
			assert.Equal(t, fiber.StatusUnauthorized, status)
		})
	}
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

	controller := NewApplicationController(applicationTestDB, applicationTestCfg, nil)

	app := fiber.New()
	app.Get("/parse", func(c *fiber.Ctx) error {
		gotFilter, gotWhere, gotParams, gotErr = controller.parseFilter(c, "profile-1")
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

	controller := NewApplicationController(applicationTestDB, applicationTestCfg, nil)

	app := fiber.New()
	app.Get("/parse", func(c *fiber.Ctx) error {
		gotFilter, gotWhere, gotParams, gotErr = controller.parseFilter(c, "")
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/parse?page=0&limit=999", nil)
	_, err := app.Test(req)
	require.NoError(t, err)
	require.NoError(t, gotErr)

	assert.Equal(t, 1, gotFilter.Page)    // clamped up to the minimum
	assert.Equal(t, 100, gotFilter.Limit) // clamped down to the maximum
	assert.Empty(t, gotWhere)
	assert.Empty(t, gotParams)
}

// TestParseFilterInvalidDate rejects an unparseable submission date.
func TestParseFilterInvalidDate(t *testing.T) {
	var gotErr error

	controller := NewApplicationController(applicationTestDB, applicationTestCfg, nil)

	app := fiber.New()
	app.Get("/parse", func(c *fiber.Ctx) error {
		_, _, _, gotErr = controller.parseFilter(c, "")
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/parse?submitted_from=not-a-date", nil)
	_, err := app.Test(req)
	require.NoError(t, err)
	assert.Error(t, gotErr)
}
