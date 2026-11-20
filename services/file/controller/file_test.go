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

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/dgrijalva/jwt-go"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qonics/qonics-appy-api/libs/shared-package/config"
	"github.com/qonics/qonics-appy-api/libs/shared-package/middleware"
	"github.com/qonics/qonics-appy-api/libs/shared-package/utils"
	"github.com/qonics/qonics-appy-api/services/file/helper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Shared harness, mirroring leazi's controller tests: one app + one DB set up
// once, then every route is exercised by a single table-driven test function
// carrying several scenario payloads.
var (
	fileTestApp *fiber.App
	fileTestDB  *pgxpool.Pool
	fileTestCfg *config.Config
)

// testJWTSecret signs the tokens consumed by the real auth middleware, so
// authenticated and unauthenticated requests flow through the same production
// code path.
const testJWTSecret = "file-test-secret"

// testIdentityEmail is attached to the identity carried by the test tokens.
const testIdentityEmail = "tester@example.com"

func init() {
	utils.IsTestMode = true

	if err := os.Chdir("../../.."); err != nil {
		panic("Failed to change directory: " + err.Error())
	}

	cfg, err := config.LoadConfig("file-service")
	if err != nil {
		panic("Failed to load config: " + err.Error())
	}
	fileTestCfg = cfg

	db, err := cfg.InitDB()
	if err != nil {
		panic("Failed to connect to test database: " + err.Error())
	}
	if _, err := db.Exec(context.Background(), "SET app.encryption_key = 'secret'"); err != nil {
		panic("Failed to set encryption key: " + err.Error())
	}
	fileTestDB = db

	controller := NewFileController(db, cfg)

	// R2 is stubbed with a client whose credentials are static, so presigning
	// (a local signing operation) succeeds without any network access.
	helper.R2ClientFactory = func() (*helper.R2Client, error) {
		return newTestR2Client(), nil
	}

	fileTestApp = fiber.New(fiber.Config{AppName: "file-service-test"})

	// Mirror routes/api.go with the real auth middleware.
	auth := middleware.AuthMiddleware(testJWTSecret)
	files := fileTestApp.Group("/api/v1/files")
	files.Post("/guest/presigned-url", controller.GuestPresignedURL)
	files.Post("/guest/confirm-upload", controller.GuestConfirmUpload)
	files.Use(auth)
	files.Post("/presigned-url", controller.GetPresignedURL)
	files.Post("/confirm-upload", controller.ConfirmFileUpload)
	files.Get("/:file_id/url", controller.GetFileURL)
}

// newTestR2Client builds an R2 client backed by static credentials and a fake
// endpoint; presigning computes a signed URL locally and never hits the wire.
func newTestR2Client() *helper.R2Client {
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion("auto"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test-key", "test-secret", "")),
	)
	if err != nil {
		panic("Failed to build test R2 client: " + err.Error())
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String("https://test-account.r2.cloudflarestorage.com")
	})
	return &helper.R2Client{Client: client, BucketName: "test-bucket"}
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

	resp, err := fileTestApp.Test(req)
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

// seedFile inserts a file row owned by the given user. A pending file has
// deleted_at set in the future (unconfirmed); a confirmed one has deleted_at
// cleared and uploaded_at set.
func seedFile(t *testing.T, ownerUserID int, pending bool) string {
	t.Helper()

	fileID := uuid.New().String()
	var deletedAt, uploadedAt any
	if pending {
		deletedAt = time.Now().Add(24 * time.Hour)
	} else {
		uploadedAt = time.Now()
	}

	_, err := fileTestDB.Exec(context.Background(), `
		INSERT INTO files (id, owner_id, owner_type, name, file_path, file_type, file_size, uploaded_at, deleted_at)
		VALUES ($1, $2, 'user', 'cv.pdf', $3, 'cv', 1024, $4, $5)
	`, fileID, fmt.Sprintf("%d", ownerUserID), "uploads/"+fileID+".pdf", uploadedAt, deletedAt)
	require.NoError(t, err)
	return fileID
}

