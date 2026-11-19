package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/qonics/qonics-appy-api/libs/shared-package/config"
	"github.com/qonics/qonics-appy-api/libs/shared-package/logger"
	"github.com/qonics/qonics-appy-api/libs/shared-package/utils"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/semaphore"
)

// defaultLogoURL is the Qonics app logo used in the shared email header. It is
// injected into every templated email unless a caller overrides LogoUrl.
const defaultLogoURL = "https://res.cloudinary.com/danzeqybu/image/upload/v1795014256/WhatsApp_Image_2026-11-18_at_5.02.48_PM_rudk0a.jpg"

type NotificationEvent struct {
	Channel       string         `json:"channel"` // EMAIL or IN_APP
	RecipientID   string         `json:"recipient_id"`
	RecipientType string         `json:"recipient_type"` // USER or EMAIL
	Subject       string         `json:"subject"`
	Template      string         `json:"template,omitempty"` // HTML template name (leazi pattern)
	Content       string         `json:"content"`
	Data          map[string]any `json:"data"`
	UserID        int            `json:"user_id,omitempty"`
	// Attempts counts failed deliveries so far (worker-internal, for retries).
	Attempts int `json:"attempts,omitempty"`
}

func main() {
	// Load .env file (for Docker environment)
	godotenv.Load()

	// Load configuration
	cfg, err := config.LoadConfig("notification-worker")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Override config with environment variables if set (for Docker)
	if os.Getenv("DATABASE_HOST") != "" {
		cfg.Database.Host = os.Getenv("DATABASE_HOST")
		cfg.Database.Port = os.Getenv("DATABASE_PORT")
		cfg.Database.User = os.Getenv("DATABASE_USER")
		cfg.Database.Password = os.Getenv("DATABASE_PASSWORD")
		cfg.Database.DBName = os.Getenv("DATABASE_DBNAME")
		cfg.Database.SSLMode = os.Getenv("DATABASE_SSLMODE")
	}
	if os.Getenv("REDIS_HOST") != "" {
		cfg.Redis.Host = os.Getenv("REDIS_HOST")
		cfg.Redis.Port = os.Getenv("REDIS_PORT")
		cfg.Redis.Password = os.Getenv("REDIS_PASSWORD")
		cfg.Redis.DB, _ = strconv.Atoi(os.Getenv("REDIS_DB"))
	}
	if os.Getenv("SMTP_HOST") != "" {
		cfg.Email.Host = os.Getenv("SMTP_HOST")
		cfg.Email.Port = os.Getenv("SMTP_PORT")
		cfg.Email.User = os.Getenv("SMTP_USER")
		cfg.Email.Password = os.Getenv("SMTP_PASSWORD")
		cfg.Email.From = os.Getenv("SMTP_FROM")
	}

	// Initialize logger
	if err := logger.InitLogger(cfg.ServiceName, "info"); err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}

	// Initialize Redis
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Host + ":" + cfg.Redis.Port,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	defer rdb.Close()

	// Initialize database
	db, err := cfg.InitDB()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Test Redis connection
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	log.Println("Notification worker started, waiting for jobs...")

	// Move failed jobs back onto the queue once their retry delay has passed.
	// The size-1 semaphore guarantees only one sweep runs at a time, even if
	// one is slow; a tick that finds a sweep still running is skipped.
	sweepSlot := semaphore.NewWeighted(1)
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if !sweepSlot.TryAcquire(1) {
				continue
			}
			go func() {
				defer sweepSlot.Release(1)
				promoteDueRetries(ctx, rdb)
			}()
		}
	}()

	// Jobs are processed concurrently (a slow SMTP send no longer holds up
	// the queue), with at most maxConcurrentJobs in flight. A slot is taken
	// before popping, so a job is only removed from Redis when it can start.
	jobSlots := semaphore.NewWeighted(maxConcurrentJobs)

	// Process notification jobs
	for {
		if err := jobSlots.Acquire(ctx, 1); err != nil {
			log.Fatalf("Failed to acquire job slot: %v", err)
		}

		// Blocking pop from the notification queue
		result, err := rdb.BRPop(ctx, 0*time.Second, notificationQueue).Result()
		if err != nil || len(result) < 2 {
			jobSlots.Release(1)
			if err != nil {
				log.Printf("Error reading from queue: %v", err)
				time.Sleep(5 * time.Second)
			}
			continue
		}

		// result[0] is the queue name, result[1] is the value
		go func(jobData string) {
			defer jobSlots.Release(1)
			handleJob(ctx, rdb, db, cfg, jobData)
		}(result[1])
	}
}

