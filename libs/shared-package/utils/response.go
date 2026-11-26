package utils

import (
	"time"

	"github.com/gofiber/fiber/v2"
)

// IsTestMode indicates if tests are running
var IsTestMode = false

// LogLevel represents the severity level for logging
type LogLevel string

const (
	DEBUG    LogLevel = "DEBUG"
	INFO     LogLevel = "INFO"
	WARNING  LogLevel = "WARNING"
	ERROR    LogLevel = "ERROR"
	CRITICAL LogLevel = "CRITICAL"
)

// Logger represents a structured log entry
type Logger struct {
	LogLevel    LogLevel `json:"log_level"`
	Message     string   `json:"message"`
	ServiceName string   `json:"service_name"`
}

// JsonErrorResponse returns a standardized JSON error response
// Logger is optional - only include for unknown/internal errors (500)
func JsonErrorResponse(c *fiber.Ctx, statusCode int, message string, logEntry ...Logger) error {
	// Log the error if a logger is provided
	if len(logEntry) > 0 && logEntry[0].Message != "" {
		// In test mode, include the error message in the response for debugging
		if IsTestMode {
			message = message + ":" + logEntry[0].Message
		}
		// In a real implementation, you would log this using the logger
	}

	return c.Status(statusCode).JSON(fiber.Map{
		"status":  statusCode,
		"message": message,
	})
}

// JsonSuccessResponse returns a standardized JSON success response
func JsonSuccessResponse(c *fiber.Ctx, message string, data interface{}) error {
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"status":  fiber.StatusOK,
		"message": message,
		"data":    data,
	})
}

// JsonPaginatedResponse returns a paginated JSON response
func JsonPaginatedResponse(c *fiber.Ctx, data interface{}, total int64, page int, limit int, totalPages int) error {
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"status": fiber.StatusOK,
		"data":   data,
		"meta": fiber.Map{
			"total":       total,
			"page":        page,
			"limit":       limit,
			"total_pages": totalPages,
		},
	})
}

// JsonPaginatedSuccessResponse returns a success response paired with a
// leazi-style pagination meta block (see GeneratePaginationData).
func JsonPaginatedSuccessResponse(c *fiber.Ctx, message string, data interface{}, meta map[string]any) error {
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"status":  fiber.StatusOK,
		"message": message,
		"data":    data,
		"meta":    meta,
	})
}

// TimeNowRFC3339 returns current time in RFC3339 format
func TimeNowRFC3339() string {
	return time.Now().Format(time.RFC3339)
}
