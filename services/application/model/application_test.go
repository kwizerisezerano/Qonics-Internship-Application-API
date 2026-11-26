package model

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// TestNewApplicationPayloadValidation covers the package-based payload validation.
func TestNewApplicationPayloadValidation(t *testing.T) {
	base := func() NewApplicationPayload {
		return NewApplicationPayload{
			PackageID:      1,
			Motivation:     "I want to learn",
			Experience:     "2 years",
			EducationLevel: "university",
			Session:        "morning",
			WorkMode:       "remote",
		}
	}

	t.Run("valid package payload", func(t *testing.T) {
		req := base()
		assert.NoError(t, Validate.Struct(&req))
	})

	t.Run("rejects invalid package_id", func(t *testing.T) {
		req := base()
		req.PackageID = 0
		assert.Error(t, Validate.Struct(&req))
	})

	t.Run("rejects negative package_id", func(t *testing.T) {
		req := base()
		req.PackageID = -1
		assert.Error(t, Validate.Struct(&req))
	})

	t.Run("rejects over-long motivation", func(t *testing.T) {
		req := base()
		req.Motivation = strings.Repeat("a", 2001)
		assert.Error(t, Validate.Struct(&req))
	})

	t.Run("rejects over-long experience", func(t *testing.T) {
		req := base()
		req.Experience = strings.Repeat("a", 2001)
		assert.Error(t, Validate.Struct(&req))
	})

	t.Run("accepts valid document ids", func(t *testing.T) {
		req := base()
		req.DocumentIDs = []string{uuid.NewString(), uuid.NewString()}
		assert.NoError(t, Validate.Struct(&req))
	})

	t.Run("rejects non-uuid document ids", func(t *testing.T) {
		req := base()
		req.DocumentIDs = []string{"doc-1"}
		assert.Error(t, Validate.Struct(&req))
	})
}

// TestStatusUpdatePayloadValidation covers the status allow-list and note bound.
func TestStatusUpdatePayloadValidation(t *testing.T) {
	t.Run("valid status", func(t *testing.T) {
		assert.NoError(t, Validate.Struct(&StatusUpdatePayload{Status: "under_review"}))
	})

	t.Run("rejects unknown status", func(t *testing.T) {
		assert.Error(t, Validate.Struct(&StatusUpdatePayload{Status: "pending"}))
	})

	t.Run("rejects over-long note", func(t *testing.T) {
		req := StatusUpdatePayload{Status: "accepted", Note: strings.Repeat("a", 2001)}
		assert.Error(t, Validate.Struct(&req))
	})
}

// TestAddNoteRequestValidation covers the required note text and its bound.
func TestAddNoteRequestValidation(t *testing.T) {
	t.Run("valid note", func(t *testing.T) {
		assert.NoError(t, Validate.Struct(&AddNoteRequest{Text: "Looks good"}))
	})

	t.Run("rejects empty note", func(t *testing.T) {
		assert.Error(t, Validate.Struct(&AddNoteRequest{}))
	})

	t.Run("rejects over-long note", func(t *testing.T) {
		assert.Error(t, Validate.Struct(&AddNoteRequest{Text: strings.Repeat("a", 2001)}))
	})
}
