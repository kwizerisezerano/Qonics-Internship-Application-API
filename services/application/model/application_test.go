package model

import (
	"testing"

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
}
