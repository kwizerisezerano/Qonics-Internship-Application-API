package utils

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// NotificationEvent represents a notification to be sent
type NotificationEvent struct {
	Channel       string                 `json:"channel"`       // EMAIL or IN_APP
	RecipientID   string                 `json:"recipient_id"`
	RecipientType string                 `json:"recipient_type"` // USER or EMAIL
	Subject       string                 `json:"subject"`
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