// validPresignedPayload returns a payload that passes validation.
func validPresignedPayload() map[string]any {
	return map[string]any{
		"file_name":   "cv.pdf",
		"file_type":   "cv",
		"file_size":   1024,
		"description": "my cv",
	}
}

// TestGetPresignedURLRoute covers POST /api/v1/files/presigned-url.
func TestGetPresignedURLRoute(t *testing.T) {
	auth := authHeaders(4242, "applicant")

	withField := func(key string, value any) map[string]any {
		payload := validPresignedPayload()
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
			payload:    validPresignedPayload(),
			wantStatus: fiber.StatusUnauthorized,
		},
		{
			name:       "validation - missing required fields",
			headers:    auth,
			payload:    map[string]any{},
			wantStatus: fiber.StatusBadRequest,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "file_name"), "expected file_name error: %v", body)
			},
		},
		{
			name:       "validation - missing file_type",
			headers:    auth,
			payload:    withField("file_type", ""),
			wantStatus: fiber.StatusBadRequest,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "file_type"), "expected file_type error: %v", body)
			},
		},
		{
			name:       "validation - unknown document type",
			headers:    auth,
			payload:    withField("file_type", "selfie"),
			wantStatus: fiber.StatusBadRequest,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "file_type"), "expected file_type error: %v", body)
			},
		},
		{
			name:       "validation - file larger than 5 MB",
			headers:    auth,
			payload:    withField("file_size", 5*1024*1024+1),
			wantStatus: fiber.StatusBadRequest,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "file_size"), "expected file_size error: %v", body)
			},
		},
		{
			name:       "bad request - disallowed extension",
			headers:    auth,
			payload:    withField("file_name", "page.html"),
			wantStatus: fiber.StatusBadRequest,
		},
		{
			name:       "success - uppercase extension accepted",
			headers:    auth,
			payload:    withField("file_name", "CV.PDF"),
			wantStatus: fiber.StatusOK,
		},
		{
			name:       "success - returns presigned url and pending file id",
			headers:    auth,
			payload:    validPresignedPayload(),
			wantStatus: fiber.StatusOK,
			check: func(t *testing.T, body map[string]any) {
				data, ok := body["data"].(map[string]any)
				require.True(t, ok, "response should have data: %v", body)
				assert.NotEmpty(t, data["file_id"])
				assert.NotEmpty(t, data["presigned_url"])
				assert.NotEmpty(t, data["object_key"])
				assert.Equal(t, float64(900), data["expires_in"])
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, body := doJSON(t, "POST", "/api/v1/files/presigned-url", test.payload, test.headers)
			assert.Equal(t, test.wantStatus, status)
			if test.check != nil {
				test.check(t, body)
			}
		})
	}
}

