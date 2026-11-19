package controller

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

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

// Validate is the validator used for incoming file payloads. It is the shared
// validator so the general rules (name, text, regex, phone, strong_password)
// are available, leazi-style.
var Validate = utils.NewValidator()

// pendingTTL is how long an uploaded but unconfirmed file is kept before the
// cleanup worker removes it (leazi-style orphan protection).
const pendingTTL = 24 * time.Hour

func NewFileController(db *pgxpool.Pool, cfg *config.Config) *FileController {
	return &FileController{
		DB:     db,
		Config: cfg,
	}
}

// ownerID returns the caller's user id as a string, used as the file owner.
func ownerID(user utils.UserPayload) string {
	return strconv.Itoa(user.ID)
}

// GetPresignedURL handles requests for generating presigned URLs for file uploads.
// It creates a "pending" file record whose deleted_at is set in the future; the
// record becomes permanent once ConfirmFileUpload clears it.
func (fc *FileController) GetPresignedURL(c *fiber.Ctx) error {
	user, ok := utils.GetUserPayload(c)
	if !ok {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, "Unauthorized")
	}

	ctx := context.Background()

	// Parse request
	request := new(model.PresignedURLRequest)
	if err := c.BodyParser(request); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid request format")
	}

	if err := Validate.Struct(request); err != nil {
		return utils.JsonValidationErrorResponse(c, err)
	}

	// Only document formats are accepted; the stored object keeps this extension.
	ext := strings.ToLower(filepath.Ext(request.FileName))
	if !slices.Contains(model.AllowedUploadExtensions, ext) {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Only PDF, DOC and DOCX files can be uploaded")
	}

	// Documents are only uploaded to go with a new application, so refuse them
	// while intake is closed instead of storing files that can never be used.
	if !user.IsAdmin() {
		intake, err := utils.GetIntakeStatus(ctx, fc.DB)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to load application settings", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     fmt.Sprintf("GetPresignedURL: Failed to load intake settings: %v", err),
				ServiceName: fc.Config.ServiceName,
			})
		}
		if !intake.Open {
			return utils.JsonErrorResponse(c, fiber.StatusForbidden, intake.Message)
		}
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
	`, fileID, ownerID(user), "user", request.FileName, objectKey, request.FileType, request.FileSize, request.Description, time.Now().Add(pendingTTL))

	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to create file record", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("GetPresignedURL: Failed to create file record: %v", err),
			ServiceName: fc.Config.ServiceName,
		})
	}

	// Record the upload request in the audit trail (leazi-style activity log).
	utils.RecordSuccessActivityLog(c, fc.DB, utils.CurrentUserID(c), "FILE_PRESIGNED_URL",
		fmt.Sprintf("Requested upload URL for %s", request.FileName),
		map[string]any{"file_id": fileID, "file_name": request.FileName})

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
// the pending record, then clears the pending marker (deleted_at) and, when
// provided, attaches the file to its owning record via the generic
// source_table / source_table_id pair plus extra metadata.
func (fc *FileController) ConfirmFileUpload(c *fiber.Ctx) error {
	user, ok := utils.GetUserPayload(c)
	if !ok {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, "Unauthorized")
	}

	ctx := context.Background()

	// Parse request
	request := new(model.ConfirmUploadRequest)
	if err := c.BodyParser(request); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid request format")
	}

	if err := Validate.Struct(request); err != nil {
		return utils.JsonValidationErrorResponse(c, err)
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
	var fileOwner string
	var deletedAt *time.Time
	err := fc.DB.QueryRow(ctx, `
		SELECT COALESCE(owner_id, ''), deleted_at
		FROM files
		WHERE id = $1
	`, request.FileID).Scan(&fileOwner, &deletedAt)

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
	if fileOwner == "" || fileOwner != ownerID(user) {
		return utils.JsonErrorResponse(c, fiber.StatusForbidden, "File ownership mismatch")
	}

	// A cleared deleted_at means the upload was already confirmed.
	if deletedAt == nil {
		return utils.JsonErrorResponse(c, fiber.StatusConflict, "File already confirmed")
	}

	// A file may only be attached to a record the caller owns (admins excepted),
	// otherwise anyone could add documents to someone else's application.
	if request.SourceTable != "" && !user.IsAdmin() {
		owns, err := fc.ownsSourceRecord(ctx, request.SourceTable, request.SourceTableID, ownerID(user))
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to verify attachment target", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     fmt.Sprintf("ConfirmFileUpload: Failed to verify source record: %v", err),
				ServiceName: fc.Config.ServiceName,
			})
		}
		if !owns {
			return utils.JsonErrorResponse(c, fiber.StatusForbidden, "You can only attach files to your own records")
		}
	}

	// Attach to owner (optional) and clear the pending marker, returning the
	// final record so the confirm payload mirrors leazi's file response.
	var extraArg any
	if request.Extra != nil {
		extraArg = request.Extra
	}

	var file model.FileResponse
	err = fc.DB.QueryRow(ctx, `
		UPDATE files
		SET deleted_at = NULL,
		    uploaded_at = NOW(),
		    source_table = COALESCE(NULLIF($1, ''), source_table),
		    source_table_id = COALESCE(NULLIF($2, ''), source_table_id),
		    description = COALESCE(NULLIF($3, ''), description),
		    extra = COALESCE($4::jsonb, extra)
		WHERE id = $5 AND owner_id = $6
		RETURNING id, name, file_path, file_type, file_size,
		          COALESCE(source_table, ''), COALESCE(source_table_id, ''),
		          COALESCE(description, '')
	`, request.SourceTable, request.SourceTableID, request.Description, extraArg, request.FileID, ownerID(user)).Scan(
		&file.ID, &file.Name, &file.FilePath, &file.FileType, &file.FileSize,
		&file.SourceTable, &file.SourceTableID, &file.Description,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return utils.JsonErrorResponse(c, fiber.StatusNotFound, "File not found or not owned by caller")
		}
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to confirm upload", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("ConfirmFileUpload: Failed to confirm upload: %v", err),
			ServiceName: fc.Config.ServiceName,
		})
	}

	// Generate a presigned URL so the client can access the confirmed file.
	// Leazi-style: a URL-generation failure does not fail the confirmation.
	r2Client, err := helper.R2ClientFactory()
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to initialize storage client", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("ConfirmFileUpload: Failed to initialize R2 client: %v", err),
			ServiceName: fc.Config.ServiceName,
		})
	}

	file.URL, err = r2Client.GeneratePresignedGetURL(file.FilePath)
	if err != nil {
		file.URL = ""
	}

	// Record the confirmation in the audit trail (leazi-style activity log).
	utils.RecordSuccessActivityLog(c, fc.DB, utils.CurrentUserID(c), "FILE_CONFIRM_UPLOAD",
		fmt.Sprintf("Confirmed upload of file %s", file.ID),
		map[string]any{"file_id": file.ID, "source_table": file.SourceTable})

	return utils.JsonSuccessResponse(c, "Upload confirmed", file)
}

// GetFileURL returns a short-lived presigned URL to download a file.
func (fc *FileController) GetFileURL(c *fiber.Ctx) error {
	user, ok := utils.GetUserPayload(c)
	if !ok {
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
	if !user.IsAdmin() && fileOwner != ownerID(user) {
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

// ownsSourceRecord reports whether the user owns the record a file is being
// attached to. Every table in utils.AvailableSourceTable needs a case here;
// unknown tables are treated as not owned.
func (fc *FileController) ownsSourceRecord(ctx context.Context, sourceTable, sourceID, userID string) (bool, error) {
	var owns bool
	switch sourceTable {
	case "applications":
		err := fc.DB.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM applications a
				JOIN applicant_profiles ap ON a.applicant_id = ap.id
				WHERE a.id::text = $1 AND ap.user_id::text = $2
			)
		`, sourceID, userID).Scan(&owns)
		return owns, err
	default:
		return false, nil
	}
}
