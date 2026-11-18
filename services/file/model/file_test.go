package model

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/qonics/qonics-appy-api/libs/shared-package/utils"
	"github.com/stretchr/testify/assert"
)

// validate is the shared validator, mirroring the controller's wire-up so the
// tests exercise the same rules the handlers apply.
var validate = utils.NewValidator()

// TestPresignedURLRequestValidation covers the required file metadata and bounds.
func TestPresignedURLRequestValidation(t *testing.T) {
	base := func() PresignedURLRequest {
		return PresignedURLRequest{
			FileName: "cv.pdf",
			FileType: "application/pdf",
			FileSize: 1024,
		}
	}

	t.Run("valid request", func(t *testing.T) {
		req := base()
		assert.NoError(t, validate.Struct(&req))
	})

	t.Run("rejects empty file name", func(t *testing.T) {
		req := base()
		req.FileName = ""
		assert.Error(t, validate.Struct(&req))
	})

	t.Run("rejects over-long file name", func(t *testing.T) {
		req := base()
		req.FileName = strings.Repeat("a", 256)
		assert.Error(t, validate.Struct(&req))
	})

	t.Run("rejects non-positive file size", func(t *testing.T) {
		req := base()
		req.FileSize = 0
		assert.Error(t, validate.Struct(&req))
	})

	t.Run("rejects over-long description", func(t *testing.T) {
		req := base()
		req.Description = strings.Repeat("a", 2001)
		assert.Error(t, validate.Struct(&req))
	})
}

// TestConfirmUploadRequestValidation covers the required file id and the
// optional source-table binding bounds.
func TestConfirmUploadRequestValidation(t *testing.T) {
	base := func() ConfirmUploadRequest {
		return ConfirmUploadRequest{FileID: uuid.NewString()}
	}

	t.Run("valid request", func(t *testing.T) {
		req := base()
		assert.NoError(t, validate.Struct(&req))
	})

	t.Run("requires file id", func(t *testing.T) {
		assert.Error(t, validate.Struct(&ConfirmUploadRequest{}))
	})

	t.Run("rejects non-uuid file id", func(t *testing.T) {
		req := base()
		req.FileID = "not-a-uuid"
		assert.Error(t, validate.Struct(&req))
	})

	t.Run("rejects over-long source table", func(t *testing.T) {
		req := base()
		req.SourceTable = strings.Repeat("a", 101)
		assert.Error(t, validate.Struct(&req))
	})
}
