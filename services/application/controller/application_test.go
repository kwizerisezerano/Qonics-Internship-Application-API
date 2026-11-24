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

	// Deterministic settings row so the intake-gate tests start from an open
	// window regardless of any state left behind by other suites sharing the
	// test database.
	if _, err := db.Exec(context.Background(), `
		INSERT INTO settings (id, applications_open, close_on, closure_message)
		VALUES (1, TRUE, NULL, '')
		ON CONFLICT (id) DO UPDATE SET applications_open = TRUE, close_on = NULL, closure_message = ''
	`); err != nil {
		panic("Failed to reset settings: " + err.Error())
	}

	controller := NewApplicationController(db, cfg, cfg.InitRedis())

	applicationTestApp = fiber.New(fiber.Config{AppName: "application-service-test"})

	// Mirror routes/api.go: the real auth middleware means a missing/invalid
	// token yields 401 and the admin-only route yields 403 for non-admins.
	auth := middleware.AuthMiddleware(testJWTSecret)

	applications := applicationTestApp.Group("/api/v1/applications")
	// Submitting is open to guests; registered before the auth middleware, as in routes/api.go.
	applications.Post("/", middleware.OptionalAuthMiddleware(testJWTSecret), controller.Create)
	applications.Use(auth)
	applications.Get("/mine", controller.ListMine)
	applications.Get("/:id", controller.Get)
	applications.Patch("/:id/status", controller.UpdateStatus, middleware.AdminMiddleware())
	applications.Post("/:id/notes", controller.AddNote, middleware.AdminMiddleware())
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
			name:       "guest without contact details is rejected",
			headers:    nil,
			payload:    validApplicationPayload(),
			wantStatus: fiber.StatusBadRequest,
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
			name:       "validation - missing motivation",
			headers:    auth,
			payload:    map[string]any{"department": "software", "specialization": "backend", "experience": "2 years", "education_level": "university", "session": "morning", "duration_months": 6, "work_mode": "remote"},
			wantStatus: fiber.StatusBadRequest,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "motivation"), "expected motivation error: %v", body)
			},
		},
		{
			name:       "validation - missing experience",
			headers:    auth,
			payload:    map[string]any{"department": "software", "specialization": "backend", "motivation": "I want to learn", "education_level": "university", "session": "morning", "duration_months": 6, "work_mode": "remote"},
			wantStatus: fiber.StatusBadRequest,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "experience"), "expected experience error: %v", body)
			},
		},
		{
			name:       "validation - missing specialization",
			headers:    auth,
			payload:    map[string]any{"department": "software", "motivation": "I want to learn", "experience": "2 years", "education_level": "university", "session": "morning", "duration_months": 6, "work_mode": "remote"},
			wantStatus: fiber.StatusBadRequest,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "specialization"), "expected specialization error: %v", body)
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