// TestConfirmFileUploadRoute covers POST /api/v1/files/confirm-upload.
func TestConfirmFileUploadRoute(t *testing.T) {
	owner, sourceID := seedApplicantWithApplication(t)
	_, someoneElsesApp := seedApplicantWithApplication(t)
	auth := authHeaders(owner, "applicant")

	pending := seedFile(t, owner, true)
	confirmed := seedFile(t, owner, false)
	otherPending := seedFile(t, 6161, true)
	successPending := seedFile(t, owner, true)
	hijackPending := seedFile(t, owner, true)

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
			payload:    map[string]any{"file_id": pending},
			wantStatus: fiber.StatusUnauthorized,
		},
		{
			name:       "validation - missing file_id",
			headers:    auth,
			payload:    map[string]any{},
			wantStatus: fiber.StatusBadRequest,
			check: func(t *testing.T, body map[string]any) {
				assert.True(t, hasValidationError(body, "file_id"), "expected file_id error: %v", body)
			},
		},
		{
			name:       "bad request - invalid source table",
			headers:    auth,
			payload:    map[string]any{"file_id": pending, "source_table": "users"},
			wantStatus: fiber.StatusBadRequest,
		},
		{
			name:       "bad request - source table without id",
			headers:    auth,
			payload:    map[string]any{"file_id": pending, "source_table": "applications"},
			wantStatus: fiber.StatusBadRequest,
		},
		{
			name:       "bad request - source id without table",
			headers:    auth,
			payload:    map[string]any{"file_id": pending, "source_table_id": sourceID},
			wantStatus: fiber.StatusBadRequest,
		},
		{
			name:       "not found - unknown file",
			headers:    auth,
			payload:    map[string]any{"file_id": uuid.New().String()},
			wantStatus: fiber.StatusNotFound,
		},
		{
			name:       "forbidden - ownership mismatch",
			headers:    auth,
			payload:    map[string]any{"file_id": otherPending},
			wantStatus: fiber.StatusForbidden,
		},
		{
			name:       "conflict - already confirmed",
			headers:    auth,
			payload:    map[string]any{"file_id": confirmed},
			wantStatus: fiber.StatusConflict,
		},
		{
			name:    "forbidden - attach to another applicant's application",
			headers: auth,
			payload: map[string]any{
				"file_id":         hijackPending,
				"source_table":    "applications",
				"source_table_id": someoneElsesApp,
			},
			wantStatus: fiber.StatusForbidden,
		},
		{
			name:    "success - confirms upload and attaches source",
			headers: auth,
			payload: map[string]any{
				"file_id":         successPending,
				"source_table":    "applications",
				"source_table_id": sourceID,
				"description":     "attached",
			},
			wantStatus: fiber.StatusOK,
			check: func(t *testing.T, body map[string]any) {
				data, ok := body["data"].(map[string]any)
				require.True(t, ok, "response should have data: %v", body)
				assert.Equal(t, "applications", data["source_table"])
				assert.Equal(t, sourceID, data["source_table_id"])
				assert.NotEmpty(t, data["url"])
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, body := doJSON(t, "POST", "/api/v1/files/confirm-upload", test.payload, test.headers)
			assert.Equal(t, test.wantStatus, status)
			if test.check != nil {
				test.check(t, body)
			}
		})
	}

	// The confirmed file must now be permanent (deleted_at cleared).
	var stillPending bool
	err := fileTestDB.QueryRow(context.Background(),
		"SELECT deleted_at IS NOT NULL FROM files WHERE id = $1", successPending).Scan(&stillPending)
	require.NoError(t, err)
	assert.False(t, stillPending, "confirmed file should no longer be pending")
}

// TestGetFileURLRoute covers GET /api/v1/files/:file_id/url.
func TestGetFileURLRoute(t *testing.T) {
	const owner = 7272
	const other = 8282

	ownerAuth := authHeaders(owner, "applicant")
	adminAuth := authHeaders(1, "admin")

	ownConfirmed := seedFile(t, owner, false)
	pendingFile := seedFile(t, owner, true)
	otherConfirmed := seedFile(t, other, false)

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
			path:       "/api/v1/files/" + ownConfirmed + "/url",
			wantStatus: fiber.StatusUnauthorized,
		},
		{
			name:       "not found - unknown file",
			headers:    ownerAuth,
			path:       "/api/v1/files/" + uuid.New().String() + "/url",
			wantStatus: fiber.StatusNotFound,
		},
		{
			name:       "not found - pending file",
			headers:    ownerAuth,
			path:       "/api/v1/files/" + pendingFile + "/url",
			wantStatus: fiber.StatusNotFound,
		},
		{
			name:       "forbidden - another user's file",
			headers:    ownerAuth,
			path:       "/api/v1/files/" + otherConfirmed + "/url",
			wantStatus: fiber.StatusForbidden,
		},
		{
			name:       "success - owner reads own file",
			headers:    ownerAuth,
			path:       "/api/v1/files/" + ownConfirmed + "/url",
			wantStatus: fiber.StatusOK,
			check: func(t *testing.T, body map[string]any) {
				data, ok := body["data"].(map[string]any)
				require.True(t, ok, "response should have data: %v", body)
				assert.Equal(t, ownConfirmed, data["file_id"])
				assert.NotEmpty(t, data["url"])
			},
		},
		{
			name:       "success - admin reads any file",
			headers:    adminAuth,
			path:       "/api/v1/files/" + otherConfirmed + "/url",
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

// TestUnauthorizedAccess asserts every protected route rejects callers without a
// token (leazi-style single table over the whole surface).
func TestUnauthorizedAccess(t *testing.T) {
	endpoints := []struct {
		method string
		path   string
	}{
		{"POST", "/api/v1/files/presigned-url"},
		{"POST", "/api/v1/files/confirm-upload"},
		{"GET", "/api/v1/files/" + uuid.New().String() + "/url"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			status, _ := doJSON(t, ep.method, ep.path, nil, nil)
			assert.Equal(t, fiber.StatusUnauthorized, status)
		})
	}
}

