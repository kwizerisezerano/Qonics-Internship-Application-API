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

// pendingTTL is how long an uploaded but unconfirmed file is kept before the
// cleanup worker removes it (leazi-style orphan protection).
const pendingTTL = 24 * time.Hour

func NewFileController(db *pgxpool.Pool, cfg *config.Config) *FileController {
	return &FileController{
		DB:     db,
		Config: cfg,
	}
}

// ownerID returns the current user id as a string, used as the file owner.
func ownerID(c *fiber.Ctx) string {
	return fmt.Sprintf("%v", c.Locals("user_id"))
}

// GetPresignedURL handles requests for generating presigned URLs for file uploads.
// It creates a "pending" file record whose deleted_at is set in the future; the
// record becomes permanent once ConfirmFileUpload clears it.
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

	// Create a pending file record. deleted_at in the future flags it as
	// "uploaded but not confirmed"; source_table/source_table_id stay empty
	// until the file is attached to its owner.
	fileID := uuid.New().String()
	_, err = fc.DB.Exec(ctx, `
		INSERT INTO files (id, owner_id, owner_type, name, file_path, file_type, file_size, description, deleted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, fileID, ownerID(c), "user", request.FileName, objectKey, request.FileType, request.FileSize, request.Description, time.Now().Add(pendingTTL))

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

// ConfirmFileUpload confirms a successful upload: it verifies the caller owns
// the pending record, that the object really exists in storage, then clears the
// pending marker (deleted_at) and, when provided, attaches the file to its
// owning record via the generic source_table / source_table_id pair plus extra
// metadata.
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

	// Only known source tables may own a file (leazi-style allow-list).
	if request.SourceTable != "" && !utils.In_array(utils.AvailableSourceTable, request.SourceTable) {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid source table")
	}

	// source_table and source_table_id must be supplied together (or neither).
	if (request.SourceTable != "" && request.SourceTableID == "") ||
		(request.SourceTable == "" && request.SourceTableID != "") {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "source_table and source_table_id must be provided together")
	}

	// Load the pending record and verify ownership.
	var fileOwner, filePath string
	var deletedAt *time.Time
	err := fc.DB.QueryRow(ctx, `
		SELECT COALESCE(owner_id, ''), file_path, deleted_at
		FROM files
		WHERE id = $1
	`, request.FileID).Scan(&fileOwner, &filePath, &deletedAt)

	if err != nil {
		if err == pgx.ErrNoRows {
			return utils.JsonErrorResponse(c, fiber.StatusNotFound, "File record not found")
		}
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to verify file", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("ConfirmFileUpload: Failed to verify file: %v", err),
			ServiceName: fc.Config.ServiceName,
		})
	}

	// Confirm the file belongs to the caller (leazi-style ownership check).
	if fileOwner == "" || fileOwner != ownerID(c) {
		return utils.JsonErrorResponse(c, fiber.StatusForbidden, "File ownership mismatch")
	}

	// A cleared deleted_at means the upload was already confirmed.
	if deletedAt == nil {
		return utils.JsonErrorResponse(c, fiber.StatusConflict, "File already confirmed")
	}

	// The object must actually exist in storage before we confirm it.
	r2Client, err := helper.R2ClientFactory()
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to initialize storage client", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("ConfirmFileUpload: Failed to initialize R2 client: %v", err),
			ServiceName: fc.Config.ServiceName,
		})
	}

	exists, err := r2Client.HeadObject(filePath)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to verify uploaded object", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("ConfirmFileUpload: Failed to head object: %v", err),
			ServiceName: fc.Config.ServiceName,
		})
	}
	if !exists {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Uploaded object not found in storage")
	}

	// Attach to owner (optional) and clear the pending marker.
	var extraArg interface{}
	if request.Extra != nil {
		extraArg = request.Extra
	}

	result, err := fc.DB.Exec(ctx, `
		UPDATE files
		SET deleted_at = NULL,
		    uploaded_at = NOW(),
		    source_table = COALESCE(NULLIF($1, ''), source_table),
		    source_table_id = COALESCE(NULLIF($2, ''), source_table_id),
		    description = COALESCE(NULLIF($3, ''), description),
		    extra = COALESCE($4::jsonb, extra)
		WHERE id = $5 AND owner_id = $6
	`, request.SourceTable, request.SourceTableID, request.Description, extraArg, request.FileID, ownerID(c))

	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to confirm upload", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("ConfirmFileUpload: Failed to confirm upload: %v", err),
			ServiceName: fc.Config.ServiceName,
		})
	}

	if result.RowsAffected() == 0 {
		return utils.JsonErrorResponse(c, fiber.StatusNotFound, "File not found or not owned by caller")
	}

	return utils.JsonSuccessResponse(c, "Upload confirmed", fiber.Map{
		"file_id": request.FileID,
	})
}

// GetFileURL returns a short-lived presigned URL to download a file.
func (fc *FileController) GetFileURL(c *fiber.Ctx) error {
	userID := c.Locals("user_id")
	if userID == nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, "Unauthorized")
	}

	ctx := context.Background()
	fileID := c.Params("file_id")

	var filePath, fileOwner string
	err := fc.DB.QueryRow(ctx, `
		SELECT file_path, COALESCE(owner_id, '') FROM files WHERE id = $1 AND deleted_at IS NULL
	`, fileID).Scan(&filePath, &fileOwner)

	if err != nil {
		if err == pgx.ErrNoRows {
			return utils.JsonErrorResponse(c, fiber.StatusNotFound, "File not found")
		}
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to fetch file", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("GetFileURL: Failed to lookup file record: %v", err),
			ServiceName: fc.Config.ServiceName,
		})
	}

	// Owners can read their own files; admins can read any file.
	if c.Locals("role") != "admin" && fileOwner != ownerID(c) {
		return utils.JsonErrorResponse(c, fiber.StatusForbidden, "Forbidden")
	}

	r2Client, err := helper.R2ClientFactory()
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to initialize storage client", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("GetFileURL: Failed to initialize R2 client: %v", err),
			ServiceName: fc.Config.ServiceName,
		})
	}

	url, err := r2Client.GeneratePresignedGetURL(filePath)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to generate file URL", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("GetFileURL: Failed to generate presigned GET URL: %v", err),
			ServiceName: fc.Config.ServiceName,
		})
	}

	return utils.JsonSuccessResponse(c, "File URL generated", fiber.Map{
		"file_id": fileID,
		"url":     url,
	})
}
