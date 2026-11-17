package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qonics/qonics-appy-api/libs/shared-package/config"
	"github.com/qonics/qonics-appy-api/libs/shared-package/utils"
	"github.com/qonics/qonics-appy-api/services/file/helper"
	"github.com/qonics/qonics-appy-api/services/file/model"
)

type FileController struct {
	DB     *pgxpool.Pool
	Config *config.Config
}

var Validate = validator.New()

func NewFileController(db *pgxpool.Pool, cfg *config.Config) *FileController {
	return &FileController{
		DB:     db,
		Config: cfg,
	}
}

// GetPresignedURL handles requests for generating presigned URLs for file uploads
func (fc *FileController) GetPresignedURL(c *fiber.Ctx) error {
	userID := c.Locals("user_id")
	if userID == nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, "Unauthorized")
	}

	ctx := context.Background()

	// Parse request
	request := new(model.PresignedURLRequest)
	if err := c.BodyParser(request); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid request format")
	}

	if err := Validate.Struct(request); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Validation failed")
	}

	// Initialize R2 client
	r2Client, err := helper.R2ClientFactory()
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to initialize storage client", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("GetPresignedURL: Failed to initialize R2 client: %v", err),
			ServiceName: fc.Config.ServiceName,
		})
	}

	// Generate presigned URL
	presignedURL, objectKey, err := r2Client.GeneratePresignedURL(request.FileName, request.FileType)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to generate presigned URL", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("GetPresignedURL: Failed to generate presigned URL: %v", err),
			ServiceName: fc.Config.ServiceName,
		})
	}

	// Create temporary file record
	_ = userID
	fileID := uuid.New().String()
	_, err = fc.DB.Exec(ctx, `
		INSERT INTO application_documents (id, application_id, document_type, file_name, file_size, file_path, uploaded_at)
		VALUES ($1, NULL, $2, $3, $4, $5, $6)
	`, fileID, request.FileType, request.FileName, request.FileSize, objectKey, time.Now())

	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to create file record", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("GetPresignedURL: Failed to create file record: %v", err),
			ServiceName: fc.Config.ServiceName,
		})
	}

	// Return presigned URL and temporary file ID
	return c.JSON(fiber.Map{
		"status":  fiber.StatusOK,
		"message": "Presigned URL generated successfully",
		"data": model.PresignedURLResponse{
			PresignedURL: presignedURL,
			FileID:       fileID,
			ObjectKey:    objectKey,
			ExpiresIn:    900, // 15 minutes in seconds
		},
	})
}

// ConfirmFileUpload handles confirming a successful file upload
func (fc *FileController) ConfirmFileUpload(c *fiber.Ctx) error {
	userID := c.Locals("user_id")
	if userID == nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, "Unauthorized")
	}

	ctx := context.Background()

	// Parse request
	request := new(model.ConfirmUploadRequest)
	if err := c.BodyParser(request); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid request format")
	}

	if err := Validate.Struct(request); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Validation failed")
	}

	// Update file record to mark as confirmed (remove deleted_at if set)
	_, err := fc.DB.Exec(ctx, `
		UPDATE application_documents
		SET uploaded_at = $1
		WHERE id = $2 AND file_path = $3
	`, time.Now(), request.FileID, request.ObjectKey)

	if err != nil {
		if err == pgx.ErrNoRows {
			return utils.JsonErrorResponse(c, fiber.StatusNotFound, "File record not found")
		}
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to confirm upload", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("ConfirmFileUpload: Failed to confirm upload: %v", err),
			ServiceName: fc.Config.ServiceName,
		})
	}

	return utils.JsonSuccessResponse(c, "Upload confirmed", fiber.Map{
		"file_id": request.FileID,
	})
}