func processNotification(ctx context.Context, rdb *redis.Client, db *pgxpool.Pool, event *NotificationEvent, cfg *config.Config) error {
	// Record initial communication log
	logID, err := recordCommunicationLog(ctx, db, event, "pending", "")
	if err != nil {
		log.Printf("Failed to record initial communication log: %v", err)
	}

	status := "failed"
	errorMessage := ""

	// Process based on channel type
	switch event.Channel {
	case "EMAIL":
		err = sendEmail(event, cfg)
		if err != nil {
			errorMessage = err.Error()
		} else {
			status = "sent"
		}
	case "IN_APP":
		err = sendInAppNotification(ctx, db, event)
		if err != nil {
			errorMessage = err.Error()
		} else {
			status = "sent"
		}
	default:
		errorMessage = fmt.Sprintf("Unknown notification channel: %s", event.Channel)
		log.Printf("Warning: %s", errorMessage)
	}

	// Update communication log status
	if logID > 0 {
		updateCommunicationLogStatus(ctx, db, logID, status, errorMessage)
	}

	return err
}

func sendEmail(event *NotificationEvent, cfg *config.Config) error {
	if cfg.Email.Host == "" || cfg.Email.User == "" {
		return fmt.Errorf("email transport is not configured (SMTP_HOST/SMTP_USER missing)")
	}
	if event.RecipientID == "" {
		return fmt.Errorf("email recipient is empty")
	}

	// Prefer an HTML template when one is set (leazi pattern); fall back to the
	// plain content if rendering fails or no template was provided.
	var (
		body        string
		contentType string
	)
	if event.Template != "" {
		// Inject shared template data used by the header/footer partials.
		if event.Data == nil {
			event.Data = map[string]any{}
		}
		if _, ok := event.Data["Year"]; !ok {
			event.Data["Year"] = time.Now().Year()
		}
		if _, ok := event.Data["LogoUrl"]; !ok {
			event.Data["LogoUrl"] = defaultLogoURL
		}

		rendered, err := utils.GenerateHtmlTemplate(event.Template, event.Data)
		if err != nil {
			log.Printf("WARNING: failed to render email template %q: %v", event.Template, err)
			body = event.Content
			contentType = "text/plain; charset=UTF-8"
		} else {
			body = rendered
			contentType = "text/html; charset=UTF-8"
		}
	} else {
		isHTML, _ := event.Data["is_html"].(bool)
		body = event.Content
		contentType = "text/plain; charset=UTF-8"
		if isHTML {
			contentType = "text/html; charset=UTF-8"
		}
	}

	from := cfg.Email.From
	if from == "" {
		from = cfg.Email.User
	}

	msg := buildEmailMessage(from, event.RecipientID, event.Subject, body, contentType)

	// App passwords (e.g. Gmail) are displayed with spaces for readability; the
	// actual credential has none, so strip them before authenticating.
	password := strings.ReplaceAll(cfg.Email.Password, " ", "")
	auth := smtp.PlainAuth("", cfg.Email.User, password, cfg.Email.Host)

	addr := cfg.Email.Host + ":" + cfg.Email.Port
	if err := smtp.SendMail(addr, auth, from, []string{event.RecipientID}, msg); err != nil {
		return fmt.Errorf("smtp send failed: %w", err)
	}

	log.Printf("[EMAIL] Sent to %s (subject=%q, template=%q)", maskRecipient(event.RecipientID), event.Subject, event.Template)
	return nil
}

// buildEmailMessage assembles a minimal RFC 5322 message. Header values are
// sanitised (CR/LF removed) to prevent header injection.
func buildEmailMessage(from, to, subject, body, contentType string) []byte {
	sanitize := strings.NewReplacer("\r", "", "\n", " ").Replace

	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", sanitize(from))
	fmt.Fprintf(&b, "To: %s\r\n", sanitize(to))
	fmt.Fprintf(&b, "Subject: %s\r\n", sanitize(subject))
	fmt.Fprintf(&b, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&b, "Content-Type: %s\r\n", contentType)
	fmt.Fprintf(&b, "\r\n%s", body)
	return []byte(b.String())
}

func sendInAppNotification(ctx context.Context, db *pgxpool.Pool, event *NotificationEvent) error {
	// Insert notification into database
	var notificationID string
	var at time.Time

	// Use current time if not provided
	if timestamp, ok := event.Data["created_at"].(string); ok {
		parsedTime, err := time.Parse(time.RFC3339, timestamp)
		if err == nil {
			at = parsedTime
		} else {
			at = time.Now()
		}
	} else {
		at = time.Now()
	}

	// For in-app notifications, we need the user_id
	userID := event.UserID
	if userID == 0 {
		// Try to get user_id from recipient_id if it's a user reference
		// For now, we'll skip if userID is not provided
		log.Printf("Skipping in-app notification: no user_id provided")
		return nil
	}

	// Generate notification ID
	notificationID = fmt.Sprintf("notif-%d", time.Now().UnixNano())

	_, err := db.Exec(ctx, `
		INSERT INTO notifications (id, user_id, title, message, at, read)
		VALUES ($1, $2, $3, $4, $5, false)
	`, notificationID, userID, event.Subject, event.Content, at)

	if err != nil {
		return fmt.Errorf("failed to insert in-app notification: %w", err)
	}

	log.Printf("[IN_APP] Notification created for user %d: %s", userID, event.Subject)
	return nil
}

