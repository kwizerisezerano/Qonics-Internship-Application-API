package model

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// TestNewApplicationPayloadValidation covers the department/specialization
// allow-list, including the cross-field rule that a specialization must belong
// to the payload's department.
func TestNewApplicationPayloadValidation(t *testing.T) {
	base := func() NewApplicationPayload {
		return NewApplicationPayload{
			Department:     "software",
			Specialization: "backend",
			Motivation:     "I want to learn",
			Experience:     "2 years",
			EducationLevel: "university",
			Session:        "morning",
			DurationMonths: 6,
			WorkMode:       "remote",
		}
	}

	t.Run("valid software payload", func(t *testing.T) {
		req := base()
		assert.NoError(t, Validate.Struct(&req))
	})

	t.Run("valid iot payload", func(t *testing.T) {
		req := base()
		req.Department = "iot"
		req.Specialization = "embedded"
		assert.NoError(t, Validate.Struct(&req))
	})

	t.Run("rejects unknown department", func(t *testing.T) {
		req := base()
		req.Department = "Engineering"
		assert.Error(t, Validate.Struct(&req))
	})

	t.Run("rejects unknown specialization", func(t *testing.T) {
		req := base()
		req.Specialization = "Software Development"
		assert.Error(t, Validate.Struct(&req))
	})

	t.Run("rejects specialization from the wrong department", func(t *testing.T) {
		req := base()
		req.Department = "iot"
		req.Specialization = "backend"
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
