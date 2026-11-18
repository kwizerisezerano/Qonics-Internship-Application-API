package model

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func intPtr(v int) *int { return &v }

// TestApplicantProfileValidation covers the general field rules on the profile
// payload: name/text content, email, phone, graduation year range and URLs.
func TestApplicantProfileValidation(t *testing.T) {
	base := func() ApplicantProfile {
		return ApplicantProfile{
			FullName:       "John Doe",
			Email:          "john@example.com",
			Phone:          "+250788000000",
			Country:        "Rwanda",
			City:           "Kigali",
			Institution:    "University of Rwanda, Kigali",
			FieldOfStudy:   "Computer Science",
			Qualification:  "BSc",
			AcademicStatus: "university",
			GraduationYear: intPtr(2025),
			GithubUrl:      "https://github.com/johndoe",
		}
	}

	t.Run("valid profile", func(t *testing.T) {
		p := base()
		assert.NoError(t, Validate.Struct(&p))
	})

	t.Run("empty profile is allowed", func(t *testing.T) {
		p := ApplicantProfile{}
		assert.NoError(t, Validate.Struct(&p))
	})

	t.Run("rejects full name with digits", func(t *testing.T) {
		p := base()
		p.FullName = "John Doe 3rd"
		assert.Error(t, Validate.Struct(&p))
	})

	t.Run("rejects invalid email", func(t *testing.T) {
		p := base()
		p.Email = "not-an-email"
		assert.Error(t, Validate.Struct(&p))
	})

	t.Run("rejects invalid phone", func(t *testing.T) {
		p := base()
		p.Phone = "123"
		assert.Error(t, Validate.Struct(&p))
	})

	t.Run("rejects city with digits", func(t *testing.T) {
		p := base()
		p.City = "Kigali1"
		assert.Error(t, Validate.Struct(&p))
	})

	t.Run("rejects institution with unsupported characters", func(t *testing.T) {
		p := base()
		p.Institution = "Uni \U0001F600"
		assert.Error(t, Validate.Struct(&p))
	})

	t.Run("accepts graduation year in range", func(t *testing.T) {
		p := base()
		p.GraduationYear = intPtr(2000)
		assert.NoError(t, Validate.Struct(&p))
	})

	t.Run("rejects graduation year out of range", func(t *testing.T) {
		p := base()
		p.GraduationYear = intPtr(1900)
		assert.Error(t, Validate.Struct(&p))
	})

	t.Run("rejects invalid URL", func(t *testing.T) {
		p := base()
		p.GithubUrl = "not-a-url"
		assert.Error(t, Validate.Struct(&p))
	})

	t.Run("rejects full name over max length", func(t *testing.T) {
		p := base()
		p.FullName = strings.Repeat("a", 151)
		assert.Error(t, Validate.Struct(&p))
	})

	t.Run("accepts valid skills", func(t *testing.T) {
		p := base()
		p.Skills = []string{"Go", "PostgreSQL"}
		assert.NoError(t, Validate.Struct(&p))
	})

	t.Run("rejects over-long skill", func(t *testing.T) {
		p := base()
		p.Skills = []string{strings.Repeat("a", 51)}
		assert.Error(t, Validate.Struct(&p))
	})

	t.Run("rejects over-long bio", func(t *testing.T) {
		p := base()
		p.Bio = strings.Repeat("a", 2001)
		assert.Error(t, Validate.Struct(&p))
	})
}

// TestAppSettingsValidation covers the closure message length limit.
func TestAppSettingsValidation(t *testing.T) {
	t.Run("valid settings", func(t *testing.T) {
		s := AppSettings{ApplicationsOpen: true, CloseOn: "2026-12-31", ClosureMessage: "Closed for the holidays"}
		assert.NoError(t, Validate.Struct(&s))
	})

	t.Run("rejects over-long closure message", func(t *testing.T) {
		s := AppSettings{ClosureMessage: strings.Repeat("a", 501)}
		assert.Error(t, Validate.Struct(&s))
	})
}

// TestMarkNotificationsReadValidation covers the required ids list.
func TestMarkNotificationsReadValidation(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		assert.NoError(t, Validate.Struct(&MarkNotificationsReadRequest{IDs: []string{uuid.NewString(), uuid.NewString()}}))
	})

	t.Run("rejects nil ids", func(t *testing.T) {
		assert.Error(t, Validate.Struct(&MarkNotificationsReadRequest{}))
	})

	t.Run("rejects non-uuid ids", func(t *testing.T) {
		assert.Error(t, Validate.Struct(&MarkNotificationsReadRequest{IDs: []string{"1", "2"}}))
	})
}