func recordCommunicationLog(ctx context.Context, db *pgxpool.Pool, event *NotificationEvent, status, errorMessage string) (int, error) {
	var logID int

	err := db.QueryRow(ctx, `
		INSERT INTO communication_logs (channel, recipient_id, recipient_type, subject, status, error_message, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
		RETURNING id
	`, event.Channel, event.RecipientID, event.RecipientType, event.Subject, status, errorMessage).Scan(&logID)

	if err != nil {
		return 0, err
	}

	return logID, nil
}

func updateCommunicationLogStatus(ctx context.Context, db *pgxpool.Pool, logID int, status, errorMessage string) error {
	_, err := db.Exec(ctx, `
		UPDATE communication_logs
		SET status = $1, error_message = $2, updated_at = NOW()
		WHERE id = $3
	`, status, errorMessage, logID)
	return err
}

const (
	notificationQueue = "notification_queue"
	// retryQueue is a sorted set of failed jobs scored by the unix time they
	// become due again.
	retryQueue = "notification_retry"
	// maxAttempts is how many times a notification is tried before giving up.
	maxAttempts = 3
)

// retryDelay is the wait before the next try after `attempts` failures:
// 30s, then 60s.
func retryDelay(attempts int) time.Duration {
	return time.Duration(30<<(attempts-1)) * time.Second
}

// scheduleRetry puts a failed job on the retry queue, or reports that it has
// used all its attempts.
func scheduleRetry(ctx context.Context, rdb *redis.Client, event *NotificationEvent) (bool, error) {
	event.Attempts++
	if event.Attempts >= maxAttempts {
		return false, nil
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return false, err
	}
	due := time.Now().Add(retryDelay(event.Attempts))
	return true, rdb.ZAdd(ctx, retryQueue, redis.Z{Score: float64(due.Unix()), Member: payload}).Err()
}

// promoteDueRetries moves retry jobs whose time has come back onto the main
// queue. ZRem decides ownership, so a job is never requeued twice.
func promoteDueRetries(ctx context.Context, rdb *redis.Client) {
	due, err := rdb.ZRangeByScore(ctx, retryQueue, &redis.ZRangeBy{Min: "-inf", Max: strconv.FormatInt(time.Now().Unix(), 10)}).Result()
	if err != nil {
		log.Printf("Error reading retry queue: %v", err)
		return
	}
	for _, job := range due {
		removed, err := rdb.ZRem(ctx, retryQueue, job).Result()
		if err != nil || removed == 0 {
			continue
		}
		if err := rdb.LPush(ctx, notificationQueue, job).Err(); err != nil {
			log.Printf("Error requeueing notification: %v", err)
		}
	}
}

// maskRecipient hides most of an email address for logs ("j***@example.com").
// Non-email recipients (user ids) are returned unchanged.
func maskRecipient(recipient string) string {
	at := strings.LastIndex(recipient, "@")
	if at < 1 {
		return recipient
	}
	return recipient[:1] + "***" + recipient[at:]
}

type CommunicationLog struct {
	ID            int    `json:"id"`
	Channel       string `json:"channel"`
	RecipientID   string `json:"recipient_id"`
	RecipientType string `json:"recipient_type"`
	Subject       string `json:"subject"`
	Status        string `json:"status"` // pending, sent, failed
	ErrorMessage  string `json:"error_message"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// maxConcurrentJobs is how many notifications are processed at the same time.
const maxConcurrentJobs = 5

// handleJob decodes and processes one queued notification, scheduling a
// retry when it fails.
func handleJob(ctx context.Context, rdb *redis.Client, db *pgxpool.Pool, cfg *config.Config, jobData string) {
	var event NotificationEvent
	if err := json.Unmarshal([]byte(jobData), &event); err != nil {
		log.Printf("Error unmarshaling job: %v", err)
		return
	}

	if err := processNotification(ctx, rdb, db, &event, cfg); err != nil {
		retrying, retryErr := scheduleRetry(ctx, rdb, &event)
		switch {
		case retryErr != nil:
			log.Printf("Error processing notification (could not schedule retry: %v): %v", retryErr, err)
		case retrying:
			log.Printf("Error processing notification (attempt %d of %d, retrying in %s): %v",
				event.Attempts, maxAttempts, retryDelay(event.Attempts), err)
		default:
			log.Printf("Giving up on notification after %d attempts: Channel=%s, Recipient=%s, Subject=%s: %v",
				maxAttempts, event.Channel, maskRecipient(event.RecipientID), event.Subject, err)
		}
		return
	}
	log.Printf("Processed notification: Channel=%s, Recipient=%s, Subject=%s",
		event.Channel, maskRecipient(event.RecipientID), event.Subject)
}