// TestCreateApplicationClosedWindow covers the settings-driven intake gate: when
// an admin closes applications (or the closing date has arrived), POST
// /api/v1/applications/ is rejected with the closure message.
func TestCreateApplicationClosedWindow(t *testing.T) {
	profile := seedProfile(t)
	auth := authHeaders(profile.UserID, "applicant")
	ctx := context.Background()

	setSettings := func(appsOpen bool, closeOn any, message string) {
		t.Helper()
		_, err := applicationTestDB.Exec(ctx,
			`UPDATE settings SET applications_open = $1, close_on = $2, closure_message = $3 WHERE id = 1`,
			appsOpen, closeOn, message)
		require.NoError(t, err)
	}
	// Always restore an open window for the other tests sharing this database.
	defer setSettings(true, nil, "")

	tests := []struct {
		name       string
		appsOpen   bool
		closeOn    any
		message    string
		wantStatus int
		wantMsg    string
	}{
		{
			name:       "forbidden - applications switched off",
			appsOpen:   false,
			closeOn:    nil,
			message:    "Intake is closed for this cohort.",
			wantStatus: fiber.StatusForbidden,
			wantMsg:    "Intake is closed for this cohort.",
		},
		{
			name:       "forbidden - closing date already passed",
			appsOpen:   true,
			closeOn:    time.Now().AddDate(0, 0, -1),
			message:    "",
			wantStatus: fiber.StatusForbidden,
			wantMsg:    "Applications are currently closed.",
		},
		{
			name:       "success - window open",
			appsOpen:   true,
			closeOn:    nil,
			message:    "",
			wantStatus: fiber.StatusOK,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setSettings(test.appsOpen, test.closeOn, test.message)
			status, body := doJSON(t, "POST", "/api/v1/applications/", validApplicationPayload(), auth)
			assert.Equal(t, test.wantStatus, status)
			if test.wantMsg != "" {
				assert.Equal(t, test.wantMsg, body["message"])
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
	other := seedProfile(t)

	// An internal note that the applicant must never receive.
	_, err := applicationTestDB.Exec(context.Background(), `
		INSERT INTO review_notes (id, application_id, author, text) VALUES ($1, $2, 'admin@example.com', 'internal only')
	`, uuid.New().String(), appID)
	require.NoError(t, err)

	notesOf := func(t *testing.T, body map[string]any) []any {
		data, ok := body["data"].(map[string]any)
		require.True(t, ok, "response should have data: %v", body)
		notes, _ := data["notes"].([]any)
		return notes
	}

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
			name:       "success - owner gets own application without internal notes",
			headers:    authHeaders(profile.UserID, "applicant"),
			path:       "/api/v1/applications/" + appID,
			wantStatus: fiber.StatusOK,
			check: func(t *testing.T, body map[string]any) {
				data, ok := body["data"].(map[string]any)
				require.True(t, ok, "response should have data: %v", body)
				assert.Equal(t, appID, data["id"])
				assert.Empty(t, notesOf(t, body), "applicant must not receive internal notes")
			},
		},
		{
			name:       "not found - another applicant's application",
			headers:    authHeaders(other.UserID, "applicant"),
			path:       "/api/v1/applications/" + appID,
			wantStatus: fiber.StatusNotFound,
		},
		{
			name:       "success - admin gets any application with notes",
			headers:    authHeaders(1, "admin"),
			path:       "/api/v1/applications/" + appID,
			wantStatus: fiber.StatusOK,
			check: func(t *testing.T, body map[string]any) {
				assert.Len(t, notesOf(t, body), 1)
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
			name:       "validation - missing status",
			headers:    admin,
			payload:    map[string]any{"note": "no status here"},
			wantStatus: fiber.StatusBadRequest,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "status"), "expected status error: %v", body)
			},
		},
		{
			name:       "bad request - invalid interview date",
			headers:    admin,
			payload:    map[string]any{"status": "interview", "interview_at": "not-a-date"},
			wantStatus: fiber.StatusBadRequest,
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
		{
			name:       "success - records interview date",
			headers:    admin,
			payload:    map[string]any{"status": "interview", "interview_at": "2026-05-01T09:00:00Z"},
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

// TestAddNoteRoute covers POST /api/v1/applications/:id/notes (admin only).
func TestAddNoteRoute(t *testing.T) {
	profile := seedProfile(t)
	appID := seedApplication(t, profile.ProfileID)
	auth := authHeaders(1, "admin")

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
			name:       "forbidden - applicant cannot add notes, even on own application",
			headers:    authHeaders(profile.UserID, "applicant"),
			payload:    map[string]any{"text": "Self review"},
			path:       "/api/v1/applications/" + appID + "/notes",
			wantStatus: fiber.StatusForbidden,
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

// TestCreateApplicationDuplicate: an applicant can't hold two live
// applications for the same specialization, but can apply for another one,
// and can re-apply once the earlier application was rejected.
func TestCreateApplicationDuplicate(t *testing.T) {
	profile := seedProfile(t)
	auth := authHeaders(profile.UserID, "applicant")

	status, body := doJSON(t, "POST", "/api/v1/applications/", validApplicationPayload(), auth)
	require.Equal(t, fiber.StatusOK, status, "first submission: %v", body)
	firstID := body["data"].(map[string]any)["id"].(string)

	status, body = doJSON(t, "POST", "/api/v1/applications/", validApplicationPayload(), auth)
	assert.Equal(t, fiber.StatusConflict, status, "duplicate should be refused: %v", body)
	assert.Contains(t, body["message"], "already have an application")

	other := validApplicationPayload()
	other["specialization"] = "frontend"
	status, body = doJSON(t, "POST", "/api/v1/applications/", other, auth)
	assert.Equal(t, fiber.StatusOK, status, "a different specialization is allowed: %v", body)

	_, err := applicationTestDB.Exec(context.Background(), `UPDATE applications SET status = 'rejected' WHERE id = $1`, firstID)
	require.NoError(t, err)
	status, body = doJSON(t, "POST", "/api/v1/applications/", validApplicationPayload(), auth)
	assert.Equal(t, fiber.StatusOK, status, "re-applying after rejection is allowed: %v", body)
}

// TestCreateApplicationSeatCapacity: on-site/hybrid interns are limited to
// model.SessionCapacity per daily session over overlapping periods; remote
// interns and non-overlapping periods are not limited.
func TestCreateApplicationSeatCapacity(t *testing.T) {
	// A start date far in the future (unique per run) keeps other tests and
	// earlier runs out of this session's seat count.
	start := time.Date(2090+rand.Intn(9), time.Month(1+rand.Intn(6)), 3, 0, 0, 0, 0, time.UTC)
	payload := func(workMode string, startAt time.Time) map[string]any {
		p := validApplicationPayload()
		p["work_mode"] = workMode
		p["session"] = "afternoon"
		p["start_date"] = startAt.Format(time.RFC3339)
		p["duration_months"] = 3
		return p
	}
	submit := func(p map[string]any) (int, map[string]any) {
		profile := seedProfile(t)
		return doJSON(t, "POST", "/api/v1/applications/", p, authHeaders(profile.UserID, "applicant"))
	}

	for i := 0; i < model.SessionCapacity; i++ {
		mode := "onsite"
		if i%2 == 1 {
			mode = "hybrid"
		}
		status, body := submit(payload(mode, start))
		require.Equal(t, fiber.StatusOK, status, "seat %d: %v", i+1, body)
	}

	status, body := submit(payload("onsite", start.AddDate(0, 1, 0)))
	assert.Equal(t, fiber.StatusConflict, status, "overlapping period should find the session full: %v", body)

	status, body = submit(payload("remote", start))
	assert.Equal(t, fiber.StatusOK, status, "remote interns need no seat: %v", body)

	status, body = submit(payload("onsite", start.AddDate(0, 3, 0)))
	assert.Equal(t, fiber.StatusOK, status, "a period starting after the others end is free: %v", body)
}

// TestAvailabilityPaginationAndSeats covers GET /api/v1/sessions/bookings:
// pagination meta, the limit cap, seats counted out of model.SessionCapacity,
// and internships that already ended being left out.
func TestAvailabilityPaginationAndSeats(t *testing.T) {
	// The latest possible start date keeps this group on the last page.
	start := time.Date(2099, time.December, 1+rand.Intn(20), 0, 0, 0, 0, time.UTC)
	for i := 0; i < 2; i++ {
		seedSeatApplication(t, seedProfile(t).ProfileID, start, "onsite", "afternoon", "submitted")
	}
	// Rejected applications free their seat; ended internships are hidden.
	seedSeatApplication(t, seedProfile(t).ProfileID, start, "hybrid", "afternoon", "rejected")
	ended := time.Date(2000, time.January, 3, 0, 0, 0, 0, time.UTC)
	seedSeatApplication(t, seedProfile(t).ProfileID, ended, "onsite", "morning", "accepted")

	status, body := doJSON(t, "GET", "/api/v1/sessions/bookings?page=1&limit=1", nil, nil)
	require.Equal(t, fiber.StatusOK, status, "body: %v", body)
	data, _ := body["data"].([]any)
	assert.Len(t, data, 1, "limit is honoured")
	meta, ok := body["meta"].(map[string]any)
	require.True(t, ok, "response should carry pagination meta: %v", body)
	total := int(meta["total"].(float64))
	assert.GreaterOrEqual(t, total, 1)

	status, body = doJSON(t, "GET", "/api/v1/sessions/bookings?limit=1000", nil, nil)
	require.Equal(t, fiber.StatusOK, status)
	assert.Equal(t, float64(100), body["meta"].(map[string]any)["per_page"], "limit is capped at 100")

	lastPage := (total + 99) / 100
	status, body = doJSON(t, "GET", fmt.Sprintf("/api/v1/sessions/bookings?limit=100&page=%d", lastPage), nil, nil)
	require.Equal(t, fiber.StatusOK, status)
	var found map[string]any
	for _, row := range body["data"].([]any) {
		b := row.(map[string]any)
		if b["start_date"] == start.Format(time.RFC3339) && b["session"] == "afternoon" {
			found = b
		}
		assert.NotEqual(t, ended.Format(time.RFC3339), b["start_date"], "ended internships must not be listed")
	}
	require.NotNil(t, found, "the seeded booking should be listed")
	assert.Equal(t, float64(model.SessionCapacity-2), found["available_seats"], "two seats taken, the rejected one is free")
}

// TestListMineHidesInternalNotes: GET /applications/mine never returns review
// notes to the applicant.
func TestListMineHidesInternalNotes(t *testing.T) {
	profile := seedProfile(t)
	appID := seedApplication(t, profile.ProfileID)
	seedNote(t, appID, "internal only")

	status, body := doJSON(t, "GET", "/api/v1/applications/mine", nil, authHeaders(profile.UserID, "applicant"))
	require.Equal(t, fiber.StatusOK, status, "body: %v", body)
	apps := body["data"].(map[string]any)["applications"].([]any)
	require.Len(t, apps, 1)
	assert.Empty(t, apps[0].(map[string]any)["notes"])
}

// TestUpdateStatusRecordsHistory: each status change is saved together with
// its history entry (newest first) and the admin's note.
func TestUpdateStatusRecordsHistory(t *testing.T) {
	profile := seedProfile(t)
	appID := seedApplication(t, profile.ProfileID)
	admin := authHeaders(1, "admin")
	path := "/api/v1/applications/" + appID + "/status"

	status, body := doJSON(t, "PATCH", path, map[string]any{"status": "shortlisted", "note": "Strong CV"}, admin)
	require.Equal(t, fiber.StatusOK, status, "body: %v", body)
	status, body = doJSON(t, "PATCH", path, map[string]any{"status": "accepted"}, admin)
	require.Equal(t, fiber.StatusOK, status, "body: %v", body)

	data := body["data"].(map[string]any)
	assert.Equal(t, "accepted", data["status"])
	history := data["history"].([]any)
	require.Len(t, history, 2, "the seeded application has no submit entry, so two changes give two entries")
	assert.Equal(t, "accepted", history[0].(map[string]any)["status"], "newest first")
	assert.Equal(t, "shortlisted", history[1].(map[string]any)["status"])
	assert.Equal(t, "Strong CV", history[1].(map[string]any)["note"])
}

// TestCreateApplicationDocuments: all documents are checked and linked in one
// go; a single foreign document rejects the whole submission and nothing is
// saved; repeating the same document id attaches it once.
func TestCreateApplicationDocuments(t *testing.T) {
	ctx := context.Background()

	t.Run("several own documents are attached", func(t *testing.T) {
		profile := seedProfile(t)
		docs := []string{seedOwnedFile(t, profile.UserID), seedOwnedFile(t, profile.UserID)}
		payload := validApplicationPayload()
		payload["document_ids"] = []string{docs[0], docs[1], docs[0]}

		status, body := doJSON(t, "POST", "/api/v1/applications/", payload, authHeaders(profile.UserID, "applicant"))
		require.Equal(t, fiber.StatusOK, status, "body: %v", body)
		assert.Len(t, body["data"].(map[string]any)["documents"], 2)
	})

	t.Run("a foreign document rejects the submission and saves nothing", func(t *testing.T) {
		profile := seedProfile(t)
		other := seedProfile(t)
		payload := validApplicationPayload()
		payload["document_ids"] = []string{seedOwnedFile(t, profile.UserID), seedOwnedFile(t, other.UserID)}

		status, body := doJSON(t, "POST", "/api/v1/applications/", payload, authHeaders(profile.UserID, "applicant"))
		assert.Equal(t, fiber.StatusBadRequest, status, "body: %v", body)

		var saved int
		require.NoError(t, applicationTestDB.QueryRow(ctx,
			`SELECT COUNT(id) FROM applications WHERE applicant_id = $1`, profile.ProfileID).Scan(&saved))
		assert.Equal(t, 0, saved)
	})
}

// TestAddNoteAuthorFromToken: the note author is the admin from the token's
// user payload, not anything in the request.
func TestAddNoteAuthorFromToken(t *testing.T) {
	appID := seedApplication(t, seedProfile(t).ProfileID)
	status, body := doJSON(t, "POST", "/api/v1/applications/"+appID+"/notes",
		map[string]any{"text": "Check references", "author": "spoofed@example.com"}, authHeaders(1, "admin"))
	require.Equal(t, fiber.StatusOK, status, "body: %v", body)
	notes := body["data"].(map[string]any)["notes"].([]any)
	require.Len(t, notes, 1)
	assert.Equal(t, testIdentityEmail, notes[0].(map[string]any)["author"])
}

// TestTokenWithoutUserID: a signed token that carries no user id has no user
// payload, so applicant endpoints answer 401.
func TestTokenWithoutUserID(t *testing.T) {
	for _, ep := range []struct{ method, path string }{
		{"GET", "/api/v1/applications/mine"},
		{"POST", "/api/v1/applications/"},
		{"GET", "/api/v1/applications/" + uuid.New().String()},
	} {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			status, _ := doJSON(t, ep.method, ep.path, validApplicationPayload(), authHeaders(0, "applicant"))
			assert.Equal(t, fiber.StatusUnauthorized, status)
		})
	}
}

// seedSeatApplication inserts an application with a start date, work mode,
// session and status (for seat and availability tests).
func seedSeatApplication(t *testing.T, profileID string, start time.Time, workMode, session, status string) string {
	t.Helper()
	appID := uuid.New().String()
	_, err := applicationTestDB.Exec(context.Background(), `
		INSERT INTO applications (id, reference, applicant_id, department, specialization,
			motivation, experience, start_date, education_level, session, duration_months, work_mode, status)
		VALUES ($1, $2, $3, 'software', 'backend', 'Build things', '2 years',
			$4, 'university', $5, 3, $6, $7)
	`, appID, fmt.Sprintf("APP-%s", uniqueSuffix()), profileID, start, session, workMode, status)
	require.NoError(t, err)
	return appID
}

// seedNote inserts an internal review note on an application.
func seedNote(t *testing.T, appID, text string) {
	t.Helper()
	_, err := applicationTestDB.Exec(context.Background(), `
		INSERT INTO review_notes (id, application_id, author, text) VALUES ($1, $2, 'admin@example.com', $3)
	`, uuid.New().String(), appID, text)
	require.NoError(t, err)
}

// TestCreateApplicationAsGuest covers applying without logging in: the contact
// details create the account, a second try with the same email is refused, and
// a request that sends a bad token is never treated as a guest.
func TestCreateApplicationAsGuest(t *testing.T) {
	email := fmt.Sprintf("guest-%d@example.com", time.Now().UnixNano())
	guestPayload := func() map[string]any {
		payload := validApplicationPayload()
		payload["full_name"] = "Guest Applicant"
		payload["email"] = email
		payload["phone"] = "+250788123456"
		return payload
	}

	t.Run("missing contact details", func(t *testing.T) {
		status, _ := doJSON(t, "POST", "/api/v1/applications/", validApplicationPayload(), nil)
		assert.Equal(t, fiber.StatusBadRequest, status)
	})

	t.Run("documents need an account", func(t *testing.T) {
		payload := guestPayload()
		payload["document_ids"] = []string{uuid.New().String()}
		status, _ := doJSON(t, "POST", "/api/v1/applications/", payload, nil)
		assert.Equal(t, fiber.StatusBadRequest, status)
	})

	t.Run("creates the account and the application", func(t *testing.T) {
		status, body := doJSON(t, "POST", "/api/v1/applications/", guestPayload(), nil)
		require.Equal(t, fiber.StatusOK, status, "body: %v", body)

		var users int
		require.NoError(t, applicationTestDB.QueryRow(context.Background(),
			`SELECT COUNT(id) FROM users WHERE email_hash = $1 AND role = 'applicant'`,
			utils.HashData(email)).Scan(&users))
		assert.Equal(t, 1, users, "a user is created immediately")

		app, _ := body["data"].(map[string]any)
		applicant, _ := app["applicant"].(map[string]any)
		assert.Equal(t, email, applicant["email"])
	})

	t.Run("existing email is refused", func(t *testing.T) {
		status, _ := doJSON(t, "POST", "/api/v1/applications/", guestPayload(), nil)
		assert.Equal(t, fiber.StatusConflict, status)
	})

	t.Run("bad token is not a guest", func(t *testing.T) {
		status, _ := doJSON(t, "POST", "/api/v1/applications/", guestPayload(),
			map[string]string{"Authorization": "Bearer not-a-token"})
		assert.Equal(t, fiber.StatusUnauthorized, status)
	})
}

// seedGuestFile inserts a document a guest uploaded and confirmed with the
// given upload token but that no application has claimed yet.
func seedGuestFile(t *testing.T, token string) string {
	t.Helper()
	fileID := uuid.New().String()
	_, err := applicationTestDB.Exec(context.Background(), `
		INSERT INTO files (id, owner_id, owner_type, name, file_path, file_type, file_size, uploaded_at, deleted_at)
		VALUES ($1, $2, 'user', 'cv.pdf', $3, 'cv', 1024, NOW(), NOW() + INTERVAL '24 hours')
	`, fileID, utils.GuestUploadOwner(token), "uploads/"+fileID+".pdf")
	require.NoError(t, err)
	return fileID
}

// TestCreateApplicationAsGuestWithDocuments: a guest's uploaded documents are
// claimed by the application (re-owned by the new user, no longer expiring),
// and documents uploaded under someone else's token are refused.
func TestCreateApplicationAsGuestWithDocuments(t *testing.T) {
	token, err := utils.SecureRandString(utils.GuestUploadTokenBytes)
	require.NoError(t, err)
	otherToken, err := utils.SecureRandString(utils.GuestUploadTokenBytes)
	require.NoError(t, err)

	payload := func(email string, token string, fileIDs ...string) map[string]any {
		p := validApplicationPayload()
		p["full_name"] = "Guest Uploader"
		p["email"] = email
		p["phone"] = "+250788123456"
		p["document_ids"] = fileIDs
		if token != "" {
			p["guest_token"] = token
		}
		return p
	}
	newEmail := func() string { return fmt.Sprintf("guestdocs-%d@example.com", time.Now().UnixNano()) }

	mine := seedGuestFile(t, token)
	theirs := seedGuestFile(t, otherToken)

	t.Run("documents without a token are refused", func(t *testing.T) {
		status, _ := doJSON(t, "POST", "/api/v1/applications/", payload(newEmail(), "", mine), nil)
		assert.Equal(t, fiber.StatusBadRequest, status)
	})

	t.Run("someone else's documents are refused and no account is left behind", func(t *testing.T) {
		email := newEmail()
		status, _ := doJSON(t, "POST", "/api/v1/applications/", payload(email, token, theirs), nil)
		assert.Equal(t, fiber.StatusBadRequest, status)

		var users int
		require.NoError(t, applicationTestDB.QueryRow(context.Background(),
			`SELECT COUNT(id) FROM users WHERE email_hash = $1`, utils.HashData(email)).Scan(&users))
		assert.Equal(t, 0, users)
	})

	t.Run("own documents are claimed", func(t *testing.T) {
		email := newEmail()
		status, body := doJSON(t, "POST", "/api/v1/applications/", payload(email, token, mine), nil)
		require.Equal(t, fiber.StatusOK, status, "body: %v", body)

		docs := body["data"].(map[string]any)["documents"].([]any)
		require.Len(t, docs, 1, "the document is attached")

		var owner string
		var expiring bool
		require.NoError(t, applicationTestDB.QueryRow(context.Background(),
			`SELECT owner_id, deleted_at IS NOT NULL FROM files WHERE id = $1`, mine).Scan(&owner, &expiring))
		assert.NotEqual(t, utils.GuestUploadOwner(token), owner, "re-owned by the new user")
		assert.False(t, expiring, "no longer on the cleanup clock")

		status, _ = doJSON(t, "POST", "/api/v1/applications/", payload(newEmail(), token, mine), nil)
		assert.Equal(t, fiber.StatusBadRequest, status, "a claimed document can't be reused")
	})
}

func TestCreateApplicationAmountFromPrices(t *testing.T) {
	ctx := context.Background()
	_, err := applicationTestDB.Exec(ctx, `DELETE FROM prices WHERE specialization = 'backend'`)
	require.NoError(t, err)

	apply := func(months int) map[string]any {
		payload := validApplicationPayload()
		payload["full_name"] = "Amount Applicant"
		payload["email"] = fmt.Sprintf("amount-%d@example.com", time.Now().UnixNano())
		payload["phone"] = "+250788123456"
		payload["specialization"] = "backend"
		payload["duration_months"] = months
		payload["amount"] = 1 // a client-sent amount must be ignored
		status, body := doJSON(t, "POST", "/api/v1/applications/", payload, nil)
		require.Equal(t, fiber.StatusOK, status, "body: %v", body)
		app, _ := body["data"].(map[string]any)
		return app
	}

	t.Run("no price set uses 200000 per month", func(t *testing.T) {
		assert.Equal(t, 400000.0, apply(2)["amount"])
	})

	t.Run("admin price for the course and period is used", func(t *testing.T) {
		_, err := applicationTestDB.Exec(ctx, `
			INSERT INTO prices (department, specialization, duration_months, price)
			VALUES ('software', 'backend', 3, 450000)`)
		require.NoError(t, err)
		defer applicationTestDB.Exec(ctx, `DELETE FROM prices WHERE specialization = 'backend'`)

		assert.Equal(t, 450000.0, apply(3)["amount"])
		assert.Equal(t, 800000.0, apply(4)["amount"], "other periods still use the default")
	})
}