// TestPresignedURLRefusedWhileIntakeClosed: applicants can't upload documents
// while applications are closed (they could never be attached); admins can.
func TestPresignedURLRefusedWhileIntakeClosed(t *testing.T) {
	ctx := context.Background()
	setOpen := func(open bool, message string) {
		t.Helper()
		_, err := fileTestDB.Exec(ctx, `
			INSERT INTO settings (id, applications_open, close_on, closure_message)
			VALUES (1, $1, NULL, $2)
			ON CONFLICT (id) DO UPDATE SET applications_open = $1, close_on = NULL, closure_message = $2
		`, open, message)
		require.NoError(t, err)
	}
	t.Cleanup(func() { setOpen(true, "") })

	setOpen(false, "Back in January")
	status, body := doJSON(t, "POST", "/api/v1/files/presigned-url", validPresignedPayload(), authHeaders(4243, "applicant"))
	assert.Equal(t, fiber.StatusForbidden, status, "body: %v", body)
	assert.Equal(t, "Back in January", body["message"])

	status, body = doJSON(t, "POST", "/api/v1/files/presigned-url", validPresignedPayload(), authHeaders(1, "admin"))
	assert.Equal(t, fiber.StatusOK, status, "admins are not blocked: %v", body)

	setOpen(true, "")
	status, body = doJSON(t, "POST", "/api/v1/files/presigned-url", validPresignedPayload(), authHeaders(4243, "applicant"))
	assert.Equal(t, fiber.StatusOK, status, "open again: %v", body)
}

// seedApplicantWithApplication creates an applicant user, their profile and one
// application, returning the user id and the application id.
func seedApplicantWithApplication(t *testing.T) (int, string) {
	t.Helper()
	ctx := context.Background()
	suffix := uniqueSuffix()
	email := "file-owner-" + suffix + "@example.com"

	var userID int
	err := fileTestDB.QueryRow(ctx, `
		INSERT INTO users (full_name_encrypted, full_name_hash, email_encrypted, email_hash,
			phone_encrypted, phone_hash, password_hash, role)
		VALUES (pgp_sym_encrypt('File Owner', 'secret'), $1, pgp_sym_encrypt($2, 'secret'), $3,
			pgp_sym_encrypt('+250788000000', 'secret'), $4, 'not-a-real-hash', 'applicant')
		RETURNING id
	`, "name-"+suffix, email, "email-"+suffix, "phone-"+suffix).Scan(&userID)
	require.NoError(t, err)

	profileID := uuid.New().String()
	_, err = fileTestDB.Exec(ctx, `
		INSERT INTO applicant_profiles (id, user_id, full_name_encrypted, full_name_hash,
			email_encrypted, email_hash, phone_encrypted, phone_hash)
		SELECT $1, id, full_name_encrypted, full_name_hash, email_encrypted, email_hash,
			phone_encrypted, phone_hash
		FROM users WHERE id = $2
	`, profileID, userID)
	require.NoError(t, err)

	appID := uuid.New().String()
	_, err = fileTestDB.Exec(ctx, `
		INSERT INTO applications (id, reference, applicant_id, department, specialization,
			motivation, experience, education_level, session, duration_months, work_mode, status)
		VALUES ($1, $2, $3, 'software', 'backend', 'Build things', '2 years',
			'university', 'morning', 6, 'remote', 'submitted')
	`, appID, "APP-"+suffix, profileID)
	require.NoError(t, err)

	return userID, appID
}

