package model

import "github.com/qonics/qonics-appy-api/libs/shared-package/utils"

// Validate is the validator used for incoming applicant payloads (profile,
// settings, notifications). It carries the shared general rules (regex, phone,
// strong_password).
var Validate = utils.NewValidator()

type ApplicantProfile struct {
	ID             string   `json:"id"`
	FullName       string   `json:"full_name" validate:"omitempty,max=150,name"`
	Email          string   `json:"email" validate:"omitempty,email"`
	Phone          string   `json:"phone" validate:"omitempty,max=30,phone"`
	Country        string   `json:"country" validate:"omitempty,max=100,name"`
	City           string   `json:"city" validate:"omitempty,max=100,name"`
	Institution    string   `json:"institution" validate:"omitempty,max=200,text"`
	FieldOfStudy   string   `json:"field_of_study" validate:"omitempty,max=200,text"`
	Qualification  string   `json:"qualification" validate:"omitempty,max=200,text"`
	AcademicStatus string   `json:"academic_status" validate:"omitempty,max=50"`
	GraduationYear *int     `json:"graduation_year" validate:"omitempty,gte=1950,lte=2100"`
	Skills         []string `json:"skills"`
	Bio            string   `json:"bio" validate:"omitempty,max=2000"`
	GithubUrl      string   `json:"github_url" validate:"omitempty,url"`
	LinkedinUrl    string   `json:"linkedin_url" validate:"omitempty,url"`
	PortfolioUrl   string   `json:"portfolio_url" validate:"omitempty,url"`
}

type ApplicantSummary struct {
	ApplicantProfile
	RegisteredAt      string `json:"registered_at"`
	TotalApplications int    `json:"total_applications"`
}

type AppNotification struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Message string `json:"message"`
	At      string `json:"at"`
	Read    bool   `json:"read"`
}

type AppSettings struct {
	ApplicationsOpen bool   `json:"applications_open"`
	CloseOn          string `json:"close_on"`
	ClosureMessage   string `json:"closure_message" validate:"omitempty,max=500"`
}

type MarkNotificationsReadRequest struct {
	IDs []string `json:"ids" validate:"required"`
}

// ApplicantFilter is the query contract for GET /applicants (admin). The tagged
// fields are turned into a parameterised WHERE clause by utils.BuildWhereClause;
// Search is a cross-column match handled in the controller (plain columns with
// ILIKE, encrypted-PII columns via their hash).
type ApplicantFilter struct {
	Page  int `query:"page"`
	Limit int `query:"limit"`

	// Search matches institution / field_of_study (ILIKE) and the applicant
	// full_name_hash / email_hash columns.
	Search string `query:"search"`

	Institution  string `query:"institution" field:"ap.institution" op:"ILIKE" validate:"omitempty,max=100"`
	FieldOfStudy string `query:"field_of_study" field:"ap.field_of_study" op:"ILIKE" validate:"omitempty,max=100"`
}

// NotificationFilter is the query contract for GET /applicant/notifications.
type NotificationFilter struct {
	Page   int   `query:"page"`
	Limit  int   `query:"limit"`
	Read   *bool `query:"read" field:"n.read" op:"="`
	Search string `query:"search"`

	// UserID scopes the list to the caller; set in the controller.
	UserID int `query:"-" field:"n.user_id" op:"="`
}
