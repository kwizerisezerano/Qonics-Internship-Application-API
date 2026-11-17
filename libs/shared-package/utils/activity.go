package utils

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ActivityLog represents an activity log entry
type ActivityLog struct {
	UserID       int
	ActivityType string
	Status       string
	Description  string
	IPAddress    string
	UserAgent    string
	Extra        any
}

// getIpAgent extracts IP address and user agent from request
func getIpAgent(c *fiber.Ctx, source ...string) (ip string, agent string) {
	if len(source) > 0 {
		ip = source[0]
		if len(source) > 1 {
			agent = source[1]
		} else {
			agent = c.Get("User-Agent")
		}
	} else {
		ip = c.IP()
		agent = c.Get("User-Agent")
	}
	return ip, agent
}

// recordActivityLog inserts an activity log into the database
func recordActivityLog(ctx context.Context, db *pgxpool.Pool, log ActivityLog, extra any) error {
	query := `
		INSERT INTO activity_logs (user_id, activity_type, status, description, ip_address, user_agent, extra, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	var extraData any
	var err error
	if extra != nil {
		extraData, err = json.Marshal(extra)
		if err != nil {
			return err
		}
	}

	_, err = db.Exec(
		ctx,
		query,
		log.UserID,
		log.ActivityType,
		log.Status,
		log.Description,
		log.IPAddress,
		log.UserAgent,
		extraData,
		time.Now(),
		time.Now(),
	)

	if err != nil {
		return err
	}
	return nil
}

// RecordSuccessActivityLog records a successful activity
func RecordSuccessActivityLog(c *fiber.Ctx, db *pgxpool.Pool, userId int, activityType string, description string, extra any, source ...string) error {
	ctx := context.Background()
	ip, agent := getIpAgent(c, source...)
	return recordActivityLog(ctx, db,
		ActivityLog{
			UserID:       userId,
			ActivityType: activityType,
			Description:  description,
			Status:       "success",
			IPAddress:    ip,
			UserAgent:    agent,
		},
		extra,
	)
}

// RecordFailedActivityLog records a failed activity
func RecordFailedActivityLog(c *fiber.Ctx, db *pgxpool.Pool, userId int, activityType string, description string, extra any, source ...string) error {
	ctx := context.Background()
	ip, agent := getIpAgent(c, source...)
	return recordActivityLog(ctx, db,
		ActivityLog{
			UserID:       userId,
			ActivityType: activityType,
			Description:  description,
			Status:       "failure",
			IPAddress:    ip,
			UserAgent:    agent,
		},
		extra,
	)
}