// TestGuestUploadFlow covers uploading documents without an account: the first
// request mints a token, the files are owned by a hash of it, confirming keeps
// the pending expiry (so unclaimed files are cleaned up), and nobody else can
// confirm them.
func TestGuestUploadFlow(t *testing.T) {
	ctx := context.Background()

	status, body := doJSON(t, "POST", "/api/v1/files/guest/presigned-url", validPresignedPayload(), nil)
	require.Equal(t, fiber.StatusOK, status, "body: %v", body)
	data := body["data"].(map[string]any)
	token := data["guest_token"].(string)
	fileID := data["file_id"].(string)
	require.True(t, utils.ValidGuestUploadToken(token), "a valid token is issued")

	var owner string
	require.NoError(t, fileTestDB.QueryRow(ctx, `SELECT owner_id FROM files WHERE id = $1`, fileID).Scan(&owner))
	assert.Equal(t, utils.GuestUploadOwner(token), owner)
	assert.NotContains(t, owner, token, "the token itself is never stored")

	t.Run("wrong token cannot confirm", func(t *testing.T) {
		other := "00000000000000000000000000000000"
		status, _ := doJSON(t, "POST", "/api/v1/files/guest/confirm-upload",
			map[string]any{"file_id": fileID, "guest_token": other}, nil)
		assert.Equal(t, fiber.StatusNotFound, status)
	})

	t.Run("confirm keeps the pending expiry", func(t *testing.T) {
		status, body := doJSON(t, "POST", "/api/v1/files/guest/confirm-upload",
			map[string]any{"file_id": fileID, "guest_token": token}, nil)
		require.Equal(t, fiber.StatusOK, status, "body: %v", body)

		var uploaded, pending bool
		require.NoError(t, fileTestDB.QueryRow(ctx,
			`SELECT uploaded_at IS NOT NULL, deleted_at > NOW() FROM files WHERE id = $1`, fileID).Scan(&uploaded, &pending))
		assert.True(t, uploaded)
		assert.True(t, pending, "unclaimed guest files stay on the cleanup clock")

		status, _ = doJSON(t, "POST", "/api/v1/files/guest/confirm-upload",
			map[string]any{"file_id": fileID, "guest_token": token}, nil)
		assert.Equal(t, fiber.StatusNotFound, status, "a second confirm is refused")
	})

	t.Run("the same token keeps adding files, up to the cap", func(t *testing.T) {
		payload := validPresignedPayload()
		payload["guest_token"] = token
		for i := 1; i < utils.MaxGuestFiles; i++ {
			status, body := doJSON(t, "POST", "/api/v1/files/guest/presigned-url", payload, nil)
			require.Equal(t, fiber.StatusOK, status, "upload %d: %v", i+1, body)
			assert.Equal(t, token, body["data"].(map[string]any)["guest_token"])
		}
		status, _ := doJSON(t, "POST", "/api/v1/files/guest/presigned-url", payload, nil)
		assert.Equal(t, fiber.StatusBadRequest, status, "over the cap")
	})

	t.Run("validation", func(t *testing.T) {
		bad := validPresignedPayload()
		bad["file_name"] = "virus.exe"
		status, _ := doJSON(t, "POST", "/api/v1/files/guest/presigned-url", bad, nil)
		assert.Equal(t, fiber.StatusBadRequest, status, "only documents")

		bad = validPresignedPayload()
		bad["guest_token"] = "not-a-token"
		status, _ = doJSON(t, "POST", "/api/v1/files/guest/presigned-url", bad, nil)
		assert.Equal(t, fiber.StatusBadRequest, status, "malformed token")
	})
}
