package utils

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// NotificationEvent represents a notification to be sent
type NotificationEvent struct {
	Channel       string                 `json:"channel"` // EMAIL or IN_APP
	RecipientID   string                 `json:"recipient_id"`
	RecipientType string                 `json:"recipient_type"` // USER or EMAIL
	Subject       string                 `json:"subject"`
	Template      string                 `json:"template,omitempty"` // HTML template name (leazi pattern)
	Content       string                 `json:"content"`
	Data          map[string]interface{} `json:"data"`
	UserID        int                    `json:"user_id,omitempty"`
}

// PublishNotification publishes a notification event to Redis queue
func PublishNotification(ctx context.Context, rdb *redis.Client, event *NotificationEvent) error {
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal notification event: %w", err)
	}

	// Push to Redis queue for the notification worker to process
	err = rdb.LPush(ctx, "notification_queue", eventJSON).Err()
	if err != nil {
		return fmt.Errorf("failed to publish notification to queue: %w", err)
	}

	return nil
}

// PublishEmailNotification publishes an email notification event
func PublishEmailNotification(ctx context.Context, rdb *redis.Client, to, subject, body string, isHTML bool) error {
	event := &NotificationEvent{
		Channel:       "EMAIL",
		RecipientID:   to,
		RecipientType: "EMAIL",
		Subject:       subject,
		Content:       body,
		Data: map[string]interface{}{
			"is_html": isHTML,
		},
	}
	return PublishNotification(ctx, rdb, event)
}

// PublishTemplatedEmailNotification publishes an HTML email rendered from a
// named template (leazi pattern). The worker resolves the template from the
// templates directory and renders it with the provided data.
func PublishTemplatedEmailNotification(ctx context.Context, rdb *redis.Client, to, subject, templateName string, data map[string]interface{}) error {
	event := &NotificationEvent{
		Channel:       "EMAIL",
		RecipientID:   to,
		RecipientType: "EMAIL",
		Subject:       subject,
		Template:      templateName,
		Data:          data,
	}
	return PublishNotification(ctx, rdb, event)
}

// PublishInAppNotification publishes an in-app notification event
func PublishInAppNotification(ctx context.Context, rdb *redis.Client, userID int, title, message string) error {
	event := &NotificationEvent{
		Channel:       "IN_APP",
		RecipientID:   fmt.Sprintf("%d", userID),
		RecipientType: "USER",
		Subject:       title,
		Content:       message,
		UserID:        userID,
		Data: map[string]interface{}{
			"created_at": TimeNowRFC3339(),
		},
	}
	return PublishNotification(ctx, rdb, event)
}

// NotifyAdmins publishes a notification to every admin account over both
// channels (in-app + email). There can be more than one admin, so this fans out
// one IN_APP and one EMAIL event per admin. The email is rendered from
// emailTemplate with emailData (leazi pattern); message is used for the in-app
// copy. It is best-effort: a failure for a single admin does not stop the
// others; the first error is returned so the caller can log it without failing
// the surrounding request.
func NotifyAdmins(ctx context.Context, db *pgxpool.Pool, rdb *redis.Client, subject, message, emailTemplate string, emailData map[string]interface{}) error {
	rows, err := db.Query(ctx, "SELECT id, email_encrypted FROM users WHERE role = $1", "admin")
	if err != nil {
		return fmt.Errorf("failed to resolve admins: %w", err)
	}
	defer rows.Close()

	type admin struct {
		id            int
		emailEncBytes []byte
	}

	var admins []admin
	for rows.Next() {
		var a admin
		if err := rows.Scan(&a.id, &a.emailEncBytes); err != nil {
			return fmt.Errorf("failed to scan admin: %w", err)
		}
		admins = append(admins, a)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to read admins: %w", err)
	}

	var firstErr error
	for _, a := range admins {
		if err := PublishInAppNotification(ctx, rdb, a.id, subject, message); err != nil && firstErr == nil {
			firstErr = err
		}

		email, err := DecryptData(ctx, db, a.emailEncBytes)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("failed to decrypt admin email: %w", err)
			}
			continue
		}
		if err := PublishTemplatedEmailNotification(ctx, rdb, email, subject, emailTemplate, emailData); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}
