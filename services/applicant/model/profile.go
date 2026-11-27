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
	Skills         []string `json:"skills" validate:"omitempty,dive,max=50"`
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
	MorningSeats     int    `json:"morning_seats" validate:"omitempty,min=0"`
	AfternoonSeats   int    `json:"afternoon_seats" validate:"omitempty,min=0"`
}

// MarkNotificationsReadRequest carries the notification ids to mark as read.
// Notification ids are generated as "notif-<unixnano>" (not UUIDs) and stored in
// a VARCHAR(50) column, so each id only needs to be non-empty and within the
// column length.
type MarkNotificationsReadRequest struct {
	IDs []string `json:"ids" validate:"required,min=1,dive,required,max=50"`
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
	Page   int    `query:"page"`
	Limit  int    `query:"limit"`
	Read   *bool  `query:"read" field:"n.read" op:"="`
	Search string `query:"search"`

	// UserID scopes the list to the caller; set in the controller.
	UserID int `query:"-" field:"n.user_id" op:"="`
}

// ActivityLog is a single audit-trail entry returned by GET /activity-logs.
// The username and email are decrypted from the owning user's PII columns.
type ActivityLog struct {
	ID           int64  `json:"id"`
	ActivityType string `json:"activity_type"`
	Status       string `json:"status"`
	Description  string `json:"description"`
	UserID       *int   `json:"user_id,omitempty"`
	Username     string `json:"username,omitempty"`
	Email        string `json:"email,omitempty"`
	Role         string `json:"role,omitempty"`
	IPAddress    string `json:"ip_address,omitempty"`
	UserAgent    string `json:"user_agent,omitempty"`
	Extra        string `json:"extra,omitempty"`
	CreatedAt    string `json:"created_at"`
}

// ActivityLogFilter is the query contract for GET /activity-logs (admin). It
// mirrors leazi's filter: dynamic `field`/`op` tags are turned into a
// parameterised WHERE clause by utils.BuildWhereClause, while Search is a
// cross-column match handled in the controller (description via ILIKE and the
// user PII columns via their SHA-256 hash).
type ActivityLogFilter struct {
	Page  int `query:"page"`
	Limit int `query:"limit"`

	// Activity type filters.
	ActivityType           string `query:"activity_type" field:"al.activity_type" op:"=" validate:"omitempty,max=100"`
	ActivityTypeNeq        string `query:"activity_type_neq" field:"al.activity_type" op:"!=" validate:"omitempty,max=100"`
	ActivityTypeContains   string `query:"activity_type_c" field:"al.activity_type" op:"ILIKE" validate:"omitempty,min=2,max=100"`
	ActivityTypeStartsWith string `query:"activity_type_sw" field:"al.activity_type" op:"LIKE" validate:"omitempty,min=1,max=100"`
	ActivityTypeEndsWith   string `query:"activity_type_ew" field:"al.activity_type" op:"LIKE" validate:"omitempty,min=1,max=100"`

	// Status filters.
	Status    string `query:"status" field:"al.status" op:"=" validate:"omitempty,oneof=success failure"`
	StatusNeq string `query:"status_neq" field:"al.status" op:"!=" validate:"omitempty,oneof=success failure"`

	// Actor filters (user_id on the log, role from the joined user).
	UserID *int   `query:"user_id" field:"al.user_id" op:"="`
	Role   string `query:"role" field:"u.role" op:"=" validate:"omitempty,oneof=applicant admin"`

	// Date range filters.
	CreatedAt     string `query:"created_at" field:"al.created_at" op:"=" validate:"omitempty"`
	CreatedAfter  string `query:"created_after" field:"al.created_at" op:">=" validate:"omitempty"`
	CreatedBefore string `query:"created_before" field:"al.created_at" op:"<=" validate:"omitempty"`

	// Search matches the description (ILIKE) and the user name/email hashes.
	Search string `query:"search"`
}

// Package is a training package with pricing, skills, and billing period.
type Package struct {
	ID                    int      `json:"id"`
	Name                  string   `json:"name" validate:"required"`
	Description           string   `json:"description"`
	Price                 float64  `json:"price" validate:"required,gte=0,lte=100000000"`
	Skills                []string `json:"skills" validate:"required"`
	Period                string   `json:"period" validate:"required,oneof=monthly yearly quarterly"`
	Currency              string   `json:"currency" validate:"omitempty,len=3,uppercase"`
	IsActive              bool     `json:"is_active"`
	MinimumDurationPeriod int      `json:"minimum_duration_period"`
}
