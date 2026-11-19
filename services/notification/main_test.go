package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/qonics/qonics-appy-api/libs/shared-package/config"
	"github.com/qonics/qonics-appy-api/libs/shared-package/utils"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateHtmlTemplate(t *testing.T) {
	// Point the shared helper at this service's templates directory regardless
	// of the current working directory (other tests may chdir).
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	utils.EmailTemplatesDir = filepath.Join(filepath.Dir(thisFile), "templates")

	tests := []struct {
		name        string
		template    string
		data        map[string]any
		wantContain []string
	}{
		{
			name:     "application submitted",
			template: "application_submitted",
			data: map[string]any{
				"name":           "Alice",
				"reference":      "APP-123",
				"department":     "software",
				"specialization": "backend",
				"Year":           2026,
			},
			wantContain: []string{"Alice", "APP-123", "software", "backend"},
		},
		{
			name:     "status updated",
			template: "application_status_updated",
			data: map[string]any{
				"name":         "Bob",
				"reference":    "APP-456",
				"status":       "Interview",
				"note":         "Please join at 10:00",
				"interview_at": "Monday, 05 January 2026 at 10:00",
				"Year":         2026,
			},
			wantContain: []string{"Bob", "APP-456", "Interview", "Monday, 05 January 2026 at 10:00", "Please join at 10:00"},
		},
		{
			name:     "admin new application",
			template: "admin_new_application",
			data: map[string]any{
				"applicant_name": "Carol",
				"reference":      "APP-789",
				"department":     "iot",
				"specialization": "embedded",
				"Year":           2026,
			},
			wantContain: []string{"Carol", "APP-789", "iot", "embedded"},
		},
		{
			name:     "user welcome",
			template: "user_welcome",
			data: map[string]any{
				"name":      "Dave",
				"email":     "dave@example.com",
				"login_url": "https://app.qonics.com/login",
				"Year":      2026,
			},
			wantContain: []string{"Dave", "dave@example.com", "https://app.qonics.com/login"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rendered, err := utils.GenerateHtmlTemplate(tt.template, tt.data)
			require.NoError(t, err)
			for _, want := range tt.wantContain {
				assert.Contains(t, rendered, want)
			}
			assert.Contains(t, rendered, "res.cloudinary.com") // shared header partial rendered (logo)
			assert.Contains(t, rendered, "Do not reply")       // shared footer partial rendered
		})
	}
}

func TestBuildEmailMessage(t *testing.T) {
	tests := []struct {
		name        string
		from        string
		to          string
		subject     string
		body        string
		contentType string
		wantHeaders []string
		wantAbsent  []string
		wantBody    string
	}{
		{
			name:        "plain text message",
			from:        "noreply@qonics.com",
			to:          "student@example.com",
			subject:     "Application Submitted",
			body:        "Your application was received.",
			contentType: "text/plain; charset=UTF-8",
			wantHeaders: []string{
				"From: noreply@qonics.com\r\n",
				"To: student@example.com\r\n",
				"Subject: Application Submitted\r\n",
				"MIME-Version: 1.0\r\n",
				"Content-Type: text/plain; charset=UTF-8\r\n",
			},
			wantBody: "Your application was received.",
		},
		{
			name:        "html message",
			from:        "noreply@qonics.com",
			to:          "admin@qonics.com",
			subject:     "New Application",
			body:        "<p>Hello</p>",
			contentType: "text/html; charset=UTF-8",
			wantHeaders: []string{
				"Content-Type: text/html; charset=UTF-8\r\n",
			},
			wantBody: "<p>Hello</p>",
		},
		{
			name:        "header injection is sanitised",
			from:        "noreply@qonics.com\r\nBcc: attacker@evil.com",
			to:          "student@example.com\nX-Evil: 1",
			subject:     "Hi\r\nBcc: attacker@evil.com",
			body:        "body",
			contentType: "text/plain; charset=UTF-8",
			wantHeaders: []string{
				"Subject: Hi Bcc: attacker@evil.com\r\n",
			},
			wantAbsent: []string{
				"\r\nBcc: attacker@evil.com\r\n",
				"\nX-Evil: 1",
			},
			wantBody: "body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := string(buildEmailMessage(tt.from, tt.to, tt.subject, tt.body, tt.contentType))

			for _, h := range tt.wantHeaders {
				assert.Contains(t, msg, h)
			}
			for _, a := range tt.wantAbsent {
				assert.NotContains(t, msg, a)
			}

			// Headers and body are separated by a single blank CRLF line.
			parts := strings.SplitN(msg, "\r\n\r\n", 2)
			require.Len(t, parts, 2)
			assert.Equal(t, tt.wantBody, parts[1])
		})
	}
}

func TestNotificationWorkerInit(t *testing.T) {
	utils.IsTestMode = true

	// Load .env from project root
	err := os.Chdir("../..")
	if err != nil {
		panic("Failed to change directory: " + err.Error())
	}

	cfg, err := config.LoadConfig("notification-worker")
	require.NoError(t, err)

	db, err := cfg.InitDB()
	require.NoError(t, err)
	defer db.Close()

	// Test database connection
	ctx := context.Background()
	var result int
	err = db.QueryRow(ctx, "SELECT 1").Scan(&result)
	require.NoError(t, err)
	assert.Equal(t, 1, result)
}

func TestMaskRecipient(t *testing.T) {
	assert.Equal(t, "j***@example.com", maskRecipient("jane.doe@example.com"))
	assert.Equal(t, "a***@x.io", maskRecipient("a@x.io"))
	assert.Equal(t, "42", maskRecipient("42"), "user ids are not masked")
	assert.Equal(t, "", maskRecipient(""))
}

func TestRetryDelay(t *testing.T) {
	assert.Equal(t, 30*time.Second, retryDelay(1))
	assert.Equal(t, 60*time.Second, retryDelay(2))
}

// TestRetryQueue covers scheduling a failed job and moving it back onto the
// main queue once due. It uses Redis logical database 15, never the live one.
func TestRetryQueue(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379", DB: 15})
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("redis not available: %v", err)
	}
	rdb.Del(ctx, retryQueue, notificationQueue)
	t.Cleanup(func() { rdb.Del(ctx, retryQueue, notificationQueue) })

	event := &NotificationEvent{Channel: "EMAIL", RecipientID: "a@example.com", Subject: "Hi"}

	retrying, err := scheduleRetry(ctx, rdb, event)
	require.NoError(t, err)
	assert.True(t, retrying, "first failure should be retried")
	assert.Equal(t, 1, event.Attempts)

	// Not due yet: nothing moves.
	promoteDueRetries(ctx, rdb)
	assert.Equal(t, int64(0), rdb.LLen(ctx, notificationQueue).Val())

	// Make it due, then it moves back to the main queue exactly once.
	members := rdb.ZRange(ctx, retryQueue, 0, -1).Val()
	require.Len(t, members, 1)
	rdb.ZAdd(ctx, retryQueue, redis.Z{Score: 0, Member: members[0]})
	promoteDueRetries(ctx, rdb)
	promoteDueRetries(ctx, rdb)
	assert.Equal(t, int64(1), rdb.LLen(ctx, notificationQueue).Val())
	assert.Equal(t, int64(0), rdb.ZCard(ctx, retryQueue).Val())

	// The last allowed failure gives up instead of scheduling another retry.
	event.Attempts = maxAttempts - 1
	retrying, err = scheduleRetry(ctx, rdb, event)
	require.NoError(t, err)
	assert.False(t, retrying)
}
