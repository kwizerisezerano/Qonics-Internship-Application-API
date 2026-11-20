package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/qonics/qonics-appy-api/libs/shared-package/utils"
	"github.com/qonics/qonics-appy-api/services/file/model"
)

// Guest uploads let someone apply without an account and still attach
// documents. The guest gets a random token on their first upload; their files
// are owned by a hash of it (utils.GuestUploadOwner) and are claimed by the
// application service when the application is submitted with the same token.
// Unclaimed files expire after pendingTTL and are removed by the orphan
// cleanup, so abandoned uploads never pile up.

// GuestPresignedURL is the public counterpart of GetPresignedURL.
func (fc *FileController) GuestPresignedURL(c *fiber.Ctx) error {
	request := new(model.GuestPresignedURLRequest)
	if err := c.BodyParser(request); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid request format")
	}
	if err := Validate.Struct(request); err != nil {
		return utils.JsonValidationErrorResponse(c, err)
	}

	token := request.GuestToken
	if token == "" {
		generated, err := utils.SecureRandString(utils.GuestUploadTokenBytes)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to start upload", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     fmt.Sprintf("GuestPresignedURL: Failed to generate guest token: %v", err),
				ServiceName: fc.Config.ServiceName,
			})
		}
		token = generated
	}
	owner := utils.GuestUploadOwner(token)

	// One token may only hold a handful of documents.
	var existing int
	if err := fc.DB.QueryRow(context.Background(),
		`SELECT COUNT(id) FROM files WHERE owner_id = $1`, owner).Scan(&existing); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to start upload", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("GuestPresignedURL: Failed to count guest files: %v", err),
			ServiceName: fc.Config.ServiceName,
		})
	}
	if existing >= utils.MaxGuestFiles {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest,
			fmt.Sprintf("You can upload at most %d documents before applying", utils.MaxGuestFiles))
	}

	resp, done := fc.createPendingUpload(c, &request.PresignedURLRequest, owner, false)
	if !done {
		return nil // the error response is already written
	}

	return c.JSON(fiber.Map{
		"status":  fiber.StatusOK,
		"message": "Presigned URL generated successfully",
		"data":    model.GuestPresignedURLResponse{PresignedURLResponse: resp, GuestToken: token},
	})
}

// GuestConfirmUpload is the public counterpart of ConfirmFileUpload. It marks
// the upload as done (uploaded_at) but keeps the pending expiry, so a file that
// is never claimed by an application is still cleaned up; the application
// service clears the expiry when it claims the file.
func (fc *FileController) GuestConfirmUpload(c *fiber.Ctx) error {
	request := new(model.GuestConfirmUploadRequest)
	if err := c.BodyParser(request); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid request format")
	}
	if err := Validate.Struct(request); err != nil {
		return utils.JsonValidationErrorResponse(c, err)
	}

	owner := utils.GuestUploadOwner(request.GuestToken)

	var file model.FileResponse
	err := fc.DB.QueryRow(context.Background(), `
		UPDATE files
		SET uploaded_at = NOW(),
		    deleted_at = $3
		WHERE id = $1 AND owner_id = $2 AND uploaded_at IS NULL AND deleted_at IS NOT NULL AND deleted_at > NOW()
		RETURNING id, name, file_path, file_type, file_size, COALESCE(description, '')
	`, request.FileID, owner, time.Now().Add(pendingTTL)).Scan(
		&file.ID, &file.Name, &file.FilePath, &file.FileType, &file.FileSize, &file.Description,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			// Unknown file, someone else's token, already confirmed or expired:
			// all answered the same so nothing about other uploads is revealed.
			return utils.JsonErrorResponse(c, fiber.StatusNotFound, "Pending upload not found")
		}
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to confirm upload", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("GuestConfirmUpload: Failed to confirm upload: %v", err),
			ServiceName: fc.Config.ServiceName,
		})
	}

	return utils.JsonSuccessResponse(c, "Upload confirmed", file)
}
